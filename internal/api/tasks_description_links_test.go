package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/api"
	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func linkTargets(t *testing.T, g *graph.Graph, src string) []string {
	t.Helper()
	var out []string
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		rows, err := tx.Query(
			`SELECT dst FROM edges WHERE src = ? AND label = 'links_to' ORDER BY dst`, src)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d string
			if err := rows.Scan(&d); err != nil {
				return err
			}
			out = append(out, d)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("read edges: %v", err)
	}
	return out
}

func danglingKeys(t *testing.T, g *graph.Graph, src string) []string {
	t.Helper()
	var out []string
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		rows, err := tx.Query(`SELECT target_key FROM dangling_links WHERE src_node_id = ?`, src)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				return err
			}
			out = append(out, k)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("read dangling: %v", err)
	}
	return out
}

func taskAPI(t *testing.T) (*http.ServeMux, *graph.Graph) {
	t.Helper()
	g := testutil.NewInMemoryTestGraph(t)
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: t.TempDir()}}, Graph: g}
	mux := http.NewServeMux()
	api.RegisterTaskRoutes(mux, a)
	return mux, g
}

func createTask(t *testing.T, mux *http.ServeMux, body string) string {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/tasks", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create task returned %d: %s", w.Code, w.Body.String())
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out.ID
}

// TestTaskDescriptionResolvesItsWikilinks is the feature: a wikilink typed into a
// description becomes a links_to edge, exactly as one typed into a note's body does.
//
// It could not before, and the reason is worth keeping: wikilink resolution is driven by
// FILE CHANGE, and a description is a field on a node that changes no file. The
// description IS mirrored into the companion note's frontmatter, but the reconciler hands
// the resolver the body BELOW the frontmatter, so nothing ever parsed it.
func TestTaskDescriptionResolvesItsWikilinks(t *testing.T) {
	mux, g := taskAPI(t)
	ctx := context.Background()

	var targetID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		targetID, err = nodes.CreateNote(ctx, tx,
			nodes.Note{Title: "Zephyr protocol", Body: "retries"}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed note: %v", err)
	}

	id := createTask(t, mux, `{"title":"Ship it","description":"blocked on [[Zephyr protocol]]"}`)

	got := linkTargets(t, g, id)
	if len(got) != 1 || got[0] != targetID {
		t.Fatalf("the description's wikilink must become an edge from the task, got %v", got)
	}
}

// TestTaskDescriptionLinksAreReplacedNotAccumulated is the half that would corrupt
// quietly. Editing a description has to REPLACE its links: the resolver clears every
// links_to edge leaving the source before rebuilding, so a link removed from the text
// disappears from the graph and a repeated save does not duplicate anything.
func TestTaskDescriptionLinksAreReplacedNotAccumulated(t *testing.T) {
	mux, g := taskAPI(t)
	ctx := context.Background()

	var first, second string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		if first, err = nodes.CreateNote(ctx, tx,
			nodes.Note{Title: "First target", Body: "a"}, nodes.Author{Name: "test"}); err != nil {
			return err
		}
		second, err = nodes.CreateNote(ctx, tx,
			nodes.Note{Title: "Second target", Body: "b"}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed notes: %v", err)
	}

	id := createTask(t, mux, `{"title":"Ship it","description":"see [[First target]]"}`)
	if got := linkTargets(t, g, id); len(got) != 1 || got[0] != first {
		t.Fatalf("setup: want the first target, got %v", got)
	}

	req := httptest.NewRequest("PATCH", "/api/tasks/"+id,
		bytes.NewBufferString(`{"description":"see [[Second target]]"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("patch returned %d: %s", w.Code, w.Body.String())
	}

	got := linkTargets(t, g, id)
	if len(got) != 1 || got[0] != second {
		t.Fatalf("editing a description must replace its links, not add to them, got %v", got)
	}
}

// TestTaskDescriptionParksADanglingLink covers the case the vault convention depends on:
// a description may name a note that has not been written yet, and that has to become a
// queue entry rather than being dropped. Adoption then promotes it into an edge from the
// task, which is the machinery that already exists for notes.
func TestTaskDescriptionParksADanglingLink(t *testing.T) {
	mux, g := taskAPI(t)

	id := createTask(t, mux, `{"title":"Ship it","description":"blocked on [[A note nobody wrote yet]]"}`)

	if got := danglingKeys(t, g, id); len(got) != 1 || got[0] != "A note nobody wrote yet" {
		t.Fatalf("a description naming a note that does not exist must park a dangling row, got %v", got)
	}
	if got := linkTargets(t, g, id); len(got) != 0 {
		t.Fatalf("nothing to link to yet, got %v", got)
	}
}
