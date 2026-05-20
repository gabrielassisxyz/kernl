package nodes

import (
	"context"
	"fmt"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// TestMemoryRefutationRoundtrip verifies CreateMemoryRefutation → GetMemoryRefutation returns identical fields.
func TestMemoryRefutationRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	mr := MemoryRefutation{
		Title:      "Refute Gravity",
		ClaimID:    "claim-1",
		Reason:     "The claim ignores relativistic effects.",
		Confidence: 0.85,
		Tags:       []string{"physics", "disputed"},
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateMemoryRefutation(ctx, tx, mr, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateMemoryRefutation: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	var got *MemoryRefutation
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = GetMemoryRefutation(ctx, tx, id)
		return err
	})
	if err != nil {
		t.Fatalf("GetMemoryRefutation: %v", err)
	}

	if got.ID != id {
		t.Errorf("id = %q, want %q", got.ID, id)
	}
	if got.Title != mr.Title {
		t.Errorf("title = %q, want %q", got.Title, mr.Title)
	}
	if got.ClaimID != mr.ClaimID {
		t.Errorf("claim_id = %q, want %q", got.ClaimID, mr.ClaimID)
	}
	if got.Reason != mr.Reason {
		t.Errorf("reason = %q, want %q", got.Reason, mr.Reason)
	}
	if got.Confidence != mr.Confidence {
		t.Errorf("confidence = %f, want %f", got.Confidence, mr.Confidence)
	}
	if len(got.Tags) != len(mr.Tags) {
		t.Errorf("tags len = %d, want %d", len(got.Tags), len(mr.Tags))
	}
}

// TestMemoryRefutationUpdateProducesOneRevision verifies updating writes a second revision.
func TestMemoryRefutationUpdateProducesOneRevision(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	mr := MemoryRefutation{
		Title:      "Original",
		ClaimID:    "claim-1",
		Reason:     "before",
		Confidence: 0.5,
		Tags:       []string{"alpha"},
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateMemoryRefutation(ctx, tx, mr, Author{Name: "creator"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateMemoryRefutation: %v", err)
	}

	updated := MemoryRefutation{
		ID:         id,
		Title:      "Updated",
		ClaimID:    "claim-1",
		Reason:     "after",
		Confidence: 0.9,
		Tags:       []string{"beta"},
	}
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateMemoryRefutation(ctx, tx, updated, Author{Name: "updater"})
	})
	if err != nil {
		t.Fatalf("UpdateMemoryRefutation: %v", err)
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

// TestMemoryRefutationDeletePreservesRevisions verifies 3 revision rows survive after C+U+D.
func TestMemoryRefutationDeletePreservesRevisions(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateMemoryRefutation(ctx, tx, MemoryRefutation{Title: "Del", ClaimID: "c1", Reason: "d", Confidence: 0.5}, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateMemoryRefutation: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateMemoryRefutation(ctx, tx, MemoryRefutation{ID: id, Title: "Del2"}, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("UpdateMemoryRefutation: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return DeleteMemoryRefutation(ctx, tx, id, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("DeleteMemoryRefutation: %v", err)
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

// TestMemoryRefutationFTSRoundtrip verifies the refutation reason is indexed by FTS.
func TestMemoryRefutationFTSRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	mr := MemoryRefutation{
		Title:      "FTS Searchable",
		ClaimID:    "claim-fts",
		Reason:     "Contains unique token mrffts222.",
		Confidence: 0.9,
	}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := CreateMemoryRefutation(ctx, tx, mr, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateMemoryRefutation: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		if err := tx.QueryRow(
			"SELECT COUNT(*) FROM nodes_fts WHERE nodes_fts MATCH 'mrffts222'",
		).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("expected FTS to find 'mrffts222' once, got %d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestMemoryRefutationListFilter verifies filtering by ClaimID returns correct subsets.
func TestMemoryRefutationListFilter(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if _, err := CreateMemoryRefutation(ctx, tx, MemoryRefutation{
			Title: "Ref1", ClaimID: "claim-1", Reason: "r1", Confidence: 0.5,
		}, Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := CreateMemoryRefutation(ctx, tx, MemoryRefutation{
			Title: "Ref2", ClaimID: "claim-1", Reason: "r2", Confidence: 0.5,
		}, Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := CreateMemoryRefutation(ctx, tx, MemoryRefutation{
			Title: "Ref3", ClaimID: "claim-2", Reason: "r3", Confidence: 0.5,
		}, Author{Name: "test"}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("CreateMemoryRefutation: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		items, err := ListMemoryRefutations(ctx, tx, MemoryRefutationFilter{ClaimID: "claim-1"})
		if err != nil {
			return err
		}
		if len(items) != 2 {
			t.Errorf("ClaimID='claim-1': expected 2, got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		items, err := ListMemoryRefutations(ctx, tx, MemoryRefutationFilter{ClaimID: "claim-2"})
		if err != nil {
			return err
		}
		if len(items) != 1 {
			t.Errorf("ClaimID='claim-2': expected 1, got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
