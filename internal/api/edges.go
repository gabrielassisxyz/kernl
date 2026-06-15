package api

import (
	"encoding/json"
	"net/http"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
)

type graphEdge struct {
	ID    string `json:"id"`
	Src   string `json:"src"`
	Dst   string `json:"dst"`
	Label string `json:"label"`
}

// RegisterEdgeRoutes exposes the raw stored edges so the graph view can render
// the actual connections between nodes. Unlike /api/nodes/{id}/related (a
// computed relevance heuristic), this returns the edges table as persisted —
// the source of truth for validating that connections are being made correctly.
func RegisterEdgeRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/edges", func(w http.ResponseWriter, r *http.Request) {
		var out []graphEdge
		err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
			rows, err := tx.Query(`
				SELECT e.id, e.src, e.dst, e.label
				FROM edges e
				JOIN nodes s ON s.id = e.src AND s.deleted_at IS NULL
				JOIN nodes d ON d.id = e.dst AND d.deleted_at IS NULL`)
			if err != nil {
				return err
			}
			defer rows.Close()
			out = []graphEdge{}
			for rows.Next() {
				var e graphEdge
				if err := rows.Scan(&e.ID, &e.Src, &e.Dst, &e.Label); err != nil {
					return err
				}
				out = append(out, e)
			}
			return rows.Err()
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})
}
