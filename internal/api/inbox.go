package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/bookmarks"
	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/inbox"
	"github.com/gabrielassisxyz/kernl/internal/ingest"
	"github.com/gabrielassisxyz/kernl/internal/suggestlog"
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
	mux.HandleFunc("POST /api/inbox/batch/analyze", func(w http.ResponseWriter, r *http.Request) {
		analyzeInboxBatchHandler(w, r, a)
	})
	mux.HandleFunc("POST /api/inbox/batch/preview", func(w http.ResponseWriter, r *http.Request) {
		previewInboxBatchHandler(w, r)
	})
	mux.HandleFunc("POST /api/inbox/batch", func(w http.ResponseWriter, r *http.Request) {
		createInboxBatchHandler(w, r, a)
	})
	mux.HandleFunc("GET /api/inbox/batch-log", func(w http.ResponseWriter, r *http.Request) {
		getInboxBatchHandler(w, r, a)
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
	ID       string `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	// SuggestedActions is the list of nodes the DA proposes this capture becomes
	// — a capture is routinely several things at once. Empty while unclassified.
	SuggestedActions  []captureActionDTO `json:"suggestedActions"`
	BatchID           string             `json:"batchId"`
	BatchSource       string             `json:"batchSource"`
	BatchSequence     int                `json:"batchSequence"`
	BatchTimestamp    string             `json:"batchTimestamp"`
	BatchContextTitle string             `json:"batchContextTitle"`
	HasPrep           bool               `json:"hasPrep"`
	Flagged           bool               `json:"flagged"`
}

// captureActionDTO is the camelCase wire view of a nodes.CaptureAction (whose
// json tags are the snake_case attrs shape). It is both what the DA suggests and
// what the inbox modal posts back.
type captureActionDTO struct {
	Target             string   `json:"target"`
	Title              string   `json:"title"`
	Body               string   `json:"body"`
	ProjectID          string   `json:"projectId"`
	ProjectTitle       string   `json:"projectTitle"`
	ProjectDescription string   `json:"projectDescription"`
	InitialTasks       []string `json:"initialTasks"`
	Tags               []string `json:"tags"`
	LinkTo             string   `json:"linkTo"`
}

func toCaptureActionDTOs(actions []nodes.CaptureAction) []captureActionDTO {
	out := make([]captureActionDTO, 0, len(actions))
	for _, a := range actions {
		out = append(out, captureActionDTO{
			Target:             a.Target,
			Title:              a.Title,
			Body:               a.Body,
			ProjectID:          a.ProjectID,
			ProjectTitle:       a.ProjectTitle,
			ProjectDescription: a.ProjectDescription,
			InitialTasks:       a.InitialTasks,
			Tags:               a.Tags,
			LinkTo:             a.LinkTo,
		})
	}
	return out
}

func fromCaptureActionDTOs(actions []captureActionDTO) []inbox.Action {
	out := make([]inbox.Action, 0, len(actions))
	for _, a := range actions {
		out = append(out, inbox.Action{
			Target:             a.Target,
			Title:              a.Title,
			Body:               a.Body,
			ProjectID:          a.ProjectID,
			ProjectTitle:       a.ProjectTitle,
			ProjectDescription: a.ProjectDescription,
			InitialTasks:       a.InitialTasks,
			Tags:               a.Tags,
			LinkTo:             a.LinkTo,
		})
	}
	return out
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
	if utf8.RuneCountInString(body) > 60 {
		return truncateRunes(body, 60) + "…"
	}
	if body == "" {
		return "Untitled capture"
	}
	return body
}

// truncateRunes returns the first n runes of s. Slicing by byte offset (e.g.
// body[:60]) corrupts multi-byte UTF-8 text (accented characters, emoji),
// which this app must handle correctly for Portuguese content.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
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
			ID:                c.ID,
			Type:              typ,
			Title:             captureTitle(c),
			Subtitle:          c.Body,
			SuggestedActions:  toCaptureActionDTOs(c.SuggestedActions),
			BatchID:           c.BatchID,
			BatchSource:       c.BatchSource,
			BatchSequence:     c.BatchSequence,
			BatchTimestamp:    c.BatchTimestamp,
			BatchContextTitle: c.BatchContextTitle,
			HasPrep:           prepSet[c.ID],
			Flagged:           len(c.SuggestedActions) > 0,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

func getInboxBatchHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	batchID := strings.TrimSpace(r.URL.Query().Get("batchId"))
	if batchID == "" {
		writeError(w, http.StatusBadRequest, "batch id is required")
		return
	}

	logStore := inbox.NewBatchLogStore(a.Graph)
	record, err := logStore.Get(r.Context(), batchID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to load batch log")
		return
	}

	if record != nil {
		response := buildBatchLogResponse(record)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	// Fallback for older batches created before batch_logs existed. Return the
	// same BatchLogResponse object shape as the primary path above so the
	// client always gets one consistent contract regardless of batch age.
	var captures []*nodes.Capture
	if err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
		var err error
		captures, err = nodes.ListCaptures(r.Context(), tx, nodes.CaptureFilter{BatchID: batchID})
		return err
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load batch")
		return
	}
	response := buildBatchLogResponseFromCaptures(batchID, captures)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// buildBatchLogResponseFromCaptures reconstructs a BatchLogResponse for
// batches created before batch_logs existed, so the primary and fallback
// paths of getInboxBatchHandler return the same camelCase object shape. The
// original raw/final split is not recoverable for these legacy batches, so
// both entry lists are approximated 1:1 from the surviving captures; the
// richer per-capture view (with the same "CAPTURE" Type default applied by
// getPendingCapturesHandler) is carried in Captures for callers that want it.
func buildBatchLogResponseFromCaptures(batchID string, captures []*nodes.Capture) inbox.BatchLogResponse {
	resp := inbox.BatchLogResponse{
		BatchID:           batchID,
		RawEntries:        make([]inbox.BatchLogEntry, 0, len(captures)),
		FinalEntries:      make([]inbox.BatchLogEntry, 0, len(captures)),
		CreatedCaptureIDs: make([]string, 0, len(captures)),
		Captures:          make([]any, 0, len(captures)),
	}
	for _, c := range captures {
		if resp.Source == "" {
			resp.Source = c.BatchSource
		}
		if resp.ContextTitle == "" {
			resp.ContextTitle = c.BatchContextTitle
		}
		entry := inbox.BatchLogEntry{
			Sequence:  c.BatchSequence,
			Body:      c.Body,
			Timestamp: c.BatchTimestamp,
		}
		resp.RawEntries = append(resp.RawEntries, entry)
		resp.FinalEntries = append(resp.FinalEntries, entry)
		resp.CreatedCaptureIDs = append(resp.CreatedCaptureIDs, c.ID)

		typ := strings.ToUpper(strings.TrimSpace(c.CapturedFrom))
		if typ == "" {
			typ = "CAPTURE"
		}
		resp.Captures = append(resp.Captures, inboxItemDTO{
			ID:                c.ID,
			Type:              typ,
			Title:             captureTitle(c),
			Subtitle:          c.Body,
			BatchID:           c.BatchID,
			BatchSource:       c.BatchSource,
			BatchSequence:     c.BatchSequence,
			BatchTimestamp:    c.BatchTimestamp,
			BatchContextTitle: c.BatchContextTitle,
		})
	}
	return resp
}

func buildBatchLogResponse(record *inbox.BatchLogRecord) inbox.BatchLogResponse {
	var rawSegments []inbox.BatchSegment
	_ = json.Unmarshal([]byte(record.RawSegmentsJSON), &rawSegments)
	var finalSegments []inbox.FinalBatchSegment
	_ = json.Unmarshal([]byte(record.FinalSegmentsJSON), &finalSegments)
	var createdIDs []string
	_ = json.Unmarshal([]byte(record.CreatedCaptureIDsJSON), &createdIDs)

	resp := inbox.BatchLogResponse{
		BatchID:           record.ID,
		Source:            record.Source,
		Separator:         record.Separator,
		ContextTitle:      record.ContextTitle,
		RawText:           record.RawText,
		CreatedCaptureIDs: createdIDs,
		RawEntries:        make([]inbox.BatchLogEntry, 0, len(rawSegments)),
		FinalEntries:      make([]inbox.BatchLogEntry, 0, len(finalSegments)),
	}
	for _, seg := range rawSegments {
		resp.RawEntries = append(resp.RawEntries, inbox.BatchLogEntry{
			Sequence:  seg.Sequence,
			Body:      seg.Body,
			Timestamp: seg.Timestamp,
		})
	}
	for _, seg := range finalSegments {
		resp.FinalEntries = append(resp.FinalEntries, inbox.BatchLogEntry{
			Sequence:  seg.Sequence,
			Body:      seg.Body,
			Timestamp: seg.Timestamp,
		})
	}
	return resp
}

type inboxBatchRequest struct {
	Text      string `json:"text"`
	Source    string `json:"source"`
	SplitMode string `json:"splitMode"`
	Separator string `json:"separator"`
	// ContextTitle is the display title for the batch, either explicit or the
	// suggestion the client accepted after reviewing an /analyze response.
	ContextTitle string `json:"contextTitle"`
	// FinalSegments, sent only to POST /api/inbox/batch, echoes back the exact
	// capture candidates the client reviewed and approved from a prior
	// /analyze response. When present, CreateBatchWithLLM persists these
	// verbatim instead of re-running (non-deterministic) LLM enrichment.
	FinalSegments []inbox.FinalBatchSegment `json:"finalSegments,omitempty"`
}

func analyzeInboxBatchHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	input, ok := decodeInboxBatchRequest(w, r)
	if !ok {
		return
	}
	llm := buildOptionalBatchLLM(a)
	analysis, err := inbox.AnalyzeBatchWithLLM(r.Context(), input, llm)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(analysis)
}

func previewInboxBatchHandler(w http.ResponseWriter, r *http.Request) {
	input, ok := decodeInboxBatchRequest(w, r)
	if !ok {
		return
	}
	segments, err := inbox.PreviewBatch(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"segments": segments})
}

func createInboxBatchHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	input, ok := decodeInboxBatchRequest(w, r)
	if !ok {
		return
	}
	llm := buildOptionalBatchLLM(a)
	result, err := inbox.CreateBatchWithLLM(r.Context(), a.Graph, input, llm)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

// buildOptionalBatchLLM returns a configured LLM client when one is available,
// or nil when LLM enrichment should be skipped. Batch analysis/create must
// remain usable without an LLM, unlike ingest workflows.
func buildOptionalBatchLLM(a *app.App) chat.LLMClient {
	if a.Config == nil || !a.Config.LLM.IsSet() {
		return nil
	}
	llm, err := chat.NewProviderFromConfig(configToLLMProviderConfig(a.Config.LLM))
	if err != nil {
		slog.Error("batch: failed to build LLM client", "error", err)
		return nil
	}
	return llm
}

func decodeInboxBatchRequest(w http.ResponseWriter, r *http.Request) (inbox.BatchInput, bool) {
	var req inboxBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return inbox.BatchInput{}, false
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return inbox.BatchInput{}, false
	}
	if len(text) > maxIngestBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "batch content is too large")
		return inbox.BatchInput{}, false
	}
	splitMode := req.SplitMode
	if strings.TrimSpace(splitMode) == "" {
		splitMode = req.Separator
	}
	return inbox.BatchInput{
		RawText:       text,
		Source:        req.Source,
		SplitMode:     splitMode,
		ContextTitle:  req.ContextTitle,
		FinalSegments: req.FinalSegments,
	}, true
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
// it accepts the list of nodes the capture becomes, each with its own title,
// project, tags and link — so one capture can land as a note *and* the task it
// implies, which the single {action} field cannot express.
func processCaptureHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	var req struct {
		Actions       []captureActionDTO `json:"actions"`
		TargetNoteID  string             `json:"targetNoteId"`  // update only
		AcceptedHunks []ingest.MergeHunk `json:"acceptedHunks"` // update only
		// The DA's original suggestion, echoed back so we can learn from
		// overrides. Empty when the client doesn't send it.
		SuggestedActions []captureActionDTO `json:"suggestedActions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Actions) == 0 {
		http.Error(w, "at least one action is required", http.StatusBadRequest)
		return
	}

	vaultRoot := a.Config.Vault.Root
	bookmarksDir := filepath.Join(vaultRoot, ".kernl", "bookmarks")
	archiver := bookmarks.NewArchiver(nil, bookmarksDir)

	err := inbox.ProcessCapture(r.Context(), a.Graph, vaultRoot, archiver, id, inbox.ProcessRequest{
		Actions:       fromCaptureActionDTOs(req.Actions),
		TargetNoteID:  req.TargetNoteID,
		AcceptedHunks: req.AcceptedHunks,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logSuggestionOverride(vaultRoot, id, req.SuggestedActions, req.Actions)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// logSuggestionOverride records the pair (what the DA proposed, what the user
// actually processed) whenever they differ, for later prompt tuning. With
// fan-out the interesting edit is the shape of the whole list — a split the DA
// missed, a target flipped, a title rewritten — so the list is logged as one
// unit rather than field by field.
func logSuggestionOverride(vaultRoot, captureID string, suggested, accepted []captureActionDTO) {
	if len(suggested) == 0 {
		return
	}
	original, err := json.Marshal(suggested)
	if err != nil {
		return
	}
	edited, err := json.Marshal(accepted)
	if err != nil {
		return
	}
	if string(original) == string(edited) {
		return
	}
	_ = suggestlog.Log(vaultRoot, suggestlog.Edit{
		Surface: "inbox", Field: "actions",
		Original: string(original), Edited: string(edited), Context: captureID,
	})
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
