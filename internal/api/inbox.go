package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

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
	mux.HandleFunc("POST /api/inbox", func(w http.ResponseWriter, r *http.Request) {
		createCaptureHandler(w, r, a)
	})
}

// createCaptureHandler creates a Capture from the web Quick Capture box,
// mirroring the `kernl capture` CLI (Capture node, pending tag) so the entry
// lands in the same inbox.
func createCaptureHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	var req struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		writeError(w, http.StatusBadRequest, "body is required")
		return
	}

	var id string
	err := a.Graph.DoWrite(r.Context(), func(tx *graph.WriteTx) error {
		var e error
		id, e = nodes.CreateCapture(r.Context(), tx, nodes.Capture{
			Body:         body,
			CapturedFrom: "web",
			Tags:         []string{"pending"},
		}, nodes.Author{Name: "web"})
		return e
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create capture")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// inboxItemDTO is the UI-shaped, camelCase view of a pending Capture consumed
// by web/pages/inbox.vue (InboxItemData). The raw nodes.Capture struct carries
// PascalCase fields and no subtitle, so it is mapped explicitly here.
type inboxItemDTO struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	Title           string `json:"title"`
	Subtitle        string `json:"subtitle"`
	SuggestedAction string `json:"suggestedAction"`
	Flagged         bool   `json:"flagged"`
}

// captureTitle derives a display title for a capture: its explicit Title when
// set, otherwise the first line of the body, truncated for the row.
func captureTitle(c *nodes.Capture) string {
	if t := strings.TrimSpace(c.Title); t != "" {
		return t
	}
	body := strings.TrimSpace(c.Body)
	if i := strings.IndexByte(body, '\n'); i >= 0 {
		body = body[:i]
	}
	if len(body) > 60 {
		return body[:60] + "…"
	}
	if body == "" {
		return "Untitled capture"
	}
	return body
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

	items := make([]inboxItemDTO, 0, len(pending))
	for _, c := range pending {
		typ := strings.ToUpper(strings.TrimSpace(c.CapturedFrom))
		if typ == "" {
			typ = "CAPTURE"
		}
		items = append(items, inboxItemDTO{
			ID:              c.ID,
			Type:            typ,
			Title:           captureTitle(c),
			Subtitle:        c.Body,
			SuggestedAction: c.SuggestedAction,
			Flagged:         c.SuggestedAction != "",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
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
	rollups, err := inbox.Rollups(r.Context(), a.Graph)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"rollups": rollups})
}
