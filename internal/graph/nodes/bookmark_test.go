package nodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// TestBookmarkRoundtrip verifies CreateBookmark → GetBookmark returns
// identical fields including tags.
func TestBookmarkRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	b := Bookmark{
		Title:       "Kernl Docs",
		URL:         "https://example.com/kernl",
		Description: "The official Kernl documentation",
		Excerpt:     "Build fast knowledge graphs",
		Tags:        []string{"docs", "rust", "graph"},
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateBookmark(ctx, tx, b, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateBookmark: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	var got *Bookmark
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = GetBookmark(ctx, tx, id)
		return err
	})
	if err != nil {
		t.Fatalf("GetBookmark: %v", err)
	}

	if got.ID != id {
		t.Errorf("id = %q, want %q", got.ID, id)
	}
	if got.Title != b.Title {
		t.Errorf("title = %q, want %q", got.Title, b.Title)
	}
	if got.URL != b.URL {
		t.Errorf("url = %q, want %q", got.URL, b.URL)
	}
	if got.Description != b.Description {
		t.Errorf("description = %q, want %q", got.Description, b.Description)
	}
	if got.Excerpt != b.Excerpt {
		t.Errorf("excerpt = %q, want %q", got.Excerpt, b.Excerpt)
	}
	if got.ArchivedAt != nil {
		t.Errorf("archived_at = %v, want nil", got.ArchivedAt)
	}
	if len(got.Tags) != len(b.Tags) {
		t.Errorf("tags len = %d, want %d", len(got.Tags), len(b.Tags))
	}
}

// TestBookmarkUpdateProducesOneRevision verifies updating writes a second revision.
func TestBookmarkUpdateProducesOneRevision(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	b := Bookmark{
		Title:       "Original",
		URL:         "https://example.com",
		Description: "before",
		Tags:        []string{"alpha"},
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateBookmark(ctx, tx, b, Author{Name: "creator"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateBookmark: %v", err)
	}

	updated := Bookmark{
		ID:          id,
		Title:       "Updated",
		URL:         "https://example.com",
		Description: "before",
		Tags:        []string{"beta"},
	}
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateBookmark(ctx, tx, updated, Author{Name: "updater"})
	})
	if err != nil {
		t.Fatalf("UpdateBookmark: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		if err := tx.QueryRow("SELECT COUNT(*) FROM revisions", id).Scan(&count); err != nil {
			return err
		}
		if count != 2 {
			t.Errorf("expected 2 revisions after update, got %d", count)
		}

		var author string
		if err := tx.QueryRow(
			"SELECT author FROM revisions WHERE node_id = ? ORDER BY created_at DESC, id DESC LIMIT 1",
			id,
		).Scan(&author); err != nil {
			return err
		}
		if author != "updater" {
			t.Errorf("latest author = %q, want %q", author, "updater")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestBookmarkDeletePreservesRevisions verifies 3 revision rows survive after C+U+D.
func TestBookmarkDeletePreservesRevisions(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateBookmark(ctx, tx, Bookmark{Title: "Del", URL: "https://d", Description: "d"}, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateBookmark: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateBookmark(ctx, tx, Bookmark{ID: id, Title: "Del2"}, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("UpdateBookmark: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return DeleteBookmark(ctx, tx, id, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("DeleteBookmark: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var revCount int
		if err := tx.QueryRow("SELECT COUNT(*) FROM revisions", id).Scan(&revCount); err != nil {
			return err
		}
		if revCount != 3 {
			return fmt.Errorf("expected 3 revisions, got %d", revCount)
		}
		var nodeCount int
		if err := tx.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = ?", id).Scan(&nodeCount); err != nil {
			return err
		}
		if nodeCount != 0 {
			return fmt.Errorf("expected node deleted, got %d", nodeCount)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestBookmarkFTSRoundtrip verifies the bookmark body is indexed by FTS.
func TestBookmarkFTSRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	b := Bookmark{
		Title:       "FTS Searchable",
		URL:         "https://example.com/fts",
		Description: "Contains unique token bmfts123",
	}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := CreateBookmark(ctx, tx, b, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateBookmark: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		if err := tx.QueryRow(
			"SELECT COUNT(*) FROM nodes_fts WHERE nodes_fts MATCH 'bmfts123'",
		).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			t.Errorf("expected FTS to find 'bmfts123', got 0 matches")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestBookmarkListFilterArchived verifies default excludes archived, include returns all.
func TestBookmarkListFilterArchived(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	archivedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if _, err := CreateBookmark(ctx, tx, Bookmark{Title: "Active1", URL: "https://a1", Description: "a1"}, Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := CreateBookmark(ctx, tx, Bookmark{Title: "Active2", URL: "https://a2", Description: "a2"}, Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := CreateBookmark(ctx, tx, Bookmark{Title: "Archived", URL: "https://a3", Description: "a3", ArchivedAt: &archivedAt}, Author{Name: "test"}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("CreateBookmark: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		items, err := ListBookmarks(ctx, tx, BookmarkFilter{})
		if err != nil {
			return err
		}
		if len(items) != 2 {
			t.Errorf("default filter: expected 2, got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		items, err := ListBookmarks(ctx, tx, BookmarkFilter{IncludeArchived: true})
		if err != nil {
			return err
		}
		if len(items) != 3 {
			t.Errorf("IncludeArchived: expected 3, got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		items, err := ListBookmarks(ctx, tx, BookmarkFilter{IncludeArchived: true, Limit: 2})
		if err != nil {
			return err
		}
		if len(items) != 2 {
			t.Errorf("Limit=2: expected 2, got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestBookmarkHighlightsRoundtrip verifies highlights persist through
// CreateBookmark → UpdateBookmark → GetBookmark.
func TestBookmarkHighlightsRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	var id string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateBookmark(ctx, tx, Bookmark{Title: "T", URL: "https://e.com"}, Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("CreateBookmark: %v", err)
	}

	// Read, append two highlights, write back (mirrors the API handler).
	var b *Bookmark
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		b, err = GetBookmark(ctx, tx, id)
		return err
	}); err != nil {
		t.Fatalf("GetBookmark: %v", err)
	}
	b.Highlights = append(b.Highlights,
		Highlight{Text: "first passage", Note: "important", CreatedAt: time.Now()},
		Highlight{Text: "second passage", CreatedAt: time.Now()},
	)
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateBookmark(ctx, tx, *b, Author{Name: "test"})
	}); err != nil {
		t.Fatalf("add highlights: %v", err)
	}

	var got *Bookmark
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = GetBookmark(ctx, tx, id)
		return err
	}); err != nil {
		t.Fatalf("GetBookmark: %v", err)
	}

	if len(got.Highlights) != 2 {
		t.Fatalf("expected 2 highlights, got %d", len(got.Highlights))
	}
	if got.Highlights[0].Text != "first passage" || got.Highlights[0].Note != "important" {
		t.Errorf("highlight[0] mismatch: %+v", got.Highlights[0])
	}
	if got.Highlights[1].Text != "second passage" || got.Highlights[1].Note != "" {
		t.Errorf("highlight[1] mismatch: %+v", got.Highlights[1])
	}
}

// TestBookmarkHighlightStorageFormat pins the persisted shape of a highlight.
//
// Highlight's json tags do double duty: NodeAttrs marshals []Highlight straight
// into the nodes.attrs column, so those tags ARE the storage format, not a wire
// format. Renaming created_at to createdAt to satisfy the REST camelCase
// contract would make every already-stored highlight read back with a zero
// timestamp — a silent data loss with no migration. The API converts through a
// DTO instead; this test is what makes that rename fail loudly.
func TestBookmarkHighlightStorageFormat(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	// A fixed instant, so an exact comparison after the round-trip is meaningful.
	at := time.Date(2026, 3, 14, 15, 9, 26, 0, time.UTC)

	var id string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateBookmark(ctx, tx, Bookmark{
			Title:      "T",
			URL:        "https://e.com",
			Highlights: []Highlight{{Text: "passage", Note: "note", CreatedAt: at}},
		}, Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("CreateBookmark: %v", err)
	}

	var rawAttrs sql.NullString
	var got *Bookmark
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		if err := tx.QueryRow(`SELECT attrs FROM nodes WHERE id = ?`, id).Scan(&rawAttrs); err != nil {
			return err
		}
		var err error
		got, err = GetBookmark(ctx, tx, id)
		return err
	}); err != nil {
		t.Fatalf("read back: %v", err)
	}

	var stored struct {
		Highlights []map[string]json.RawMessage `json:"highlights"`
	}
	if err := json.Unmarshal([]byte(rawAttrs.String), &stored); err != nil {
		t.Fatalf("unmarshal stored attrs: %v", err)
	}
	if len(stored.Highlights) != 1 {
		t.Fatalf("expected 1 stored highlight, got %d", len(stored.Highlights))
	}
	for _, key := range []string{"text", "note", "created_at"} {
		if _, ok := stored.Highlights[0][key]; !ok {
			t.Errorf("storage format changed: key %q missing from attrs %s", key, rawAttrs.String)
		}
	}
	if _, ok := stored.Highlights[0]["createdAt"]; ok {
		t.Errorf("camelCase createdAt leaked into the storage format: %s", rawAttrs.String)
	}

	if len(got.Highlights) != 1 {
		t.Fatalf("expected 1 highlight back, got %d", len(got.Highlights))
	}
	if !got.Highlights[0].CreatedAt.Equal(at) {
		t.Errorf("highlight timestamp did not survive storage: got %v, want %v", got.Highlights[0].CreatedAt, at)
	}
	if got.Highlights[0].Text != "passage" || got.Highlights[0].Note != "note" {
		t.Errorf("highlight fields did not survive storage: %+v", got.Highlights[0])
	}
}
