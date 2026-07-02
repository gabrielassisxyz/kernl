package api_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/api"
	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/vault/frontmatter"
)

// Creating a note through the API must inject the node id up front. Otherwise
// the reconciler injects it out-of-band right after the editor loads the file,
// bumping the mtime and turning the editor's next autosave into a false 409.
func TestVaultCreateInjectsID(t *testing.T) {
	root := t.TempDir()
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}}

	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)

	body := "---\ntitle: Fresh Note\ntags: [uat]\n---\n\n# Fresh Note\n"
	req := httptest.NewRequest("POST", "/api/vault/file?path=fresh-note.md", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("ETag") == "" {
		t.Error("expected ETag header on create so the editor can baseline conflicts")
	}

	saved, err := os.ReadFile(filepath.Join(root, "fresh-note.md"))
	if err != nil {
		t.Fatal(err)
	}
	fm, err := frontmatter.Parse(saved)
	if err != nil {
		t.Fatalf("saved note has invalid frontmatter: %v", err)
	}
	if fm.ID == "" {
		t.Error("expected an injected id in the created note's frontmatter")
	}
	if fm.Title != "Fresh Note" {
		t.Errorf("title must be preserved, got %q", fm.Title)
	}
	if !strings.Contains(string(saved), "# Fresh Note") {
		t.Error("body must be preserved")
	}
}

// A create whose body already carries an id must keep it untouched.
func TestVaultCreateKeepsExistingID(t *testing.T) {
	root := t.TempDir()
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}}

	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)

	body := "---\nid: 019f14ca-58ad-7203-8cf8-487f765f0001\ntitle: Existing\n---\nbody\n"
	req := httptest.NewRequest("POST", "/api/vault/file?path=existing.md", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	saved, err := os.ReadFile(filepath.Join(root, "existing.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(saved) != body {
		t.Errorf("body with existing id must be written verbatim, got:\n%s", saved)
	}
}

// DELETE must remove the file (graph cleanup is the watcher's job) and refuse
// paths that escape the vault root.
func TestVaultDeleteFile(t *testing.T) {
	root := t.TempDir()
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}}

	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)

	target := filepath.Join(root, "doomed.md")
	if err := os.WriteFile(target, []byte("---\nid: x\n---\nbye\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("DELETE", "/api/vault/file?path=doomed.md", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("expected file to be removed")
	}

	// Missing file → 404.
	req = httptest.NewRequest("DELETE", "/api/vault/file?path=doomed.md", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing file, got %d", w.Code)
	}

	// Traversal → 400, file outside the vault untouched.
	outside := filepath.Join(filepath.Dir(root), "outside.md")
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest("DELETE", "/api/vault/file?path=../outside.md", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for traversal, got %d", w.Code)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Error("file outside the vault must not be touched")
	}
}
