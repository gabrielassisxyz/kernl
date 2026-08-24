package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/api"
	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

// noteIDFromDisk reads the id: line the write handler injected, so the test
// can look the note up in the graph without depending on the response body.
func noteIDFromDisk(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back %s: %v", path, err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if id, ok := strings.CutPrefix(line, "id: "); ok {
			return strings.TrimSpace(id)
		}
	}
	t.Fatalf("no id: line found in %s:\n%s", path, raw)
	return ""
}

// TestVaultWriteTagsLandOnTheGraphNode is the integration case: tags sent on
// the query param must reach the node's own tag rows, not merely echo back in
// the HTTP response. Asserting through GetNote is what proves the reconciler
// path adopted them, the same path an ordinary vault-watcher pass takes.
func TestVaultWriteTagsLandOnTheGraphNode(t *testing.T) {
	root := t.TempDir()
	g := testutil.NewInMemoryTestGraph(t)
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}, Graph: g}

	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)

	const path = "tagged-note.md"
	body := "---\ntitle: Tagged\n---\n\nbody\n"
	req := httptest.NewRequest("POST", "/api/vault/file?path="+path+"&tags=handoff,checkpoint", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("write returned %d: %s", w.Code, w.Body.String())
	}

	id := noteIDFromDisk(t, filepath.Join(root, path))

	// The write handler only injects the frontmatter; the graph node is
	// carried by the reconciler that runs against the file. Drive that same
	// path here instead of reimplementing it, so the test proves what a real
	// cold start would do.
	ctx := context.Background()
	r := reconcile.New(g, root)
	if err := r.OnCreate(ctx, filepath.Join(root, path)); err != nil {
		t.Fatalf("reconcile note: %v", err)
	}

	var note *nodes.Note
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		note, err = nodes.GetNote(ctx, tx, id)
		return err
	}); err != nil {
		t.Fatalf("get note: %v", err)
	}

	got := append([]string{}, note.Tags...)
	sort.Strings(got)
	want := []string{"checkpoint", "handoff"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("node tags = %v, want %v", got, want)
	}
}

// TestVaultWriteWithoutTagsLeavesExistingTagsUntouched is the regression: a
// write with no --tags at all must not clear a note's existing tags. Omitting
// the flag has to be indistinguishable, on the wire, from never touching tags -
// collapsing that with "" is the data-loss bug the flag exists to avoid.
func TestVaultWriteWithoutTagsLeavesExistingTagsUntouched(t *testing.T) {
	root := t.TempDir()
	g := testutil.NewInMemoryTestGraph(t)
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}, Graph: g}

	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)

	const path = "keeps-tags.md"
	ctx := context.Background()

	// First write establishes the tags.
	first := "---\ntitle: Keeps Tags\n---\n\nbody\n"
	req := httptest.NewRequest("POST", "/api/vault/file?path="+path+"&tags=keep-me", bytes.NewBufferString(first))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first write returned %d: %s", w.Code, w.Body.String())
	}

	// Second write omits --tags entirely, and - as a real round trip through
	// 'kernl note read' would - resends the frontmatter the first write left
	// on disk, tags: block included, only the body text changed.
	firstOnDisk, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatalf("read back after first write: %v", err)
	}
	second := strings.Replace(string(firstOnDisk), "body\n", "body edited\n", 1)
	req2 := httptest.NewRequest("POST", "/api/vault/file?path="+path, bytes.NewBufferString(second))
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("second write returned %d: %s", w2.Code, w2.Body.String())
	}

	written, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(string(written), "keep-me") {
		t.Fatalf("a write with no --tags must leave the existing tags: block untouched, got:\n%s", written)
	}

	id := noteIDFromDisk(t, filepath.Join(root, path))
	r := reconcile.New(g, root)
	if err := r.OnCreate(ctx, filepath.Join(root, path)); err != nil {
		t.Fatalf("reconcile note: %v", err)
	}
	var note *nodes.Note
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		note, err = nodes.GetNote(ctx, tx, id)
		return err
	}); err != nil {
		t.Fatalf("get note: %v", err)
	}
	if len(note.Tags) != 1 || note.Tags[0] != "keep-me" {
		t.Fatalf("node tags = %v, want [keep-me]", note.Tags)
	}
}

// TestVaultWriteEmptyTagsParamClearsExistingTags is the other half of the
// present/absent distinction: 'tags=' (present, empty) must actively clear
// the tags, not be mistaken for the param being absent. A handler that
// branches on Get("tags") != "" instead of Has("tags") passes the previous
// test and fails this one, which is exactly the bug this pair exists to
// separate.
func TestVaultWriteEmptyTagsParamClearsExistingTags(t *testing.T) {
	root := t.TempDir()
	g := testutil.NewInMemoryTestGraph(t)
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}, Graph: g}

	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)

	const path = "clears-tags.md"
	ctx := context.Background()

	first := "---\ntitle: Clears Tags\n---\n\nbody\n"
	req := httptest.NewRequest("POST", "/api/vault/file?path="+path+"&tags=drop-me", bytes.NewBufferString(first))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first write returned %d: %s", w.Code, w.Body.String())
	}

	// The second write's body still carries the tags: block the first write
	// left on disk (the honest round-trip shape); tags= must strip it anyway,
	// or this test would pass even if InjectTags were never called.
	firstOnDisk, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatalf("read back after first write: %v", err)
	}
	second := strings.Replace(string(firstOnDisk), "body\n", "body edited\n", 1)
	req2 := httptest.NewRequest("POST", "/api/vault/file?path="+path+"&tags=", bytes.NewBufferString(second))
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("second write returned %d: %s", w2.Code, w2.Body.String())
	}

	written, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if strings.Contains(string(written), "drop-me") {
		t.Fatalf("'--tags \"\"' must clear the tags: block, got:\n%s", written)
	}

	id := noteIDFromDisk(t, filepath.Join(root, path))
	r := reconcile.New(g, root)
	if err := r.OnCreate(ctx, filepath.Join(root, path)); err != nil {
		t.Fatalf("reconcile note: %v", err)
	}
	var note *nodes.Note
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		note, err = nodes.GetNote(ctx, tx, id)
		return err
	}); err != nil {
		t.Fatalf("get note: %v", err)
	}
	if len(note.Tags) != 0 {
		t.Fatalf("node tags = %v, want none", note.Tags)
	}
}

// TestVaultWriteRejectsUnwritableTagsRatherThanDropThem is the fix for a
// handler that used to swallow InjectTags' error the same way InjectID's is
// swallowed: 'if injected, injErr := ...; injErr == nil { body = injected }'.
// InjectID is genuinely best-effort - it runs on every write whether or not
// the caller asked for it. InjectTags only ever runs because --tags was
// explicitly passed, so an error there must fail the request loudly: a 200
// that quietly wrote the note WITHOUT the tags the caller believes it sent
// is a request the caller has no way to know went wrong.
func TestVaultWriteRejectsUnwritableTagsRatherThanDropThem(t *testing.T) {
	root := t.TempDir()
	g := testutil.NewInMemoryTestGraph(t)
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}, Graph: g}

	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)

	const path = "malformed.md"
	// Frontmatter that fails to parse before InjectTags ever gets to run.
	body := "---\n\t\tt\n---\n"
	req := httptest.NewRequest("POST", "/api/vault/file?path="+path+"&tags=alpha,beta", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Fatalf("a write whose --tags could not be applied must not return 200, got %d: %s", w.Code, w.Body.String())
	}

	// Nothing must land on disk: a caller who sees a non-200 for a write
	// error has no reason to think the file exists at all, so a half-write
	// (body saved, tags silently missing) would be its own way of lying.
	if _, err := os.Stat(filepath.Join(root, path)); err == nil {
		t.Fatalf("the file must not be written when --tags could not be applied")
	}
}
