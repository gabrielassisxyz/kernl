package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

// createTaskWith posts a task and returns its id.
func createTaskWith(t *testing.T, r http.Handler, body map[string]any) string {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(raw))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("creating task: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.ID
}

// patchTask sends a PATCH and asserts it succeeded.
func patchTask(t *testing.T, r http.Handler, id string, body map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("PATCH", "/api/tasks/"+id, bytes.NewReader(raw))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("patching task: expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

// companionDescription reads the description out of the companion note's file.
func companionDescription(t *testing.T, a *app.App, vault, entityID string) string {
	t.Helper()
	raw := companionBytes(t, a, vault, entityID)
	var fm struct {
		Description string `yaml:"description"`
	}
	block, _, found := bytes.Cut(bytes.TrimPrefix(raw, []byte("---\n")), []byte("\n---\n"))
	if !found {
		t.Fatalf("no frontmatter block in companion file:\n%s", raw)
	}
	if err := yaml.Unmarshal(block, &fm); err != nil {
		t.Fatalf("companion frontmatter does not parse: %v\n%s", err, raw)
	}
	return fm.Description
}

func companionBytes(t *testing.T, a *app.App, vault, entityID string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(vault, filepath.FromSlash(companionPath(t, a, entityID))))
	if err != nil {
		t.Fatalf("read companion file: %v", err)
	}
	return raw
}

// TestTaskDescriptionLandsInCompanionFrontmatter covers the create half: the
// description a task is born with is visible at the top of its note.
func TestTaskDescriptionLandsInCompanionFrontmatter(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)

	const want = "For simple tasks that need no human input."
	id := createTaskWith(t, r, map[string]any{"title": "Write tests", "description": want})

	if got := companionDescription(t, a, vault, id); got != want {
		t.Errorf("companion frontmatter description = %q, want %q", got, want)
	}
}

// TestPatchingDescriptionRewritesTheCompanion covers the edit half, and the
// reason the edit path had to exist at all: CreateCompanionNote wrote the file
// once and nothing ever wrote it again, so a description changed after creation
// left the note showing the original forever.
func TestPatchingDescriptionRewritesTheCompanion(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)
	ctx := context.Background()

	id := createTaskWith(t, r, map[string]any{"title": "Write tests", "description": "first take"})
	patchTask(t, r, id, map[string]any{"description": "Rule: the file loses, the node wins."})

	if got := companionDescription(t, a, vault, id); got != "Rule: the file loses, the node wins." {
		t.Errorf("companion file was not rewritten: description = %q", got)
	}

	// The node is the authority, so it has to carry the same text.
	var stored string
	if err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT json_extract(attrs, '$.description') FROM nodes WHERE id = ?`, id).Scan(&stored)
	}); err != nil {
		t.Fatalf("read task attrs: %v", err)
	}
	if stored != "Rule: the file loses, the node wins." {
		t.Errorf("task node description = %q", stored)
	}

	// Clearing it removes the key rather than writing an empty one.
	patchTask(t, r, id, map[string]any{"description": ""})
	if raw := companionBytes(t, a, vault, id); bytes.Contains(raw, []byte("description:")) {
		t.Errorf("a cleared description left an empty key behind:\n%s", raw)
	}
}

// TestDescriptionRewriteKeepsTheBody is the promise that makes frontmatter the
// right home for the description: the working material underneath is the user's
// and a sync must not be able to reach it.
func TestDescriptionRewriteKeepsTheBody(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)

	id := createTaskWith(t, r, map[string]any{"title": "Write tests", "description": "first take"})
	full := filepath.Join(vault, filepath.FromSlash(companionPath(t, a, id)))

	raw, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read companion: %v", err)
	}
	const mine = "\n## My research\n\nA paragraph kernl did not write.\n"
	if err := os.WriteFile(full, append(raw, mine...), 0o644); err != nil {
		t.Fatalf("append body: %v", err)
	}

	patchTask(t, r, id, map[string]any{"description": "second take"})

	after := string(companionBytes(t, a, vault, id))
	if !strings.Contains(after, mine) {
		t.Errorf("the rewrite ate the body:\n%s", after)
	}
	if !strings.Contains(after, "description: second take") {
		t.Errorf("the description was not updated:\n%s", after)
	}
}

// TestDescriptionRewriteLeavesNoDriftForColdStart is the load-bearing one. The
// hash in note_paths has to describe the bytes kernl just wrote, in the same
// transaction as the write. Otherwise the next cold start sees a file that
// changed behind its back, takes the OnChange branch, and records a revision for
// an edit nobody made - and the same class of divergence, a live note the path
// cache had lost, is what made one file fail on every start for a month.
func TestDescriptionRewriteLeavesNoDriftForColdStart(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)
	ctx := context.Background()

	id := createTaskWith(t, r, map[string]any{"title": "Write tests", "description": "first take"})
	patchTask(t, r, id, map[string]any{"description": "second take"})

	// The cache agrees with the file byte for byte.
	var cached string
	if err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(
			`SELECT p.content_hash FROM note_paths p
			 JOIN edges e ON e.src = p.uuid
			 WHERE e.dst = ? AND e.label = 'describes'`, id,
		).Scan(&cached)
	}); err != nil {
		t.Fatalf("read cached hash: %v", err)
	}
	if onDisk := reconcile.HashBytes(companionBytes(t, a, vault, id)); cached != onDisk {
		t.Fatalf("note_paths hash %q does not describe the file (%q)", cached, onDisk)
	}

	countNotes := func() int {
		var n int
		if err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
			return tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE type = 'note' AND deleted_at IS NULL`).Scan(&n)
		}); err != nil {
			t.Fatalf("count notes: %v", err)
		}
		return n
	}
	before := countNotes()

	rec := reconcile.New(a.Graph, vault)
	if err := rec.ColdStart(ctx); err != nil {
		t.Fatalf("ColdStart: %v", err)
	}

	// A clean start touches nothing: no create, no change, no tombstone. Every
	// counter still zero is the whole assertion.
	if stats := rec.Stats(); stats != (reconcile.Stats{}) {
		t.Errorf("cold start had work to do after a description edit: %+v", stats)
	}
	if after := countNotes(); after != before {
		t.Errorf("cold start changed the note count: had %d, now %d", before, after)
	}
}

// TestDescriptionRewriteFindsTheCompanionByEdge pins the resolution rule. Two
// tasks sharing a title get suffixed file names, so the title stops predicting
// the path; anything re-deriving the slug would rewrite the first task's note
// when the second one is edited.
func TestDescriptionRewriteFindsTheCompanionByEdge(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)

	first := createTaskWith(t, r, map[string]any{"title": "Same Title", "description": "the first one"})
	second := createTaskWith(t, r, map[string]any{"title": "Same Title", "description": "the second one"})

	patchTask(t, r, second, map[string]any{"description": "edited the second"})

	if got := companionDescription(t, a, vault, second); got != "edited the second" {
		t.Errorf("second task's companion = %q", got)
	}
	if got := companionDescription(t, a, vault, first); got != "the first one" {
		t.Errorf("the edit landed on the wrong file: first task's companion = %q", got)
	}
}

// TestRenamingATaskLeavesTheCompanionFileAlone: the path stopped being a
// function of the title when duplicate titles became legal, so a rename that
// re-slugified would either collide or orphan the file's frontmatter id.
func TestRenamingATaskLeavesTheCompanionFileAlone(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)

	id := createTaskWith(t, r, map[string]any{"title": "Original title"})
	before := companionPath(t, a, id)

	patchTask(t, r, id, map[string]any{"title": "A completely different title"})

	if after := companionPath(t, a, id); after != before {
		t.Errorf("companion path changed with the title: %q -> %q", before, after)
	}
	if _, err := os.Stat(filepath.Join(vault, filepath.FromSlash(before))); err != nil {
		t.Errorf("the companion file left its recorded path: %v", err)
	}
}

// TestDescriptionSyncSkipsADeletedFile: a companion whose file the user removed
// stays removed. Recreating it here would resurrect a note they threw away, and
// the edit itself must still succeed.
func TestDescriptionSyncSkipsADeletedFile(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)

	id := createTaskWith(t, r, map[string]any{"title": "Write tests", "description": "first take"})
	full := filepath.Join(vault, filepath.FromSlash(companionPath(t, a, id)))
	if err := os.Remove(full); err != nil {
		t.Fatalf("remove companion: %v", err)
	}

	patchTask(t, r, id, map[string]any{"description": "second take"})

	if _, err := os.Stat(full); !os.IsNotExist(err) {
		t.Errorf("the deleted companion file came back: %v", err)
	}
}
