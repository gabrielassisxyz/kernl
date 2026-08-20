package planning_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
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

func seedTask(t *testing.T, g *graph.Graph, title, description, status string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateTask(ctx, tx, nodes.Task{Title: title, Description: description, Status: status}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed task %q: %v", title, err)
	}
	return id
}

func seedProject(t *testing.T, g *graph.Graph, title, description, status string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateProject(ctx, tx, nodes.Project{Title: title, Description: description, Status: status}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed project %q: %v", title, err)
	}
	return id
}

// seedCompanionNote creates a note and the links_to edge a companion note
// carries back to its entity, mirroring what the wikilink resolver writes when
// it resolves the [[entity|title]] link in a companion note's body.
func seedCompanionNote(t *testing.T, g *graph.Graph, title, body, entityID string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateNote(ctx, tx, nodes.Note{Title: title, Body: body}, nodes.Author{Name: "test"})
		if err != nil {
			return err
		}
		_, err = edges.Create(ctx, tx, edges.Edge{
			Src: id, Dst: entityID, Type: edges.EdgeTypeLinksTo,
		}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed companion note %q: %v", title, err)
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

func seedDANote(t *testing.T, g *graph.Graph, title, body string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateNote(ctx, tx, nodes.Note{
			Title: title, Body: body,
			Origin: nodes.OriginPrep,
			Tags:   []string{"da", "prep"},
		}, nodes.Author{Name: "da"})
		return err
	}); err != nil {
		t.Fatalf("seed DA note %q: %v", title, err)
	}
	return id
}

// TestBuildContext_ExcludesDAAuthoredNotes is the guard against the DA reading
// its own output back as the user's knowledge. A briefing is written FROM a
// capture; retrieving it as context for that same capture is how the classifier
// came to propose merging the user's words into a machine-written note.
func TestBuildContext_ExcludesDAAuthoredNotes(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	seedDANote(t, g, "Briefing: Improve HTML to PDF conversion", "HTML to PDF conversion pipelines usually rely on a headless browser.")
	seedNote(t, g, "PDF toolchain", "My HTML to PDF conversion notes: weasyprint beats wkhtmltopdf for print CSS.")

	notes, err := planning.BuildContext(ctx, g, "improve html to pdf conversion", 8)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if len(notes) == 0 {
		t.Fatal("expected the user's own note to still be retrieved")
	}
	for _, n := range notes {
		if strings.HasPrefix(n.Title, "Briefing:") {
			t.Errorf("DA briefing surfaced as planning context: %q (all: %+v)", n.Title, notes)
		}
	}
}

// TestBuildContext_ExcludesDANotesFromLinkedSignal covers the structural half of
// the retrieval: a briefing is edge-linked to the nodes it was prepared for, so
// excluding it from content search alone would let it back in through the links.
func TestBuildContext_ExcludesDANotesFromLinkedSignal(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	noteID := seedNote(t, g, "PDF toolchain", "weasyprint beats wkhtmltopdf for print CSS.")
	briefingID := seedDANote(t, g, "Briefing: Improve HTML to PDF conversion", "A primer on HTML to PDF pipelines.")
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := edges.Create(ctx, tx, edges.Edge{
			Src: briefingID, Dst: noteID, Label: "related", Type: edges.EdgeTypeRelated,
		}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("link briefing: %v", err)
	}

	notes, err := planning.BuildContext(ctx, g, noteID, 8)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	for _, n := range notes {
		if n.ID == briefingID {
			t.Errorf("DA briefing surfaced through the structural signal: %+v", notes)
		}
	}
}

// TestLoadTelos_InjectsTaggedNotes verifies telos-tagged note content is folded
// into a single always-on context block, while untagged notes are left out.
func TestLoadTelos_InjectsTaggedNotes(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	seedTaggedNote(t, g, "Who I am", "I am a solo builder optimizing for leverage.", []string{"telos"})
	seedTaggedNote(t, g, "My goals", "Ship the magic loop end-to-end this quarter.", []string{"telos", "da"})
	seedNote(t, g, "Caching strategy", "We use an LRU cache.") // untagged - must not appear

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

func refuteClaim(t *testing.T, g *graph.Graph, claimID string) {
	t.Helper()
	ctx := context.Background()
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		ref, err := nodes.CreateMemoryRefutation(ctx, tx, nodes.MemoryRefutation{
			Title: "Refute", ClaimID: claimID, Reason: "obsolete",
		}, nodes.Author{Name: "test"})
		if err != nil {
			return err
		}
		_, err = edges.Create(ctx, tx, edges.Edge{
			Src: ref, Dst: claimID, Label: "refutes", Type: edges.EdgeType("refutes"),
		}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("refute claim %s: %v", claimID, err)
	}
}

func findVia(notes []planning.ContextNote, via string) []planning.ContextNote {
	var out []planning.ContextNote
	for _, n := range notes {
		if n.Via == via {
			out = append(out, n)
		}
	}
	return out
}

// TestBuildContext_SurfacesActiveClaim verifies a memory claim matching the seed
// is folded into context as a via=claim entry, so claims feed every consumer.
func TestBuildContext_SurfacesActiveClaim(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	seedClaim(t, g, "Deploy cadence", "We deploy on Fridays using a canary rollout.")

	notes, err := planning.BuildContext(ctx, g, "what is our deploy canary cadence", 8)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	claims := findVia(notes, "claim")
	if len(claims) != 1 {
		t.Fatalf("expected 1 claim surfaced, got %d (full: %+v)", len(claims), notes)
	}
	if !strings.Contains(claims[0].Snippet, "canary rollout") {
		t.Errorf("claim snippet should carry the statement, got %q", claims[0].Snippet)
	}
}

// TestBuildContext_ResolvesNotePath verifies content notes carry their vault
// path (resolved from note_paths) while claims carry nil - the null convention
// the CLI serializes as JSON null, never as an empty string.
func TestBuildContext_ResolvesNotePath(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	noteID := seedNote(t, g, "Caching strategy", "We use an LRU cache.")
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`INSERT INTO note_paths (uuid, path) VALUES (?, ?)`, noteID, "notes/caching.md")
		return err
	}); err != nil {
		t.Fatalf("seed note_paths: %v", err)
	}
	seedClaim(t, g, "Deploy cadence", "We deploy on Fridays using a canary rollout.")

	notes, err := planning.BuildContext(ctx, g, "caching deploy canary", 8)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}

	content := findVia(notes, "content")
	if len(content) == 0 {
		t.Fatal("expected the caching note to surface")
	}
	if content[0].Path == nil || *content[0].Path != "notes/caching.md" {
		t.Errorf("content note path should resolve to notes/caching.md, got %v", content[0].Path)
	}

	claims := findVia(notes, "claim")
	if len(claims) == 0 {
		t.Fatal("expected the deploy claim to surface")
	}
	if claims[0].Path != nil {
		t.Errorf("claim path must be nil (no file on disk), got %v", claims[0].Path)
	}
}

// TestBuildContext_NoteWithoutFileHasNilPath verifies a note node with no
// note_paths row (created without a file, before the reconciler adopts it)
// surfaces with a nil path - serialized as JSON null - rather than an empty
// string, so consumers can tell "no file by design" from "failed to resolve".
func TestBuildContext_NoteWithoutFileHasNilPath(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	seedNote(t, g, "Caching strategy", "We use an LRU cache.")

	notes, err := planning.BuildContext(ctx, g, "caching strategy", 8)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	content := findVia(notes, "content")
	if len(content) == 0 {
		t.Fatal("expected the caching note to surface")
	}
	if content[0].Path != nil {
		t.Errorf("note without a file must have a nil path, got %q", *content[0].Path)
	}
}

// TestBuildContext_NotePathQueryErrorPropagates verifies a genuine note_paths
// query failure (here a missing table, standing in for a corrupted index or
// schema mismatch) surfaces as an error instead of silently serializing every
// note's path as null.
func TestBuildContext_NotePathQueryErrorPropagates(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	seedNote(t, g, "Caching strategy", "We use an LRU cache.")
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`DROP TABLE note_paths`)
		return err
	}); err != nil {
		t.Fatalf("drop note_paths: %v", err)
	}

	_, err := planning.BuildContext(ctx, g, "caching strategy", 8)
	if err == nil {
		t.Fatal("expected BuildContext to fail when note_paths is unreadable, got nil error")
	}
}

// TestBuildContext_RefutedClaimExcluded verifies a refuted claim never surfaces,
// reusing the shared non-refuted gate.
func TestBuildContext_RefutedClaimExcluded(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	id := seedClaim(t, g, "Old deploy", "We deploy by manual canary on Fridays.")
	refuteClaim(t, g, id)

	notes, err := planning.BuildContext(ctx, g, "what is our deploy canary cadence", 8)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if claims := findVia(notes, "claim"); len(claims) != 0 {
		t.Errorf("refuted claim must not surface, got %+v", claims)
	}
}

// TestBuildContext_ClaimsCapped verifies notes still return as before and claims
// supplement them, capped so they cannot dominate the context.
func TestBuildContext_ClaimsCapped(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	seedNote(t, g, "Canary strategy", "Our canary rollout strategy gates on error rate.")
	for _, s := range []string{
		"Canary one goes to 1% of traffic.",
		"Canary two goes to 5% of traffic.",
		"Canary three goes to 25% of traffic.",
		"Canary four goes to 50% of traffic.",
		"Canary five goes to 100% of traffic.",
		"Canary six is a full rollout.",
	} {
		seedClaim(t, g, "Canary step", s)
	}

	notes, err := planning.BuildContext(ctx, g, "canary rollout strategy", 8)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if got := findVia(notes, "content"); len(got) != 1 || got[0].Title != "Canary strategy" {
		t.Errorf("note should still return as before, got %+v", got)
	}
	if claims := findVia(notes, "claim"); len(claims) > 4 {
		t.Errorf("claims must be capped at 4, got %d", len(claims))
	}
}

// TestBuildContext_NoClaimsNotesOnly verifies a seed matching no claims returns
// notes only, with no error and no empty claim noise.
func TestBuildContext_NoClaimsNotesOnly(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	seedNote(t, g, "Caching strategy", "We use an LRU cache with a write-through policy.")
	seedClaim(t, g, "Unrelated", "The office plants need watering on Mondays.")

	notes, err := planning.BuildContext(ctx, g, "how should we design caching", 8)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if claims := findVia(notes, "claim"); len(claims) != 0 {
		t.Errorf("non-matching claim must not surface, got %+v", claims)
	}
	if len(findVia(notes, "content")) == 0 {
		t.Error("expected the caching note to still surface")
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

// TestBuildContext_PortugueseSeedIsNotDecidedByFunctionWords guards the property that a
// question's grammar must not outvote its subject. A note is scored by how many distinct
// seed terms it matches, so every word left in the seed counts the same. The vault and the
// questions asked of it are mostly Portuguese, and an English-only stopword list left "que",
// "sobre", "foi" and their neighbours in the seed: a note carrying those and nothing else
// outscored the note actually about the subject. Measured against the real vault on
// 2026-08-19, this took Recall@10 over the regression set from 0.200 to 0.600.
func TestBuildContext_PortugueseSeedIsNotDecidedByFunctionWords(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	seedNote(t, g, "Navegação por arestas",
		"Como a navegação entre notas acontece seguindo arestas do grafo.")
	// Carries the question's function words and none of its subject. With an English-only
	// stopword list this note matches more seed terms than the one above and wins.
	seedNote(t, g, "Diário de bordo",
		"O que já foi decidido sobre isso ainda está para ser escrito, qual seja o caso.")

	notes, err := planning.BuildContext(ctx, g, "o que já foi decidido sobre navegação de notas", 8)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if len(notes) == 0 {
		t.Fatal("expected at least one note")
	}
	if notes[0].Title != "Navegação por arestas" {
		t.Errorf("function words decided the ranking: got %q first, full: %+v", notes[0].Title, notes)
	}
}

// TestBuildContext_ReturnsTaskWithMatchingDescription verifies a task whose
// description matches the seed surfaces as planning context, with its type set
// and its description as the snippet - the reasoning that used to be invisible
// to the planner.
func TestBuildContext_ReturnsTaskWithMatchingDescription(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	seedTask(t, g, "Report composer", "The composer writes a markdown report for every run.", nodes.TaskStatusInProgress)

	notes, err := planning.BuildContext(ctx, g, "markdown report composer", 8)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	var found bool
	for _, n := range notes {
		if n.Type == "task" && n.Title == "Report composer" {
			found = true
			if !strings.Contains(n.Snippet, "markdown report") {
				t.Errorf("task snippet should carry the description, got %q", n.Snippet)
			}
		}
	}
	if !found {
		t.Fatalf("expected the task to surface, got %+v", notes)
	}
}

// TestBuildContext_ReturnsProjectWithMatchingDescription verifies a project
// whose description matches the seed surfaces as planning context.
func TestBuildContext_ReturnsProjectWithMatchingDescription(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	seedProject(t, g, "Report composer", "The composer writes a markdown report for every run.", "active")

	notes, err := planning.BuildContext(ctx, g, "markdown report composer", 8)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	var found bool
	for _, n := range notes {
		if n.Type == "project" && n.Title == "Report composer" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the project to surface, got %+v", notes)
	}
}

// TestBuildContext_ExcludesFinishedWork verifies the state filter: a done or
// closed task, and an archived project, are finished work and must not feed
// planning context.
func TestBuildContext_ExcludesFinishedWork(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	seedTask(t, g, "Done task", "The composer writes a markdown report for every run.", nodes.TaskStatusDone)
	seedTask(t, g, "Closed task", "The composer writes a markdown report for every run.", nodes.TaskStatusClosed)
	seedProject(t, g, "Archived project", "The composer writes a markdown report for every run.", "archived")

	notes, err := planning.BuildContext(ctx, g, "markdown report composer", 8)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	for _, n := range notes {
		if n.Type == "task" || n.Type == "project" {
			t.Errorf("finished work must not surface, got %+v", notes)
		}
	}
}

// TestBuildContext_DedupesEntityAgainstCompanion verifies a task and its
// companion note - two nodes about one thing - do not both occupy slots. The
// higher-scoring one (the task, whose description matches more terms) is kept.
func TestBuildContext_DedupesEntityAgainstCompanion(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	taskID := seedTask(t, g, "Report composer", "The composer writes a markdown report for every run.", nodes.TaskStatusInProgress)
	companionID := seedCompanionNote(t, g, "Report composer", "Notes for the report composer.", taskID)

	notes, err := planning.BuildContext(ctx, g, "report composer markdown", 8)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	var taskSeen, companionSeen bool
	for _, n := range notes {
		if n.ID == taskID {
			taskSeen = true
		}
		if n.ID == companionID {
			companionSeen = true
		}
	}
	if taskSeen && companionSeen {
		t.Errorf("task and companion must not both appear, got %+v", notes)
	}
	if !taskSeen {
		t.Errorf("expected the higher-scoring task to be kept, got %+v", notes)
	}
}

// TestBuildContext_TypeIsPresent verifies every result carries its node type,
// so a consumer can tell a note from a task without guessing from a path.
func TestBuildContext_TypeIsPresent(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	seedNote(t, g, "Caching strategy", "We use an LRU cache.")
	seedTask(t, g, "Report composer", "The composer writes a markdown report.", nodes.TaskStatusInProgress)

	notes, err := planning.BuildContext(ctx, g, "caching report composer", 8)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	types := map[string]bool{}
	for _, n := range notes {
		if n.Type == "" {
			t.Errorf("every result must carry a type, got %+v", notes)
		}
		types[n.Type] = true
	}
	if !types["note"] || !types["task"] {
		t.Errorf("expected both a note and a task with their types, got %v (full: %+v)", types, notes)
	}
}

// TestBuildContext_NoteWinsATieAgainstAnEntity pins the tie-break. When a note
// and an entity match the same number of distinct seed terms, the note comes
// first: it is the canonical representation, and its body carries the fuller
// reasoning where the entity carries only what its description happens to say.
//
// The rule was measured before it was written: without it the generated easy
// question set drops from Recall@5 1.000 to 0.950, two targets losing their
// place to an entity they tied with. That measurement lives in another repo and
// takes minutes, so it cannot be the thing that guards this ordering.
func TestBuildContext_NoteWinsATieAgainstAnEntity(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	// Same salient vocabulary on both, so they tie on matched terms. The task is
	// seeded first, so a stable sort alone would leave it in front.
	const shared = "zephyr backoff retries"
	seedTask(t, g, "Zephyr rollout", shared, nodes.TaskStatusInProgress)
	noteID := seedNote(t, g, "Zephyr protocol", shared)

	notes, err := planning.BuildContext(ctx, g, shared, 8)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	if len(notes) < 2 {
		t.Fatalf("expected both the note and the task, got %+v", notes)
	}
	if notes[0].ID != noteID {
		t.Errorf("a note must win a tie against an entity, got %q (type %q) first: %+v",
			notes[0].Title, notes[0].Type, notes)
	}
}

// TestLinkingScoringDemotesTheLongDocument is the whole reason the two scorings
// exist separately. A long note contains a little of everything, so against a seed
// that is itself a note it matches more terms than the note that is genuinely
// about the subject, and term counting hands it the top slot.
//
// Measured over the 282 links somebody made by reading: under term counting one
// machine log was offered in 40 of 40 queries whatever the subject. Without a test
// the two entry points look like duplication and get merged back.
func TestLinkingScoringDemotesTheLongDocument(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	// The note that is about zephyr, and nothing else.
	focused := seedNote(t, g, "Zephyr protocol", "The zephyr protocol handles retries.")
	// A log that mentions zephyr once among everything else it mentions.
	seedNote(t, g, "Machine log", "zephyr retries "+strings.Repeat(
		"disk kernel mount network backup upgrade service unit timer package ", 60))
	// Enough unrelated notes that "zephyr" is rare and the log's other words are not.
	for i := 0; i < 30; i++ {
		seedNote(t, g, fmt.Sprintf("Ops note %d", i),
			"disk kernel mount network backup upgrade service unit timer package")
	}

	seed := "zephyr retries disk kernel mount network backup upgrade service unit timer package"

	byCount, err := planning.BuildContext(ctx, g, seed, 8)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	byLength, err := planning.BuildContextForLinking(ctx, g, seed, 8)
	if err != nil {
		t.Fatalf("BuildContextForLinking: %v", err)
	}
	if len(byCount) == 0 || len(byLength) == 0 {
		t.Fatalf("both scorings must return something, got %d and %d", len(byCount), len(byLength))
	}
	if byCount[0].ID == byLength[0].ID {
		t.Fatalf("the two scorings must disagree here, both returned %q first", byCount[0].Title)
	}
	if byLength[0].ID != focused {
		t.Errorf("linking must put the note the seed is about first, got %q", byLength[0].Title)
	}
}

// TestBuildContextForLinking_ReturnsNotesOnly holds the boundary between the two
// paths: a link candidate has to be something a wikilink can point at, and only a
// note has a file. An entity in the list is a slot spent on a target the writer
// never sees, because the caller discards it.
func TestBuildContextForLinking_ReturnsNotesOnly(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	seedNote(t, g, "Zephyr protocol", "The zephyr protocol handles retries.")
	seedTask(t, g, "Ship zephyr", "The zephyr rollout needs retries measured.", nodes.TaskStatusInProgress)
	seedProject(t, g, "Zephyr", "Everything zephyr, retries included.", nodes.DefaultProjectStatus)

	notes, err := planning.BuildContextForLinking(ctx, g, "zephyr retries", 8)
	if err != nil {
		t.Fatalf("BuildContextForLinking: %v", err)
	}
	if len(notes) == 0 {
		t.Fatal("expected the note to be returned")
	}
	for _, n := range notes {
		if n.Type != "note" {
			t.Errorf("linking must return notes only, got %q of type %q", n.Title, n.Type)
		}
	}
}

// TestBuildContextForLinking_CompanionSurvivesItsEntity is the case over-fetching
// cannot reach. The ranking collapses an entity and its companion note into one
// slot, keeping whichever scores higher; when the entity wins, the companion is
// marked seen and never returned. A caller that then drops the entity for not
// being a note loses both halves, so the companion has to be retrieved in the
// entity's place rather than filtered out after it.
func TestBuildContextForLinking_CompanionSurvivesItsEntity(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	// The entity carries the vocabulary; its companion carries one mention, which
	// is what a thin companion looks like in the vault.
	taskID := seedTask(t, g, "Zephyr retries", "The zephyr protocol retries every failed delivery.", nodes.TaskStatusInProgress)
	companionID := seedCompanionNote(t, g, "Zephyr retries", "Notes for zephyr.", taskID)

	// The seed carries four terms the entity's description matches and only one
	// the companion body does, so the entity outscores its own companion.
	const seed = "zephyr protocol retries delivery"

	byCount, err := planning.BuildContext(ctx, g, seed, 8)
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}
	// The premise: on the planning path the entity takes the pair's only slot.
	// Without it this test would pass for the wrong reason.
	if len(byCount) != 1 || byCount[0].ID != taskID {
		t.Fatalf("premise broken: planning should return the entity alone, got %+v", byCount)
	}

	notes, err := planning.BuildContextForLinking(ctx, g, seed, 8)
	if err != nil {
		t.Fatalf("BuildContextForLinking: %v", err)
	}
	var found bool
	for _, n := range notes {
		if n.ID == companionID {
			found = true
		}
	}
	if !found {
		t.Errorf("the companion note must be offered when its entity is not a candidate, got %+v", notes)
	}
}
