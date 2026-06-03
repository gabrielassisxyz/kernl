package ingest

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

func seedReview(t *testing.T, g *graph.Graph) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateIngestReview(ctx, tx, nodes.IngestReview{
			Title:   "Conflict found",
			Action:  "Contradiction Callout",
			Payload: "The note contradicts an earlier claim.",
		}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed review: %v", err)
	}
	return id
}

func openGraph(t *testing.T) *graph.Graph {
	t.Helper()
	g, err := graph.Open(context.Background(), graph.Config{Path: filepath.Join(t.TempDir(), "g.db")})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { g.Close() })
	return g
}

func reviewCount(t *testing.T, g *graph.Graph) int {
	t.Helper()
	var n int
	_ = g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		items, err := nodes.ListIngestReviews(context.Background(), tx, nodes.IngestReviewFilter{})
		n = len(items)
		return err
	})
	return n
}

func TestResolveReviewCreatePage(t *testing.T) {
	ctx := context.Background()
	g := openGraph(t)
	vault := t.TempDir()
	id := seedReview(t, g)

	if err := ResolveReview(ctx, g, vault, id, "Create Page"); err != nil {
		t.Fatalf("ResolveReview: %v", err)
	}

	// Review removed, a note created, and a .md written to the vault.
	if reviewCount(t, g) != 0 {
		t.Errorf("expected review removed")
	}
	var noteCount int
	_ = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE type='note'`).Scan(&noteCount)
	})
	if noteCount != 1 {
		t.Errorf("expected 1 note created, got %d", noteCount)
	}
}

func TestResolveReviewSkip(t *testing.T) {
	ctx := context.Background()
	g := openGraph(t)
	id := seedReview(t, g)
	if err := ResolveReview(ctx, g, "", id, "Skip"); err != nil {
		t.Fatalf("ResolveReview Skip: %v", err)
	}
	if reviewCount(t, g) != 0 {
		t.Errorf("expected review removed on Skip")
	}
}

func TestResolveReviewUnimplemented(t *testing.T) {
	ctx := context.Background()
	g := openGraph(t)
	id := seedReview(t, g)
	err := ResolveReview(ctx, g, "", id, "Deep Research")
	if !errors.Is(err, ErrActionNotImplemented) {
		t.Fatalf("expected ErrActionNotImplemented, got %v", err)
	}
	if reviewCount(t, g) != 1 {
		t.Errorf("unimplemented action should leave the review in the queue")
	}
}
