package api

import (
	"encoding/json"
	"net/http"
)

func RegisterAppRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/app-update", appUpdateHandler)
}

func appUpdateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "up_to_date"})
}