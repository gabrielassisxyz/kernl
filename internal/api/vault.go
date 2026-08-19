package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hymkor/trash-go"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/notes"
	"github.com/gabrielassisxyz/kernl/internal/planning/linksuggest"
	"github.com/gabrielassisxyz/kernl/internal/vault/frontmatter"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

// Where POST /api/vault/append puts the block. An ordinary journal grows at the
// end; a newest-first log grows at the start, or - when it opens with a preamble
// the reader must see before the entries - directly under the `---` that divides
// the two.
//
// All three are named structures, not offsets. That is the line this route does
// not cross: an offset-addressed insert goes stale between the read that found
// the number and the write that uses it, and corrupts the note without a word.
// A named anchor is either present or the request fails.
const (
	appendPositionStart      = "start"
	appendPositionEnd        = "end"
	appendPositionAfterBreak = "after-break"
)

// appendPositions is the accepted set, in the order the error message lists
// them, so a rejected value and the CLI's help agree on the vocabulary.
var appendPositions = []string{appendPositionStart, appendPositionEnd, appendPositionAfterBreak}

// vaultFileWriteResponse is what POST /api/vault/file answers with. The
// suggestions are the links offered for this write; accepted and rejected are
// derived from the previous write's suggestions against the body's wikilinks,
// so the writer learns which of its earlier offers it took.
type vaultFileWriteResponse struct {
	Status      string                `json:"status"`
	Suggestions []nodes.LinkCandidate `json:"suggestions"`
	Accepted    []nodes.LinkCandidate `json:"accepted"`
	Rejected    []nodes.LinkCandidate `json:"rejected"`
}

func RegisterVaultRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("GET /api/vault/list", func(w http.ResponseWriter, r *http.Request) {
		root := a.Config.Vault.Root
		if root == "" {
			home, _ := os.UserHomeDir()
			root = filepath.Join(home, ".kernl", "vault")
		}

		var files []string
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() && strings.HasSuffix(info.Name(), ".md") {
				rel, _ := filepath.Rel(root, path)
				files = append(files, rel)
			}
			return nil
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"files": files})
	})

	// Vault files joined with their graph nodes (via the reconciler's path
	// cache). One request gives the UI path→type (list badges) and id→path
	// (wikilink navigation) without N+1 frontmatter parsing.
	mux.HandleFunc("GET /api/vault/notes", func(w http.ResponseWriter, r *http.Request) {
		type vaultNote struct {
			Path   string `json:"path"`
			ID     string `json:"id"`
			Type   string `json:"type"`
			Title  string `json:"title"`
			Author string `json:"author"`
		}
		out := []vaultNote{}
		err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
			rows, err := tx.Query(`
				SELECT np.path, np.uuid, COALESCE(n.type, ''), COALESCE(n.title, ''),
				       COALESCE(json_extract(n.attrs, '$.author'), '')
				FROM note_paths np
				LEFT JOIN nodes n ON n.id = np.uuid AND n.deleted_at IS NULL`)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var vn vaultNote
				var rawAuthor string
				if err := rows.Scan(&vn.Path, &vn.ID, &vn.Type, &vn.Title, &rawAuthor); err != nil {
					return err
				}
				vn.Author = reconcile.ResolveAuthor(rawAuthor).Name
				out = append(out, vn)
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

	mux.HandleFunc("GET /api/vault/file", func(w http.ResponseWriter, r *http.Request) {
		root := a.Config.Vault.Root
		if root == "" {
			home, _ := os.UserHomeDir()
			root = filepath.Join(home, ".kernl", "vault")
		}

		filePath := r.URL.Query().Get("path")
		if filePath == "" {
			http.Error(w, "missing path", http.StatusBadRequest)
			return
		}

		fullPath, err := resolveVaultFilePath(root, filePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		// Expose the file mtime so the editor uses the server's clock as the
		// conflict-detection baseline (If-Match on save), not the client's.
		if info, statErr := os.Stat(fullPath); statErr == nil {
			lm := info.ModTime().Format(time.RFC3339)
			w.Header().Set("Last-Modified", lm)
			w.Header().Set("ETag", lm)
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(data)
	})

	mux.HandleFunc("POST /api/vault/file", func(w http.ResponseWriter, r *http.Request) {
		root := a.Config.Vault.Root
		if root == "" {
			home, _ := os.UserHomeDir()
			root = filepath.Join(home, ".kernl", "vault")
		}

		filePath := r.URL.Query().Get("path")
		if filePath == "" {
			http.Error(w, "missing path", http.StatusBadRequest)
			return
		}

		fullPath, err := resolveVaultFilePath(root, filePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Inject the node id at creation. Without it the reconciler injects one
		// out-of-band right after the editor loads the file, bumping the mtime and
		// turning the editor's very next autosave into a false 409 conflict.
		// InjectID is a no-op when an id is already present.
		if strings.HasSuffix(fullPath, ".md") {
			if injected, injErr := frontmatter.InjectID(body, uuid.Must(uuid.NewV7()).String()); injErr == nil {
				body = injected
			}
		}

		err = os.WriteFile(fullPath, body, 0644)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Link suggestions: the channel says where the write came from, and the
		// gate offers links to the assistant (cli) and unidentified clients but
		// not to the user writing in the web UI. The state is written straight
		// to the node's attrs; the reconciler preserves it on its own pass.
		channel := r.Header.Get("X-Kernl-Client")
		noLinksReason := r.URL.Query().Get("noLinksReason")

		resp := vaultFileWriteResponse{Status: "saved"}
		if a.Graph != nil && strings.HasSuffix(fullPath, ".md") {
			_, noteBody := notes.SplitFrontmatter(string(body))
			state := reconcile.LinkState{Channel: channel, NoLinksReason: noLinksReason}
			if linksuggest.ShouldSuggest(channel) {
				suggestions, err := linksuggest.Suggest(r.Context(), a.Graph, noteBody, 8)
				if err != nil {
					slog.Warn("link suggestion failed; note saved without suggestions", "path", filePath, "err", err)
				} else {
					state.Suggestions = suggestions
					resp.Suggestions = suggestions
					if prev, err := previousLinkState(r.Context(), a.Graph, body); err != nil {
						slog.Warn("reading previous link state failed", "path", filePath, "err", err)
					} else {
						resp.Accepted, resp.Rejected = linksuggest.DeriveReceipts(prev, noteBody)
					}
				}
			}
			if err := reconcile.SetLinkState(r.Context(), a.Graph, root, fullPath, state); err != nil {
				slog.Warn("writing link state failed; note saved without it", "path", filePath, "err", err)
			}
		}

		if info, statErr := os.Stat(fullPath); statErr == nil {
			lm := info.ModTime().Format(time.RFC3339)
			w.Header().Set("Last-Modified", lm)
			w.Header().Set("ETag", lm)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	})

	// Add a block at one of the note's named positions without a
	// read-modify-write round trip. POST /api/vault/file is the wrong tool for a
	// log that only grows in one place: the caller has to ship the whole file
	// back, twice the transfer for a 160 KB note, and anything that writes
	// between the read and the write is silently overwritten.
	//
	// The graph is deliberately left to the vault watcher, exactly as the
	// full-file write leaves it: OnChange re-reads the file, updates the note
	// node and refreshes note_paths.content_hash from the bytes on disk. Writing
	// the hash here as well would only race that same write.
	mux.HandleFunc("POST /api/vault/append", func(w http.ResponseWriter, r *http.Request) {
		root := a.Config.Vault.Root
		if root == "" {
			home, _ := os.UserHomeDir()
			root = filepath.Join(home, ".kernl", "vault")
		}

		filePath := r.URL.Query().Get("path")
		if filePath == "" {
			http.Error(w, "missing path", http.StatusBadRequest)
			return
		}
		position := r.URL.Query().Get("position")
		if position == "" {
			position = appendPositionEnd
		}
		if !slices.Contains(appendPositions, position) {
			http.Error(w, "position must be one of "+strings.Join(appendPositions, ", "), http.StatusBadRequest)
			return
		}

		fullPath, err := resolveVaultFilePath(root, filePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		block, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if strings.TrimSpace(string(block)) == "" {
			http.Error(w, "the block to add is empty", http.StatusBadRequest)
			return
		}

		// Append never creates. A typo'd path that silently produced a new note
		// would scatter one log across several files, and the caller would find
		// out only when the entries went missing.
		current, err := os.ReadFile(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				http.Error(w, "no such note: "+filePath+" - append only adds to a note that exists; create it first with POST /api/vault/file", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var updated string
		switch position {
		case appendPositionStart:
			updated = notes.PrependBlock(string(current), string(block))
		case appendPositionAfterBreak:
			var found bool
			updated, found = notes.InsertAfterThematicBreak(string(current), string(block))
			if !found {
				// Falling back to either end would be the worst outcome: every
				// entry still reads correctly on its own, so the note would rot
				// into the wrong shape with nothing ever complaining.
				http.Error(w, "no thematic break in "+filePath+" - position="+appendPositionAfterBreak+" inserts under the first `---` line below the frontmatter, and this note has none", http.StatusConflict)
				return
			}
		default:
			updated = notes.AppendBlock(string(current), string(block))
		}
		if err := os.WriteFile(fullPath, []byte(updated), 0644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if info, statErr := os.Stat(fullPath); statErr == nil {
			lm := info.ModTime().Format(time.RFC3339)
			w.Header().Set("Last-Modified", lm)
			w.Header().Set("ETag", lm)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":   "appended",
			"path":     filePath,
			"position": position,
			"bytes":    len(block),
		})
	})

	// Deleting the file is enough: the vault watcher's OnDelete reconciles the
	// graph node away, the same path an external (file-explorer) delete takes.
	mux.HandleFunc("DELETE /api/vault/file", func(w http.ResponseWriter, r *http.Request) {
		root := a.Config.Vault.Root
		if root == "" {
			home, _ := os.UserHomeDir()
			root = filepath.Join(home, ".kernl", "vault")
		}

		filePath := r.URL.Query().Get("path")
		if filePath == "" {
			http.Error(w, "missing path", http.StatusBadRequest)
			return
		}

		// Destructive op: refuse anything that escapes the vault root.
		fullPath, err := resolveVaultFilePath(root, filePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// resolveVaultFilePath treats the root itself as contained; deleting it
		// would trash the whole vault, so only paths *below* it are targets.
		if absRoot, absErr := filepath.Abs(root); absErr != nil || fullPath == absRoot {
			http.Error(w, "path must stay within the vault", http.StatusBadRequest)
			return
		}

		if err := trash.Throw(fullPath); err != nil {
			if os.IsNotExist(err) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// previousLinkState reads the link suggestions a note already carries, so the
// endpoint can derive which of them the writer accepted on this write. It
// returns nil when the note has no id or does not exist yet.
func previousLinkState(ctx context.Context, g *graph.Graph, body []byte) ([]nodes.LinkCandidate, error) {
	fm, err := frontmatter.Parse(body)
	if err != nil || fm.ID == "" {
		return nil, nil
	}
	state, err := reconcile.LinkStateFor(ctx, g, fm.ID)
	if err != nil {
		return nil, err
	}
	return state.Suggestions, nil
}
