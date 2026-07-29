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

const appendableNote = "---\nid: 019f14ca-58ad-7203-8cf8-487f765f0002\ntitle: Log\n---\n\n# Log\n\n## older\n\ntext\n"

// appendVault sets up a vault holding one note and returns the mux plus the
// note's absolute path.
func appendVault(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	root := t.TempDir()
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}}
	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)

	full := filepath.Join(root, "log.md")
	if err := os.WriteFile(full, []byte(appendableNote), 0o644); err != nil {
		t.Fatal(err)
	}
	return mux, full
}

func postAppend(t *testing.T, mux *http.ServeMux, query, block string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/vault/append?"+query, bytes.NewBufferString(block))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestVaultAppendAddsToTheEnd(t *testing.T) {
	mux, full := appendVault(t)

	w := postAppend(t, mux, "path=log.md", "## newer\n\nfresh\n")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("ETag") == "" {
		t.Error("expected an ETag so a caller can baseline the note it just grew")
	}

	got := readFile(t, full)
	if !strings.HasPrefix(got, appendableNote) {
		t.Fatalf("everything that was already there must survive byte for byte, got:\n%q", got)
	}
	if !strings.HasSuffix(got, "\n## newer\n\nfresh\n") {
		t.Fatalf("the block must land last, got:\n%q", got)
	}
}

// The reason the verb exists in the first place: the caller ships only the new
// block, and the note is not rewritten around it.
func TestVaultAppendToTheStartKeepsFrontmatterAndTheRestIntact(t *testing.T) {
	mux, full := appendVault(t)

	w := postAppend(t, mux, "path=log.md&position=start", "## newest\n\nfresh\n")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	got := readFile(t, full)
	fm, err := frontmatter.Parse([]byte(got))
	if err != nil {
		t.Fatalf("prepending must leave parseable frontmatter: %v\n%s", err, got)
	}
	if fm.ID != "019f14ca-58ad-7203-8cf8-487f765f0002" {
		t.Fatalf("the note id must survive a prepend, got %q", fm.ID)
	}
	if !strings.HasSuffix(got, "# Log\n\n## older\n\ntext\n") {
		t.Fatalf("the existing body must follow the new block untouched, got:\n%q", got)
	}
	if strings.Index(got, "## newest") > strings.Index(got, "# Log") {
		t.Fatal("the new block must come before the existing body")
	}
}

// Creating on append would scatter one log across several files whenever a path
// is misspelled, and the caller would notice only when entries went missing.
func TestVaultAppendRefusesAMissingNote(t *testing.T) {
	mux, _ := appendVault(t)

	w := postAppend(t, mux, "path=nope.md", "## entry\n")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "nope.md") {
		t.Errorf("the error must name the note, got %q", w.Body.String())
	}
}

func TestVaultAppendRejectsBadInput(t *testing.T) {
	mux, full := appendVault(t)
	before := readFile(t, full)

	cases := []struct {
		name  string
		query string
		block string
	}{
		{"unknown position", "path=log.md&position=middle", "## entry\n"},
		{"empty block", "path=log.md", "  \n\n"},
		{"missing path", "position=end", "## entry\n"},
		{"traversal", "path=../outside.md", "## entry\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := postAppend(t, mux, tc.query, tc.block)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
	if readFile(t, full) != before {
		t.Error("a rejected request must not have touched the note")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
