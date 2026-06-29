package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/bookmarks"
	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/inbox"
	"github.com/gabrielassisxyz/kernl/internal/ingest"
)

func RegisterInboxRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/inbox/pending", func(w http.ResponseWriter, r *http.Request) {
		getPendingCapturesHandler(w, r, a)
	})
	mux.HandleFunc("POST /api/inbox/{id}/convert", func(w http.ResponseWriter, r *http.Request) {
		convertCaptureHandler(w, r, a)
	})
	mux.HandleFunc("POST /api/inbox/{id}/process", func(w http.ResponseWriter, r *http.Request) {
		processCaptureHandler(w, r, a)
	})
	mux.HandleFunc("POST /api/inbox/{id}/merge-plan", func(w http.ResponseWriter, r *http.Request) {
		mergePlanCaptureHandler(w, r, a)
	})
	mux.HandleFunc("POST /api/inbox/{id}/reopen", func(w http.ResponseWriter, r *http.Request) {
		reopenCaptureHandler(w, r, a)
	})
	mux.HandleFunc("POST /api/inbox/{id}/prep", func(w http.ResponseWriter, r *http.Request) {
		prepCaptureHandler(w, r, a)
	})
	mux.HandleFunc("GET /api/inbox/{id}/prep", func(w http.ResponseWriter, r *http.Request) {
		getPrepHandler(w, r, a)
	})
	mux.HandleFunc("GET /api/nodes/{id}/briefing", func(w http.ResponseWriter, r *http.Request) {
		getBriefingHandler(w, r, a)
	})
	mux.HandleFunc("GET /api/inbox/processed", func(w http.ResponseWriter, r *http.Request) {
		getProcessedHandler(w, r, a)
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
	ID                 string `json:"id"`
	Type               string `json:"type"`
	Title              string `json:"title"`
	Subtitle           string `json:"subtitle"`
	SuggestedAction    string `json:"suggestedAction"`
	SuggestedProjectID string `json:"suggestedProjectId"`
	HasPrep            bool   `json:"hasPrep"`
	Flagged            bool   `json:"flagged"`
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
	prepSet := map[string]bool{}

	err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		pending, err = nodes.ListCaptures(ctx, tx, nodes.CaptureFilter{
			Tags: []string{"pending"},
		})
		if err != nil {
			return err
		}
		for _, c := range pending {
			if prepID, err := inbox.PrepFor(ctx, tx, c.ID); err == nil && prepID != "" {
				prepSet[c.ID] = true
			}
		}
		return nil
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
			ID:                 c.ID,
			Type:               typ,
			Title:              captureTitle(c),
			Subtitle:           c.Body,
			SuggestedAction:    c.SuggestedAction,
			SuggestedProjectID: c.SuggestedProjectID,
			HasPrep:            prepSet[c.ID],
			Flagged:            c.SuggestedAction != "",
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

// processCaptureHandler is the structured successor to convertCaptureHandler:
// it accepts an explicit target plus optional project/link/title so the inbox
// modal can file a capture as a task under a project, link a note/bookmark to
// another node, or override the title — none of which the single {action} field
// can express.
func processCaptureHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	var req struct {
		Target        string             `json:"target"`        // note | bookmark | task | discard | update | convert
		ProjectID     string             `json:"projectId"`     // task only
		LinkTo        string             `json:"linkTo"`        // note/bookmark only
		Title         string             `json:"title"`         // optional override
		TargetNoteID  string             `json:"targetNoteId"`  // update only
		AcceptedHunks []ingest.MergeHunk `json:"acceptedHunks"` // update only
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Target == "" {
		http.Error(w, "target is required", http.StatusBadRequest)
		return
	}

	vaultRoot := a.Config.Vault.Root
	bookmarksDir := filepath.Join(vaultRoot, ".kernl", "bookmarks")
	archiver := bookmarks.NewArchiver(nil, bookmarksDir)

	err := inbox.ProcessCapture(r.Context(), a.Graph, vaultRoot, archiver, id, inbox.ProcessRequest{
		Target:        req.Target,
		ProjectID:     req.ProjectID,
		LinkTo:        req.LinkTo,
		Title:         req.Title,
		TargetNoteID:  req.TargetNoteID,
		AcceptedHunks: req.AcceptedHunks,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// mergePlanCaptureHandler plans an "Update" for a capture: it resolves the
// best-matching note and asks the LLM for the additive hunks the user will
// accept or reject in DiffSuggest. An empty targetNoteId signals the frontend to
// fall back to creating a note.
func mergePlanCaptureHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	if !a.Config.LLM.IsSet() {
		writeError(w, http.StatusServiceUnavailable, "no llm provider configured")
		return
	}
	llm, err := chat.NewProviderFromConfig(configToLLMProviderConfig(a.Config.LLM))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "llm: "+err.Error())
		return
	}
	plan, err := inbox.PlanCaptureMerge(r.Context(), a.Graph, llm, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plan)
}

// reopenCaptureHandler reverses a process: removes the derived node and returns
// the capture to the pending queue (the inbox undo).
func reopenCaptureHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	if err := inbox.Reopen(r.Context(), a.Graph, a.Config.Vault.Root, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// prepCaptureHandler manually triggers a DA briefing for a capture and returns
// the prep note. Idempotent: an already-prepped capture returns its note.
func prepCaptureHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	if !a.Config.LLM.IsSet() {
		writeError(w, http.StatusServiceUnavailable, "no llm provider configured")
		return
	}
	llm, err := chat.NewProviderFromConfig(configToLLMProviderConfig(a.Config.LLM))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "llm: "+err.Error())
		return
	}
	noteID, err := inbox.Prep(r.Context(), a.Graph, llm, a.Config.Vault.Root, a.Config.Inbox.DASubdir, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writePrepNote(w, r, a, noteID)
}

// getPrepHandler returns a capture's existing prep note, or 404 if none.
func getPrepHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	var noteID string
	err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
		var e error
		noteID, e = inbox.PrepFor(r.Context(), tx, id)
		return e
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if noteID == "" {
		http.Error(w, "no prep", http.StatusNotFound)
		return
	}
	writePrepNote(w, r, a, noteID)
}

// getBriefingHandler returns the DA briefing surfaced for a processed node
// (task/note/bookmark), or 404 if none.
func getBriefingHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	var noteID string
	err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
		var e error
		noteID, e = inbox.BriefingFor(r.Context(), tx, id)
		return e
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if noteID == "" {
		http.Error(w, "no briefing", http.StatusNotFound)
		return
	}
	writePrepNote(w, r, a, noteID)
}

// writePrepNote responds with the prep note's id, title, and body.
func writePrepNote(w http.ResponseWriter, r *http.Request, a *app.App, noteID string) {
	var note *nodes.Note
	err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
		var e error
		note, e = nodes.GetNote(r.Context(), tx, noteID)
		return e
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": note.ID, "title": note.Title, "body": note.Body})
}

func getProcessedHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	items, err := inbox.ListProcessed(r.Context(), a.Graph)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if items == nil {
		items = []inbox.ProcessedItem{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
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
