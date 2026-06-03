package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/notes"
)

func RegisterNotesRoutes(mux *http.ServeMux, a *app.App) {
	// Tag hierarchy sourced from the graph (one request, vs the editor's old
	// N+1 fetch-every-file-and-parse-frontmatter approach).
	mux.HandleFunc("GET /api/notes/tags", func(w http.ResponseWriter, r *http.Request) {
		tree, err := notes.TagTree(r.Context(), a.Graph)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tree)
	})

	// DA diff-suggest: given a note path and an instruction, ask the LLM for a
	// revised version and return line-aligned hunks the editor surfaces for
	// accept/reject. The user always commits — the LLM never writes directly.
	// Gated on cfg.LLM.IsSet() (same as chat); 503 when no provider configured.
	mux.HandleFunc("POST /api/notes/suggest", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path        string `json:"path"`
			Instruction string `json:"instruction"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Path == "" || req.Instruction == "" {
			http.Error(w, "path and instruction are required", http.StatusBadRequest)
			return
		}
		if !a.Config.LLM.IsSet() {
			http.Error(w, "no LLM provider configured — add an llm section to kernl.yaml", http.StatusServiceUnavailable)
			return
		}

		root := a.Config.Vault.Root
		if root == "" {
			home, _ := os.UserHomeDir()
			root = filepath.Join(home, ".kernl", "vault")
		}
		current, err := os.ReadFile(filepath.Join(root, req.Path))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		llm, err := chat.NewProviderFromConfig(configToLLMProviderConfig(a.Config.LLM))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Send only the body to the LLM and diff only the body, so frontmatter
		// (the note's id) can never be touched by a suggestion.
		_, body := notes.SplitFrontmatter(string(current))
		prompt := fmt.Sprintf("You are editing the body of a markdown note. Apply the instruction and output ONLY the full revised body — no frontmatter, no commentary, no code fences.\n\nInstruction: %s\n\nNote body:\n%s", req.Instruction, body)
		resp, err := llm.Chat(r.Context(), []chat.Message{{Role: "user", Content: prompt}}, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		hunks := notes.DiffBody(string(current), resp.Content)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"hunks": hunks})
	})

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
