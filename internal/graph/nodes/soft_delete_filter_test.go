package nodes

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// Captures and bookmarks are only ever hard-deleted today, so nothing sets
// deleted_at on them — but their listers were the only ones not filtering it,
// which means the day anything tombstones one it silently reappears in the
// inbox (and undo looks broken). These tests tombstone a row directly and pin
// the filter, matching every other node lister.
func TestListCapturesSkipsTombstonedRows(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	var liveID, deadID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		liveID, err = CreateCapture(ctx, tx, Capture{Body: "still pending", Tags: []string{"pending"}}, Author{Name: "test"})
		if err != nil {
			return err
		}
		deadID, err = CreateCapture(ctx, tx, Capture{Body: "tombstoned", Tags: []string{"pending"}}, Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tombstone(t, g, deadID)

	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		caps, err := ListCaptures(ctx, tx, CaptureFilter{Tags: []string{"pending"}})
		if err != nil {
			return err
		}
		if len(caps) != 1 || caps[0].ID != liveID {
			t.Errorf("ListCaptures returned %d captures, want only the live one", len(caps))
		}
		if _, err := GetCapture(ctx, tx, deadID); err != graph.ErrNotFound {
			t.Errorf("GetCapture(tombstoned) = %v, want ErrNotFound", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
	}
}

func TestListBookmarksSkipsTombstonedRows(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	var liveID, deadID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		liveID, err = CreateBookmark(ctx, tx, Bookmark{URL: "https://live.example", Title: "Live"}, Author{Name: "test"})
		if err != nil {
			return err
		}
		deadID, err = CreateBookmark(ctx, tx, Bookmark{URL: "https://dead.example", Title: "Dead"}, Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	tombstone(t, g, deadID)

	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		marks, err := ListBookmarks(ctx, tx, BookmarkFilter{})
		if err != nil {
			return err
		}
		if len(marks) != 1 || marks[0].ID != liveID {
			t.Errorf("ListBookmarks returned %d bookmarks, want only the live one", len(marks))
		}
		if _, err := GetBookmark(ctx, tx, deadID); err != graph.ErrNotFound {
			t.Errorf("GetBookmark(tombstoned) = %v, want ErrNotFound", err)
		}
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
	}
}

func tombstone(t *testing.T, g *graph.Graph, id string) {
	t.Helper()
	if err := g.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`UPDATE nodes SET deleted_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`, id)
		return err
	}); err != nil {
		t.Fatalf("tombstone %s: %v", id, err)
	}
}
