package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/approvals"
)

// The approval routes serve the gates a dispatched agent is blocked on.
//
// They read the same directory the bridge writes, which is why they work in
// every process that can reach it - `kernl serve`, the GUI an epic run stands
// up, and a CLI talking to either. Nothing is held in memory: an answer given
// through one process is seen by an agent waiting on another.
//
// These handlers used to answer 501 because no capture flow existed. Before
// that they were worse than absent - POST reported success for an id that never
// existed - so the honest failure is kept for the one case that survives: an
// App built without a store.

// approvalActions maps the two client vocabularies onto the store's canonical
// answers. The gate words (approve/reject) and the session words
// (accept/always_approve/decline) name the same three outcomes; a route accepts
// only the vocabulary it advertises, so a word aimed at the other route is
// refused here rather than acted on differently than the caller meant.
var (
	gateActions = map[string]approvals.Action{
		"approve": approvals.ActionAllow,
		"reject":  approvals.ActionDeny,
	}
	sessionActions = map[string]approvals.Action{
		"accept":         approvals.ActionAllow,
		"always_approve": approvals.ActionAllowAlways,
		"decline":        approvals.ActionDeny,
	}
)

func RegisterApprovalRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/approvals", listApprovalsHandler(a))
	mux.HandleFunc("POST /api/approvals/{id}/actions", resolveApprovalHandler(a, gateActions))
	mux.HandleFunc("POST /api/terminal/{sessionId}/approvals/{approvalId}", resolveSessionApprovalHandler(a))
}

func listApprovalsHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		store, ok := approvalStore(w, a)
		if !ok {
			return
		}
		filter := approvals.ApprovalFilter{
			RepoPath: r.URL.Query().Get("repoPath"),
			Status:   r.URL.Query().Get("status"),
		}
		if r.URL.Query().Get("activeOnly") == "true" {
			filter.ActiveOnly = true
		}

		list, err := store.List(filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// A bare array, never an object: the CLI and the GUI both decode this
		// as a list, and an empty store is [] rather than null.
		writeJSON(w, list)
	}
}

func resolveApprovalHandler(a *app.App, vocabulary map[string]approvals.Action) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		applyApprovalAction(w, r, a, r.PathValue("id"), vocabulary, "")
	}
}

// resolveSessionApprovalHandler additionally checks that the approval belongs
// to the session in the path. Without that, the session segment would be
// decoration and a mistyped session id would answer someone else's gate.
func resolveSessionApprovalHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		applyApprovalAction(w, r, a, r.PathValue("approvalId"), sessionActions, r.PathValue("sessionId"))
	}
}

func applyApprovalAction(w http.ResponseWriter, r *http.Request, a *app.App, id string, vocabulary map[string]approvals.Action, wantSession string) {
	store, ok := approvalStore(w, a)
	if !ok {
		return
	}
	var body struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "KERNL DISPATCH FAILURE: approval action body is not JSON: "+err.Error())
		return
	}
	action, known := vocabulary[body.Action]
	if !known {
		writeError(w, http.StatusBadRequest, "KERNL DISPATCH FAILURE: unknown approval action "+body.Action+" for this route - valid: "+strings.Join(sortedWords(vocabulary), ", "))
		return
	}

	if wantSession != "" {
		req, err := store.Get(id)
		if err != nil {
			writeApprovalError(w, err)
			return
		}
		if req.SessionID != wantSession {
			writeError(w, http.StatusNotFound, "KERNL DISPATCH FAILURE: approval "+id+" does not belong to session "+wantSession)
			return
		}
	}

	resolved, err := store.Decide(id, action, body.Reason)
	if err != nil {
		writeApprovalError(w, err)
		return
	}
	writeJSON(w, resolved)
}

// writeApprovalError keeps "you named something that does not exist" (404)
// distinct from "it exists but can no longer be answered" (409). A caller that
// cannot tell those apart retries the one that will never succeed.
func writeApprovalError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, approvals.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case strings.Contains(err.Error(), "already") || strings.Contains(err.Error(), "expired"):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}

// approvalStore reports the one honest failure left: an App with no store at
// all. It answers 501 rather than an empty list, because "this cannot report
// anything" and "nothing is pending" must not look the same.
func approvalStore(w http.ResponseWriter, a *app.App) (*approvals.Store, bool) {
	if a == nil || a.Approvals == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":       "approvals are unavailable: this server was built without an approval store, so no gate can be listed or answered",
			"implemented": false,
		})
		return nil, false
	}
	return a.Approvals, true
}

func sortedWords(vocabulary map[string]approvals.Action) []string {
	words := make([]string, 0, len(vocabulary))
	for w := range vocabulary {
		words = append(words, w)
	}
	// Small fixed sets; a stable order keeps the error message identical run
	// to run, which matters because it is asserted in tests and read by agents.
	for i := 1; i < len(words); i++ {
		for j := i; j > 0 && words[j] < words[j-1]; j-- {
			words[j], words[j-1] = words[j-1], words[j]
		}
	}
	return words
}
