package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/notes"
)

func RegisterNotesRoutes(mux *http.ServeMux, a *app.App) {
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
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

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
		_, _ = w.Write([]byte(`{"status":"saved", "last_modified": "` + newLastModified + `"}`))
	})

	// Apply hunks the user accepted from a DA chat suggestion. The DA never
	// writes: it proposes hunks (via the suggest_note_edit chat tool), the user
	// picks which to accept, and this endpoint applies exactly those to the file.
	// Hunks carry full-document offsets past the frontmatter, so the note's
	// frontmatter and id are preserved.
	mux.HandleFunc("POST /api/notes/apply-hunks", func(w http.ResponseWriter, r *http.Request) {
		root := a.Config.Vault.Root
		if root == "" {
			home, _ := os.UserHomeDir()
			root = filepath.Join(home, ".kernl", "vault")
		}

		var req struct {
			Path  string              `json:"path"`
			Hunks []notes.SuggestHunk `json:"hunks"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Path == "" {
			http.Error(w, "path is required", http.StatusBadRequest)
			return
		}
		if len(req.Hunks) == 0 {
			http.Error(w, "no hunks to apply", http.StatusBadRequest)
			return
		}

		fullPath, err := resolveVaultFilePath(root, req.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		current, err := os.ReadFile(fullPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		updated := notes.ApplySuggestHunks(string(current), req.Hunks)
		if updated == string(current) {
			// Every hunk was out of range / a no-op — surface it rather than
			// silently pretending success.
			http.Error(w, "hunks did not apply to the current note (it may have changed)", http.StatusConflict)
			return
		}
		if err := os.WriteFile(fullPath, []byte(updated), 0644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		info, _ := os.Stat(fullPath)
		newLastModified := info.ModTime().Format(time.RFC3339)
		w.Header().Set("ETag", newLastModified)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "applied", "last_modified": newLastModified})
	})
}

func resolveVaultFilePath(root, relPath string) (string, error) {
	if strings.TrimSpace(relPath) == "" {
		return "", fmt.Errorf("path is required")
	}
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	cleanRel := filepath.Clean(filepath.FromSlash(relPath))
	if filepath.IsAbs(cleanRel) || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path must stay within the vault")
	}
	fullPath, err := filepath.Abs(filepath.Join(cleanRoot, cleanRel))
	if err != nil {
		return "", err
	}
	rootRel, err := filepath.Rel(cleanRoot, fullPath)
	if err != nil || rootRel == ".." || strings.HasPrefix(rootRel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path must stay within the vault")
	}
	return fullPath, nil
}
