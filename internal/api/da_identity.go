package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// daIdentityState is the deserialized attrs payload for a da_identity node. This
// is the storage shape, not the wire shape: it must keep matching what is already
// written in the graph.
type daIdentityState struct {
	SystemPrompt string `json:"system_prompt"`
	DisplayName  string `json:"display_name"`
}

// daIdentityResponse is the wire shape. The handler used to encode the domain
// node straight to the client, which has no JSON tags, so the API answered in Go
// field names while every other endpoint answers camelCase.
type daIdentityResponse struct {
	ID           string    `json:"id"`
	DisplayName  string    `json:"displayName"`
	SystemPrompt string    `json:"systemPrompt"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

func newDAIdentityResponse(di *nodes.DAIdentity) daIdentityResponse {
	return daIdentityResponse{
		ID:           di.ID,
		DisplayName:  di.DisplayName,
		SystemPrompt: di.SystemPrompt,
		CreatedAt:    di.CreatedAt,
		UpdatedAt:    di.UpdatedAt,
	}
}

// scanDAIdentity reads a DAIdentity from a nodes row using the provided querier.
func scanDAIdentityFromRow(q interface{ QueryRow(string, ...any) *sql.Row }) (*nodes.DAIdentity, error) {
	var id sql.NullString
	var title, attrsRaw sql.NullString
	var createdAt, updatedAt sql.NullString

	err := q.QueryRow(
		`SELECT id, title, attrs, created_at, updated_at FROM nodes WHERE type = ? ORDER BY created_at ASC LIMIT 1`,
		nodes.TypeDAIdentity,
	).Scan(&id, &title, &attrsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, graph.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var attrs daIdentityState
	if attrsRaw.Valid && attrsRaw.String != "" {
		_ = json.Unmarshal([]byte(attrsRaw.String), &attrs)
	}

	di := &nodes.DAIdentity{
		ID:           id.String,
		SystemPrompt: attrs.SystemPrompt,
		DisplayName:  attrs.DisplayName,
	}
	if t, err := time.Parse(time.RFC3339, createdAt.String); err == nil {
		di.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt.String); err == nil {
		di.UpdatedAt = t
	}
	if di.DisplayName == "" && title.Valid {
		di.DisplayName = title.String
	}

	return di, nil
}

// RegisterDARoutes registers DA identity REST endpoints on mux.
func RegisterDARoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/da/identity", daIdentityGetHandler(a))
	mux.HandleFunc("PUT /api/da/identity", daIdentityPutHandler(a))
}

func daIdentityGetHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var di *nodes.DAIdentity

		// Attempt read-only path first.
		err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
			var innerErr error
			di, innerErr = scanDAIdentityFromRow(tx)
			return innerErr
		})

		if err != nil && !errors.Is(err, graph.ErrNotFound) {
			slog.Error("DA identity read", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to read DA identity")
			return
		}

		// If not found, create atomically with defaults.
		if di == nil {
			err = a.Graph.DoWrite(r.Context(), func(tx *graph.WriteTx) error {
				var innerErr error
				// Re-check under write lock.
				di, innerErr = scanDAIdentityFromRow(tx)
				if innerErr != nil && !errors.Is(innerErr, graph.ErrNotFound) {
					return innerErr
				}
				if di != nil {
					return nil // already exists
				}

				newDI := &nodes.DAIdentity{
					SystemPrompt: "You are a helpful assistant integrated with the Kernl knowledge graph. You help the user reason about their notes, projects, and ideas. You speak in the user's language.",
					DisplayName:  "Kernl Assistant",
				}

				id, createErr := nodes.CreateDAIdentity(r.Context(), tx, newDI, nodes.Author{Name: "kernl"})
				if createErr != nil {
					return createErr
				}
				newDI.ID = id
				di = newDI
				return nil
			})
			if err != nil {
				slog.Error("DA identity create", "error", err)
				writeError(w, http.StatusInternalServerError, "failed to create DA identity")
				return
			}
		}

		_ = json.NewEncoder(w).Encode(newDAIdentityResponse(di))
	}
}

func daIdentityPutHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var body struct {
			SystemPrompt *string `json:"systemPrompt"`
			DisplayName  *string `json:"displayName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		err := a.Graph.DoWrite(r.Context(), func(tx *graph.WriteTx) error {
			di, err := scanDAIdentityFromRow(tx)
			if err != nil {
				return err
			}

			if body.SystemPrompt != nil && *body.SystemPrompt != "" {
				di.SystemPrompt = *body.SystemPrompt
			}
			if body.DisplayName != nil && *body.DisplayName != "" {
				di.DisplayName = *body.DisplayName
			}

			return nodes.SaveDAIdentity(r.Context(), tx, di, nodes.Author{Name: "kernl"})
		})

		if errors.Is(err, graph.ErrNotFound) {
			writeError(w, http.StatusNotFound, "DA identity not found")
			return
		}
		if err != nil {
			slog.Error("DA identity update", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to update DA identity")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
