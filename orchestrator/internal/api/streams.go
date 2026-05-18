package api

import (
	"encoding/json"
	"errors"
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

	mux.HandleFunc("POST /api/sessions/{id}/nudge", func(w http.ResponseWriter, r *http.Request) {
		serveNudge(w, r, a)
	})

	mux.HandleFunc("GET /api/sessions/{id}/nudge-prompts", func(w http.ResponseWriter, r *http.Request) {
		serveNudgePrompts(w, r, a)
	})
}

// serveNudgePrompts returns the pre-substituted prompt text for each preset
// so the web UI can pre-fill its editable textarea without duplicating the
// templates client-side.
func serveNudgePrompts(w http.ResponseWriter, r *http.Request, a *app.App) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeJSONError(w, http.StatusBadRequest, "sessionID required")
		return
	}
	if a.NudgeRegistry == nil {
		writeJSONError(w, http.StatusInternalServerError, "nudge registry not configured")
		return
	}
	rec, ok := a.NudgeRegistry.Get(sessionID)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "unknown session")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"beadId":             rec.BeadID,
		"opencodeSessionId":  rec.OpencodeSessionID,
		"running":            rec.Running,
		"generic":            app.DefaultNudgePrompt(app.NudgePresetGeneric, rec.BeadID, rec.RepoPath),
		"advance_status":     app.DefaultNudgePrompt(app.NudgePresetAdvanceStatus, rec.BeadID, rec.RepoPath),
	})
}

type nudgeRequest struct {
	Preset string `json:"preset"`
	Prompt string `json:"prompt"`
}

func serveNudge(w http.ResponseWriter, r *http.Request, a *app.App) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeJSONError(w, http.StatusBadRequest, "sessionID required")
		return
	}

	var body nudgeRequest
	// Body is optional — empty body means "use default generic preset".
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	preset := app.NudgePreset(body.Preset)
	if preset == "" {
		preset = app.NudgePresetGeneric
	}

	err := a.Nudge(sessionID, app.NudgeOptions{Preset: preset, Prompt: body.Prompt})
	switch {
	case err == nil:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "dispatched", "sessionId": sessionID})
	case errors.Is(err, app.ErrNudgeUnknownSession):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, app.ErrNudgeRunning):
		writeJSONError(w, http.StatusConflict, err.Error())
	case errors.Is(err, app.ErrNudgeNoOpencodeSession):
		writeJSONError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeJSONError(w, http.StatusInternalServerError, err.Error())
	}
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
