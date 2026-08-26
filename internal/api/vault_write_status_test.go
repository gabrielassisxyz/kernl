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
)

// vaultWriteResponse is the document shape every POST /api/vault/file answer
// must carry: a status field, whatever else it contains.
type vaultWriteResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

// vaultFileWrite decodes one POST /api/vault/file answer into the status
// contract, failing the test when the body is not a parseable document at all.
func vaultFileWrite(t *testing.T, root string, a *app.App, path string, body string) (int, vaultWriteResponse) {
	t.Helper()
	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)

	req := httptest.NewRequest("POST", "/api/vault/file?path="+path, bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var doc vaultWriteResponse
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("every write answer must be a JSON document, got %d %q: %v", w.Code, w.Body.String(), err)
	}
	return w.Code, doc
}

// TestVaultWriteSuccessAnswersAStatusDocument is the happy half of the
// contract: a write that landed says so, in the document, on the first try -
// not only after the caller reads the note back.
func TestVaultWriteSuccessAnswersAStatusDocument(t *testing.T) {
	root := t.TempDir()
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}}
	code, doc := vaultFileWrite(t, root, a, "status-note.md", "---\ntitle: Status\n---\n\nbody\n")
	if code != http.StatusOK {
		t.Fatalf("a healthy write must answer 200, got %d", code)
	}
	if doc.Status != "saved" {
		t.Fatalf("status = %q, want %q", doc.Status, "saved")
	}
}

// TestVaultWriteStorageFailureAnswersAStatusDocument forces the write path to
// fail - the vault root is a regular file, so the handler cannot create the
// note's parent directory and the write never reaches disk. The failure must
// be distinguishable from a success by the response alone: a JSON document
// whose status says error, with the reason attached. This is the failure-half
// assertion the bead demands - a suite that only ever sees the happy path
// passes with the status stripped from the error path.
func TestVaultWriteStorageFailureAnswersAStatusDocument(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(root, []byte("occupied"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}}

	code, doc := vaultFileWrite(t, root, a, "status-note.md", "# body\n")
	if code != http.StatusInternalServerError {
		t.Fatalf("an unwritable vault must answer 500, got %d", code)
	}
	if doc.Status != "error" {
		t.Fatalf("status = %q, want %q - a failed write must say so in the response", doc.Status, "error")
	}
	if doc.Error == "" {
		t.Fatal("an error document must name the reason")
	}
}

// TestVaultWriteRefusalAnswersAStatusDocument is the other forced failure, at
// a different point in the handler: an unrecognised permission is refused
// before the file lands. Same contract - a 400 that says error, not a bare
// text body a --json caller cannot parse the outcome from.
func TestVaultWriteRefusalAnswersAStatusDocument(t *testing.T) {
	root := t.TempDir()
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}}

	code, doc := vaultFileWrite(t, root, a, "policy-note.md", "---\npermission: read-only\n---\n\nbody\n")
	if code != http.StatusBadRequest {
		t.Fatalf("an unrecognised permission must be refused with 400, got %d", code)
	}
	if doc.Status != "error" {
		t.Fatalf("status = %q, want %q", doc.Status, "error")
	}
	if doc.Error == "" {
		t.Fatal("an error document must name the reason")
	}
}
