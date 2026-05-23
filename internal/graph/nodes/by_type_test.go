package nodes

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/internal/ids"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func TestByTypeHappyPath(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if _, err := tx.Exec(`INSERT INTO nodes(id, type, title) VALUES (?, 'note', ?)`, ids.New(), "n1"); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO nodes(id, type, title) VALUES (?, 'note', ?)`, ids.New(), "n2"); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO nodes(id, type, title) VALUES (?, 'note', ?)`, ids.New(), "n3"); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO nodes(id, type, title) VALUES (?, 'bead', ?)`, ids.New(), "b1"); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO nodes(id, type, title) VALUES (?, 'bead', ?)`, ids.New(), "b2"); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	var got []Meta
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = ByType(ctx, tx, "note")
		return err
	})
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 notes, got %d", len(got))
	}
}

func TestByTypeTagFilterAND(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	var noteA, noteB string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		noteA = ids.New()
		if _, err := tx.Exec(`INSERT INTO nodes(id, type, title) VALUES (?, 'note', 'a')`, noteA); err != nil {
			return err
		}
		noteB = ids.New()
		if _, err := tx.Exec(`INSERT INTO nodes(id, type, title) VALUES (?, 'note', 'b')`, noteB); err != nil {
			return err
		}

		// Insert tag rows and assignments directly (avoid import cycle with tags package)
		for _, tag := range []string{"x", "y"} {
			_, err := tx.Exec(`INSERT OR IGNORE INTO tags(id, name) VALUES (?, ?)`, tag, tag)
			if err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`INSERT INTO node_tags(node_id, tag_id) SELECT ?, id FROM tags WHERE name = ?`, noteA, "x"); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO node_tags(node_id, tag_id) SELECT ?, id FROM tags WHERE name = ?`, noteA, "y"); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO node_tags(node_id, tag_id) SELECT ?, id FROM tags WHERE name = ?`, noteB, "x"); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	var got []Meta
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = ByType(ctx, tx, "note", WithTags("x", "y"))
		return err
	})
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}
	if len(got) != 1 || got[0].ID != noteA {
		t.Errorf("expected 1 result (noteA), got %v", got)
	}
}

func TestByTypeLimit(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		for i := 0; i < 3; i++ {
			if _, err := tx.Exec(`INSERT INTO nodes(id, type, title) VALUES (?, 'note', ?)`, ids.New(), "n"); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	var got []Meta
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = ByType(ctx, tx, "note", WithLimit(2))
		return err
	})
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 with limit, got %d", len(got))
	}
}

func TestByTypeEmptyResult(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		got, err := ByType(ctx, tx, "nonexistent_type")
		if err != nil {
			return err
		}
		if len(got) != 0 {
			t.Errorf("expected empty, got %d", len(got))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}
}

func TestByTypeDeterministicOrder(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	// Create two notes with identical updated_at (default is now, so insert same time)
	var idA, idB string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		idA = ids.New()
		if _, err := tx.Exec(`INSERT INTO nodes(id, type, title, updated_at) VALUES (?, 'note', 'a', '2025-01-01T00:00:00Z')`, idA); err != nil {
			return err
		}
		idB = ids.New()
		if _, err := tx.Exec(`INSERT INTO nodes(id, type, title, updated_at) VALUES (?, 'note', 'b', '2025-01-01T00:00:00Z')`, idB); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Run twice and verify identical ordering
	var first, second []Meta
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		first, err = ByType(ctx, tx, "note")
		return err
	})
	if err != nil {
		t.Fatalf("ByType first: %v", err)
	}
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		second, err = ByType(ctx, tx, "note")
		return err
	})
	if err != nil {
		t.Fatalf("ByType second: %v", err)
	}

	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("expected 2 results, got %d / %d", len(first), len(second))
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Errorf("non-deterministic order at %d: %s vs %s", i, first[i].ID, second[i].ID)
		}
	}
	// Tie-break should be id ASC
	if first[0].ID > first[1].ID {
		t.Errorf("expected id ASC tie-break, got %s then %s", first[0].ID, first[1].ID)
	}
}
