package api

import (
	"encoding/json"
	"net/http"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/tags"
	"github.com/gabrielassisxyz/kernl/internal/vault/companion"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

// noteIndexEntry is one row of the vault index: a note file, everything the
// list needs to sort, group, filter and label it, and nothing else. The body
// stays out - the index paints 700+ rows and the editor fetches the one file it
// opens.
type noteIndexEntry struct {
	ID       string   `json:"id"`
	Path     string   `json:"path"`
	Title    string   `json:"title"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
	// Author is resolved through reconcile.ResolveAuthor, so it reads "human",
	// "agent:da" or a human identifier - never the raw frontmatter string. A
	// client tells "not mine" by the "agent:" prefix rather than by knowing
	// which spellings mean the DA.
	Author    string `json:"author"`
	Pinned    bool   `json:"pinned"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// noteIndexResponse carries the pinned tags alongside the notes because both
// tabs of the vault panel are drawn from one payload: the Files tab lists the
// notes, and the Tags tab derives its whole tree from the same rows' tags. Two
// requests would mean the tree and the rows could disagree about what the vault
// holds for as long as one of them is in flight.
type noteIndexResponse struct {
	Notes      []noteIndexEntry `json:"notes"`
	PinnedTags []string         `json:"pinnedTags"`
}

func registerNotesIndexRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/notes", func(w http.ResponseWriter, r *http.Request) {
		out := noteIndexResponse{Notes: []noteIndexEntry{}, PinnedTags: []string{}}

		err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
			ordered, byID, err := readNoteIndexRows(tx)
			if err != nil {
				return err
			}
			if err := attachNoteTags(tx, byID); err != nil {
				return err
			}
			for _, entry := range ordered {
				out.Notes = append(out.Notes, *entry)
			}
			out.PinnedTags, err = tags.PinnedNames(r.Context(), tx)
			return err
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read the vault index: "+err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})

	// Pin state is the only thing about a note this route changes; the note's
	// content is a file and belongs to the vault routes.
	mux.HandleFunc("PATCH /api/notes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "missing note id")
			return
		}
		var body struct {
			Pinned *bool `json:"pinned"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
			return
		}
		if body.Pinned == nil {
			writeError(w, http.StatusBadRequest, "nothing to update: provide pinned")
			return
		}

		err := a.Graph.DoWrite(r.Context(), func(tx *graph.WriteTx) error {
			return reconcile.SetPinned(r.Context(), tx, id, *body.Pinned)
		})
		if err == graph.ErrNotFound {
			writeError(w, http.StatusNotFound, "no vault note with that id")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to pin the note: "+err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// The tag name travels in the body, not the path: tag names contain
	// slashes ("sys/audit" and four siblings are in the vault today), and a
	// path parameter would either reject them or need escaping at every call
	// site.
	mux.HandleFunc("PATCH /api/notes/tags", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name   string `json:"name"`
			Pinned *bool  `json:"pinned"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body: "+err.Error())
			return
		}
		if body.Name == "" {
			writeError(w, http.StatusBadRequest, "missing tag name")
			return
		}
		if body.Pinned == nil {
			writeError(w, http.StatusBadRequest, "nothing to update: provide pinned")
			return
		}

		err := a.Graph.DoWrite(r.Context(), func(tx *graph.WriteTx) error {
			return tags.SetPinned(r.Context(), tx, body.Name, *body.Pinned, nodes.Author{Name: "api"})
		})
		if err == graph.ErrNotFound {
			writeError(w, http.StatusNotFound, "no note carries that tag")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to pin the tag: "+err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// readNoteIndexRows returns the index rows newest first, plus the same entries
// keyed by node id so tags can be attached in a second pass without losing that
// order.
//
// The category comes from the entity the note describes, not from the note's
// own type: every file in the vault is a "note" node, and a project's or task's
// companion is a separate note joined to it by a describes edge. Reading
// nodes.type here would label all of them "note".
//
// It is a correlated subquery rather than a join because an entity can carry
// more than one describes edge - the backfill sweep documents exactly that
// case, an entity whose first companion died - and a join would then emit the
// same note twice.
func readNoteIndexRows(tx *graph.ReadTx) ([]*noteIndexEntry, map[string]*noteIndexEntry, error) {
	rows, err := tx.Query(`
		SELECT np.uuid, np.path, np.pinned, n.title, n.created_at, n.updated_at,
		       COALESCE(json_extract(n.attrs, '$.author'), '') AS author,
		       COALESCE((
		           SELECT e_target.type FROM edges e
		           JOIN nodes e_target ON e_target.id = e.dst AND e_target.deleted_at IS NULL
		           WHERE e.src = np.uuid AND e.label = ?
		           LIMIT 1
		       ), 'note') AS category
		FROM note_paths np
		JOIN nodes n ON n.id = np.uuid AND n.deleted_at IS NULL
		ORDER BY n.updated_at DESC`, companion.EdgeLabel)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	ordered := []*noteIndexEntry{}
	byID := map[string]*noteIndexEntry{}
	for rows.Next() {
		entry := &noteIndexEntry{Tags: []string{}}
		var pinned int
		var rawAuthor string
		if err := rows.Scan(&entry.ID, &entry.Path, &pinned, &entry.Title,
			&entry.CreatedAt, &entry.UpdatedAt, &rawAuthor, &entry.Category); err != nil {
			return nil, nil, err
		}
		entry.Pinned = pinned != 0
		entry.Author = reconcile.ResolveAuthor(rawAuthor).Name
		ordered = append(ordered, entry)
		byID[entry.ID] = entry
	}
	return ordered, byID, rows.Err()
}

// attachNoteTags fills in each entry's tags in one pass over node_tags, so an
// index of 700 notes costs one query rather than 700.
func attachNoteTags(tx *graph.ReadTx, byID map[string]*noteIndexEntry) error {
	rows, err := tx.Query(`
		SELECT nt.node_id, tg.name
		FROM node_tags nt
		JOIN tags tg ON tg.id = nt.tag_id
		ORDER BY tg.name ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var nodeID, name string
		if err := rows.Scan(&nodeID, &name); err != nil {
			return err
		}
		if entry, ok := byID[nodeID]; ok {
			entry.Tags = append(entry.Tags, name)
		}
	}
	return rows.Err()
}
