package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hymkor/trash-go"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/vault/frontmatter"
)

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
			Path  string `json:"path"`
			ID    string `json:"id"`
			Type  string `json:"type"`
			Title string `json:"title"`
		}
		out := []vaultNote{}
		err := a.Graph.DoRead(r.Context(), func(tx *graph.ReadTx) error {
			rows, err := tx.Query(`
				SELECT np.path, np.uuid, COALESCE(n.type, ''), COALESCE(n.title, '')
				FROM note_paths np
				LEFT JOIN nodes n ON n.id = np.uuid AND n.deleted_at IS NULL`)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var vn vaultNote
				if err := rows.Scan(&vn.Path, &vn.ID, &vn.Type, &vn.Title); err != nil {
					return err
				}
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

		if info, statErr := os.Stat(fullPath); statErr == nil {
			lm := info.ModTime().Format(time.RFC3339)
			w.Header().Set("Last-Modified", lm)
			w.Header().Set("ETag", lm)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"saved"}`))
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
