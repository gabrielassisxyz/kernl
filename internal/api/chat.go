package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/memory"
	"github.com/gabrielassisxyz/kernl/internal/planning"
)

// RegisterChatRoutes registers chat REST + SSE endpoints.
func RegisterChatRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("POST /api/chat/sessions", createChatSessionHandler(a))
	mux.HandleFunc("GET /api/chat/sessions/{id}", getChatSessionHandler(a))
	mux.HandleFunc("POST /api/chat/sessions/{id}/messages", postChatMessageHandler(a))
	mux.HandleFunc("GET /api/chat/sessions/{id}/events", chatEventsHandler(a))
	mux.HandleFunc("POST /api/chat/sessions/{id}/learned", postLearnedCandidateHandler(a))
	mux.HandleFunc("GET /api/nodes", listNodesHandler(a))
}

func createChatSessionHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		cs := &nodes.ChatSession{
			Messages: []nodes.ChatMessage{},
		}

		// Substrate-aware planning: when the session is opened with a seed, pull
		// the relevant vault notes and seed the conversation with them as system
		// context, so the DA plans WITH the user's notes already in scope — the
		// keystone seam, no manual hunting/pasting.
		var body struct {
			Seed string `json:"seed"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body) // body is optional
		if seed := strings.TrimSpace(body.Seed); seed != "" {
			if pnotes, err := planning.BuildContext(ctx, a.Graph, seed, 8); err == nil && len(pnotes) > 0 {
				var b strings.Builder
				fmt.Fprintf(&b, "The user is planning around: %q.\nRelevant notes already in their vault (use them as context):\n\n", seed)
				for _, n := range pnotes {
					fmt.Fprintf(&b, "- %s: %s\n", n.Title, n.Snippet)
				}
				cs.Messages = append(cs.Messages, nodes.ChatMessage{Role: "system", Content: b.String()})
			}
		}

		var id string
		err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
			var err error
			id, err = nodes.CreateChatSession(ctx, tx, cs, nodes.Author{Name: "kernl"})
			return err
		})
		if err != nil {
			slog.Error("create chat session", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to create session")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chatSessionCreatedDTO{
			ID:        id,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func getChatSessionHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		ctx := r.Context()
		var cs *nodes.ChatSession
		err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
			var err error
			cs, err = nodes.GetChatSession(ctx, tx, id)
			return err
		})
		if errors.Is(err, graph.ErrNotFound) {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		if err != nil {
			slog.Error("get chat session", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to get session")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(newChatSessionDTO(cs))
	}
}

func postChatMessageHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		ctx := r.Context()

		var body struct {
			Content     string `json:"content"`
			ScopeNodeID string `json:"scope_node_id"`
			// DraftActions is the routing the user has on screen while triaging a
			// capture. It rides along with the message because the LLM runs from
			// the persisted session (the SSE stream), not from this request: a
			// draft that stays in the browser never reaches the DA, which would
			// then discuss a routing the user has already edited away.
			DraftActions []captureActionDTO `json:"draftActions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		// Verify session exists first.
		var cs *nodes.ChatSession
		if err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
			var err error
			cs, err = nodes.GetChatSession(ctx, tx, id)
			return err
		}); err != nil {
			if errors.Is(err, graph.ErrNotFound) {
				writeError(w, http.StatusNotFound, "session not found")
				return
			}
			slog.Error("get chat session for append", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to append message")
			return
		}

		cs.Messages = append(cs.Messages, nodes.ChatMessage{
			Role:      "user",
			Content:   body.Content,
			Timestamp: time.Now().UTC(),
		})
		cs.DerivedScopeNodeID = body.ScopeNodeID
		cs.DraftRouting = renderDraftRouting(body.DraftActions)

		err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
			return nodes.SaveChatSession(ctx, tx, cs, nodes.Author{Name: "kernl"})
		})
		if err != nil {
			slog.Error("post chat message", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to append message")
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}
}

// renderDraftRouting flattens the on-screen routing into the lines the prompt
// shows the DA. It is prose, not JSON: the DA reads this to know what the user
// changed, and only ever writes a routing back through the suggest_routing tool.
func renderDraftRouting(actions []captureActionDTO) string {
	var b strings.Builder
	for _, a := range actions {
		fmt.Fprintf(&b, "- %s: %s", a.Target, a.Title)
		if a.ProjectID != "" {
			fmt.Fprintf(&b, " (project %s)", a.ProjectID)
		}
		if a.DueDate != "" {
			fmt.Fprintf(&b, " (due %s)", a.DueDate)
		}
		if len(a.Tags) > 0 {
			fmt.Fprintf(&b, " [%s]", strings.Join(a.Tags, ", "))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// postLearnedCandidateHandler persists the user's decision on a DA-learned
// memory candidate. Keep (and Edit, which is Keep with a modified statement)
// writes a MemoryClaim with a `source` edge to the session; Discard records a
// negative signal on the session so the same candidate is not re-proposed.
func postLearnedCandidateHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		ctx := r.Context()

		var body struct {
			Action    string `json:"action"` // "keep" | "discard"
			Subject   string `json:"subject"`
			Statement string `json:"statement"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		// Confirm the session exists.
		if err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
			_, err := nodes.GetChatSession(ctx, tx, id)
			return err
		}); err != nil {
			if errors.Is(err, graph.ErrNotFound) {
				writeError(w, http.StatusNotFound, "session not found")
				return
			}
			slog.Error("learned: load session", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to load session")
			return
		}

		switch body.Action {
		case "keep":
			statement := strings.TrimSpace(body.Statement)
			if statement == "" {
				writeError(w, http.StatusBadRequest, "statement is required to keep")
				return
			}
			var claimID string
			if err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
				// Clear the candidate from the session so the UI card goes away.
				latestCS, err := nodes.GetChatSession(ctx, tx.AsReadTx(), id)
				if err == nil {
					clearLearnedCandidate(latestCS, statement)
					_ = nodes.SaveChatSession(ctx, tx, latestCS, nodes.Author{Name: "kernl"})
				}

				claimID, err = memory.AddMemoryClaim(ctx, tx, id, body.Subject, statement)
				return err
			}); err != nil {
				slog.Error("learned: add claim", "error", err)
				writeError(w, http.StatusInternalServerError, "failed to persist claim")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"status": "kept", "id": claimID})

		case "discard":
			statement := strings.TrimSpace(body.Statement)
			if err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
				latestCS, err := nodes.GetChatSession(ctx, tx.AsReadTx(), id)
				if err != nil {
					return err
				}
				latestCS.DiscardedCandidates = append(latestCS.DiscardedCandidates, statement)
				clearLearnedCandidate(latestCS, statement)
				return nodes.SaveChatSession(ctx, tx, latestCS, nodes.Author{Name: "kernl"})
			}); err != nil {
				slog.Error("learned: record discard", "error", err)
				writeError(w, http.StatusInternalServerError, "failed to record discard")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"status": "discarded"})

		default:
			writeError(w, http.StatusBadRequest, "action must be 'keep' or 'discard'")
		}
	}
}

func clearLearnedCandidate(cs *nodes.ChatSession, statement string) {
	for i := range cs.Messages {
		if cs.Messages[i].LearnedCandidate != nil && cs.Messages[i].LearnedCandidate.Statement == statement {
			cs.Messages[i].LearnedCandidate = nil
		}
	}
}

func chatEventsHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		ctx := r.Context()

		// Verify session exists.
		var exists bool
		_ = a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
			_, err := nodes.GetChatSession(ctx, tx, id)
			exists = err == nil
			return nil
		})
		if !exists {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}

		if !a.Config.LLM.IsSet() {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "data: {\"event\":\"error\",\"message\":\"No LLM provider configured. Add llm section to kernl.yaml.\"}\n\n")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}

		llmClient, err := chat.NewProviderFromConfig(configToLLMProviderConfig(a.Config.LLM))
		if err != nil {
			slog.Error("create llm client", "error", err)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "data: {\"event\":\"error\",\"message\":\"Failed to initialize LLM provider: %s\"}\n\n", err.Error())
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			slog.Error("ResponseWriter does not support flushing")
			return
		}

		writer := &sseEventWriter{w: w, flusher: flusher}
		// The engine leaves a goroutine behind (the learned-candidate proposal),
		// and it outlives this handler. Once we return, the ResponseWriter is
		// spent: writing to it is undefined and flushing it dereferences a nil
		// buffer — in a detached goroutine that panic takes the whole server down.
		defer writer.close()

		engine, err := chat.NewChatEngine(a, id, writer, llmClient, chat.NewGraphPermissionChecker(a))
		if err != nil {
			slog.Error("create chat engine", "error", err)
			return
		}
		if err := engine.RunSession(ctx); err != nil {
			slog.Error("chat engine run", "error", err)
		}
	}
}

func listNodesHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var out []map[string]any
		err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
			rows, err := tx.Query(`SELECT id, title, type FROM nodes WHERE deleted_at IS NULL`)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id, title, nodeType string
				if err := rows.Scan(&id, &title, &nodeType); err != nil {
					return err
				}
				out = append(out, map[string]any{
					"id":    id,
					"title": title,
					"type":  nodeType,
				})
			}
			return rows.Err()
		})
		if err != nil {
			slog.Error("list nodes", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to list nodes")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	}
}

// sseEventWriter adapts http.ResponseWriter to chat.ChatEventWriter.
//
// It goes inert when the handler that owns the ResponseWriter returns. The
// engine spawns a background goroutine that emits a state event when it is done
// extracting a memory, and that goroutine regularly outlives the stream — the
// code even said so, and assumed a late write was harmless. It is not: flushing
// a finished response panics on a nil buffer, and a panic in a goroutine nobody
// recovers kills the process. The event is not lost: the next stream reads the
// session from the graph.
type sseEventWriter struct {
	mu      sync.Mutex
	closed  bool
	w       http.ResponseWriter
	flusher http.Flusher
}

func (s *sseEventWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return len(p), nil
	}
	return s.w.Write(p)
}

func (s *sseEventWriter) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.flusher.Flush()
}

// close is called when the handler returns; every later write is dropped.
func (s *sseEventWriter) close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}

// configToLLMProviderConfig converts config.LLMConfig to chat.LLMProviderConfig.
func configToLLMProviderConfig(cfg config.LLMConfig) chat.LLMProviderConfig {
	return chat.LLMProviderConfig{
		Provider: cfg.Provider,
		APIKey:   cfg.APIKey,
		Model:    cfg.Model,
		Endpoint: cfg.Endpoint,
	}
}
