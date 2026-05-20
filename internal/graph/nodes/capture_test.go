package nodes

import (
	"context"
	"fmt"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// TestCaptureRoundtrip verifies CreateCapture → GetCapture returns identical fields.
func TestCaptureRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	c := Capture{
		Title:        "Captured Page",
		Body:         "Interesting content here.",
		CapturedFrom: "web/example.com/page",
		Tags:         []string{"research", "web"},
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateCapture(ctx, tx, c, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	var got *Capture
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = GetCapture(ctx, tx, id)
		return err
	})
	if err != nil {
		t.Fatalf("GetCapture: %v", err)
	}

	if got.ID != id {
		t.Errorf("id = %q, want %q", got.ID, id)
	}
	if got.Title != c.Title {
		t.Errorf("title = %q, want %q", got.Title, c.Title)
	}
	if got.Body != c.Body {
		t.Errorf("body = %q, want %q", got.Body, c.Body)
	}
	if got.CapturedFrom != c.CapturedFrom {
		t.Errorf("captured_from = %q, want %q", got.CapturedFrom, c.CapturedFrom)
	}
	if len(got.Tags) != len(c.Tags) {
		t.Errorf("tags len = %d, want %d", len(got.Tags), len(c.Tags))
	}
}

// TestCaptureUpdateProducesOneRevision verifies updating writes a second revision.
func TestCaptureUpdateProducesOneRevision(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	c := Capture{
		Title:        "Original",
		Body:         "before",
		CapturedFrom: "web/example.com",
		Tags:         []string{"alpha"},
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateCapture(ctx, tx, c, Author{Name: "creator"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}

	updated := Capture{
		ID:           id,
		Title:        "Updated",
		Body:         "after",
		CapturedFrom: "web/example.com",
		Tags:         []string{"beta"},
	}
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateCapture(ctx, tx, updated, Author{Name: "updater"})
	})
	if err != nil {
		t.Fatalf("UpdateCapture: %v", err)
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

// TestCaptureDeletePreservesRevisions verifies 3 revision rows survive after C+U+D.
func TestCaptureDeletePreservesRevisions(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateCapture(ctx, tx, Capture{Title: "Del", Body: "d", CapturedFrom: "web/d"}, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateCapture(ctx, tx, Capture{ID: id, Title: "Del2"}, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("UpdateCapture: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return DeleteCapture(ctx, tx, id, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("DeleteCapture: %v", err)
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

// TestCaptureFTSRoundtrip verifies the capture body is indexed by FTS.
func TestCaptureFTSRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	c := Capture{
		Title:        "FTS Searchable",
		Body:         "Contains unique token capfts789",
		CapturedFrom: "web/example.com",
	}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := CreateCapture(ctx, tx, c, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		if err := tx.QueryRow(
			"SELECT COUNT(*) FROM nodes_fts WHERE nodes_fts MATCH 'capfts789'",
		).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("expected FTS to find 'capfts789' once, got %d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestCaptureListFilter verifies filtering by CapturedFromPrefix returns correct subsets.
func TestCaptureListFilter(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if _, err := CreateCapture(ctx, tx, Capture{Title: "WebA", Body: "wa", CapturedFrom: "web/a"}, Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := CreateCapture(ctx, tx, Capture{Title: "WebB", Body: "wb", CapturedFrom: "web/b"}, Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := CreateCapture(ctx, tx, Capture{Title: "DocX", Body: "dx", CapturedFrom: "doc/x"}, Author{Name: "test"}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		items, err := ListCaptures(ctx, tx, CaptureFilter{CapturedFromPrefix: "web/"})
		if err != nil {
			return err
		}
		if len(items) != 2 {
			t.Errorf("CapturedFromPrefix='web/': expected 2, got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		items, err := ListCaptures(ctx, tx, CaptureFilter{CapturedFromPrefix: "doc/"})
		if err != nil {
			return err
		}
		if len(items) != 1 {
			t.Errorf("CapturedFromPrefix='doc/': expected 1, got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
