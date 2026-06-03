package planning_test

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/gabrielassisxyz/kernl/internal/planning"
)

func seedNote(t *testing.T, g *graph.Graph, title, body string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateNote(ctx, tx, nodes.Note{Title: title, Body: body}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed %q: %v", title, err)
	}
	return id
}

// TestBuildContext_TopicalRetrieval verifies a free-text planning seed surfaces
// content-matching notes (the topical signal structural relevance cannot give).
func TestBuildContext_TopicalRetrieval(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	seedNote(t, g, "Caching strategy", "We use an LRU cache with a write-through policy for hot keys.")
	seedNote(t, g, "Auth design", "OAuth device-code flow for the CLI and PKCE for the web app.")
	seedNote(t, g, "Cache invalidation", "Invalidate cache entries on write; TTL fallback of 60s.")

	notes, err := planning.BuildContext(ctx, g, "how should we design caching and cache invalidation", 8)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if len(notes) < 2 {
		t.Fatalf("expected at least 2 cache-related notes, got %d: %+v", len(notes), notes)
	}

	titles := map[string]bool{}
	for _, n := range notes {
		titles[n.Title] = true
		if n.Via != "content" {
			t.Errorf("expected via=content for text seed, got %q", n.Via)
		}
		if n.Snippet == "" {
			t.Errorf("expected a snippet for %q", n.Title)
		}
	}
	if !titles["Caching strategy"] || !titles["Cache invalidation"] {
		t.Errorf("expected the two caching notes surfaced, got titles %v", titles)
	}
	// Ranking is the contract: the cache notes (multiple matching terms) must
	// outrank a note that only matched a generic shared term like "design".
	if notes[0].Title != "Caching strategy" && notes[0].Title != "Cache invalidation" {
		t.Errorf("expected a caching note ranked first, got %q (full: %+v)", notes[0].Title, notes)
	}
}

func TestBuildContext_EmptySeed(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)
	notes, err := planning.BuildContext(ctx, g, "   ", 8)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if len(notes) != 0 {
		t.Errorf("empty seed should return no notes, got %d", len(notes))
	}
}
