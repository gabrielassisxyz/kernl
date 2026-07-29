package api

import (
	"net/http"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/vault/companion"
)

// companionBackfillDTO is what both the dry run and the write return: the same
// list, so a caller can diff what it was told against what happened instead of
// trusting a count.
type companionBackfillDTO struct {
	DryRun   bool                    `json:"dryRun"`
	Entities []companionBackfillItem `json:"entities"`
}

type companionBackfillItem struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
	Path  string `json:"path,omitempty"` // written path; empty on a dry run
}

// registerCompanionBackfillRoutes exposes the companion sweep.
//
// It lives behind the server rather than in the CLI because the server owns the
// graph: a CLI writing straight to the database would be a second writer against
// the file `kernl serve` already holds open, which is the exact reason doctor
// only ever opens it read-only.
func registerCompanionBackfillRoutes(mux *http.ServeMux, a *app.App) {
	// GET lists what is missing and writes nothing. It is the dry run, and it is a
	// separate method rather than a flag so that reading can never be one typo
	// away from writing.
	mux.HandleFunc("GET /api/vault/companions/missing", func(w http.ResponseWriter, r *http.Request) {
		var orphans []companion.Orphan
		err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
			var err error
			orphans, err = companion.Missing(r.Context(), tx)
			return err
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "scan for missing companions: "+err.Error())
			return
		}
		out := companionBackfillDTO{DryRun: true, Entities: make([]companionBackfillItem, 0, len(orphans))}
		for _, o := range orphans {
			out.Entities = append(out.Entities, companionBackfillItem{ID: o.ID, Type: o.Type, Title: o.Title})
		}
		writeJSON(w, out)
	})

	mux.HandleFunc("POST /api/vault/companions/backfill", func(w http.ResponseWriter, r *http.Request) {
		root := a.Config.Vault.Root
		if root == "" {
			writeError(w, http.StatusPreconditionFailed, "no vault configured: set vault.root before backfilling companion notes")
			return
		}

		var orphans []companion.Orphan
		var files []companion.File
		// One transaction for the whole sweep, matching how a single entity gets
		// its companion: either every node, edge and note_paths row lands or none
		// does, so a failure halfway cannot leave a partial repair to reason about.
		err := a.Graph.DoWrite(r.Context(), func(tx *graph.WriteTx) error {
			var err error
			orphans, err = companion.Missing(r.Context(), tx)
			if err != nil {
				return err
			}
			files, err = companion.Backfill(r.Context(), tx, root, orphans)
			return err
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "backfill companions: "+err.Error())
			return
		}

		out := companionBackfillDTO{Entities: make([]companionBackfillItem, 0, len(orphans))}
		for i, o := range orphans {
			item := companionBackfillItem{ID: o.ID, Type: o.Type, Title: o.Title}
			// The graph committed, so the file has to follow or the note_paths hash
			// describes bytes that are not on disk.
			if err := companion.WriteFile(root, files[i]); err != nil {
				writeError(w, http.StatusInternalServerError, "write companion file: "+err.Error())
				return
			}
			item.Path = files[i].RelPath()
			out.Entities = append(out.Entities, item)
		}
		writeJSON(w, out)
	})
}
