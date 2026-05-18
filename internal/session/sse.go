package session

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

// ServeSSE streams terminal events to an HTTP client using Server-Sent Events.
// It replays buffered events then forwards live events until the client disconnects
// or the session exits.
func (m *SessionConnectionManager) ServeSSE(w http.ResponseWriter, r *http.Request, sessionID string) {
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

	m.Connect(sessionID)

	ch, unsub := m.ConnectAndSubscribe(sessionID)
	defer unsub()

	buf := m.GetBuffer(sessionID)
	for _, evt := range buf {
		data, err := json.Marshal(evt)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
	}
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			slog.Info("[connection-manager] SSE client disconnected",
				"sessionId", sessionID)
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(TerminalEvent{
				Type:    evt.Type,
				Content: evt.Content,
				BeadID:  evt.BeadID,
				Time:    evt.Time,
			})
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}