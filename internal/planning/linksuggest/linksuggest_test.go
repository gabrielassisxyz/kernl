package linksuggest_test

import (
	"context"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/gabrielassisxyz/kernl/internal/planning/linksuggest"
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
		t.Fatalf("seed note %q: %v", title, err)
	}
	return id
}

func seedClaim(t *testing.T, g *graph.Graph, title, statement string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateMemoryClaim(ctx, tx, nodes.MemoryClaim{
			Title: title, Statement: statement, Confidence: 1.0,
		}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed claim %q: %v", title, err)
	}
	return id
}

func seedTask(t *testing.T, g *graph.Graph, title, description string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateTask(ctx, tx, nodes.Task{Title: title, Description: description, Status: nodes.TaskStatusInProgress}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed task %q: %v", title, err)
	}
	return id
}

// TestSuggestFiltersClaims verifies a memory claim matching the seed is not
// offered as a link candidate: a claim is not a note and has no file, so it can
// never be a link target.
func TestSuggestFiltersClaims(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	seedNote(t, g, "Caching strategy", "We use an LRU cache with a write-through policy for hot keys.")
	seedClaim(t, g, "Cache claim", "Caching is done with an LRU cache.")

	candidates, err := linksuggest.Suggest(ctx, g, "caching strategy", 8)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}

	var titles []string
	for _, c := range candidates {
		titles = append(titles, c.Title)
		if c.ID == "" {
			t.Errorf("candidate %q has no id", c.Title)
		}
		if c.Snippet == "" {
			t.Errorf("candidate %q has no snippet", c.Title)
		}
	}
	if !slices.Contains(titles, "Caching strategy") {
		t.Errorf("expected the note as a candidate, got %v", titles)
	}
	if slices.Contains(titles, "Cache claim") {
		t.Errorf("a claim is not a linkable note and must be filtered, got %v", titles)
	}
}

// TestSuggestFiltersTasksAndProjects verifies a task or project matching the
// seed is not offered as a link candidate: an entity is not a note and cannot
// be a wikilink target.
func TestSuggestFiltersTasksAndProjects(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	seedNote(t, g, "Caching strategy", "We use an LRU cache with a write-through policy for hot keys.")
	seedTask(t, g, "Cache task", "Caching is done with an LRU cache.")

	candidates, err := linksuggest.Suggest(ctx, g, "caching strategy", 8)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}

	var titles []string
	for _, c := range candidates {
		titles = append(titles, c.Title)
	}
	if !slices.Contains(titles, "Caching strategy") {
		t.Errorf("expected the note as a candidate, got %v", titles)
	}
	if slices.Contains(titles, "Cache task") {
		t.Errorf("a task is not a linkable note and must be filtered, got %v", titles)
	}
}

// TestDeriveReceiptsSplitsAcceptedAndRejected verifies a candidate is accepted
// when the body links to it by title, id, or filename stem, and rejected
// otherwise.
func TestDeriveReceiptsSplitsAcceptedAndRejected(t *testing.T) {
	path := "projects/kernl.md"
	stemPath := "notes/stem-note.md"
	prev := []nodes.LinkCandidate{
		{ID: "id-1", Title: "Kernl", Path: &path, Snippet: "s"},
		{ID: "id-2", Title: "By Stem", Path: &stemPath, Snippet: "s"},
		{ID: "id-3", Title: "Unlinked", Snippet: "s"},
	}
	body := "See [[Kernl]] and [[stem-note]] for the details."

	accepted, rejected := linksuggest.DeriveReceipts(prev, body)

	if len(accepted) != 2 {
		t.Fatalf("accepted = %+v, want 2 entries", accepted)
	}
	acceptedIDs := []string{accepted[0].ID, accepted[1].ID}
	if !slices.Contains(acceptedIDs, "id-1") || !slices.Contains(acceptedIDs, "id-2") {
		t.Errorf("accepted = %+v, want id-1 (by title) and id-2 (by stem)", accepted)
	}
	if len(rejected) != 1 || rejected[0].ID != "id-3" {
		t.Errorf("rejected = %+v, want [id-3]", rejected)
	}
}

// TestShouldSuggestGatesOnTheChannel verifies the gate: cli and unidentified
// clients get suggestions, the web UI does not.
func TestShouldSuggestGatesOnTheChannel(t *testing.T) {
	for _, channel := range []string{"cli", "api", ""} {
		if !linksuggest.ShouldSuggest(channel) {
			t.Errorf("ShouldSuggest(%q) = false, want true", channel)
		}
	}
	if linksuggest.ShouldSuggest("ui") {
		t.Error("ShouldSuggest(ui) = true, want false: the user finds connections himself")
	}
}
