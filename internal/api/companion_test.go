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
	companionAssertions(t, a, vault, resp.ID, "projects")
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
	companionAssertions(t, a, vault, resp.ID, "tasks")
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
	companionAssertions(t, a, vault, resp.ID, "bookmarks")
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
