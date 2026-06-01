package dispatch

import (
	"encoding/json"
	"net/http"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

type EpicRunAPIRequest struct {
	Interactive bool   `json:"interactive"`
	ShapeID     string `json:"shape_id"`
}

// HandleEpicRunAPI handles the U3 API confirmation prompting requirement.
func HandleEpicRunAPI(be backend.BackendPort, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		epicID := r.PathValue("id")
		if epicID == "" {
			http.Error(w, "missing epic id", http.StatusBadRequest)
			return
		}

		var req EpicRunAPIRequest
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}

		repoPath := cfg.Registry.Repos[0].Path

		// Is the epic autonomous?
		isAuto := false
		epicBead, err := be.Get(epicID, repoPath)
		if err == nil {
			isAuto = IsEpicAutonomous(epicBead)
		}

		if isAuto && req.Interactive {
			// If shape_id is provided, it's the second confirming request
			if req.ShapeID == "" {
				res, err := InferWorkflow(r.Context(), cfg.LLM, epicBead)
				if err != nil {
					http.Error(w, "inference failed: "+err.Error(), http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusPreconditionRequired) // 428
				json.NewEncoder(w).Encode(map[string]string{
					"shape_id":  res.ShapeID,
					"rationale": res.Rationale,
				})
				return
			}
		}

		// Proceed with shape
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"shape":  req.ShapeID,
		})
	}
}
