package api_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/api"
	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func writeNoteWithPermission(t *testing.T, root string, a *app.App, permission string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)

	body := "---\ntitle: Policy note\npermission: " + permission + "\n---\n\nA note that declares a policy.\n"
	req := httptest.NewRequest("POST", "/api/vault/file?path=policy-note.md", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// TestVaultWriteRefusesInvalidPermission verifies the refusal reaches the caller
// and, more importantly, that nothing lands on disk.
//
// noteFromFile refuses the value too, but that runs on the reconciler's own
// pass: the write would answer "saved", the file would exist, and the graph
// would never adopt it. A note that exists as a file and not as a node is the
// orphan the shared write primitive was built to remove, and it is invisible
// from the caller's side.
func TestVaultWriteRefusesInvalidPermission(t *testing.T) {
	root := t.TempDir()
	g := testutil.NewInMemoryTestGraph(t)
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}, Graph: g}

	w := writeNoteWithPermission(t, root, a, "read-only")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("an unrecognised permission must be refused with 400, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "policy-note.md")); !os.IsNotExist(err) {
		t.Errorf("a refused write must not leave a file behind, but policy-note.md exists")
	}
}

// TestVaultWriteAcceptsValidPermission is the other direction: the guard must
// not refuse a value the resolver accepts.
func TestVaultWriteAcceptsValidPermission(t *testing.T) {
	for _, permission := range []string{"ask", "edit"} {
		t.Run(permission, func(t *testing.T) {
			root := t.TempDir()
			g := testutil.NewInMemoryTestGraph(t)
			a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}, Graph: g}

			w := writeNoteWithPermission(t, root, a, permission)
			if w.Code != http.StatusOK {
				t.Fatalf("permission %q must be accepted, got %d: %s", permission, w.Code, w.Body.String())
			}
			if _, err := os.Stat(filepath.Join(root, "policy-note.md")); err != nil {
				t.Errorf("an accepted write must leave the file: %v", err)
			}
		})
	}
}
