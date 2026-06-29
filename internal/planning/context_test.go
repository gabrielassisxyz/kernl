package planning_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/gabrielassisxyz/kernl/internal/planning"
)

func seedNote(t *testing.T, g *graph.Graph, title, body string) string {
	t.Helper()
	return seedTaggedNote(t, g, title, body, nil)
}

func seedTaggedNote(t *testing.T, g *graph.Graph, title, body string, tags []string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateNote(ctx, tx, nodes.Note{Title: title, Body: body, Tags: tags}, nodes.Author{Name: "test"})
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

// TestLoadTelos_InjectsTaggedNotes verifies telos-tagged note content is folded
// into a single always-on context block, while untagged notes are left out.
func TestLoadTelos_InjectsTaggedNotes(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	seedTaggedNote(t, g, "Who I am", "I am a solo builder optimizing for leverage.", []string{"telos"})
	seedTaggedNote(t, g, "My goals", "Ship the magic loop end-to-end this quarter.", []string{"telos", "da"})
	seedNote(t, g, "Caching strategy", "We use an LRU cache.") // untagged — must not appear

	block, err := planning.LoadTelos(ctx, g)
	if err != nil {
		t.Fatalf("LoadTelos: %v", err)
	}
	if block == "" {
		t.Fatal("expected a non-empty Telos block")
	}
	for _, want := range []string{"solo builder optimizing for leverage", "Ship the magic loop end-to-end", "Who I am", "My goals"} {
		if !strings.Contains(block, want) {
			t.Errorf("Telos block missing %q\nblock:\n%s", want, block)
		}
	}
	if strings.Contains(block, "LRU cache") {
		t.Errorf("untagged note leaked into Telos block:\n%s", block)
	}
}

// TestLoadTelos_NoneIsEmpty verifies the absence of telos notes yields an empty
// string (no header noise), so callers can inject unconditionally.
func TestLoadTelos_NoneIsEmpty(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	seedNote(t, g, "Caching strategy", "We use an LRU cache.")

	block, err := planning.LoadTelos(ctx, g)
	if err != nil {
		t.Fatalf("LoadTelos: %v", err)
	}
	if block != "" {
		t.Errorf("expected empty Telos block with no telos notes, got:\n%s", block)
	}
}

// TestLoadTelos_SizeCapped verifies a large Telos body is truncated so it cannot
// crowd out the conversation.
func TestLoadTelos_SizeCapped(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	seedTaggedNote(t, g, "Manifesto", strings.Repeat("identity ", 5000), []string{"telos"})

	block, err := planning.LoadTelos(ctx, g)
	if err != nil {
		t.Fatalf("LoadTelos: %v", err)
	}
	if len(block) > 4100 { // maxTelosBytes (4000) + the trailing "\n…" marker
		t.Errorf("Telos block not capped: %d bytes", len(block))
	}
	if !strings.HasSuffix(block, "…") {
		t.Errorf("expected truncation marker on capped block")
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
