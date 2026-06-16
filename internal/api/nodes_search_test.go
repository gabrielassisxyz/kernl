package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
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
