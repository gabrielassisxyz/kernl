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

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/vault/layout"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

// newCompanionTestApp builds an app backed by a real on-disk graph and a temp
// vault root, so companion-note file writes can be asserted.
func newCompanionTestApp(t *testing.T) (*app.App, string) {
	t.Helper()
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	t.Cleanup(func() { g.Close() })
	vault := t.TempDir()
	cfg := &config.Config{Vault: config.VaultConfig{Root: vault}}
	// NewRouter wires bead routes that index Registry.Repos[0]; give it one.
	cfg.Registry.Repos = []config.RepoEntry{{Path: t.TempDir()}}
	a := &app.App{Graph: g, Config: cfg}
	return a, vault
}

// companionAssertions verifies all four facets of a companion note for entityID.
func companionAssertions(t *testing.T, a *app.App, vault, entityID, folder string) {
	t.Helper()
	ctx := context.Background()

	var noteID, notePath string
	err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		// (b) a describes edge note->entity, whose src is a live note node (a).
		row := tx.QueryRow(
			`SELECT e.src FROM edges e
			 JOIN nodes n ON n.id = e.src AND n.type = 'note' AND n.deleted_at IS NULL
			 WHERE e.dst = ? AND e.label = 'describes'`,
			entityID,
		)
		if err := row.Scan(&noteID); err != nil {
			return err
		}
		// (d) note_paths maps the companion note.
		return tx.QueryRow(`SELECT path FROM note_paths WHERE uuid = ?`, noteID).Scan(&notePath)
	})
	if err != nil {
		t.Fatalf("companion graph lookup for %s: %v", entityID, err)
	}
	if noteID == "" {
		t.Fatalf("no companion note node for entity %s", entityID)
	}

	wantPrefix := folder + "/"
	if len(notePath) < len(wantPrefix) || notePath[:len(wantPrefix)] != wantPrefix {
		t.Errorf("note_paths path %q does not start with %q", notePath, wantPrefix)
	}

	// (c) the markdown file exists on disk with the note id in frontmatter.
	full := filepath.Join(vault, filepath.FromSlash(notePath))
	data, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("companion file not on disk (%s): %v", full, err)
	}
	if !bytes.Contains(data, []byte("id: "+noteID)) {
		t.Errorf("companion file %s missing frontmatter id %q:\n%s", full, noteID, data)
	}
}

func TestCompanionNoteCreatedForProject(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)

	body, _ := json.Marshal(map[string]string{"title": "Launch Plan"})
	req := httptest.NewRequest("POST", "/api/projects", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	companionAssertions(t, a, vault, resp.ID, layout.ProjectsFolder)
}

func TestCompanionNoteCreatedForTask(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)

	body, _ := json.Marshal(map[string]string{"title": "Write tests"})
	req := httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	companionAssertions(t, a, vault, resp.ID, layout.TasksFolder)
}

func TestCompanionNoteCreatedForBookmark(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)

	body, _ := json.Marshal(map[string]string{"url": "https://example.com/article"})
	req := httptest.NewRequest("POST", "/api/bookmarks", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	companionAssertions(t, a, vault, resp.ID, layout.BookmarksFolder)
}

// TestCompanionNoteHandlesDuplicateTitles guards the whole create path: the slug
// is derived from the title and note_paths.path is UNIQUE, so two entities named
// the same collide. The collision is not cosmetic - the companion note is written
// in the same transaction as the entity, so a duplicate path fails the INSERT and
// takes the entity creation down with it. On disk the second write would also
// clobber the first note's file, orphaning its frontmatter id.
func TestCompanionNoteHandlesDuplicateTitles(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)

	create := func(title string) string {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"title": title})
		req := httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("creating task %q: expected 201, got %d: %s", title, w.Code, w.Body.String())
		}
		var resp struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return resp.ID
	}

	first := create("Same Title")
	second := create("Same Title")

	companionAssertions(t, a, vault, first, layout.TasksFolder)
	companionAssertions(t, a, vault, second, layout.TasksFolder)

	firstPath := companionPath(t, a, first)
	secondPath := companionPath(t, a, second)
	if firstPath == secondPath {
		t.Fatalf("both companions map to the same path %q", firstPath)
	}
}

// TestCompanionNoteNeverClobbersHandWrittenFile covers the other half of the
// collision: a note the user wrote by hand inside a generated folder has no
// note_paths row yet, so the table alone cannot see it. Creating an entity whose
// slug matches must step aside instead of overwriting the file.
func TestCompanionNoteNeverClobbersHandWrittenFile(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)

	handWritten := filepath.Join(vault, filepath.FromSlash(layout.TasksFolder), "read-the-docs.md")
	if err := os.MkdirAll(filepath.Dir(handWritten), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	const original = "my own note, not kernl's\n"
	if err := os.WriteFile(handWritten, []byte(original), 0o644); err != nil {
		t.Fatalf("write hand-made note: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"title": "Read the docs"})
	req := httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	kept, err := os.ReadFile(handWritten)
	if err != nil {
		t.Fatalf("read hand-made note: %v", err)
	}
	if string(kept) != original {
		t.Errorf("hand-made note was overwritten:\n%s", kept)
	}
}

// companionPath returns the note_paths row of the companion note describing
// entityID.
func companionPath(t *testing.T, a *app.App, entityID string) string {
	t.Helper()
	var path string
	err := a.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		return tx.QueryRow(
			`SELECT p.path FROM note_paths p
			 JOIN edges e ON e.src = p.uuid
			 WHERE e.dst = ? AND e.label = 'describes'`,
			entityID,
		).Scan(&path)
	})
	if err != nil {
		t.Fatalf("companion path for %s: %v", entityID, err)
	}
	return path
}

// TestCompanionNoteAdoptedByReconciler is the load-bearing test for the
// companion-note design: after the handler writes the markdown file (with
// frontmatter id == note node id) and the note_paths row (with matching hash),
// a reconciler ColdStart over the vault must NOT create a duplicate note node.
func TestCompanionNoteAdoptedByReconciler(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)
	ctx := context.Background()

	body, _ := json.Marshal(map[string]string{"title": "Adopt Me"})
	req := httptest.NewRequest("POST", "/api/projects", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
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
	if before != 1 {
		t.Fatalf("expected exactly 1 companion note before reconcile, got %d", before)
	}

	rec := reconcile.New(a.Graph, vault)
	if err := rec.ColdStart(ctx); err != nil {
		t.Fatalf("ColdStart: %v", err)
	}

	if after := countNotes(); after != before {
		t.Errorf("reconciler duplicated the companion note: had %d, now %d", before, after)
	}
}
