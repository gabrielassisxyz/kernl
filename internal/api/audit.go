package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/tags"
)

type DecisionResponse struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	Context    string    `json:"context"`
	Outcome    string    `json:"outcome"`
	Tags       []string  `json:"tags"`
	RelatedIDs []string  `json:"related_ids"`
}

func RegisterAuditRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/audit/decisions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var response []DecisionResponse
		err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
			decisions, err := nodes.ListDecisions(r.Context(), tx, nodes.DecisionFilter{
				Tags:  []string{tags.Autonomous},
				Limit: 100,
			})
			if err != nil {
				return err
			}

			response = make([]DecisionResponse, 0, len(decisions))
			for _, d := range decisions {
				in, err := edges.Incoming(r.Context(), tx, d.ID, edges.WithType("audit-log"))
				if err != nil {
					return err
				}

				var related []string
				for _, e := range in {
					related = append(related, e.Src)
				}

				response = append(response, DecisionResponse{
					ID:         d.ID,
					CreatedAt:  d.CreatedAt,
					Title:      d.Title,
					Body:       d.Body,
					Context:    d.Context,
					Outcome:    d.Outcome,
					Tags:       d.Tags,
					RelatedIDs: related,
				})
			}
			return nil
		})

		if err != nil {
			slog.Error("KERNL DISPATCH FAILURE: list audit decisions", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("KERNL DISPATCH FAILURE: list audit decisions — %v", err))
			return
		}

		if response == nil {
			response = []DecisionResponse{}
		}
		json.NewEncoder(w).Encode(response)
	})
}
