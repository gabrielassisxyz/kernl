package linksuggest_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/gabrielassisxyz/kernl/internal/planning/linksuggest"
	"github.com/gabrielassisxyz/kernl/internal/vault/companion"
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

	candidates, err := linksuggest.Suggest(ctx, g, "caching strategy", 8, "", 3)
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

	candidates, err := linksuggest.Suggest(ctx, g, "caching strategy", 8, "", 3)
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

// TestSuggestExcludesOwnID verifies a note whose id is excluded is not offered
// as a link candidate to itself. From the second write of a note onward it is
// already in the graph and matches its own body, so without the exclusion it
// would come back first among the candidates.
func TestSuggestExcludesOwnID(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	body := "We use an LRU cache with a write-through policy for hot keys."
	id := seedNote(t, g, "Caching strategy", body)

	candidates, err := linksuggest.Suggest(ctx, g, body, 8, id, 3)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}

	for _, c := range candidates {
		if c.ID == id {
			t.Errorf("a note must not be offered as a link candidate to itself, got %q", c.Title)
		}
	}
}

// TestSuggestEmptyExcludeChangesNothing verifies an empty exclusion id filters
// nothing: on the first write the note has no id yet, and it must not
// accidentally drop a real candidate.
func TestSuggestEmptyExcludeChangesNothing(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	body := "We use an LRU cache with a write-through policy for hot keys."
	id := seedNote(t, g, "Caching strategy", body)

	candidates, err := linksuggest.Suggest(ctx, g, body, 8, "", 3)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}

	var ids []string
	for _, c := range candidates {
		ids = append(ids, c.ID)
	}
	if !slices.Contains(ids, id) {
		t.Errorf("empty exclusion id must not filter the note itself, got %v", ids)
	}
}

// TestDeriveReceiptsRejectsABaseNameOnlyLink pins the receipt to what the graph
// will actually contain.
//
// wikilink.Resolver looks a stem up as the vault-relative path
// (`note_paths WHERE path = target || '.md'`) and falls back to the title, so
// `[[foo]]` for a note at notes/foo.md resolves to neither and grows no edge.
// Reporting it accepted told a caller it had linked something it had not:
// measured 2026-08-20 over a 20-note batch, all 20 candidates came back accepted
// and 7 edges existed. Every miss was a note outside the vault root, which is
// where every companion note lives.
func TestDeriveReceiptsRejectsABaseNameOnlyLink(t *testing.T) {
	notePath := "kernl/tasks/some-task.md"
	prev := []nodes.LinkCandidate{{ID: "id-1", Title: "Some task", Path: &notePath, Snippet: "s"}}

	accepted, rejected := linksuggest.DeriveReceipts(prev, "See [[some-task]].")

	if len(accepted) != 0 {
		t.Errorf("a base-name link grows no edge, so it must not be reported accepted, got %+v", accepted)
	}
	if len(rejected) != 1 {
		t.Errorf("rejected = %+v, want the candidate", rejected)
	}
}

// TestDeriveReceiptsSplitsAcceptedAndRejected verifies a candidate is accepted
// when the body links to it by title, id, or vault-relative path stem, and
// rejected otherwise. The stem is the whole path because that is the key
// wikilink.Resolver looks up; see TestDeriveReceiptsRejectsABaseNameOnlyLink.
func TestDeriveReceiptsSplitsAcceptedAndRejected(t *testing.T) {
	path := "projects/kernl.md"
	stemPath := "notes/stem-note.md"
	prev := []nodes.LinkCandidate{
		{ID: "id-1", Title: "Kernl", Path: &path, Snippet: "s"},
		{ID: "id-2", Title: "By Stem", Path: &stemPath, Snippet: "s"},
		{ID: "id-3", Title: "Unlinked", Snippet: "s"},
	}
	body := "See [[Kernl]] and [[notes/stem-note]] for the details."

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

// TestSuggestExclusionDoesNotCostASlot verifies the excluded note gives its slot
// back. Dropping the self-candidate from a list BuildContext already closed at
// limit removes the bad link and keeps the loss it was costing, which is the
// half-fix this guards against.
func TestSuggestExclusionDoesNotCostASlot(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	const shared = "write-through cache policy for hot keys"
	self := seedNote(t, g, "Caching strategy", shared)
	for i := 0; i < 5; i++ {
		seedNote(t, g, fmt.Sprintf("Sibling %d", i), shared)
	}

	const limit = 3
	withExclusion, err := linksuggest.Suggest(ctx, g, shared, limit, self, 3)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if len(withExclusion) != limit {
		t.Errorf("excluding the note being written cost a slot: got %d candidates, want %d",
			len(withExclusion), limit)
	}
}

// linkCompanion writes the describes edge that makes a note the companion of an
// entity, which is the same edge companion.Create writes.
func linkCompanion(t *testing.T, g *graph.Graph, noteID, entityID string) {
	t.Helper()
	ctx := context.Background()
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := edges.Create(ctx, tx, edges.Edge{
			Src: noteID, Dst: entityID, Label: companion.EdgeLabel,
		}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("describes edge: %v", err)
	}
}

// TestCompanionSeedsFromItsEntity is the case the batch link pass exists for.
//
// A companion is created holding one generated line and nothing else, and 468 of
// 636 of them still have under 200 bytes of body. Seeding on that asks the ranker
// to find neighbours for a template: a companion for the homelab project came back
// offered notes about the editor UI, matched on the word "Notes". The subject is
// one edge away, on the entity, and it is already indexed there.
func TestCompanionSeedsFromItsEntity(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	target := seedNote(t, g, "Zephyr protocol", "The zephyr protocol handles retries and backoff.")
	seedNote(t, g, "Editor indent width", "Notes editor indent width is configurable.")

	entity := seedTask(t, g, "Zephyr rollout", "Roll the zephyr protocol out with backoff on retries.")
	thin := seedNote(t, g, "Zephyr rollout", "Notes for [["+entity+"|Zephyr rollout]].\n")
	linkCompanion(t, g, thin, entity)

	got, err := linksuggest.Suggest(ctx, g, "Notes for [["+entity+"|Zephyr rollout]].\n", 8, thin, 3)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	var titles []string
	for _, c := range got {
		titles = append(titles, c.Title)
	}
	if !slices.Contains(titles, "Zephyr protocol") {
		t.Errorf("a companion must be offered what its ENTITY is about, got %v", titles)
	}
	_ = target
}

// TestSeedWithoutAnEntityIsUnchanged: a note that is nobody's companion must be
// seeded exactly as written. Enriching one of those would put an entity's words
// into a query that never asked for them.
func TestSeedWithoutAnEntityIsUnchanged(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	plain := seedNote(t, g, "Zephyr protocol", "The zephyr protocol handles retries.")
	seedNote(t, g, "Unrelated", "Something else entirely about disks.")

	got, err := linksuggest.Suggest(ctx, g, "The zephyr protocol handles retries.", 8, plain, 3)
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	for _, c := range got {
		if c.Title == "Unrelated" {
			t.Errorf("an unrelated note must not surface for a plain seed, got %v", c.Title)
		}
	}
}
