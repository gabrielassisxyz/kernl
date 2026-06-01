package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gabrielassisxyz/kernl/internal/app"
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
	svc := ingest.NewService(a.Graph, mm, &ingest.StubExtractor{})

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

		go func() {
			if err := svc.ProcessFile(r.Context(), body.FilePath, body.NodeID); err != nil {
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

		err := a.Graph.DoWrite(r.Context(), func(tx *graph.WriteTx) error {
			return nodes.DeleteIngestReview(r.Context(), tx, id, nodes.Author{Name: "api"})
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}
