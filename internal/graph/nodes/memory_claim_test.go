package nodes

import (
	"context"
	"fmt"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// TestMemoryClaimRoundtrip verifies CreateMemoryClaim → GetMemoryClaim returns identical fields.
func TestMemoryClaimRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	mc := MemoryClaim{
		Title:      "Gravity Claim",
		Statement:  "Gravity accelerates objects at 9.8 m/s² on Earth.",
		Confidence: 0.95,
		Subject:    "physics",
		Source:     "textbook",
		Tags:       []string{"science", "verified"},
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateMemoryClaim(ctx, tx, mc, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateMemoryClaim: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	var got *MemoryClaim
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = GetMemoryClaim(ctx, tx, id)
		return err
	})
	if err != nil {
		t.Fatalf("GetMemoryClaim: %v", err)
	}

	if got.ID != id {
		t.Errorf("id = %q, want %q", got.ID, id)
	}
	if got.Title != mc.Title {
		t.Errorf("title = %q, want %q", got.Title, mc.Title)
	}
	if got.Statement != mc.Statement {
		t.Errorf("statement = %q, want %q", got.Statement, mc.Statement)
	}
	if got.Confidence != mc.Confidence {
		t.Errorf("confidence = %f, want %f", got.Confidence, mc.Confidence)
	}
	if got.Subject != mc.Subject {
		t.Errorf("subject = %q, want %q", got.Subject, mc.Subject)
	}
	if got.Source != mc.Source {
		t.Errorf("source = %q, want %q", got.Source, mc.Source)
	}
	if len(got.Tags) != len(mc.Tags) {
		t.Errorf("tags len = %d, want %d", len(got.Tags), len(mc.Tags))
	}
}

// TestMemoryClaimUpdateProducesOneRevision verifies updating writes a second revision.
func TestMemoryClaimUpdateProducesOneRevision(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	mc := MemoryClaim{
		Title:      "Original",
		Statement:  "before",
		Confidence: 0.5,
		Subject:    "test",
		Source:     "src",
		Tags:       []string{"alpha"},
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateMemoryClaim(ctx, tx, mc, Author{Name: "creator"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateMemoryClaim: %v", err)
	}

	updated := MemoryClaim{
		ID:         id,
		Title:      "Updated",
		Statement:  "after",
		Confidence: 0.9,
		Subject:    "test",
		Source:     "src",
		Tags:       []string{"beta"},
	}
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateMemoryClaim(ctx, tx, updated, Author{Name: "updater"})
	})
	if err != nil {
		t.Fatalf("UpdateMemoryClaim: %v", err)
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

// TestMemoryClaimDeletePreservesRevisions verifies 3 revision rows survive after C+U+D.
func TestMemoryClaimDeletePreservesRevisions(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateMemoryClaim(ctx, tx, MemoryClaim{Title: "Del", Statement: "d", Confidence: 0.5, Subject: "x", Source: "x"}, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateMemoryClaim: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateMemoryClaim(ctx, tx, MemoryClaim{ID: id, Title: "Del2"}, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("UpdateMemoryClaim: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return DeleteMemoryClaim(ctx, tx, id, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("DeleteMemoryClaim: %v", err)
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

// TestMemoryClaimFTSRoundtrip verifies the claim statement is indexed by FTS.
func TestMemoryClaimFTSRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	mc := MemoryClaim{
		Title:      "FTS Searchable",
		Statement:  "Contains unique token mclfts111.",
		Confidence: 0.9,
		Subject:    "fts",
		Source:     "test",
	}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := CreateMemoryClaim(ctx, tx, mc, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateMemoryClaim: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		if err := tx.QueryRow(
			"SELECT COUNT(*) FROM nodes_fts WHERE nodes_fts MATCH 'mclfts111'",
		).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("expected FTS to find 'mclfts111' once, got %d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestMemoryClaimListFilter verifies filtering by Subject and MinConfidence.
func TestMemoryClaimListFilter(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if _, err := CreateMemoryClaim(ctx, tx, MemoryClaim{
			Title: "Gravity", Statement: "st", Confidence: 0.8, Subject: "physics", Source: "src",
		}, Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := CreateMemoryClaim(ctx, tx, MemoryClaim{
			Title: "Relativity", Statement: "st", Confidence: 0.9, Subject: "physics", Source: "src",
		}, Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := CreateMemoryClaim(ctx, tx, MemoryClaim{
			Title: "Calculus", Statement: "st", Confidence: 0.95, Subject: "math", Source: "src",
		}, Author{Name: "test"}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("CreateMemoryClaim: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		items, err := ListMemoryClaims(ctx, tx, MemoryClaimFilter{Subject: "physics"})
		if err != nil {
			return err
		}
		if len(items) != 2 {
			t.Errorf("Subject='physics': expected 2, got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		items, err := ListMemoryClaims(ctx, tx, MemoryClaimFilter{MinConfidence: 0.9})
		if err != nil {
			return err
		}
		if len(items) != 2 {
			t.Errorf("MinConfidence=0.9: expected 2, got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
