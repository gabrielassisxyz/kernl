package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
)

type graphEdge struct {
	ID    string `json:"id"`
	Src   string `json:"src"`
	Dst   string `json:"dst"`
	Label string `json:"label"`
}

// resolvedNodeEdge is the per-node edge view. Unlike graphEdge it describes the
// neighbour at the far end of the hop, not the raw endpoints, and it marks the
// direction so a caller walking the graph one hop at a time knows which way
// each connection points. Via and depth are constant for now (always the asked
// node, always one hop) but are part of the contract so a future --depth keeps
// the output honest instead of quietly changing what a row means.
type resolvedNodeEdge struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	Path      string `json:"path"`
	Label     string `json:"label"`
	Direction string `json:"direction"`
	Via       string `json:"via"`
	Depth     int    `json:"depth"`
}

// RegisterNodeEdgeRoutes exposes the stored edges attached to a single node:
// GET /api/nodes/{id}/edges?label=&type=&resolve=.
//
// Unlike /api/nodes/{id}/related (a computed relevance heuristic that walks up
// to two hops and throws the label away), this returns exactly the persisted
// edges where the node is src or dst - one hop, every label kept, direction
// marked. Nodes soft-deleted on either end are excluded.
func RegisterNodeEdgeRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/nodes/{id}/edges", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		q := r.URL.Query()
		label := q.Get("label")
		typ := q.Get("type")
		resolve := false
		if raw := q.Get("resolve"); raw != "" {
			if v, err := strconv.ParseBool(raw); err == nil {
				resolve = v
			}
		}

		var out any
		var missing bool
		err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
			// A node that does not exist and a node with no edges both produce an
			// empty list, and this route exists to be WALKED: a caller following a
			// wrong id would get silence where it needs an error. Distinguishing
			// them is the same rule the vault convention states for a broken link
			// versus a placeholder one, and the same rule the retrieval receipt
			// states for an error versus zero results.
			var exists bool
			if err := tx.QueryRow(
				`SELECT 1 FROM nodes WHERE id = ? AND deleted_at IS NULL`, id,
			).Scan(&exists); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					missing = true
					return nil
				}
				return err
			}
			var err error
			if resolve {
				out, err = resolvedEdgesForNode(tx, id, label, typ)
			} else {
				out, err = edgesForNode(tx, id, label, typ)
			}
			return err
		})
		if missing {
			http.Error(w, "no such node: "+id, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})
}

// edgesForNode returns the raw {id,src,dst,label} edges where the node is src
// or dst, the same shape the global /api/edges route returns but filtered to
// one node's neighbourhood.
func edgesForNode(tx *graph.ReadTx, id, label, typ string) ([]graphEdge, error) {
	query := `
		SELECT e.id, e.src, e.dst, e.label
		FROM edges e
		JOIN nodes s ON s.id = e.src AND s.deleted_at IS NULL
		JOIN nodes d ON d.id = e.dst AND d.deleted_at IS NULL
		WHERE (e.src = ? OR e.dst = ?)`
	args := []any{id, id}
	if label != "" {
		query += ` AND e.label = ?`
		args = append(args, label)
	}
	if typ != "" {
		query += ` AND CASE WHEN e.src = ? THEN d.type ELSE s.type END = ?`
		args = append(args, id, typ)
	}
	query += ` ORDER BY e.created_at ASC, e.id ASC`

	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []graphEdge{}
	for rows.Next() {
		var e graphEdge
		if err := rows.Scan(&e.ID, &e.Src, &e.Dst, &e.Label); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// resolvedEdgesForNode returns the enriched view: every field describes the
// neighbour, direction is "out" when the asked node is the edge source and
// "in" when it is the destination, via is the asked node, and depth is 1.
func resolvedEdgesForNode(tx *graph.ReadTx, id, label, typ string) ([]resolvedNodeEdge, error) {
	query := `
		SELECT
			nb.id,
			nb.title,
			nb.type,
			COALESCE(np.path, ''),
			e.label,
			CASE WHEN e.src = ? THEN 'out' ELSE 'in' END
		FROM edges e
		JOIN nodes s ON s.id = e.src AND s.deleted_at IS NULL
		JOIN nodes d ON d.id = e.dst AND d.deleted_at IS NULL
		JOIN nodes nb ON nb.id = CASE WHEN e.src = ? THEN e.dst ELSE e.src END
		LEFT JOIN note_paths np ON np.uuid = nb.id
		WHERE (e.src = ? OR e.dst = ?)`
	args := []any{id, id, id, id}
	if label != "" {
		query += ` AND e.label = ?`
		args = append(args, label)
	}
	if typ != "" {
		query += ` AND nb.type = ?`
		args = append(args, typ)
	}
	query += ` ORDER BY e.created_at ASC, e.id ASC`

	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []resolvedNodeEdge{}
	for rows.Next() {
		var e resolvedNodeEdge
		if err := rows.Scan(&e.ID, &e.Title, &e.Type, &e.Path, &e.Label, &e.Direction); err != nil {
			return nil, err
		}
		e.Via = id
		e.Depth = 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// RegisterEdgeRoutes exposes the raw stored edges so the graph view can render
// the actual connections between nodes. Unlike /api/nodes/{id}/related (a
// computed relevance heuristic), this returns the edges table as persisted  -
// the source of truth for validating that connections are being made correctly.
func RegisterEdgeRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/edges", func(w http.ResponseWriter, r *http.Request) {
		var out []graphEdge
		err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
			rows, err := tx.Query(`
				SELECT e.id, e.src, e.dst, e.label
				FROM edges e
				JOIN nodes s ON s.id = e.src AND s.deleted_at IS NULL
				JOIN nodes d ON d.id = e.dst AND d.deleted_at IS NULL`)
			if err != nil {
				return err
			}
			defer rows.Close()
			out = []graphEdge{}
			for rows.Next() {
				var e graphEdge
				if err := rows.Scan(&e.ID, &e.Src, &e.Dst, &e.Label); err != nil {
					return err
				}
				out = append(out, e)
			}
			return rows.Err()
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})
}
