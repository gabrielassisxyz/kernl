package api

import (
	"fmt"
	"log/slog"
	"net/http"
)

func RegisterStreamRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sessions/{id}/events", sessionEventsHandler)
}

func sessionEventsHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher.Flush()

	sessionID := r.PathValue("id")

	ctx := r.Context()
	select {
	case <-ctx.Done():
		slog.Info("SSE client disconnected", "sessionId", sessionID)
		return
	case <-r.Context().Done():
		_, _ = fmt.Fprintf(w, "data: {\"type\":\"connected\",\"sessionId\":\"%s\"}\n\n", sessionID)
		flusher.Flush()
	}
}