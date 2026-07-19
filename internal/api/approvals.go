package api

import (
	"encoding/json"
	"net/http"
)

// The approval subsystem is scaffolding that was never wired end-to-end: the
// types exist (approvals.ApprovalRegistry, terminal.PendingApprovalRecord), the
// execution helper exists (terminal.PerformApprovalAction), and the agent
// adapters can be configured with an MCP approval bridge — but nothing in the
// runtime populates any registry. NewApprovalRegistry, RecordPendingApproval,
// SetApprovalResponder and the *WithBridge arg builders have no runtime caller,
// so no live agent permission prompt ever becomes a record these routes could
// serve.
//
// These handlers used to return an empty list and an empty success object. That
// was actively harmful: POST /api/approvals/{id}/actions reported success for an
// id that never existed (the CLI's `approval resolve apr-999` printed "Resolved"
// at exit 0), and GET /api/approvals reported "nothing pending" when the truth
// is "this cannot report anything". A caller — human or agent — could not tell a
// working-but-idle gate from an unbuilt one.
//
// Until the capture flow is built (tracked as a future project in BACKLOG.md,
// "Human judgment gates"), these routes answer 501 Not Implemented with a plain
// message. 501 makes the CLI exit non-zero with the message verbatim and lets a
// triage caller show "approvals: unavailable" instead of a false "0 pending".
// When the feature lands, replace these with real handlers wired to app.App's
// terminal manager / approval registry.

const approvalsNotImplemented = "approvals are not implemented yet: no judgment-gate capture flow is wired, so there is nothing to list and no action can be applied — tracked as a future project in BACKLOG.md"

func RegisterApprovalRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/approvals", approvalNotImplementedHandler)
	mux.HandleFunc("POST /api/approvals/{id}/actions", approvalNotImplementedHandler)
	mux.HandleFunc("POST /api/terminal/{sessionId}/approvals/{approvalId}", approvalNotImplementedHandler)
}

func approvalNotImplementedHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":       approvalsNotImplemented,
		"implemented": false,
	})
}
