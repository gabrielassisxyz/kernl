package api

import (
	"encoding/json"
	"net/http"
)

func RegisterApprovalRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/approvals", listApprovalsHandler)
	mux.HandleFunc("POST /api/approvals/{id}/actions", applyApprovalActionHandler)
	mux.HandleFunc("POST /api/terminal/{sessionId}/approvals/{approvalId}", terminalApprovalHandler)
}

func listApprovalsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]struct{}{})
}

func applyApprovalActionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct{}{})
}

func terminalApprovalHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct{}{})
}
