package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// listBeadsCache memoizes the result of /api/beads for a short window.
// Without it, the dashboard's 2s polling fallback (+ any other client) all
// hit `bd list` concurrently and pile up - observed during the kernl-npp
// run on 2026-05-17 with bd timing out at 5s, then the next request
// hitting the same path while the first was still in flight.
type listBeadsCache struct {
	mu   sync.Mutex
	data []backend.Bead
	at   time.Time
	ttl  time.Duration
}

func (c *listBeadsCache) get(load func() ([]backend.Bead, error)) ([]backend.Bead, error) {
	c.mu.Lock()
	if c.data != nil && time.Since(c.at) < c.ttl {
		out := c.data
		c.mu.Unlock()
		return out, nil
	}
	c.mu.Unlock()

	beads, err := load()
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.data = beads
	c.at = time.Now()
	c.mu.Unlock()
	return beads, nil
}

func RegisterBeadRoutes(mux *http.ServeMux, a *app.App) {
	repoPath, repoErr := resolveBeadsRepoPath(a)
	if repoErr != nil {
		registerFailingBeadRoutes(mux, repoErr)
		return
	}

	listCache := &listBeadsCache{ttl: 2 * time.Second}

	mux.HandleFunc("GET /api/beads", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		beads, err := listCache.get(func() ([]backend.Bead, error) {
			return a.Backend.List(nil, repoPath)
		})
		if err != nil {
			slog.Error("KERNL DISPATCH FAILURE: list beads", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("KERNL DISPATCH FAILURE: list beads - %v - Fix: check backend connectivity and repo path %q", err, repoPath))
			return
		}
		if beads == nil {
			beads = []backend.Bead{}
		}
		json.NewEncoder(w).Encode(beads)
	})

	mux.HandleFunc("GET /api/beads/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := r.PathValue("id")
		bead, err := a.Backend.Get(id, repoPath)
		if err != nil {
			slog.Error("KERNL DISPATCH FAILURE: get bead", "error", err, "beadId", id)
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("KERNL DISPATCH FAILURE: get bead %s - %v - Fix: check backend connectivity and repo path %q", id, err, repoPath))
			return
		}
		if bead == nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("KERNL DISPATCH FAILURE: bead %s not found - bead does not exist in repo %q - Fix: verify the bead id", id, repoPath))
			return
		}
		json.NewEncoder(w).Encode(bead)
	})

	mux.HandleFunc("POST /api/beads", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var input backend.CreateBeadInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("KERNL DISPATCH FAILURE: invalid create bead body - %v - Fix: send valid JSON matching CreateBeadInput", err))
			return
		}
		bead, err := a.Backend.Create(input, repoPath)
		if err != nil {
			slog.Error("KERNL DISPATCH FAILURE: create bead", "error", err)
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("KERNL DISPATCH FAILURE: create bead - %v - Fix: check backend connectivity and repo path %q", err, repoPath))
			return
		}
		if bead == nil {
			writeError(w, http.StatusInternalServerError, "KERNL DISPATCH FAILURE: created bead is nil - backend returned no data - Fix: check backend implementation")
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(bead)
	})

	mux.HandleFunc("PATCH /api/beads/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := r.PathValue("id")
		var input backend.UpdateBeadInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("KERNL DISPATCH FAILURE: invalid update bead body - %v - Fix: send valid JSON matching UpdateBeadInput", err))
			return
		}
		if err := a.Backend.Update(id, input, repoPath); err != nil {
			slog.Error("KERNL DISPATCH FAILURE: update bead", "error", err, "beadId", id)
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("KERNL DISPATCH FAILURE: update bead %s - %v - Fix: check backend connectivity and repo path %q", id, err, repoPath))
			return
		}
		bead, _ := a.Backend.Get(id, repoPath)
		if bead == nil {
			json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
			return
		}
		json.NewEncoder(w).Encode(bead)
	})

	mux.HandleFunc("POST /api/beads/{id}/close", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := r.PathValue("id")
		var body struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		result, err := a.Backend.Close(id, body.Reason, repoPath)
		if err != nil {
			slog.Error("KERNL DISPATCH FAILURE: close bead", "error", err, "beadId", id)
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("KERNL DISPATCH FAILURE: close bead %s - %v - Fix: check backend connectivity and repo path %q", id, err, repoPath))
			return
		}
		if result == nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("KERNL DISPATCH FAILURE: bead %s not found for close - bead does not exist in repo %q - Fix: verify the bead id", id, repoPath))
			return
		}
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("POST /api/beads/{id}/mark-terminal", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := r.PathValue("id")
		var body struct {
			TargetState string `json:"targetState"`
			Reason      string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := a.Backend.MarkTerminal(id, body.TargetState, body.Reason, repoPath); err != nil {
			slog.Error("KERNL DISPATCH FAILURE: mark terminal bead", "error", err, "beadId", id)
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("KERNL DISPATCH FAILURE: mark terminal bead %s - %v - Fix: check backend connectivity and repo path %q", id, err, repoPath))
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "marked_terminal"})
	})

	mux.HandleFunc("POST /api/beads/{id}/rollback", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := r.PathValue("id")
		var body struct {
			TargetState string `json:"targetState"`
			Reason      string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := a.Backend.Rewind(id, body.TargetState, body.Reason, repoPath); err != nil {
			slog.Error("KERNL DISPATCH FAILURE: rollback bead", "error", err, "beadId", id)
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("KERNL DISPATCH FAILURE: rollback bead %s - %v - Fix: check backend connectivity and repo path %q", id, err, repoPath))
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "rolled_back"})
	})

	mux.HandleFunc("POST /api/beads/{id}/refine-scope", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := r.PathValue("id")
		var body struct {
			Description string `json:"description"`
			Notes       string `json:"notes"`
			Acceptance  string `json:"acceptance"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		input := backend.UpdateBeadInput{
			Description: body.Description,
			Notes:       body.Notes,
			Acceptance:  body.Acceptance,
		}
		if err := a.Backend.Update(id, input, repoPath); err != nil {
			slog.Error("KERNL DISPATCH FAILURE: refine scope bead", "error", err, "beadId", id)
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("KERNL DISPATCH FAILURE: refine scope bead %s - %v - Fix: check backend connectivity and repo path %q", id, err, repoPath))
			return
		}
		bead, _ := a.Backend.Get(id, repoPath)
		if bead == nil {
			json.NewEncoder(w).Encode(map[string]string{"status": "refined"})
			return
		}
		json.NewEncoder(w).Encode(bead)
	})

	// revert-decision is the decision model's §6 middle row: one decision
	// was wrong, the work stands, so the bead reopens carrying that decision
	// as a constraint. See app.RevertDecisionAndReopenBead for why this is
	// composed as one operation rather than left as two separate calls a
	// client could invoke out of order or only halfway.
	mux.HandleFunc("POST /api/beads/{id}/revert-decision", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := r.PathValue("id")
		var body struct {
			DecisionID  string `json:"decisionId"`
			TargetState string `json:"targetState"`
			Reason      string `json:"reason"`
		}
		// Unlike the sibling routes above, a malformed body here must not
		// decode silently to a zero-value struct: encoding/json leaves an
		// unmatched field at its zero value with no error, so a truncated
		// body or a typo'd key (target_state instead of targetState) would
		// otherwise reach RevertDecisionAndReopenBead as an ordinary missing
		// --state, come back as a 500, and invite a retry that can never
		// succeed - the request itself is malformed, not the backend.
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("KERNL DISPATCH FAILURE: invalid revert-decision body - %v - Fix: send valid JSON matching {targetState, reason, decisionId?}", err))
			return
		}
		result, err := app.RevertDecisionAndReopenBead(r.Context(), a.Graph, a.Backend, repoPath, app.RevertDecisionInput{
			BeadID:      id,
			DecisionID:  body.DecisionID,
			TargetState: body.TargetState,
			Reason:      body.Reason,
		})
		if err != nil {
			slog.Error("KERNL DISPATCH FAILURE: revert decision for bead", "error", err, "beadId", id)
			writeError(w, revertDecisionErrorStatus(err), err.Error())
			return
		}
		json.NewEncoder(w).Encode(map[string]string{
			"status":      "reverted",
			"decisionId":  result.DecisionID,
			"targetState": result.TargetState,
		})
	})
}

// resolveBeadsRepoPath names the repository the bead routes must query.
//
// a.RepoPath is the repository app.NewAppForRepo built this App against -
// the run these routes are actually serving, which is what `epic run
// --repo <target>` needs its own GUI to show. The registry.repos[0]
// fallback exists only for an App assembled directly (a bare struct
// literal, as hand-built test Apps do) rather than through NewApp /
// NewAppForRepo, both of which always set RepoPath; it matches `kernl
// serve`'s existing single-repo behavior exactly.
func resolveBeadsRepoPath(a *app.App) (string, error) {
	if a.RepoPath != "" {
		return a.RepoPath, nil
	}
	if a.Config != nil && len(a.Config.Registry.Repos) > 0 {
		return a.Config.Registry.Repos[0].Path, nil
	}
	return "", fmt.Errorf("KERNL DISPATCH FAILURE: no repository resolved for the bead routes - App.RepoPath is unset and registry.repos is empty - Fix: add a repo to registry.repos in kernl.yaml, or build the App via app.NewAppForRepo")
}

// registerFailingBeadRoutes wires every bead route to a handler that reports
// the unresolved repository, rather than letting each handler discover it
// independently (and rather than reaching into a.Config.Registry.Repos[0]
// unchecked, which panics on an empty slice). Fail Loud applies at the route
// boundary: a client hitting any bead endpoint gets the same, actionable
// error instead of a 404 for a route that silently never got its real
// handler installed.
func registerFailingBeadRoutes(mux *http.ServeMux, err error) {
	fail := func(w http.ResponseWriter, r *http.Request) {
		slog.Error(err.Error())
		writeError(w, http.StatusInternalServerError, err.Error())
	}
	for _, route := range []string{
		"GET /api/beads",
		"GET /api/beads/{id}",
		"POST /api/beads",
		"PATCH /api/beads/{id}",
		"POST /api/beads/{id}/close",
		"POST /api/beads/{id}/mark-terminal",
		"POST /api/beads/{id}/rollback",
		"POST /api/beads/{id}/refine-scope",
		"POST /api/beads/{id}/revert-decision",
	} {
		mux.HandleFunc(route, fail)
	}
}

// revertDecisionErrorStatus maps app.RevertDecisionAndReopenBead's error
// into an HTTP status: a bad request (missing field, invalid target state,
// a decision id that does not resolve) is 400, not 500, because retrying it
// unchanged can never succeed; the named bead not existing in this
// repository's tracker is 404; anything else - a graph write or tracker
// update genuinely failing - is the 500 this route always returned before.
func revertDecisionErrorStatus(err error) int {
	var notFound app.RevertDecisionNotFoundError
	if errors.As(err, &notFound) {
		return http.StatusNotFound
	}
	var inputErr app.RevertDecisionInputError
	if errors.As(err, &inputErr) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
