package api

import (
	"encoding/json"
	"net/http"
)

func RegisterBeadRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/beads", listBeadsHandler)
	mux.HandleFunc("GET /api/beads/{id}", getBeadHandler)
	mux.HandleFunc("POST /api/beads", createBeadHandler)
	mux.HandleFunc("PATCH /api/beads/{id}", updateBeadHandler)
	mux.HandleFunc("POST /api/beads/{id}/close", closeBeadHandler)
	mux.HandleFunc("POST /api/beads/{id}/mark-terminal", markTerminalHandler)
	mux.HandleFunc("POST /api/beads/{id}/rollback", rollbackBeadHandler)
	mux.HandleFunc("POST /api/beads/{id}/refine-scope", refineScopeHandler)
}

func listBeadsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]struct{}{})
}

func getBeadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct{}{})
}

func createBeadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(struct{}{})
}

func updateBeadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct{}{})
}

func closeBeadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct{}{})
}

func markTerminalHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct{}{})
}

func rollbackBeadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct{}{})
}

func refineScopeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct{}{})
}