package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// RegisterChatResolveRoutes registers the permission resolution endpoint.
func RegisterChatResolveRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("POST /api/chat/sessions/{id}/resolve-permission", chatResolveHandler(a))
}

func chatResolveHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		ctx := r.Context()

		// Load the session.
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
			slog.Error("resolve load session", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to load session")
			return
		}

		var body struct {
			ToolCallID string  `json:"tool_call_id"`
			Action     string  `json:"action"`
			Feedback   *string `json:"feedback,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		if cs.PendingPermission == nil || cs.PendingPermission.ToolCallID != body.ToolCallID {
			writeError(w, http.StatusConflict, "no matching pending permission")
			return
		}

		// Clear pending permission.
		cs.PendingPermission = nil
		if err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
			return nodes.SaveChatSession(ctx, tx, cs, nodes.Author{Name: "kernl"})
		}); err != nil {
			slog.Error("resolve save session", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to save session")
			return
		}

		wantsSSE := r.Header.Get("Accept") == "text/event-stream"
		if wantsSSE {
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
			if err := engine.ResumeSession(ctx, body.Action); err != nil {
				slog.Error("resume session", "error", err)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"status": "resolved"})
	}
}
