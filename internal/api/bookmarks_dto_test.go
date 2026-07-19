package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// seedBookmarkWithHighlight wires an app + mux over an in-memory graph holding a
// single bookmark with one highlight, and returns the bookmark's id. Bookmarks
// are seeded through the graph rather than POST /api/bookmarks because the
// create handler archives in a background goroutine, which would reach the
// network.
func seedBookmarkWithHighlight(t *testing.T, at time.Time) (*http.ServeMux, string) {
	t.Helper()
	g := testutil.NewInMemoryTestGraph(t)
	a := &app.App{
		Config: &config.Config{Vault: config.VaultConfig{Root: t.TempDir()}},
		Graph:  g,
	}
	mux := http.NewServeMux()
	RegisterBookmarkRoutes(mux, a)

	var id string
	if err := g.DoWrite(t.Context(), func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateBookmark(t.Context(), tx, nodes.Bookmark{
			Title:      "Test Bookmark",
			URL:        "https://example.com",
			Highlights: []nodes.Highlight{{Text: "passage", Note: "note", CreatedAt: at}},
		}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed bookmark: %v", err)
	}
	return mux, id
}

// TestBookmarkHighlightsJSONContract pins the nested highlight objects to
// camelCase. The outer Bookmark fields were already covered; the nested
// Highlight struct keeps snake_case tags on purpose because those tags are the
// *storage* format (NodeAttrs marshals []Highlight straight into attrs), so the
// only thing that can make the wire camelCase is the DTO.
func TestBookmarkHighlightsJSONContract(t *testing.T) {
	mux, _ := seedBookmarkWithHighlight(t, time.Now())

	req := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var items []struct {
		Highlights []map[string]json.RawMessage `json:"highlights"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 1 || len(items[0].Highlights) != 1 {
		t.Fatalf("expected 1 bookmark with 1 highlight, got %+v", items)
	}

	assertJSONKeys(t, items[0].Highlights[0],
		[]string{"text", "note", "createdAt"},
		[]string{"created_at", "CreatedAt", "Text", "Note"},
	)
}

// TestAddHighlightJSONContract covers the write side end to end: the created
// highlight comes back camelCase, and a follow-up read shows it persisted.
func TestAddHighlightJSONContract(t *testing.T) {
	mux, id := seedBookmarkWithHighlight(t, time.Now())

	body := bytes.NewBufferString(`{"text":"a new passage","note":"a new note"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/bookmarks/"+id+"/highlights", body)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	var created map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created highlight: %v", err)
	}
	assertJSONKeys(t, created,
		[]string{"text", "note", "createdAt"},
		[]string{"created_at", "CreatedAt"},
	)

	req = httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var items []struct {
		Highlights []struct {
			Text string `json:"text"`
		} `json:"highlights"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 1 || len(items[0].Highlights) != 2 {
		t.Fatalf("expected the appended highlight to persist, got %+v", items)
	}
	if items[0].Highlights[1].Text != "a new passage" {
		t.Errorf("appended highlight not returned: %+v", items[0].Highlights[1])
	}
}
