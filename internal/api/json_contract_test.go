package api

import (
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

// assertJSONKeys guards the REST camelCase contract: a node struct reaching the
// wire without json tags silently exports Go field names, which no frontend
// contract can rely on. Asserting on the raw keys is the only check that
// survives — decoding into the struct passes either way.
func assertJSONKeys(t *testing.T, obj map[string]json.RawMessage, want, reject []string) {
	t.Helper()
	for _, k := range want {
		if _, ok := obj[k]; !ok {
			t.Errorf("missing camelCase key %q; got keys %v", k, keysOf(obj))
		}
	}
	for _, k := range reject {
		if _, ok := obj[k]; ok {
			t.Errorf("PascalCase key %q leaked into the wire format", k)
		}
	}
}

func keysOf(obj map[string]json.RawMessage) []string {
	out := make([]string, 0, len(obj))
	for k := range obj {
		out = append(out, k)
	}
	return out
}

func TestMemoryClaimsJSONContract(t *testing.T) {
	_, mux := setupMemoryTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/memory/claims?topic=go-programming", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var res struct {
		Claims []map[string]json.RawMessage `json:"claims"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(res.Claims) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(res.Claims))
	}

	assertJSONKeys(t, res.Claims[0],
		[]string{"id", "createdAt", "updatedAt", "title", "statement", "confidence", "subject", "source", "tags"},
		[]string{"ID", "CreatedAt", "UpdatedAt", "Title", "Statement", "Confidence", "Subject", "Source", "Tags"},
	)
}

func TestIngestQueueJSONContract(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	tempDir := t.TempDir()

	a := &app.App{
		Config: &config.Config{Vault: config.VaultConfig{Root: tempDir}},
		Graph:  g,
	}
	mux := http.NewServeMux()
	RegisterIngestRoutes(mux, a)

	req := httptest.NewRequest(http.MethodGet, "/api/ingest/queue", nil)
	if err := g.DoWrite(req.Context(), func(tx *graph.WriteTx) error {
		_, err := nodes.CreateIngestReview(req.Context(), tx, nodes.IngestReview{
			Title:        "Test Review",
			SourceNodeID: "n1",
			Action:       "Create Page",
			Payload:      "body",
			ContentHash:  "deadbeef",
		}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed ingest review: %v", err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var items []map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 queue item, got %d", len(items))
	}

	assertJSONKeys(t, items[0],
		[]string{"id", "createdAt", "updatedAt", "title", "sourceNodeId", "action", "payload", "contentHash", "tags"},
		[]string{"ID", "CreatedAt", "UpdatedAt", "Title", "SourceNodeID", "Action", "Payload", "ContentHash", "Tags"},
	)
}

func TestBookmarksJSONContract(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	tempDir := t.TempDir()

	a := &app.App{
		Config: &config.Config{Vault: config.VaultConfig{Root: tempDir}},
		Graph:  g,
	}
	mux := http.NewServeMux()
	RegisterBookmarkRoutes(mux, a)

	// Seeded through the graph rather than POST /api/bookmarks: the create
	// handler archives in a background goroutine, which would reach the network.
	req := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
	archivedAt := time.Now()
	if err := g.DoWrite(req.Context(), func(tx *graph.WriteTx) error {
		_, err := nodes.CreateBookmark(req.Context(), tx, nodes.Bookmark{
			Title:       "Test Bookmark",
			URL:         "https://example.com",
			Description: "a description",
			ArchivedAt:  &archivedAt,
			Excerpt:     "an excerpt",
			Highlights:  []nodes.Highlight{{Text: "passage", Note: "note", CreatedAt: archivedAt}},
		}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed bookmark: %v", err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var items []map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(items))
	}

	assertJSONKeys(t, items[0],
		[]string{"id", "createdAt", "updatedAt", "title", "url", "description", "archivedAt", "excerpt", "tags", "highlights"},
		[]string{"ID", "CreatedAt", "UpdatedAt", "Title", "URL", "Description", "ArchivedAt", "Excerpt", "Tags", "Highlights"},
	)
}
