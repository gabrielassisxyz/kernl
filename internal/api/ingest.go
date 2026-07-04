package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/ingest"
)

func RegisterIngestRoutes(mux *http.ServeMux, a *app.App) {
	vaultRoot := ""
	if a.Config != nil {
		vaultRoot = a.Config.Vault.Root
	}
	mm := ingest.NewManifestManager(vaultRoot)
	_ = mm.Load()

	// Use a real LLM-backed extractor only when an LLM is configured; otherwise
	// fall back to the stub so behavior is unchanged and no tokens are spent.
	// The same client backs the Update merge planner.
	llm := buildIngestLLM(a)
	var extractor ingest.Extractor = &ingest.StubExtractor{}
	if llm != nil {
		extractor = ingest.NewLLMExtractor(llm)
	}
	svc := ingest.NewService(a.Graph, mm, extractor)

	mux.HandleFunc("POST /api/ingest/trigger", ingestTriggerHandler(svc))
	mux.HandleFunc("POST /api/ingest/paste", ingestPasteHandler(svc, vaultRoot))
	mux.HandleFunc("POST /api/ingest/upload", ingestUploadHandler(svc, vaultRoot))
	mux.HandleFunc("GET /api/ingest/queue", ingestQueueListHandler(a))
	mux.HandleFunc("POST /api/ingest/queue/{id}/resolve", ingestQueueResolveHandler(a))
	mux.HandleFunc("POST /api/ingest/queue/{id}/merge-plan", ingestMergePlanHandler(a, llm))
}

// maxIngestBytes caps paste/upload size so a huge file can't stall extraction.
const maxIngestBytes = 2 << 20 // 2 MiB

// stageAndProcess writes raw ingest content to a staging file inside the vault
// and runs it through the SAME ProcessFile pipeline as trigger, so paste and
// upload share extraction, manifest dedup, and review creation. Processing is
// detached (the request returns 202) because extraction can call the LLM.
func stageAndProcess(svc *ingest.Service, vaultRoot string, content []byte) error {
	if vaultRoot == "" {
		return errors.New("no vault configured")
	}
	dir := filepath.Join(vaultRoot, ".kernl", "ingest-staging")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	staged := filepath.Join(dir, uuid.Must(uuid.NewV7()).String()+".md")
	if err := os.WriteFile(staged, content, 0o644); err != nil {
		return err
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := svc.ProcessFile(ctx, staged, ""); err != nil {
			slog.Error("ingest staged content failed", "error", err)
		}
	}()
	return nil
}

func ingestPasteHandler(svc *ingest.Service, vaultRoot string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Title string `json:"title"`
			Text  string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		text := strings.TrimSpace(body.Text)
		if text == "" {
			http.Error(w, "text is required", http.StatusBadRequest)
			return
		}
		if len(text) > maxIngestBytes {
			http.Error(w, "pasted content is too large", http.StatusRequestEntityTooLarge)
			return
		}

		// A title, when given, becomes a leading H1 so the extractor can anchor on it.
		content := text
		if t := strings.TrimSpace(body.Title); t != "" {
			content = "# " + t + "\n\n" + text
		}

		if err := stageAndProcess(svc, vaultRoot, []byte(content)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}
}

func ingestUploadHandler(svc *ingest.Service, vaultRoot string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(maxIngestBytes); err != nil {
			http.Error(w, "could not parse upload", http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "missing file field", http.StatusBadRequest)
			return
		}
		defer file.Close()

		ext := strings.ToLower(filepath.Ext(header.Filename))
		if ext != ".md" && ext != ".txt" {
			http.Error(w, "only .md and .txt files are supported", http.StatusUnsupportedMediaType)
			return
		}

		content, err := io.ReadAll(io.LimitReader(file, maxIngestBytes+1))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(content) > maxIngestBytes {
			http.Error(w, "file is too large", http.StatusRequestEntityTooLarge)
			return
		}
		if len(strings.TrimSpace(string(content))) == 0 {
			http.Error(w, "file is empty", http.StatusBadRequest)
			return
		}
		if !utf8.Valid(content) {
			http.Error(w, "file is not valid UTF-8 text", http.StatusBadRequest)
			return
		}

		if err := stageAndProcess(svc, vaultRoot, content); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}
}

// buildIngestLLM constructs the configured LLM client, or returns nil when no
// LLM is configured (so callers degrade gracefully without spending tokens).
func buildIngestLLM(a *app.App) chat.LLMClient {
	if a.Config == nil || !a.Config.LLM.IsSet() {
		return nil
	}
	llm, err := chat.NewProviderFromConfig(configToLLMProviderConfig(a.Config.LLM))
	if err != nil {
		slog.Error("ingest: failed to build LLM client", "error", err)
		return nil
	}
	return llm
}

func ingestTriggerHandler(svc *ingest.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			FilePath string `json:"file_path"`
			NodeID   string `json:"node_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		// Detached context: the request context is canceled the moment this
		// handler returns, which would abort the background write.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if err := svc.ProcessFile(ctx, body.FilePath, body.NodeID); err != nil {
				slog.Error("ingest trigger failed", "error", err)
			}
		}()

		w.WriteHeader(http.StatusAccepted)
	}
}

func ingestQueueListHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var items []*nodes.IngestReview
		err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
			var err error
			items, err = nodes.ListIngestReviews(r.Context(), tx, nodes.IngestReviewFilter{})
			return err
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if items == nil {
			items = []*nodes.IngestReview{}
		}
		json.NewEncoder(w).Encode(items)
	}
}

func ingestQueueResolveHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}

		var body struct {
			Action        string             `json:"action"`
			TargetNoteID  string             `json:"targetNoteId"`
			AcceptedHunks []ingest.MergeHunk `json:"acceptedHunks"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body) // empty body → Skip

		vaultRoot := ""
		if a.Config != nil {
			vaultRoot = a.Config.Vault.Root
		}

		var update *ingest.UpdateInput
		if body.Action == "Update" {
			update = &ingest.UpdateInput{TargetNoteID: body.TargetNoteID, AcceptedHunks: body.AcceptedHunks}
		}

		err := ingest.ResolveReview(r.Context(), a.Graph, vaultRoot, id, body.Action, update)
		if errors.Is(err, ingest.ErrActionNotImplemented) {
			http.Error(w, "action not implemented yet: "+body.Action, http.StatusNotImplemented)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

// ingestMergePlanHandler plans an Update merge: it resolves the target note and
// asks the LLM for the additive hunks the user will accept or reject. An empty
// targetNoteId in the response signals the frontend to fall back to Create Page.
func ingestMergePlanHandler(a *app.App, llm chat.LLMClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		if llm == nil {
			http.Error(w, "merge requires an LLM, none configured", http.StatusServiceUnavailable)
			return
		}

		plan, err := ingest.PlanMerge(r.Context(), a.Graph, llm, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(plan)
	}
}
