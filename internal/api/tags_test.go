package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/tags"
)

// seedTaggedNodes creates one node of each of the three types a tag surface has
// to bring together, plus a system-tagged capture, and returns their ids by type.
func seedTaggedNodes(t *testing.T, a *app.App) map[string]string {
	t.Helper()
	ctx := context.Background()
	author := nodes.Author{Name: "test"}
	ids := map[string]string{}

	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		noteID, err := nodes.CreateNote(ctx, tx, nodes.Note{
			Title: "NAS rebuild notes",
			Body:  "zfs",
			Tags:  []string{"homelab/nas"},
		}, author)
		if err != nil {
			return err
		}
		ids["note"] = noteID
		// Notes are file-backed; the listing joins note_paths to give the Notes
		// UI something to navigate by.
		if _, err := tx.Exec(`INSERT INTO note_paths(uuid, path) VALUES (?, ?)`, noteID, "homelab/nas.md"); err != nil {
			return err
		}

		taskID, err := nodes.CreateTask(ctx, tx, nodes.Task{
			Title: "Replace the failing disk",
			Tags:  []string{"homelab"},
		}, author)
		if err != nil {
			return err
		}
		ids["task"] = taskID

		bookmarkID, err := nodes.CreateBookmark(ctx, tx, nodes.Bookmark{
			URL:   "https://example.com/zfs",
			Title: "ZFS guide",
			Tags:  []string{"homelab/nas"},
		}, author)
		if err != nil {
			return err
		}
		ids["bookmark"] = bookmarkID

		captureID, err := nodes.CreateCapture(ctx, tx, nodes.Capture{
			Title: "look into this",
			Body:  "look into this",
			Tags:  []string{tags.Pending},
		}, author)
		if err != nil {
			return err
		}
		ids["capture"] = captureID
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return ids
}

func getTagTree(t *testing.T, r http.Handler, query string) []tagTreeDTO {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/tags"+query, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/tags%s: expected 200, got %d: %s", query, w.Code, w.Body.String())
	}
	var out []tagTreeDTO
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func getTagNodes(t *testing.T, r http.Handler, query string) []taggedNodeDTO {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/tags/nodes"+query, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/tags/nodes%s: expected 200, got %d: %s", query, w.Code, w.Body.String())
	}
	var out []taggedNodeDTO
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// The whole point of the surface: one request, one subject, every type it touches.
func TestTagNodesReturnsMixedTypes(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)
	ids := seedTaggedNodes(t, a)

	got := getTagNodes(t, r, "?tag=homelab")
	if len(got) != 3 {
		t.Fatalf("expected note + task + bookmark under homelab, got %d: %+v", len(got), got)
	}

	byID := map[string]taggedNodeDTO{}
	for _, n := range got {
		byID[n.ID] = n
	}
	for typ, id := range map[string]string{"note": ids["note"], "task": ids["task"], "bookmark": ids["bookmark"]} {
		n, ok := byID[id]
		if !ok {
			t.Fatalf("%s %s missing from the homelab listing", typ, id)
		}
		if n.Type != typ {
			t.Errorf("node %s: type = %q, want %q", id, n.Type, typ)
		}
		if n.Title == "" || n.UpdatedAt == "" {
			t.Errorf("node %s: title/updatedAt not hydrated: %+v", id, n)
		}
	}

	// Only the note carries a vault path — the other types navigate by id.
	if got := byID[ids["note"]].Path; got != "homelab/nas.md" {
		t.Errorf("note path = %q, want homelab/nas.md", got)
	}
	if got := byID[ids["task"]].Path; got != "" {
		t.Errorf("task carries a path %q; only notes are file-backed", got)
	}
}

func TestTagNodesFilters(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)
	ids := seedTaggedNodes(t, a)

	// ?type= narrows to one type.
	got := getTagNodes(t, r, "?tag=homelab&type=task")
	if len(got) != 1 || got[0].ID != ids["task"] {
		t.Fatalf("?type=task = %+v, want only the task", got)
	}

	// A child tag does not answer for its parent's nodes.
	got = getTagNodes(t, r, "?tag=homelab/nas")
	if len(got) != 2 {
		t.Fatalf("homelab/nas = %d nodes, want the note and the bookmark: %+v", len(got), got)
	}

	// ?descendants=false is the exact tag only.
	got = getTagNodes(t, r, "?tag=homelab&descendants=false")
	if len(got) != 1 || got[0].ID != ids["task"] {
		t.Fatalf("descendants=false = %+v, want only the directly-tagged task", got)
	}

	// Tag names are normalised on the way in, as they were on the way out.
	if got := getTagNodes(t, r, "?tag=Homelab"); len(got) != 3 {
		t.Errorf("?tag=Homelab = %d nodes, want the same 3 as ?tag=homelab", len(got))
	}
}

// An unknown tag is an empty subject, not a missing page.
func TestTagNodesUnknownTagIsEmptyNot404(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)
	seedTaggedNodes(t, a)

	if got := getTagNodes(t, r, "?tag=nothing-here"); len(got) != 0 {
		t.Errorf("unknown tag = %+v, want []", got)
	}

	// A name too malformed to normalise is a client error, though.
	for _, query := range []string{"", "?tag=", "?tag=foo//bar", "?tag=/foo"} {
		req := httptest.NewRequest("GET", "/api/tags/nodes"+query, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("GET /api/tags/nodes%s: expected 400, got %d", query, w.Code)
		}
	}
}

func TestListTagsHidesSystemTagsAndNests(t *testing.T) {
	a, _ := newCompanionTestApp(t)
	r := NewRouter(a)
	seedTaggedNodes(t, a)

	tree := getTagTree(t, r, "")
	if len(tree) != 1 || tree[0].Name != "homelab" {
		t.Fatalf("tree roots = %+v, want a single homelab root (sys/* hidden)", tree)
	}

	root := tree[0]
	// The count is subtree-inclusive and counts nodes once: 1 task on homelab,
	// plus the note and the bookmark on homelab/nas.
	if root.Count != 3 {
		t.Errorf("homelab count = %d, want 3 (its own node plus its descendants')", root.Count)
	}
	wantByType := map[string]int{"note": 1, "task": 1, "bookmark": 1}
	for typ, want := range wantByType {
		if root.ByType[typ] != want {
			t.Errorf("homelab byType[%s] = %d, want %d (full: %v)", typ, root.ByType[typ], want, root.ByType)
		}
	}

	if len(root.Children) != 1 || root.Children[0].Name != "homelab/nas" {
		t.Fatalf("homelab children = %+v, want [homelab/nas]", root.Children)
	}
	child := root.Children[0]
	if child.Segment != "nas" {
		t.Errorf("child segment = %q, want nas (the level, not the full name)", child.Segment)
	}
	if child.Count != 2 {
		t.Errorf("homelab/nas count = %d, want 2", child.Count)
	}

	// System tags are bookkeeping, not subjects — visible only on request.
	withSystem := getTagTree(t, r, "?includeSystem=true")
	var sys *tagTreeDTO
	for i := range withSystem {
		if withSystem[i].Name == "sys" {
			sys = &withSystem[i]
		}
	}
	if sys == nil {
		t.Fatalf("?includeSystem=true = %+v, want a sys branch", withSystem)
	}
	if len(sys.Children) != 1 || sys.Children[0].Name != tags.Pending {
		t.Errorf("sys children = %+v, want [%s]", sys.Children, tags.Pending)
	}
}
