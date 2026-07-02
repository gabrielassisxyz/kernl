package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/api"
	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/notes"
)

// Applying accepted hunks writes the file and preserves the frontmatter/id —
// the whole point of routing DA edits through a diff (UAT N4).
func TestApplyHunksPreservesFrontmatter(t *testing.T) {
	root := t.TempDir()
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}}
	mux := http.NewServeMux()
	api.RegisterNotesRoutes(mux, a)

	original := "---\nid: keep-me-123\ntitle: Atlas\n---\n\nfirst line\n"
	if err := os.WriteFile(filepath.Join(root, "atlas.md"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	// Compute the hunks the DA would propose for a new body.
	hunks := notes.DiffBody(original, "first line\n\nan appended paragraph.\n")
	body, _ := json.Marshal(map[string]any{"path": "atlas.md", "hunks": hunks})

	req := httptest.NewRequest("POST", "/api/notes/apply-hunks", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	saved, err := os.ReadFile(filepath.Join(root, "atlas.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(saved)
	if !contains(s, "id: keep-me-123") {
		t.Errorf("note id must be preserved:\n%s", s)
	}
	if !contains(s, "an appended paragraph.") {
		t.Errorf("accepted edit not applied:\n%s", s)
	}
}

// Rejecting (sending no hunks) must be a 400 — there is nothing to apply, and
// the file must be untouched.
func TestApplyHunksRejectsEmpty(t *testing.T) {
	root := t.TempDir()
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}}
	mux := http.NewServeMux()
	api.RegisterNotesRoutes(mux, a)

	original := "---\nid: x\n---\nbody\n"
	_ = os.WriteFile(filepath.Join(root, "n.md"), []byte(original), 0o644)

	body, _ := json.Marshal(map[string]any{"path": "n.md", "hunks": []any{}})
	req := httptest.NewRequest("POST", "/api/notes/apply-hunks", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty hunks, got %d", w.Code)
	}
	saved, _ := os.ReadFile(filepath.Join(root, "n.md"))
	if string(saved) != original {
		t.Error("file must be untouched when nothing is applied")
	}
}

func contains(s, sub string) bool { return bytes.Contains([]byte(s), []byte(sub)) }
