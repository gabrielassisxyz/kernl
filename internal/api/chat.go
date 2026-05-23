package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// RegisterChatRoutes registers chat REST + SSE endpoints.
func RegisterChatRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("POST /api/chat/sessions", createChatSessionHandler(a))
	mux.HandleFunc("GET /api/chat/sessions/{id}", getChatSessionHandler(a))
	mux.HandleFunc("POST /api/chat/sessions/{id}/messages", postChatMessageHandler(a))
	mux.HandleFunc("GET /api/chat/sessions/{id}/events", chatEventsHandler(a))
	mux.HandleFunc("GET /api/nodes", listNodesHandler(a))
}

func createChatSessionHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		cs := &nodes.ChatSession{
			Messages: []nodes.ChatMessage{},
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
		json.NewEncoder(w).Encode(map[string]any{
			"id":         id,
			"created_at": time.Now().UTC().Format(time.RFC3339),
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
		json.NewEncoder(w).Encode(cs)
	}
}

func postChatMessageHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		ctx := r.Context()

		var body struct {
			Content     string `json:"content"`
			ScopeNodeID string `json:"scope_node_id"`
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
		engine := chat.NewChatEngine(a, id, writer, NoopLLMClient{}, chat.NewGraphPermissionChecker(a))
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
type sseEventWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func (s *sseEventWriter) Write(p []byte) (int, error) { return s.w.Write(p) }
func (s *sseEventWriter) Flush()                        { s.flusher.Flush() }

// NoopLLMClient is a placeholder until a real LLM client is wired in.
type NoopLLMClient struct{}

func (NoopLLMClient) Chat(ctx context.Context, messages []chat.Message, tools []chat.Tool) (*chat.ChatResponse, error) {
	return &chat.ChatResponse{Content: "Hello! This is a stub response."}, nil
}
