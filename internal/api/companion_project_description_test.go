package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

// createProjectWith posts a project and returns its id.
func createProjectWith(t *testing.T, r http.Handler, body map[string]any) string {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/projects", bytes.NewReader(raw))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("creating project: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.ID
}

// patchProject sends a PATCH and asserts it succeeded.
func patchProject(t *testing.T, r http.Handler, id string, body map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("PATCH", "/api/projects/"+id, bytes.NewReader(raw))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("patching project: expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

// TestProjectDescriptionLandsInCompanionFrontmatter covers the create half. A
// project's description used to live only in the node's attrs blob, so the one
// sentence saying what the project is could not be read from the vault at all.
func TestProjectDescriptionLandsInCompanionFrontmatter(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)

	const want = "A live page showing what this desktop is spending its resources on."
	id := createProjectWith(t, r, map[string]any{"title": "hardware-usage", "description": want})

	if got := companionDescription(t, a, vault, id); got != want {
		t.Errorf("companion frontmatter description = %q, want %q", got, want)
	}
}

// TestPatchingProjectDescriptionRewritesTheCompanion covers the edit half. The
// create half alone was refused on purpose: a value written once and never
// updated is a lie the file tells on every later edit.
func TestPatchingProjectDescriptionRewritesTheCompanion(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)
	ctx := context.Background()

	id := createProjectWith(t, r, map[string]any{"title": "hardware-usage", "description": "first take"})
	patchProject(t, r, id, map[string]any{"description": "Rule: the file loses, the node wins."})

	if got := companionDescription(t, a, vault, id); got != "Rule: the file loses, the node wins." {
		t.Errorf("companion file was not rewritten: description = %q", got)
	}

	var stored string
	if err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT json_extract(attrs, '$.description') FROM nodes WHERE id = ?`, id).Scan(&stored)
	}); err != nil {
		t.Fatalf("read project attrs: %v", err)
	}
	if stored != "Rule: the file loses, the node wins." {
		t.Errorf("project node description = %q", stored)
	}

	patchProject(t, r, id, map[string]any{"description": ""})
	if raw := companionBytes(t, a, vault, id); bytes.Contains(raw, []byte("description:")) {
		t.Errorf("a cleared description left an empty key behind:\n%s", raw)
	}
}

// TestPatchingProjectTitleKeepsTheDescription guards the read-modify-write the
// project patch does that the task patch does not: title and description share
// one attrs blob, so patching the title reads the stored description back and
// writes it again. A regression there would blank the frontmatter on a rename.
func TestPatchingProjectTitleKeepsTheDescription(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)

	const want = "The description a rename must not touch."
	id := createProjectWith(t, r, map[string]any{"title": "old name", "description": want})
	patchProject(t, r, id, map[string]any{"title": "new name"})

	if got := companionDescription(t, a, vault, id); got != want {
		t.Errorf("renaming the project changed its companion description: got %q, want %q", got, want)
	}
}

// TestProjectDescriptionRewriteKeepsTheBody is what makes frontmatter the right
// home for the description: the working material underneath belongs to the user
// and a sync must not be able to reach it.
func TestProjectDescriptionRewriteKeepsTheBody(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)

	id := createProjectWith(t, r, map[string]any{"title": "hardware-usage", "description": "first take"})
	full := filepath.Join(vault, filepath.FromSlash(companionPath(t, a, id)))
	raw, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read companion: %v", err)
	}
	const body = "\n## Research\n\nEverything I know about this, by hand.\n"
	if err := os.WriteFile(full, append(raw, []byte(body)...), 0o644); err != nil {
		t.Fatalf("append body: %v", err)
	}

	patchProject(t, r, id, map[string]any{"description": "second take"})

	if got := companionBytes(t, a, vault, id); !bytes.Contains(got, []byte(body)) {
		t.Errorf("the sync ate the hand-written body:\n%s", got)
	}
}

// TestProjectDescriptionRewriteLeavesNoDriftForColdStart pins the half that is
// invisible until the next boot: a file kernl rewrote without refreshing
// note_paths.content_hash reads as a user edit to the reconciler.
func TestProjectDescriptionRewriteLeavesNoDriftForColdStart(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)
	ctx := context.Background()

	id := createProjectWith(t, r, map[string]any{"title": "hardware-usage", "description": "first take"})
	patchProject(t, r, id, map[string]any{"description": "second take"})

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

	rec := reconcile.New(a.Graph, vault)
	if err := rec.ColdStart(ctx); err != nil {
		t.Fatalf("ColdStart: %v", err)
	}
	if stats := rec.Stats(); stats != (reconcile.Stats{}) {
		t.Errorf("cold start had work to do after a description edit: %+v", stats)
	}
}
