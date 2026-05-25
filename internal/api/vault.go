package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
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

		fullPath := filepath.Join(root, filePath)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Write(data)
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

		fullPath := filepath.Join(root, filePath)
		os.MkdirAll(filepath.Dir(fullPath), 0755)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		err = os.WriteFile(fullPath, body, 0644)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"saved"}`))
	})
}
