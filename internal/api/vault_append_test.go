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

// logShapedVaultNote mirrors a real machine log: frontmatter, a preamble with a
// fenced recipe holding a bare `---`, the rule, then entries newest first.
const logShapedVaultNote = "---\nid: 019f14ca-58ad-7203-8cf8-487f765f0004\ntitle: Log\n---\n\n" +
	"# Log\n\nDo not read this file whole.\n\n" +
	"```sh\nrg '^## ' \"$LOG\"\n\n---\ndone\n```\n\n" +
	"---\n\n## 2026-07-29 - newest\n\nbody\n"

func appendVaultWith(t *testing.T, contents string) (*http.ServeMux, string) {
	t.Helper()
	root := t.TempDir()
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}}
	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)

	full := filepath.Join(root, "log.md")
	if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return mux, full
}

func TestVaultAppendAfterBreakLandsUnderTheRule(t *testing.T) {
	mux, full := appendVaultWith(t, logShapedVaultNote)

	w := postAppend(t, mux, "path=log.md&position=after-break", "## 2026-07-30 - newer\n\nfresh\n")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	got := readFile(t, full)
	fm, err := frontmatter.Parse([]byte(got))
	if err != nil {
		t.Fatalf("the frontmatter must still parse: %v\n%s", err, got)
	}
	if fm.ID != "019f14ca-58ad-7203-8cf8-487f765f0004" {
		t.Fatalf("the note id must survive, got %q", fm.ID)
	}
	if !strings.Contains(got, "# Log\n\nDo not read this file whole.") {
		t.Error("the preamble must stay above the rule, untouched")
	}
	if !strings.Contains(got, "```sh\nrg '^## ' \"$LOG\"\n\n---\ndone\n```") {
		t.Error("the fenced recipe must come through byte for byte")
	}
	if !strings.HasSuffix(got, "---\n\n## 2026-07-30 - newer\n\nfresh\n\n## 2026-07-29 - newest\n\nbody\n") {
		t.Fatalf("the block must sit between the rule and the previous newest entry, got:\n%s", got)
	}
}

// Falling back to an end would let a log whose separator someone deleted rot
// into the wrong shape while every individual entry still looked right.
func TestVaultAppendAfterBreakRefusesANoteWithNoRule(t *testing.T) {
	noRule := "---\nid: x\ntitle: No rule\n---\n\n# Log\n\n## 2026-07-29 - only entry\n\nbody\n"
	mux, full := appendVaultWith(t, noRule)

	w := postAppend(t, mux, "path=log.md&position=after-break", "## newer\n")
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "log.md") || !strings.Contains(w.Body.String(), "after-break") {
		t.Errorf("the error must name the note and the placement, got %q", w.Body.String())
	}
	if readFile(t, full) != noRule {
		t.Error("a refused insert must leave the note byte-identical")
	}
}

// The frontmatter's closing fence is three dashes on a line of their own. A
// note whose only such line is the fence has no rule, and must be refused.
func TestVaultAppendAfterBreakDoesNotMistakeTheFrontmatterFenceForARule(t *testing.T) {
	mux, full := appendVaultWith(t, "---\nid: x\ntitle: Fence only\n---\n\nbody\n")

	w := postAppend(t, mux, "path=log.md&position=after-break", "## newer\n")
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(readFile(t, full), "## newer") {
		t.Fatal("nothing may have been written under the frontmatter fence")
	}
}

func TestVaultAppendRejectsAnUnknownPositionNamingTheAcceptedOnes(t *testing.T) {
	mux, _ := appendVaultWith(t, logShapedVaultNote)

	w := postAppend(t, mux, "path=log.md&position=middle", "## entry\n")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	for _, name := range []string{"start", "end", "after-break"} {
		if !strings.Contains(w.Body.String(), name) {
			t.Errorf("the error must list %q, got %q", name, w.Body.String())
		}
	}
}
