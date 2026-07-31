package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// decisionsPageLimit caps how many decisions one request returns. There is
// no cursor, offset, or continuation token behind it - real pagination is
// tracked separately (kernl task 019fb5cc-3da4-7769-983f-61ab77a88e77) and
// deliberately not built here. What this cap must not do is drop rows
// without saying so: see the X-Kernl-Truncated response header below.
const decisionsPageLimit = 100

type DecisionResponse struct {
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"createdAt"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	Context     string    `json:"context"`
	Outcome     string    `json:"outcome"`
	ImpactOnUse *string   `json:"impactOnUse"`
	Tags        []string  `json:"tags"`
	RelatedIDs  []string  `json:"relatedIds"`
}

// RegisterAuditRoutes serves Phase 3 decision records - deliberated,
// bead-scoped design decisions written by app.WriteDecisionRecordNode (see
// internal/app/decision_record.go) - not dispatch.LogAutonomousDecision's
// auto-approved-permission records, which carry a different tag ("autonomous")
// and are deliberately excluded here; splitting that concern onto its own
// route is a later question with no route to answer it yet.
func RegisterAuditRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/audit/decisions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var response []DecisionResponse
		var truncated bool
		err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
			// Fetches one more row than the page limit purely to detect
			// whether there was one - ListDecisions has no count-only mode,
			// and a caller needs to know it hit the cap, not just receive a
			// suspiciously round 100 rows.
			decisions, err := nodes.ListDecisions(r.Context(), tx, nodes.DecisionFilter{
				Tags:  []string{app.PhaseThreeDecisionTag},
				Limit: decisionsPageLimit + 1,
			})
			if err != nil {
				return err
			}
			if len(decisions) > decisionsPageLimit {
				truncated = true
				decisions = decisions[:decisionsPageLimit]
			}

			response = make([]DecisionResponse, 0, len(decisions))
			for _, d := range decisions {
				in, err := edges.Incoming(r.Context(), tx, d.ID, edges.WithType(edges.EdgeTypeHasDecision))
				if err != nil {
					return err
				}

				var related []string
				for _, e := range in {
					related = append(related, e.Src)
				}

				response = append(response, DecisionResponse{
					ID:          d.ID,
					CreatedAt:   d.CreatedAt,
					Title:       d.Title,
					Body:        d.Body,
					Context:     d.Context,
					Outcome:     d.Outcome,
					ImpactOnUse: d.ImpactOnUse,
					Tags:        d.Tags,
					RelatedIDs:  related,
				})
			}
			return nil
		})

		if err != nil {
			slog.Error("KERNL DISPATCH FAILURE: list audit decisions", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("KERNL DISPATCH FAILURE: list audit decisions - %v", err))
			return
		}

		if response == nil {
			response = []DecisionResponse{}
		}
		// Set before the body write (json.Encoder.Encode below triggers the
		// implicit 200 that locks headers in) so a caller can tell "this is
		// all of them" from "this is the newest decisionsPageLimit of more"
		// without the endpoint's own response shape changing.
		w.Header().Set("X-Kernl-Truncated", strconv.FormatBool(truncated))
		json.NewEncoder(w).Encode(response)
	})
}
