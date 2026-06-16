package api

import (
	"encoding/json"
	"net/http"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/memory"
)

func RegisterMemoryRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/memory/topics", func(w http.ResponseWriter, r *http.Request) {
		var topics []string

		err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
			rows, err := tx.Query(`SELECT DISTINCT json_extract(attrs, '$.subject') FROM nodes WHERE type = 'memory_claim' AND json_extract(attrs, '$.subject') IS NOT NULL`)
			if err != nil {
				return err
			}
			defer rows.Close()

			for rows.Next() {
				var topic string
				if err := rows.Scan(&topic); err != nil {
					continue
				}
				if topic != "" {
					topics = append(topics, topic)
				}
			}
			return rows.Err()
		})

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if topics == nil {
			topics = []string{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"topics": topics})
	})

	mux.HandleFunc("GET /api/memory/claims", func(w http.ResponseWriter, r *http.Request) {
		topic := r.URL.Query().Get("topic")
		if topic == "" {
			http.Error(w, "missing topic parameter", http.StatusBadRequest)
			return
		}

		var claims []*nodes.MemoryClaim
		err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
			var err error
			claims, err = memory.SynthesizeTopic(r.Context(), tx, topic)
			return err
		})

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if claims == nil {
			claims = make([]*nodes.MemoryClaim, 0)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"claims": claims})
	})

	mux.HandleFunc("POST /api/memory/claims/{id}/refute", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "missing claim id", http.StatusBadRequest)
			return
		}

		var req struct {
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		var refutationID string
		err := a.Graph.DoWrite(r.Context(), func(tx *graph.WriteTx) error {
			var err error
			refutationID, err = memory.RefuteMemoryClaim(r.Context(), tx, id, req.Reason)
			return err
		})

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status": "refuted",
			"id":     refutationID,
		})
	})
}
