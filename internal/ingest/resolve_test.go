package ingest

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
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

	if err := ResolveReview(ctx, g, vault, id, "Create Page", nil); err != nil {
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
	if err := ResolveReview(ctx, g, "", id, "Skip", nil); err != nil {
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
	err := ResolveReview(ctx, g, "", id, "Deep Research", nil)
	if !errors.Is(err, ErrActionNotImplemented) {
		t.Fatalf("expected ErrActionNotImplemented, got %v", err)
	}
	if reviewCount(t, g) != 1 {
		t.Errorf("unimplemented action should leave the review in the queue")
	}
}

// seedReviewWith inserts a review with a custom payload and source node id.
func seedReviewWith(t *testing.T, g *graph.Graph, title, payload, sourceID string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateIngestReview(ctx, tx, nodes.IngestReview{
			Title:        title,
			Payload:      payload,
			SourceNodeID: sourceID,
		}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed review: %v", err)
	}
	return id
}

// createNote is a test helper that inserts a note and returns its id.
func createNote(t *testing.T, g *graph.Graph, title, body string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateNote(ctx, tx, nodes.Note{Title: title, Body: body}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("create note: %v", err)
	}
	return id
}

func edgeCount(t *testing.T, g *graph.Graph, src, dst string) int {
	t.Helper()
	var n int
	_ = g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM edges WHERE src = ? AND dst = ?`, src, dst).Scan(&n)
	})
	return n
}

func noteBody(t *testing.T, g *graph.Graph, id string) string {
	t.Helper()
	var n *nodes.Note
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var err error
		n, err = nodes.GetNote(context.Background(), tx, id)
		return err
	}); err != nil {
		t.Fatalf("get note: %v", err)
	}
	return n.Body
}

// Covers AE (Update): a confident target merges the accepted hunks into that
// note, links it to the source, and removes the review.
func TestResolveReviewUpdateMergesIntoTarget(t *testing.T) {
	ctx := context.Background()
	g := openGraph(t)

	sourceID := createNote(t, g, "Source", "origin material")
	targetID := createNote(t, g, "Quantum entanglement", "Entanglement links particle states.")
	reviewID := seedReviewWith(t, g, "Entanglement update",
		"Quantum entanglement enables teleportation protocols.", sourceID)

	update := &UpdateInput{
		TargetNoteID:  targetID,
		AcceptedHunks: []MergeHunk{{ID: "0", Content: "Entanglement enables teleportation protocols."}},
	}
	if err := ResolveReview(ctx, g, "", reviewID, "Update", update); err != nil {
		t.Fatalf("ResolveReview Update: %v", err)
	}

	body := noteBody(t, g, targetID)
	if !strings.Contains(body, "teleportation protocols") {
		t.Errorf("expected merged hunk in target body, got %q", body)
	}
	if reviewCount(t, g) != 0 {
		t.Errorf("expected review removed after Update")
	}
	if edgeCount(t, g, targetID, sourceID) != 1 {
		t.Errorf("expected an edge connecting the updated note to its source")
	}
}

// Update with no confident target falls back to Create Page.
func TestResolveReviewUpdateFallsBackToCreatePage(t *testing.T) {
	ctx := context.Background()
	g := openGraph(t)

	reviewID := seedReviewWith(t, g, "Brand new topic", "A wholly unrelated subject with no match.", "")

	if err := ResolveReview(ctx, g, "", reviewID, "Update", &UpdateInput{}); err != nil {
		t.Fatalf("ResolveReview Update: %v", err)
	}

	if reviewCount(t, g) != 0 {
		t.Errorf("expected review removed")
	}
	var noteCount int
	_ = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE type='note'`).Scan(&noteCount)
	})
	if noteCount != 1 {
		t.Errorf("expected fallback to create 1 note, got %d", noteCount)
	}
}

// Rejecting all merge hunks leaves the target note unchanged but still resolves
// the review and connects the note.
func TestResolveReviewUpdateRejectAllUnchanged(t *testing.T) {
	ctx := context.Background()
	g := openGraph(t)

	sourceID := createNote(t, g, "Source", "origin material")
	targetID := createNote(t, g, "Topic", "Original body text.")
	reviewID := seedReviewWith(t, g, "No-op update", "Original body text.", sourceID)

	update := &UpdateInput{TargetNoteID: targetID, AcceptedHunks: nil}
	if err := ResolveReview(ctx, g, "", reviewID, "Update", update); err != nil {
		t.Fatalf("ResolveReview Update: %v", err)
	}

	if got := noteBody(t, g, targetID); got != "Original body text." {
		t.Errorf("expected target unchanged, got %q", got)
	}
	if reviewCount(t, g) != 0 {
		t.Errorf("expected review removed even with no accepted hunks")
	}
}

// Create Page connects the new note to its source node — it is not orphaned.
func TestResolveReviewCreatePageConnectsSource(t *testing.T) {
	ctx := context.Background()
	g := openGraph(t)

	sourceID := createNote(t, g, "Source", "origin material")
	reviewID := seedReviewWith(t, g, "Fresh page", "Some ingested knowledge.", sourceID)

	if err := ResolveReview(ctx, g, "", reviewID, "Create Page", nil); err != nil {
		t.Fatalf("ResolveReview Create Page: %v", err)
	}

	var newNoteID string
	_ = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(
			`SELECT id FROM nodes WHERE type='note' AND id != ? AND title='Fresh page'`, sourceID,
		).Scan(&newNoteID)
	})
	if newNoteID == "" {
		t.Fatalf("created note not found")
	}
	if edgeCount(t, g, newNoteID, sourceID) != 1 {
		t.Errorf("expected an edge connecting the new note to its source")
	}
}
