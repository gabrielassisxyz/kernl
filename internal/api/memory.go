package api

import (
	"encoding/json"
	"net/http"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/memory"
	"github.com/gabrielassisxyz/kernl/internal/planning"
)

func RegisterMemoryRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/memory/topics", func(w http.ResponseWriter, r *http.Request) {
		var topics []string

		err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
			rows, err := tx.Query(`SELECT DISTINCT json_extract(attrs, '$.subject') FROM nodes WHERE type = 'memory_claim' AND json_extract(attrs, '$.subject') IS NOT NULL`)
			if err != nil {
				return err
			}
			defer rows.Close()

			for rows.Next() {
				var topic string
				if err := rows.Scan(&topic); err != nil {
					continue
				}
				if topic != "" {
					topics = append(topics, topic)
				}
			}
			return rows.Err()
		})

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if topics == nil {
			topics = []string{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"topics": topics})
	})

	mux.HandleFunc("GET /api/memory/claims", func(w http.ResponseWriter, r *http.Request) {
		topic := r.URL.Query().Get("topic")
		if topic == "" {
			http.Error(w, "missing topic parameter", http.StatusBadRequest)
			return
		}

		var claims []*nodes.MemoryClaim
		err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
			var err error
			claims, err = memory.SynthesizeTopic(r.Context(), tx, topic)
			return err
		})

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if claims == nil {
			claims = make([]*nodes.MemoryClaim, 0)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"claims": claims})
	})

	// Telos: the human-authored half of Memory — notes tagged `telos` that are
	// ALWAYS injected into the DA's context (vs. claims, which are retrieved by
	// relevance). The endpoint returns the notes plus the live injection
	// footprint so the surface can show the user exactly what the DA always sees.
	mux.HandleFunc("GET /api/memory/telos", func(w http.ResponseWriter, r *http.Request) {
		type telosNote struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			Body      string `json:"body"`
			Path      string `json:"path"` // vault file path, "" when not yet on disk
			UpdatedAt string `json:"updatedAt"`
		}
		out := []telosNote{}

		err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
			rows, err := tx.Query(`
				SELECT n.id, n.title,
				       COALESCE(json_extract(n.attrs, '$.body'), ''),
				       COALESCE(np.path, ''),
				       COALESCE(n.updated_at, '')
				FROM nodes n
				JOIN node_tags nt ON nt.node_id = n.id
				JOIN tags tg ON tg.id = nt.tag_id
				LEFT JOIN note_paths np ON np.uuid = n.id
				WHERE n.type = 'note' AND n.deleted_at IS NULL AND tg.name = ?
				ORDER BY n.updated_at DESC`, planning.TelosTag)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var tn telosNote
				if err := rows.Scan(&tn.ID, &tn.Title, &tn.Body, &tn.Path, &tn.UpdatedAt); err != nil {
					return err
				}
				out = append(out, tn)
			}
			return rows.Err()
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// The exact block the chat engine would inject, so the footprint shown
		// to the user is the real one (including cap/truncation).
		tc, err := planning.LoadTelosContext(r.Context(), a.Graph)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"notes": out,
			"injection": map[string]any{
				"bytes":     tc.Bytes,
				"capBytes":  tc.CapBytes,
				"truncated": tc.Truncated,
			},
		})
	})

	mux.HandleFunc("POST /api/memory/claims/{id}/refute", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "missing claim id", http.StatusBadRequest)
			return
		}

		var req struct {
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		var refutationID string
		err := a.Graph.DoWrite(r.Context(), func(tx *graph.WriteTx) error {
			var err error
			refutationID, err = memory.RefuteMemoryClaim(r.Context(), tx, id, req.Reason)
			return err
		})

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status": "refuted",
			"id":     refutationID,
		})
	})
}
