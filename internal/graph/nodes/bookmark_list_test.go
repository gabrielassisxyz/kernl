package nodes

import (
	"context"
	"fmt"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// TestBookmarkListRoundtrip verifies CreateBookmarkList → GetBookmarkList returns identical fields.
func TestBookmarkListRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	bl := BookmarkList{
		Title:       "Reading List",
		Description: "Books to read in 2025",
		Tags:        []string{"reading", "books"},
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateBookmarkList(ctx, tx, bl, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateBookmarkList: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	var got *BookmarkList
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = GetBookmarkList(ctx, tx, id)
		return err
	})
	if err != nil {
		t.Fatalf("GetBookmarkList: %v", err)
	}

	if got.ID != id {
		t.Errorf("id = %q, want %q", got.ID, id)
	}
	if got.Title != bl.Title {
		t.Errorf("title = %q, want %q", got.Title, bl.Title)
	}
	if got.Description != bl.Description {
		t.Errorf("description = %q, want %q", got.Description, bl.Description)
	}
	if len(got.Tags) != len(bl.Tags) {
		t.Errorf("tags len = %d, want %d", len(got.Tags), len(bl.Tags))
	}
}

// TestBookmarkListUpdateProducesOneRevision verifies updating writes a second revision.
func TestBookmarkListUpdateProducesOneRevision(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	bl := BookmarkList{
		Title:       "Original",
		Description: "before",
		Tags:        []string{"alpha"},
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateBookmarkList(ctx, tx, bl, Author{Name: "creator"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateBookmarkList: %v", err)
	}

	updated := BookmarkList{
		ID:          id,
		Title:       "Updated",
		Description: "after",
		Tags:        []string{"beta"},
	}
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateBookmarkList(ctx, tx, updated, Author{Name: "updater"})
	})
	if err != nil {
		t.Fatalf("UpdateBookmarkList: %v", err)
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

// TestBookmarkListDeletePreservesRevisions verifies 3 revision rows survive after C+U+D.
func TestBookmarkListDeletePreservesRevisions(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateBookmarkList(ctx, tx, BookmarkList{Title: "Del", Description: "d"}, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateBookmarkList: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateBookmarkList(ctx, tx, BookmarkList{ID: id, Title: "Del2"}, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("UpdateBookmarkList: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return DeleteBookmarkList(ctx, tx, id, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("DeleteBookmarkList: %v", err)
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

// TestBookmarkListFTSRoundtrip verifies the bookmark list body is indexed by FTS.
func TestBookmarkListFTSRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	bl := BookmarkList{
		Title:       "FTS Searchable",
		Description: "Contains unique token bmlfts456",
	}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := CreateBookmarkList(ctx, tx, bl, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateBookmarkList: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		if err := tx.QueryRow(
			"SELECT COUNT(*) FROM nodes_fts WHERE nodes_fts MATCH 'bmlfts456'",
		).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("expected FTS to find 'bmlfts456' once, got %d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestBookmarkListListFilter verifies filtering by Tags and Limit returns correct subsets.
func TestBookmarkListListFilter(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if _, err := CreateBookmarkList(ctx, tx, BookmarkList{Title: "Dev", Description: "d1", Tags: []string{"dev", "coding"}}, Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := CreateBookmarkList(ctx, tx, BookmarkList{Title: "Design", Description: "d2", Tags: []string{"design", "ux"}}, Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := CreateBookmarkList(ctx, tx, BookmarkList{Title: "Dev2", Description: "d3", Tags: []string{"dev", "ops"}}, Author{Name: "test"}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("CreateBookmarkList: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		items, err := ListBookmarkLists(ctx, tx, BookmarkListFilter{Tags: []string{"dev"}})
		if err != nil {
			return err
		}
		if len(items) != 2 {
			t.Errorf("filter by dev tag: expected 2, got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		items, err := ListBookmarkLists(ctx, tx, BookmarkListFilter{Limit: 2})
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
