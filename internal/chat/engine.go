package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/planning"
)

// DenialReason explains why a permission was denied.
type DenialReason string

// PermissionChecker determines whether the agent may read a node.
type PermissionChecker interface {
	CanRead(ctx context.Context, nodeID string) (bool, DenialReason, error)
}

// Resolution describes the user's decision on a pending permission request.
type Resolution struct {
	ToolCallID      string
	Action          string // "allow" or "deny"
	RequestedNodeID string
	Feedback        *string
}

// ChatEngine handles a single chat session's request/response lifecycle.
type ChatEngine struct {
	sessionID         string
	eventWriter       ChatEventWriter
	llmClient         LLMClient
	permissionChecker PermissionChecker
	app               *app.App
}

// NewChatEngine creates a new chat engine for a session.
// Returns an error if pc is nil — a PermissionChecker must always be provided.
func NewChatEngine(app *app.App, sessionID string, w ChatEventWriter, llm LLMClient, pc PermissionChecker) (*ChatEngine, error) {
	if pc == nil {
		return nil, errors.New("permissionChecker is required")
	}
	return &ChatEngine{
		sessionID:         sessionID,
		eventWriter:       w,
		llmClient:         llm,
		permissionChecker: pc,
		app:               app,
	}, nil
}

// RunSession loads the chat session, streams LLM output, and handles tool calls.
func (e *ChatEngine) RunSession(ctx context.Context) error {
	var cs *nodes.ChatSession
	var di *nodes.DAIdentity

	// Load session and DA identity.
	if err := e.app.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		cs, err = nodes.GetChatSession(ctx, tx, e.sessionID)
		if err != nil {
			return err
		}
		di, err = nodes.GetDAIdentity(ctx, tx)
		if err != nil && !errors.Is(err, graph.ErrNotFound) {
			return err
		}
		return nil
	}); err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	// Emit state event first (for reconnections).
	if err := e.emitStateEvent(cs); err != nil {
		return err
	}

	// Telos: the user's standing identity/goals, always folded into context.
	// A load failure is non-fatal — Telos supplements the prompt, it must not
	// break the chat.
	telos, err := planning.LoadTelos(ctx, e.app.Graph)
	if err != nil {
		slog.Warn("load telos", "error", err)
		telos = ""
	}

	// Build messages.
	messages := e.buildMessages(cs, di, telos)

	// If there's a pending permission from a previous run, re-emit it and return.
	if cs.PendingPermission != nil {
		return e.emitPermissionRequiredEvent(cs.PendingPermission)
	}

	return e.runAgentLoop(ctx, cs, messages)
}

func (e *ChatEngine) runAgentLoop(ctx context.Context, cs *nodes.ChatSession, messages []Message) error {
	tools := []Tool{readNodeTool(), searchNotesTool()}

	resp, err := e.llmClient.Chat(ctx, messages, tools)
	if err != nil {
		_ = e.emitErrorEvent(fmt.Sprintf("LLM error: %v", err))
		return nil
	}

	// Text-only response.
	if len(resp.ToolCalls) == 0 {
		if resp.Content != "" {
			if err := e.emitTokenEvent(resp.Content); err != nil {
				return err
			}
			// Persist the assistant turn. Without this the session holds only
			// user messages, and the state event emitted on the next SSE
			// reconnect wipes every DA reply from the client.
			cs.Messages = append(cs.Messages, nodes.ChatMessage{
				Role:      "assistant",
				Content:   resp.Content,
				Timestamp: time.Now().UTC(),
			})
			if err := e.saveSession(ctx, cs); err != nil {
				slog.Warn("persist assistant message", "error", err)
			}
		}

		// Emit assistant_done so the frontend can unlock the input immediately.
		_ = e.writeEvent(map[string]any{"event": "assistant_done"})

		// U9: propose a learned memory from the just-completed exchange in the background.
		go e.proposeLearnedCandidate(ctx, e.sessionID, resp.Content)
		return e.emitDoneEvent()
	}

	// Handle tool calls: check permission, fetch node, recurse.
	for _, tc := range resp.ToolCalls {
		if tc.Function.Name == "read_node" {
			args := struct {
				NodeID string `json:"node_id"`
			}{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				_ = e.emitErrorEvent(fmt.Sprintf("invalid tool arguments: %v", err))
				return nil
			}

			allowed, _, err := e.permissionChecker.CanRead(ctx, args.NodeID)
			if err != nil {
				_ = e.emitErrorEvent(fmt.Sprintf("permission check error: %v", err))
				return nil
			}
			if !allowed {
				// Persist pending permission and emit event (U4 replaces this stub).
				pp := &nodes.PendingPermissionState{
					ToolCallID:        tc.ID,
					RequestedNodeID:   args.NodeID,
					RequestedNodePath: "",
					Status:            "pending",
					CreatedAt:         time.Now().UTC(),
				}
				cs.PendingPermission = pp
				if err := e.saveSession(ctx, cs); err != nil {
					return err
				}
				if err := e.emitPermissionRequiredEvent(pp); err != nil {
					return err
				}
				return nil
			}

			// Fetch node content.
			var content string
			if err := e.app.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
				note, err := nodes.GetNote(ctx, tx, args.NodeID)
				if err != nil {
					return err
				}
				content = note.Body
				return nil
			}); err != nil {
				_ = e.emitErrorEvent(fmt.Sprintf("read node: %v", err))
				return nil
			}

			// Append tool result to messages and recurse.
			messages = append(messages, Message{
				Role:    "tool",
				Content: fmt.Sprintf("read_node(%s) = %s", args.NodeID, content),
			})
			return e.runAgentLoop(ctx, cs, messages)
		}

		if tc.Function.Name == "search_notes" {
			args := struct {
				Query string `json:"query"`
			}{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				_ = e.emitErrorEvent(fmt.Sprintf("invalid tool arguments: %v", err))
				return nil
			}

			notes, err := planning.BuildContext(ctx, e.app.Graph, args.Query, 8)
			if err != nil {
				_ = e.emitErrorEvent(fmt.Sprintf("search error: %v", err))
				return nil
			}

			var b strings.Builder
			if len(notes) == 0 {
				b.WriteString("no matching notes")
			}
			for _, n := range notes {
				fmt.Fprintf(&b, "- [%s] %s: %s\n", n.ID, n.Title, n.Snippet)
			}

			messages = append(messages, Message{
				Role:    "tool",
				Content: fmt.Sprintf("search_notes(%q) =\n%s", args.Query, b.String()),
			})
			return e.runAgentLoop(ctx, cs, messages)
		}
	}

	return e.emitDoneEvent()
}

func (e *ChatEngine) buildMessages(cs *nodes.ChatSession, di *nodes.DAIdentity, telos string) []Message {
	var msgs []Message
	if di != nil && di.SystemPrompt != "" {
		msgs = append(msgs, Message{Role: "system", Content: di.SystemPrompt})
	}
	if telos != "" {
		msgs = append(msgs, Message{Role: "system", Content: telos})
	}
	for _, m := range cs.Messages {
		msgs = append(msgs, Message{Role: m.Role, Content: m.Content})
	}
	return msgs
}

func (e *ChatEngine) saveSession(ctx context.Context, cs *nodes.ChatSession) error {
	return e.app.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.SaveChatSession(ctx, tx, cs, nodes.Author{Name: "kernl"})
	})
}

func (e *ChatEngine) emitTokenEvent(content string) error {
	return e.writeEvent(map[string]any{
		"event":   "token",
		"content": content,
	})
}

func (e *ChatEngine) emitDoneEvent() error {
	return e.writeEvent(map[string]any{
		"event": "done",
	})
}

func (e *ChatEngine) emitErrorEvent(msg string) error {
	slog.Error("chat engine error", "msg", msg)
	return e.writeEvent(map[string]any{
		"event":   "error",
		"message": msg,
	})
}

func (e *ChatEngine) emitStateEvent(cs *nodes.ChatSession) error {
	return e.writeEvent(map[string]any{
		"event":              "state",
		"messages":           cs.Messages,
		"pending_permission": cs.PendingPermission,
	})
}

func (e *ChatEngine) emitPermissionRequiredEvent(pp *nodes.PendingPermissionState) error {
	return e.writeEvent(map[string]any{
		"event":        "permission_required",
		"tool_call_id": pp.ToolCallID,
		"node_id":      pp.RequestedNodeID,
		"node_path":    pp.RequestedNodePath,
		"description":  "The agent wants to read this node.",
	})
}

// learnedExtractorPrompt instructs a cheap second pass to surface a single
// durable memory worth remembering — not transient/transactional chatter.
const learnedExtractorPrompt = `You extract durable memories for a personal knowledge assistant.
Given the latest exchange, decide whether the USER expressed a lasting personal
preference, fact, or standing goal worth remembering across future sessions —
not a one-off or transactional request.
Respond ONLY with a JSON object, no prose, no code fences:
{"durable": true|false, "subject": "<2-4 word topic>", "statement": "<the memory in third person, one sentence>"}
If nothing durable was expressed, respond {"durable": false}.`

// learnedCandidate is the structured output of the post-response extractor.
type learnedCandidate struct {
	Durable   bool   `json:"durable"`
	Subject   string `json:"subject"`
	Statement string `json:"statement"`
}

// proposeLearnedCandidate runs a cheap second pass over the just-completed
// exchange and, if it finds a durable preference/fact, emits a
// `learned_candidate` event for the human-in-the-loop Keep/Edit/Discard card.
//
// Decision: runs detached so it never blocks the chat UI. If the user sends a
// message while this runs, it safely appends the candidate to the persisted
// session state via transaction.
func (e *ChatEngine) proposeLearnedCandidate(ctx context.Context, sessionID string, assistantContent string) {
	if strings.TrimSpace(assistantContent) == "" {
		return
	}

	var cs *nodes.ChatSession
	if err := e.app.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var err error
		cs, err = nodes.GetChatSession(context.Background(), tx, sessionID)
		return err
	}); err != nil {
		return
	}

	lastUser := lastUserMessage(cs)
	if lastUser == "" {
		return
	}

	msgs := []Message{
		{Role: "system", Content: learnedExtractorPrompt},
		{Role: "user", Content: fmt.Sprintf("User said: %q\nAssistant replied: %q", lastUser, assistantContent)},
	}
	// Use context.Background() because the request ctx might be canceled if the user
	// navigates away or sends a new message closing the connection.
	resp, err := e.llmClient.Chat(context.Background(), msgs, nil)
	if err != nil {
		slog.Warn("learned extraction", "error", err)
		return
	}

	cand, ok := parseLearnedCandidate(resp.Content)
	if !ok || !cand.Durable || strings.TrimSpace(cand.Statement) == "" {
		return
	}
	if isDiscardedCandidate(cs, cand.Statement) {
		return
	}

	// Update the session in a transaction to avoid racing with new user messages.
	err = e.app.Graph.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		latestCS, err := nodes.GetChatSession(context.Background(), tx.AsReadTx(), sessionID)
		if err != nil {
			return err
		}
		for i := len(latestCS.Messages) - 1; i >= 0; i-- {
			if latestCS.Messages[i].Role == "assistant" && latestCS.Messages[i].Content == assistantContent {
				latestCS.Messages[i].LearnedCandidate = &nodes.LearnedCandidateState{
					Subject:   cand.Subject,
					Statement: cand.Statement,
				}
				break
			}
		}
		return nodes.SaveChatSession(context.Background(), tx, latestCS, nodes.Author{Name: "kernl"})
	})

	if err != nil {
		slog.Warn("save learned candidate", "error", err)
		return
	}

	// Try to emit state to the current stream. If the user sent a new message,
	// this stream is likely closed, but that's fine—the new stream will pick it up from the DB.
	_ = e.app.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		latest, err := nodes.GetChatSession(context.Background(), tx, sessionID)
		if err == nil {
			_ = e.emitStateEvent(latest)
		}
		return nil
	})
}

// lastUserMessage returns the most recent user-authored message content.
func lastUserMessage(cs *nodes.ChatSession) string {
	for i := len(cs.Messages) - 1; i >= 0; i-- {
		if cs.Messages[i].Role == "user" {
			return cs.Messages[i].Content
		}
	}
	return ""
}

// isDiscardedCandidate reports whether the user already rejected this statement
// in the session, so it is not re-proposed (the discard negative signal).
func isDiscardedCandidate(cs *nodes.ChatSession, statement string) bool {
	want := strings.ToLower(strings.TrimSpace(statement))
	for _, d := range cs.DiscardedCandidates {
		if strings.ToLower(strings.TrimSpace(d)) == want {
			return true
		}
	}
	return false
}

// parseLearnedCandidate extracts the JSON object from a model reply, tolerating
// surrounding prose or code fences.
func parseLearnedCandidate(content string) (learnedCandidate, bool) {
	start := strings.IndexByte(content, '{')
	end := strings.LastIndexByte(content, '}')
	if start == -1 || end == -1 || end < start {
		return learnedCandidate{}, false
	}
	var cand learnedCandidate
	if err := json.Unmarshal([]byte(content[start:end+1]), &cand); err != nil {
		return learnedCandidate{}, false
	}
	return cand, true
}

func (e *ChatEngine) writeEvent(v map[string]any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(e.eventWriter, "data: %s\n\n", data)
	if err != nil {
		return err
	}
	e.eventWriter.Flush()
	return nil
}

func readNodeTool() Tool {
	return Tool{
		Name:        "read_node",
		Description: "Read a graph node by ID.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"node_id": {"type": "string"}
			},
			"required": ["node_id"]
		}`),
	}
}

func searchNotesTool() Tool {
	return Tool{
		Name:        "search_notes",
		Description: "Search the user's notes and graph by topic or keywords; returns the most relevant notes (id, title, snippet). Call this to find what the user has written before answering — do not ask the user for a node ID.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {"type": "string"}
			},
			"required": ["query"]
		}`),
	}
}

// ResumeSession continues after a permission is resolved. (U4 will implement fully.)
func (e *ChatEngine) ResumeSession(ctx context.Context, resolution string) error {
	return e.RunSession(ctx)
}
