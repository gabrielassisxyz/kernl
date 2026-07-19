package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/api"
	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

// traversalVault lays out a vault root with a sibling file that lives OUTSIDE
// it, so every traversal assertion can prove the attacker's target was neither
// read, written nor deleted.
func traversalVault(t *testing.T) (root, outside, outsideBody string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "vault")
	if err := os.MkdirAll(filepath.Join(root, "sub", "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside = filepath.Join(base, "secret.txt")
	outsideBody = "TOP SECRET private key\n"
	if err := os.WriteFile(outside, []byte(outsideBody), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, outside, outsideBody
}

// traversalPaths are the client-supplied paths every vault-facing route must
// refuse. The %2f entries matter because net/http percent-decodes the query
// before the handler ever sees it — the guard must run on the decoded value.
func traversalPaths(outside string) map[string]string {
	return map[string]string{
		"relative escape":  "../secret.txt",
		"deep escape":      "../../../etc/passwd",
		"absolute path":    outside,
		"absolute etc":     "/etc/passwd",
		"encoded escape":   "..%2f..%2fetc%2fpasswd",
		"escape then back": "sub/../../secret.txt",
	}
}

// JSON-body routes never see a percent-decoded path, so "..%2f" is a literal
// (nonsense) filename inside the vault there, not an escape. Only the
// query-string routes need that case.
func jsonTraversalPaths(outside string) map[string]string {
	paths := traversalPaths(outside)
	delete(paths, "encoded escape")
	return paths
}

// A rejection must say the path is out of bounds without echoing the resolved
// absolute path or the vault root — that would hand an attacker the layout of
// the filesystem they are trying to map.
func assertNoPathLeak(t *testing.T, body, root string) {
	t.Helper()
	if strings.Contains(body, root) {
		t.Errorf("rejection body leaks the vault root %q: %s", root, body)
	}
	if strings.Contains(body, "/etc/passwd") {
		t.Errorf("rejection body echoes the resolved path: %s", body)
	}
}

func TestVaultGetFileRejectsTraversal(t *testing.T) {
	root, outside, outsideBody := traversalVault(t)
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}}
	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)

	for name, p := range traversalPaths(outside) {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/vault/file?path="+p, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), outsideBody) {
				t.Fatal("response leaked the contents of a file outside the vault")
			}
			assertNoPathLeak(t, w.Body.String(), root)
		})
	}
}

// A guard that also breaks legitimate nested notes is a regression, not a fix.
func TestVaultGetFileAllowsNestedPath(t *testing.T) {
	root, _, _ := traversalVault(t)
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}}
	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)

	want := "---\nid: nested\n---\nnested body\n"
	if err := os.WriteFile(filepath.Join(root, "sub", "dir", "note.md"), []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/vault/file?path=sub/dir/note.md", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for a nested vault path, got %d: %s", w.Code, w.Body.String())
	}
	if w.Body.String() != want {
		t.Errorf("unexpected body: %q", w.Body.String())
	}
	if w.Header().Get("ETag") == "" || w.Header().Get("Last-Modified") == "" {
		t.Error("ETag/Last-Modified must still be set so the editor can baseline conflicts")
	}
}

func TestVaultPostFileRejectsTraversal(t *testing.T) {
	root, outside, outsideBody := traversalVault(t)
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}}
	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)

	for name, p := range traversalPaths(outside) {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/vault/file?path="+p, bytes.NewBufferString("pwned\n"))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
			assertNoPathLeak(t, w.Body.String(), root)
			got, err := os.ReadFile(outside)
			if err != nil {
				t.Fatalf("file outside the vault must still exist: %v", err)
			}
			if string(got) != outsideBody {
				t.Fatalf("file outside the vault was overwritten: %q", string(got))
			}
		})
	}
}

func TestVaultPostFileAllowsNestedPath(t *testing.T) {
	root, _, _ := traversalVault(t)
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}}
	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)

	// A directory that does not exist yet: the handler's MkdirAll must survive.
	req := httptest.NewRequest("POST", "/api/vault/file?path=sub/fresh/deep.md", bytes.NewBufferString("---\ntitle: Deep\n---\nbody\n"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for a nested vault path, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "sub", "fresh", "deep.md")); err != nil {
		t.Fatalf("nested note was not written: %v", err)
	}
}

func TestVaultDeleteFileRejectsTraversal(t *testing.T) {
	root, outside, outsideBody := traversalVault(t)
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}}
	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)

	paths := traversalPaths(outside)
	// The vault root itself is not a deletable target either.
	paths["vault root"] = "."

	for name, p := range paths {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("DELETE", "/api/vault/file?path="+p, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
			assertNoPathLeak(t, w.Body.String(), root)
			got, err := os.ReadFile(outside)
			if err != nil {
				t.Fatalf("file outside the vault must still exist: %v", err)
			}
			if string(got) != outsideBody {
				t.Fatalf("file outside the vault was modified: %q", string(got))
			}
			if _, err := os.Stat(root); err != nil {
				t.Fatalf("the vault root must survive: %v", err)
			}
		})
	}
}

func TestNotesSaveRejectsTraversal(t *testing.T) {
	root, outside, outsideBody := traversalVault(t)
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}}
	mux := http.NewServeMux()
	api.RegisterNotesRoutes(mux, a)

	for name, p := range traversalPaths(outside) {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/notes/save?path="+p, bytes.NewBufferString("pwned\n"))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
			assertNoPathLeak(t, w.Body.String(), root)
			got, err := os.ReadFile(outside)
			if err != nil {
				t.Fatalf("file outside the vault must still exist: %v", err)
			}
			if string(got) != outsideBody {
				t.Fatalf("file outside the vault was overwritten: %q", string(got))
			}
		})
	}
}

// Saving a nested note must keep working, including the If-Match conflict
// detection the editor's autosave depends on.
func TestNotesSaveAllowsNestedPathAndKeepsConflictCheck(t *testing.T) {
	root, _, _ := traversalVault(t)
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}}
	mux := http.NewServeMux()
	api.RegisterNotesRoutes(mux, a)

	req := httptest.NewRequest("POST", "/api/notes/save?path=sub/dir/note.md", bytes.NewBufferString("first\n"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for a nested vault path, got %d: %s", w.Code, w.Body.String())
	}
	saved, err := os.ReadFile(filepath.Join(root, "sub", "dir", "note.md"))
	if err != nil || string(saved) != "first\n" {
		t.Fatalf("nested note not saved: %v %q", err, string(saved))
	}
	if w.Header().Get("ETag") == "" {
		t.Error("ETag must still be returned for the next autosave baseline")
	}

	// A stale If-Match must still produce a 409, not be swallowed by the guard.
	req = httptest.NewRequest("POST", "/api/notes/save?path=sub/dir/note.md", bytes.NewBufferString("second\n"))
	req.Header.Set("If-Match", "2000-01-01T00:00:00Z")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a stale If-Match, got %d: %s", w.Code, w.Body.String())
	}
}

func TestNotesSuggestRejectsTraversal(t *testing.T) {
	root, outside, _ := traversalVault(t)
	// The LLM must be configured, otherwise the handler short-circuits on 503
	// and the containment guard is never exercised. No request is ever made:
	// the guard rejects before any provider call.
	a := &app.App{Config: &config.Config{
		Vault: config.VaultConfig{Root: root},
		LLM:   config.LLMConfig{Provider: "openai", APIKey: "unused", Model: "unused", Endpoint: "http://127.0.0.1:1"},
	}}
	mux := http.NewServeMux()
	api.RegisterNotesRoutes(mux, a)

	for name, p := range jsonTraversalPaths(outside) {
		t.Run(name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"path": p, "instruction": "exfiltrate"})
			req := httptest.NewRequest("POST", "/api/notes/suggest", bytes.NewReader(body))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
			assertNoPathLeak(t, w.Body.String(), root)
		})
	}
}

// A legitimate nested path must pass the guard. It stops at the file read (404)
// rather than reaching the provider, which keeps this test hermetic while still
// proving the guard is not what rejected it.
func TestNotesSuggestAllowsNestedPath(t *testing.T) {
	root, _, _ := traversalVault(t)
	a := &app.App{Config: &config.Config{
		Vault: config.VaultConfig{Root: root},
		LLM:   config.LLMConfig{Provider: "openai", APIKey: "unused", Model: "unused", Endpoint: "http://127.0.0.1:1"},
	}}
	mux := http.NewServeMux()
	api.RegisterNotesRoutes(mux, a)

	body, _ := json.Marshal(map[string]string{"path": "sub/dir/absent.md", "instruction": "tidy up"})
	req := httptest.NewRequest("POST", "/api/notes/suggest", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 (guard passed, file missing), got %d: %s", w.Code, w.Body.String())
	}
}
