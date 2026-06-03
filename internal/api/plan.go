package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/planning"
)

// RegisterPlanRoutes exposes substrate-aware planning context: the notes from
// the vault relevant to a planning seed, ready to inject into the DA planner.
func RegisterPlanRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/plan/context", func(w http.ResponseWriter, r *http.Request) {
		seed := r.URL.Query().Get("seed")
		limit := 8
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 {
				limit = n
			}
		}

		notes, err := planning.BuildContext(r.Context(), a.Graph, seed, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"seed": seed, "notes": notes})
	})
}
