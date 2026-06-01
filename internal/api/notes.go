package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/notes"
)

func RegisterNotesRoutes(mux *http.ServeMux, a *app.App) {
	mux.HandleFunc("POST /api/notes/save", func(w http.ResponseWriter, r *http.Request) {
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

		clientLastModifiedStr := r.Header.Get("If-Match")
		
		fullPath := filepath.Join(root, filePath)
		os.MkdirAll(filepath.Dir(fullPath), 0755)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		err = notes.CheckConflict(fullPath, clientLastModifiedStr)
		if err == notes.ErrConflict {
			http.Error(w, "conflict", http.StatusConflict) // 409
			return
		} else if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		err = os.WriteFile(fullPath, body, 0644)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		info, _ := os.Stat(fullPath)
		newLastModified := info.ModTime().Format(time.RFC3339)

		w.Header().Set("ETag", newLastModified)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"saved", "last_modified": "` + newLastModified + `"}`))
	})
}
