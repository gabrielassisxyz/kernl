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
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/vault/layout"
)

// seedBareTask creates a task straight through the node layer, with no companion,
// which is exactly the state the entities predating the fix are in.
func seedBareTask(t *testing.T, a *app.App, title, description string) string {
	t.Helper()
	var id string
	if err := a.Graph.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateTask(context.Background(), tx, nodes.Task{
			Title: title, Description: description,
		}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return id
}

func decodeBackfill(t *testing.T, body []byte) struct {
	DryRun   bool `json:"dryRun"`
	Entities []struct {
		ID    string `json:"id"`
		Type  string `json:"type"`
		Title string `json:"title"`
		Path  string `json:"path"`
	} `json:"entities"`
} {
	t.Helper()
	var out struct {
		DryRun   bool `json:"dryRun"`
		Entities []struct {
			ID    string `json:"id"`
			Type  string `json:"type"`
			Title string `json:"title"`
			Path  string `json:"path"`
		} `json:"entities"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	return out
}

// The dry run has to be exactly that: a list, and not one byte written.
func TestCompanionMissingListsWithoutWriting(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)
	id := seedBareTask(t, a, "Task sem companheira", "a descrição")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/vault/companions/missing", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	got := decodeBackfill(t, w.Body.Bytes())
	if !got.DryRun {
		t.Error("GET must report itself as a dry run")
	}
	if len(got.Entities) != 1 || got.Entities[0].ID != id {
		t.Fatalf("expected only %s, got %+v", id, got.Entities)
	}
	if got.Entities[0].Path != "" {
		t.Errorf("dry run reported a written path %q", got.Entities[0].Path)
	}
	if _, err := os.Stat(filepath.Join(vault, layout.TasksFolder)); !os.IsNotExist(err) {
		t.Errorf("dry run touched the vault: %v", err)
	}
}

// The write is the whole point: after it, the entity is in the markdown that the
// backup actually carries.
func TestCompanionBackfillWritesTheMissingNotes(t *testing.T) {
	a, vault := newCompanionTestApp(t)
	r := NewRouter(a)
	first := seedBareTask(t, a, "Primeira", "descrição da primeira")
	second := seedBareTask(t, a, "Segunda", "")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/vault/companions/backfill", bytes.NewReader([]byte("{}"))))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	got := decodeBackfill(t, w.Body.Bytes())
	if got.DryRun {
		t.Error("POST must not report itself as a dry run")
	}
	if len(got.Entities) != 2 {
		t.Fatalf("expected 2 entities, got %+v", got.Entities)
	}
	for _, e := range got.Entities {
		if e.Path == "" {
			t.Errorf("entity %s reported no written path", e.ID)
		}
	}

	// The four facets the API path guarantees hold for the backfilled ones too:
	// note node, describes edge, note_paths row, and the file on disk.
	companionAssertions(t, a, vault, first, layout.TasksFolder)
	companionAssertions(t, a, vault, second, layout.TasksFolder)

	// The description travels from the node's attrs, which is the only place the
	// sweep can read it from: nobody passed it in.
	path := companionPathFor(t, a, first)
	data, err := os.ReadFile(filepath.Join(vault, filepath.FromSlash(path)))
	if err != nil {
		t.Fatalf("read companion: %v", err)
	}
	if !bytes.Contains(data, []byte("descrição da primeira")) {
		t.Errorf("companion lost the description from the node attrs:\n%s", data)
	}
}

// Running it twice must be a no-op, not a second note. Repair commands get run
// again out of doubt, and a sweep that duplicates on the second run is worse than
// one that never ran.
func TestCompanionBackfillIsIdempotent(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)
	seedBareTask(t, a, "Uma só", "")

	for i := range 2 {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", "/api/vault/companions/backfill", bytes.NewReader([]byte("{}"))))
		if w.Code != http.StatusOK {
			t.Fatalf("run %d: expected 200, got %d: %s", i+1, w.Code, w.Body.String())
		}
		got := decodeBackfill(t, w.Body.Bytes())
		want := 1
		if i == 1 {
			want = 0
		}
		if len(got.Entities) != want {
			t.Errorf("run %d wrote %d companion(s), want %d", i+1, len(got.Entities), want)
		}
	}
}

// An entity created through the API already has a companion, so the sweep must
// leave it alone: this is what stops the repair from being a duplicator.
func TestCompanionBackfillSkipsEntitiesThatHaveOne(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)

	body, _ := json.Marshal(map[string]string{"title": "Criada pela API"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(body)))
	if w.Code != http.StatusCreated {
		t.Fatalf("create task: %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/vault/companions/missing", nil))
	got := decodeBackfill(t, w.Body.Bytes())
	if len(got.Entities) != 0 {
		t.Errorf("sweep wants to rewrite entities that already have a companion: %+v", got.Entities)
	}
}

// companionPathFor returns the note_paths path of an entity's companion.
func companionPathFor(t *testing.T, a *app.App, entityID string) string {
	t.Helper()
	var path string
	if err := a.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		return tx.QueryRow(
			`SELECT p.path FROM edges e
			 JOIN nodes n ON n.id = e.src AND n.type = 'note' AND n.deleted_at IS NULL
			 JOIN note_paths p ON p.uuid = n.id
			 WHERE e.dst = ? AND e.label = 'describes'`,
			entityID,
		).Scan(&path)
	}); err != nil {
		t.Fatalf("companion path for %s: %v", entityID, err)
	}
	return path
}
