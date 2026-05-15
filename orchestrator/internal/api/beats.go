package api

import (
	"encoding/json"
	"net/http"
)

func RegisterBeatRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/beats", listBeatsHandler)
	mux.HandleFunc("GET /api/beats/{id}", getBeatHandler)
	mux.HandleFunc("POST /api/beats", createBeatHandler)
	mux.HandleFunc("PATCH /api/beats/{id}", updateBeatHandler)
	mux.HandleFunc("POST /api/beats/{id}/close", closeBeatHandler)
	mux.HandleFunc("POST /api/beats/{id}/mark-terminal", markTerminalHandler)
	mux.HandleFunc("POST /api/beats/{id}/rollback", rollbackBeatHandler)
	mux.HandleFunc("POST /api/beats/{id}/refine-scope", refineScopeHandler)
}

func listBeatsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]struct{}{})
}

func getBeatHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct{}{})
}

func createBeatHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(struct{}{})
}

func updateBeatHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct{}{})
}

func closeBeatHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct{}{})
}

func markTerminalHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct{}{})
}

func rollbackBeatHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct{}{})
}

func refineScopeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct{}{})
}