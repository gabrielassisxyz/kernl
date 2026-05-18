package api

import (
	"net/http"

	"github.com/gabrielassisxyz/kernl/internal/app"
)

func RegisterStreamRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/sessions/{id}/events", func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("id")
		if a.SCM == nil {
			http.Error(w, "session connection manager not configured", http.StatusInternalServerError)
			return
		}
		a.SCM.ServeSSE(w, r, sessionID)
	})
}
