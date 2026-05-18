package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/epic"
)

func RegisterEpicRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/epics/{id}/events", func(w http.ResponseWriter, r *http.Request) {
		epicID := r.PathValue("id")
		if a.EpicEvents == nil {
			http.Error(w, "epic event hub not configured", http.StatusInternalServerError)
			return
		}
		serveEpicSSE(w, r, a.EpicEvents, epicID)
	})
}

func serveEpicSSE(w http.ResponseWriter, r *http.Request, hub *epic.EpicEventHub, epicID string) {
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

	ch, unsub := hub.Subscribe(epicID)
	defer unsub()

	buf := hub.GetBuffer(epicID)
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
			slog.Info("[epic-events] SSE client disconnected",
				"epicId", epicID)
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
