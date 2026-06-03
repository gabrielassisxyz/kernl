package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

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
	var extractor ingest.Extractor = &ingest.StubExtractor{}
	if a.Config != nil && a.Config.LLM.IsSet() {
		if llm, err := chat.NewProviderFromConfig(configToLLMProviderConfig(a.Config.LLM)); err != nil {
			slog.Error("ingest: failed to build LLM client, falling back to stub extractor", "error", err)
		} else {
			extractor = ingest.NewLLMExtractor(llm)
		}
	}
	svc := ingest.NewService(a.Graph, mm, extractor)

	mux.HandleFunc("POST /api/ingest/trigger", ingestTriggerHandler(svc))
	mux.HandleFunc("GET /api/ingest/queue", ingestQueueListHandler(a))
	mux.HandleFunc("POST /api/ingest/queue/{id}/resolve", ingestQueueResolveHandler(a))
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
			Action string `json:"action"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body) // empty body → Skip

		vaultRoot := ""
		if a.Config != nil {
			vaultRoot = a.Config.Vault.Root
		}

		err := ingest.ResolveReview(r.Context(), a.Graph, vaultRoot, id, body.Action)
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
