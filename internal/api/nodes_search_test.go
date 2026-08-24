package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/search"
)

// TestNodeSearchPrefixAndTypeFilter covers the autocomplete contract: prefix
// matching on title, the optional type filter, and that the node type is
// returned in each result.
func TestNodeSearchPrefixAndTypeFilter(t *testing.T) {
	a := newTestAppWithGraphWithLLM(t)
	ctx := context.Background()

	if err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if _, err := nodes.CreateNote(ctx, tx, nodes.Note{Title: "Linktree", Body: "a tree of links"}, nodes.Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := nodes.CreateNote(ctx, tx, nodes.Note{Title: "Roadmap", Body: "plans"}, nodes.Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := nodes.CreateBookmark(ctx, tx, nodes.Bookmark{URL: "https://example.com", Title: "Library docs"}, nodes.Author{Name: "test"}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	r := NewRouter(a)

	type result struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Type  string `json:"type"`
	}
	doSearch := func(q string) []result {
		req := httptest.NewRequest("GET", "/api/nodes/search?q="+q, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("q=%q: expected 200, got %d: %s", q, w.Code, w.Body.String())
		}
		var out []result
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("q=%q: decode: %v", q, err)
		}
		return out
	}

	// Prefix matching: "lin" must match "Linktree".
	hits := doSearch("lin")
	found := false
	for _, h := range hits {
		if h.Title == "Linktree" {
			found = true
			if h.Type != "note" {
				t.Errorf("expected type note for Linktree, got %q", h.Type)
			}
		}
	}
	if !found {
		t.Fatalf("prefix search 'lin' did not match Linktree, got %+v", hits)
	}

	// Type filter: searching "Lib" with type=note must NOT return the bookmark.
	req := httptest.NewRequest("GET", "/api/nodes/search?q=Lib&type=note", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("type filter: expected 200, got %d", w.Code)
	}
	var typed []result
	if err := json.Unmarshal(w.Body.Bytes(), &typed); err != nil {
		t.Fatalf("type filter decode: %v", err)
	}
	for _, h := range typed {
		if h.Type != "note" {
			t.Errorf("type=note filter leaked %q (%s)", h.Type, h.Title)
		}
	}

	// Without the type filter, the bookmark prefix match is visible.
	bm := doSearch("Lib")
	foundBookmark := false
	for _, h := range bm {
		if h.Type == "bookmark" && h.Title == "Library docs" {
			foundBookmark = true
		}
	}
	if !foundBookmark {
		t.Errorf("expected bookmark 'Library docs' in unfiltered search, got %+v", bm)
	}
}

// TestNodeSearchEmptyQueryReturnsEmptyArray verifies blank q yields [] not 500.
func TestNodeSearchEmptyQueryReturnsEmptyArray(t *testing.T) {
	a := newTestAppWithGraphWithLLM(t)
	r := NewRouter(a)

	req := httptest.NewRequest("GET", "/api/nodes/search?q=%20", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for blank query, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "[]\n" && got != "[]" {
		t.Errorf("expected empty array, got %q", got)
	}
}

// doNodeSearch issues a GET /api/nodes/search and decodes the response.
func doNodeSearch(t *testing.T, r http.Handler, q string) []nodeSearchResult {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/nodes/search?q="+q, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("q=%q: expected 200, got %d: %s", q, w.Code, w.Body.String())
	}
	var out []nodeSearchResult
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("q=%q: decode: %v", q, err)
	}
	return out
}

// TestNodeSearchTitleOnly proves the handler passes the title-only option: a
// term in the body only or the tag only is absent, while a term in the title
// is present. The body and tag fixtures must not carry the term in their
// titles, or the absence assertion would pass trivially.
func TestNodeSearchTitleOnly(t *testing.T) {
	a := newTestAppWithGraphWithLLM(t)
	ctx := context.Background()

	if err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if _, err := nodes.CreateNote(ctx, tx, nodes.Note{Title: "Unrelated", Body: "the resticprofile backup runs nightly"}, nodes.Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := nodes.CreateNote(ctx, tx, nodes.Note{Title: "Unrelated", Body: "unrelated body", Tags: []string{"resticprofile"}}, nodes.Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := nodes.CreateNote(ctx, tx, nodes.Note{Title: "resticprofile backup", Body: "unrelated"}, nodes.Author{Name: "test"}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	hits := doNodeSearch(t, NewRouter(a), "resticprofile")
	if len(hits) != 1 || hits[0].Title != "resticprofile backup" {
		t.Fatalf("expected only the title match, got %+v", hits)
	}
}

// TestCollapseCompanions proves a task and its companion note collapse to one
// entry while two unrelated nodes with similar titles stay two.
func TestCollapseCompanions(t *testing.T) {
	a := newTestAppWithGraphWithLLM(t)
	ctx := context.Background()

	var taskID, companionID, otherID string
	if err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		taskID, err = nodes.CreateTask(ctx, tx, nodes.Task{Title: "Report composer", Description: "writes reports", Status: nodes.TaskStatusInProgress}, nodes.Author{Name: "test"})
		if err != nil {
			return err
		}
		companionID, err = nodes.CreateNote(ctx, tx, nodes.Note{Title: "Report composer", Body: "notes"}, nodes.Author{Name: "test"})
		if err != nil {
			return err
		}
		if _, err := edges.Create(ctx, tx, edges.Edge{Src: companionID, Dst: taskID, Type: edges.EdgeTypeLinksTo}, nodes.Author{Name: "test"}); err != nil {
			return err
		}
		otherID, err = nodes.CreateNote(ctx, tx, nodes.Note{Title: "Report composer notes", Body: "unrelated"}, nodes.Author{Name: "test"})
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	hits := []search.Hit{
		{NodeID: taskID, Title: "Report composer"},
		{NodeID: companionID, Title: "Report composer"},
		{NodeID: otherID, Title: "Report composer notes"},
	}
	types := map[string]string{
		taskID:      "task",
		companionID: "note",
		otherID:     "note",
	}

	var out []nodeSearchResult
	if err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		out, err = collapseCompanions(tx, hits, types)
		return err
	}); err != nil {
		t.Fatalf("collapseCompanions: %v", err)
	}

	if len(out) != 2 {
		t.Fatalf("expected 2 entries (pair collapsed, unrelated kept), got %d: %+v", len(out), out)
	}
	ids := map[string]bool{}
	for _, r := range out {
		ids[r.ID] = true
	}
	if ids[taskID] && ids[companionID] {
		t.Errorf("task and companion both present, want one: %+v", out)
	}
	if !ids[taskID] && !ids[companionID] {
		t.Errorf("expected one of the task/companion pair, got %+v", out)
	}
	if !ids[otherID] {
		t.Errorf("unrelated note missing: %+v", out)
	}
}
