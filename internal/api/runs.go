package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// runDTO is the camelCase shape the web client consumes. The domain node has
// no JSON tags of its own (see the note on daIdentityResponse in
// da_identity.go), so it is never encoded to the wire directly.
type runDTO struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	Title        string    `json:"title"`
	WorkflowName string    `json:"workflowName"`
	Status       string    `json:"status"`
	RunData      string    `json:"runData"`
	Tags         []string  `json:"tags"`
}

func newRunDTO(wr *nodes.WorkflowRun) runDTO {
	return runDTO{
		ID:           wr.ID,
		CreatedAt:    wr.CreatedAt,
		UpdatedAt:    wr.UpdatedAt,
		Title:        wr.Title,
		WorkflowName: wr.WorkflowName,
		Status:       wr.Status,
		RunData:      wr.RunData,
		Tags:         tagList(wr.Tags),
	}
}

// RegisterRunRoutes registers the read-only workflow run REST endpoints.
//
// Only reads are wired here. Writing a run (CreateWorkflowRun/UpdateWorkflowRun)
// belongs to whatever drives an epic through its stages - cmd/kernl/epic.go and
// internal/app - and is a separate concern from exposing what already landed
// in the graph.
func RegisterRunRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/runs", func(w http.ResponseWriter, r *http.Request) {
		listRunsHandler(w, r, a)
	})
	mux.HandleFunc("GET /api/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		getRunHandler(w, r, a)
	})
}

func listRunsHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	ctx := r.Context()
	filter := nodes.WorkflowRunFilter{
		WorkflowName: r.URL.Query().Get("workflowName"),
		Status:       r.URL.Query().Get("status"),
	}
	if tags := r.URL.Query().Get("tags"); tags != "" {
		filter.Tags = strings.Split(tags, ",")
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		// Neighbouring handlers (nodes_search.go, nodes_related.go, plan.go)
		// silently ignore a malformed limit and fall back to a default. That is
		// fine for a typeahead or planning widget, but this endpoint lists a
		// table that grows with every orchestrator run: ListWorkflowRuns only
		// applies LIMIT when it is > 0, so a silently-ignored "-5" or "0" would
		// not shrink the result, it would return every run in the graph with no
		// signal that the bound was dropped. Reject anything that is not a
		// positive integer instead of letting a bad value turn into "give me
		// everything".
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			writeError(w, http.StatusBadRequest, "invalid limit: must be a positive integer, got "+raw)
			return
		}
		filter.Limit = limit
	}

	out := []runDTO{}
	err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		runs, err := nodes.ListWorkflowRuns(ctx, tx, filter)
		if err != nil {
			return err
		}
		for _, wr := range runs {
			out = append(out, newRunDTO(wr))
		}
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list runs: "+err.Error())
		return
	}

	writeJSON(w, out)
}

func getRunHandler(w http.ResponseWriter, r *http.Request, a *app.App) {
	ctx := r.Context()
	id := r.PathValue("id")

	var wr *nodes.WorkflowRun
	err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		wr, err = nodes.GetWorkflowRun(ctx, tx, id)
		return err
	})
	if errors.Is(err, graph.ErrNotFound) {
		writeError(w, http.StatusNotFound, "run not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get run: "+err.Error())
		return
	}

	writeJSON(w, newRunDTO(wr))
}
