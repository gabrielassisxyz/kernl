package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/search"
)

const (
	nodeSearchDefaultLimit = 10
	nodeSearchMaxLimit     = 50
)

type nodeSearchResult struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// RegisterNodeSearchRoutes exposes prefix search over node titles for the
// editor's wikilink autocomplete: GET /api/nodes/search?q=&type=&limit=.
func RegisterNodeSearchRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/nodes/search", nodeSearchHandler(a))
}

func nodeSearchHandler(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		q := strings.TrimSpace(r.URL.Query().Get("q"))
		out := []nodeSearchResult{}
		// A blank query is a valid "nothing typed yet" state, not an error.
		if q == "" {
			writeJSON(w, out)
			return
		}

		limit := nodeSearchDefaultLimit
		if raw := r.URL.Query().Get("limit"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		if limit > nodeSearchMaxLimit {
			limit = nodeSearchMaxLimit
		}

		opts := []search.Option{search.WithPrefix()}
		if typ := strings.TrimSpace(r.URL.Query().Get("type")); typ != "" {
			opts = append(opts, search.WithTypes(typ))
		}

		err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
			hits, err := search.Search(ctx, tx, q, opts...)
			if err != nil {
				return err
			}
			if len(hits) > limit {
				hits = hits[:limit]
			}
			if len(hits) == 0 {
				return nil
			}

			// Resolve node types in one query (the search Hit lacks type).
			placeholders := make([]string, len(hits))
			args := make([]any, len(hits))
			for i, h := range hits {
				placeholders[i] = "?"
				args[i] = h.NodeID
			}
			types := make(map[string]string, len(hits))
			rows, err := tx.Query(
				`SELECT id, type FROM nodes WHERE deleted_at IS NULL AND id IN (`+strings.Join(placeholders, ", ")+`)`,
				args...,
			)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id, typ string
				if err := rows.Scan(&id, &typ); err != nil {
					return err
				}
				types[id] = typ
			}
			if err := rows.Err(); err != nil {
				return err
			}

			// Preserve search rank order; drop hits whose node vanished/tombstoned.
			for _, h := range hits {
				typ, ok := types[h.NodeID]
				if !ok {
					continue
				}
				out = append(out, nodeSearchResult{ID: h.NodeID, Title: h.Title, Type: typ})
			}
			return nil
		})
		if err != nil {
			slog.Error("node search", "error", err)
			writeError(w, http.StatusInternalServerError, "search failed")
			return
		}

		writeJSON(w, out)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
