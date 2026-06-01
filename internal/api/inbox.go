package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/bookmarks"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/inbox"
)

func RegisterInboxRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/inbox/pending", func(w http.ResponseWriter, r *http.Request) {
		getPendingCapturesHandler(w, r, a)
	})
	mux.HandleFunc("POST /api/inbox/{id}/convert", func(w http.ResponseWriter, r *http.Request) {
		convertCaptureHandler(w, r, a)
	})
	mux.HandleFunc("GET /api/inbox/rollups", func(w http.ResponseWriter, r *http.Request) {
		getRollupsHandler(w, r, a)
	})
}

func getPendingCapturesHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	ctx := r.Context()
	var pending []*nodes.Capture

	err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		pending, err = nodes.ListCaptures(ctx, tx, nodes.CaptureFilter{
			Tags: []string{"pending"},
		})
		return err
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pending)
}

func convertCaptureHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	var req struct {
		Action string `json:"action"` // note, bookmark, discard
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Provide an archiver. Usually, we store bookmarks under ~/.kernl/bookmarks
	// We check Vault root to base this off, or fallback.
	vaultRoot := a.Config.Vault.Root
	bookmarksDir := filepath.Join(vaultRoot, ".kernl", "bookmarks")
	archiver := bookmarks.NewArchiver(nil, bookmarksDir)

	err := inbox.Process(r.Context(), a.Graph, vaultRoot, archiver, id, req.Action)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func getRollupsHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	w.Header().Set("Content-Type", "application/json")
	// Stub for rollups
	json.NewEncoder(w).Encode(map[string]any{"rollups": []string{}})
}
