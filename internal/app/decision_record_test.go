package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

// wellFormedDecisionEntry is the normal, single-decision shape - the direct
// Go value a caller building graph-writing input (WriteDecisionRecordNode,
// decisionFromRecordEntry) works with. Not a string: the artifact this
// decision came from is JSON now, not markdown, and there is no reason to
// round-trip through text just to build a struct these tests already know
// the shape of - mustMarshalDecisionRecordJSON below produces the on-disk
// text for tests that specifically exercise the file-reading/parsing path.
var wellFormedDecisionEntry = backend.DecisionRecordEntry{
	Decision:          "Use edges.EdgeTypeHasDecision for the bead/epic link.",
	OptionsConsidered: "1. A bare string literal.\n2. A new typed constant.",
	TradeOffs:         "A typed constant is one more name to learn, but closes the set.",
	Rationale:         "Matches the existing closed set (related, depends_on, blocks, part_of, links_to).",
}

// wellFormedDecisionRecordJSON is wellFormedDecisionEntry as the artifact an
// agent would write to disk.
var wellFormedDecisionRecordJSON = mustMarshalDecisionRecordJSON(wellFormedDecisionEntry)

// mustMarshalDecisionRecordJSON encodes entries as a decision_record
// artifact's on-disk JSON envelope - what an agent (or a test simulating
// one) actually writes to disk. Named "must": every call site in this file
// passes a value already known to marshal cleanly (plain strings, no cycles
// or channels), so a marshal failure here would mean this file's own
// fixture is broken, not a runtime condition a test needs to assert on.
func mustMarshalDecisionRecordJSON(entries ...backend.DecisionRecordEntry) string {
	b, err := json.Marshal(struct {
		Decisions []backend.DecisionRecordEntry `json:"decisions"`
	}{Decisions: entries})
	if err != nil {
		panic(err)
	}
	return string(b)
}

// --- decisionFromRecordEntry / SplitDecisionBody: the four parts stay
// individually recoverable (criterion 1). ---

func TestDecisionFromRecordEntry_FourPartsIndividuallyRecoverable(t *testing.T) {
	d := decisionFromRecordEntry(wellFormedDecisionEntry, time.Now())

	if !strings.Contains(d.Context, "Use edges.EdgeTypeHasDecision") {
		t.Errorf("Context = %q, want the decision field's full text", d.Context)
	}
	if d.Title == "" {
		t.Error("Title is empty, want a title derived from the decision field")
	}
	if d.Outcome != strings.TrimSpace(wellFormedDecisionEntry.Rationale) {
		t.Errorf("Outcome = %q, want exactly the rationale field: %q", d.Outcome, wellFormedDecisionEntry.Rationale)
	}

	options, tradeOffs, ok := SplitDecisionBody(d.Body)
	if !ok {
		t.Fatalf("SplitDecisionBody could not split Body built by buildDecisionBody: %q", d.Body)
	}
	if options != strings.TrimSpace(wellFormedDecisionEntry.OptionsConsidered) {
		t.Errorf("recovered options = %q, want %q", options, wellFormedDecisionEntry.OptionsConsidered)
	}
	if tradeOffs != strings.TrimSpace(wellFormedDecisionEntry.TradeOffs) {
		t.Errorf("recovered trade-offs = %q, want %q", tradeOffs, wellFormedDecisionEntry.TradeOffs)
	}
	// The two parts must not be interchangeable - if buildDecisionBody ever
	// concatenated them without a boundary, this would still pass with
	// options and tradeOffs swapped or merged; guard the actual content.
	if strings.Contains(options, "one more name to learn") {
		t.Error("options-considered text contains trade-offs content - boundary lost")
	}
	if strings.Contains(tradeOffs, "A bare string literal") {
		t.Error("trade-offs text contains options-considered content - boundary lost")
	}
}

// TestDecisionFromRecordEntry_TitleFallsBackToDecisionFirstLine proves an
// entry with no explicit Title still gets a usable one, derived from the
// decision field - Title is optional in the schema precisely so a
// single-decision record (the common case) never needs to repeat itself.
func TestDecisionFromRecordEntry_TitleFallsBackToDecisionFirstLine(t *testing.T) {
	d := decisionFromRecordEntry(wellFormedDecisionEntry, time.Now())
	want := fallbackDecisionTitle(wellFormedDecisionEntry.Decision)
	if d.Title != want {
		t.Errorf("Title = %q, want %q (derived from the decision field)", d.Title, want)
	}
}

// TestDecisionFromRecordEntry_ExplicitTitleWins proves an entry that DOES
// set Title uses it verbatim rather than deriving one - the field a record
// with several decisions needs to tell them apart.
func TestDecisionFromRecordEntry_ExplicitTitleWins(t *testing.T) {
	entry := wellFormedDecisionEntry
	entry.Title = "Edge type for the bead/epic link"
	d := decisionFromRecordEntry(entry, time.Now())
	if d.Title != "Edge type for the bead/epic link" {
		t.Errorf("Title = %q, want the explicit title verbatim", d.Title)
	}
}

// TestDecisionFromRecordSections_ImpactOnUseIsNilNotWritten pins that mapping
// a freshly-parsed record never sets ImpactOnUse - it is written later by
// the run's composer, not at decision time (see AGENTS.md and Decision's own
// doc comment). Inverting this (setting it to a non-nil default) would
// silently mark every record as already composed on day one.
func TestDecisionFromRecordSections_ImpactOnUseIsNilNotWritten(t *testing.T) {
	d := decisionFromRecordEntry(wellFormedDecisionEntry, time.Now())
	if d.ImpactOnUse != nil {
		t.Errorf("ImpactOnUse = %q, want nil - decision time never writes it", *d.ImpactOnUse)
	}
}

func TestSplitDecisionBody_RejectsBodyItDidNotBuild(t *testing.T) {
	_, _, ok := SplitDecisionBody("plain prose with no headings at all")
	if ok {
		t.Error("SplitDecisionBody reported ok=true for a body it could not have built")
	}
}

// TestSplitDecisionBody_FencedHeadingInOptionsIsNotABoundary is the review's
// exact repro: the options-considered text quotes "## Trade-offs" inside a
// fenced code example while discussing it. A naive strings.Index search for
// the literal heading text finds that fenced occurrence first and truncates
// the real options content there, handing the real separator (and
// everything after it) to trade-offs instead. SplitDecisionBody must ignore
// a heading inside a fence, exactly as the gate's own parser does.
func TestSplitDecisionBody_FencedHeadingInOptionsIsNotABoundary(t *testing.T) {
	options := "Preserve this example:\n\n" +
		"```markdown\n## Trade-offs\nexample text\n```"
	tradeOffs := "The real trade-off."

	body := buildDecisionBody(options, tradeOffs)

	gotOptions, gotTradeOffs, ok := SplitDecisionBody(body)
	if !ok {
		t.Fatalf("SplitDecisionBody could not split: %q", body)
	}
	if !strings.Contains(gotOptions, "```markdown") || !strings.Contains(gotOptions, "example text") {
		t.Errorf("recovered options lost the fenced example: %q", gotOptions)
	}
	if gotTradeOffs != tradeOffs {
		t.Errorf("recovered trade-offs = %q, want %q", gotTradeOffs, tradeOffs)
	}
}

// TestSplitDecisionBody_InlineMentionOfHeadingTextIsNotABoundary covers the
// review's other example: prose that mentions the heading text inline,
// mid-line, rather than as a real heading. A substring search matches this
// too; a real ATX/setext heading must start its own line.
func TestSplitDecisionBody_InlineMentionOfHeadingTextIsNotABoundary(t *testing.T) {
	options := "We considered naming it the literal `## Trade-offs` string but rejected that."
	tradeOffs := "The real trade-off text."

	body := buildDecisionBody(options, tradeOffs)

	gotOptions, gotTradeOffs, ok := SplitDecisionBody(body)
	if !ok {
		t.Fatalf("SplitDecisionBody could not split: %q", body)
	}
	if gotOptions != options {
		t.Errorf("recovered options = %q, want %q", gotOptions, options)
	}
	if gotTradeOffs != tradeOffs {
		t.Errorf("recovered trade-offs = %q, want %q", gotTradeOffs, tradeOffs)
	}
}

// --- recordDecisionIfGateType: no-op unless the gate is decision_record
// (criterion 6, at the wiring boundary: nothing downstream fires for any
// other gate type or a stage with no gate at all). ---

func TestRecordDecisionIfGateType_NoOpForOtherGateTypes(t *testing.T) {
	wf := backend.WorkflowDescriptor{
		ExitGates: map[string][]backend.WorkflowExitGate{
			"implementation": {{Type: "commit_marker"}},
		},
	}
	gateCtx := backend.ExitGateContext{FromState: "implementation"}

	// deps and bead are deliberately zero-value (deps.Config nil, no
	// Backend): a no-op must return before ever touching either, so passing
	// zero values doubles as proof the function never even tries to open a
	// graph, or fetch an epic, for a gate type it does not own.
	if err := recordDecisionIfGateType(context.Background(), wf, gateCtx, DriveBeadDeps{}, &backend.Bead{ID: "kb-1"}, "kb-1", ""); err != nil {
		t.Fatalf("recordDecisionIfGateType: %v", err)
	}
}

func TestRecordDecisionIfGateType_NoOpWhenNoGateForState(t *testing.T) {
	wf := backend.WorkflowDescriptor{ExitGates: map[string][]backend.WorkflowExitGate{}}
	gateCtx := backend.ExitGateContext{FromState: "implementation"}

	if err := recordDecisionIfGateType(context.Background(), wf, gateCtx, DriveBeadDeps{}, &backend.Bead{ID: "kb-1"}, "kb-1", ""); err != nil {
		t.Fatalf("recordDecisionIfGateType: %v", err)
	}
}

// --- readDecisionRecords: re-read failures halt loudly, they never
// silently skip (part of criterion 7). ---

func TestReadDecisionRecords_FileMissingFails(t *testing.T) {
	gate := backend.WorkflowExitGate{Type: "decision_record", Path: "<artifact_dir>/decision-record.json"}
	gateCtx := backend.ExitGateContext{FromState: "implementation", ArtifactDir: t.TempDir(), BeadID: "kb-1"}

	_, err := readDecisionRecords(gate, gateCtx, "kb-1")
	if err == nil {
		t.Fatal("expected an error for a decision record that does not exist on disk")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error %q does not carry the KERNL DISPATCH FAILURE marker", err.Error())
	}
}

func TestReadDecisionRecords_FieldMissingOnRereadFails(t *testing.T) {
	dir := t.TempDir()
	// Missing the "rationale" field entirely - simulates the record changing,
	// or the gate and this read disagreeing, between the gate's own read and
	// this one.
	incomplete := `{"decisions":[{"decision":"Use X.","optionsConsidered":"X or Y.","tradeOffs":"X is simpler."}]}`
	if err := os.WriteFile(filepath.Join(dir, "decision-record.json"), []byte(incomplete), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	gate := backend.WorkflowExitGate{Type: "decision_record", Path: "<artifact_dir>/decision-record.json"}
	gateCtx := backend.ExitGateContext{FromState: "implementation", ArtifactDir: dir, BeadID: "kb-1"}

	_, err := readDecisionRecords(gate, gateCtx, "kb-1")
	if err == nil {
		t.Fatal("expected an error for a record missing the rationale field on re-read")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error %q does not carry the KERNL DISPATCH FAILURE marker", err.Error())
	}
	if !strings.Contains(err.Error(), "rationale") {
		t.Errorf("error %q does not name the missing field", err.Error())
	}
}

func TestReadDecisionRecords_WellFormedRecordExtractsEntry(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "decision-record.json"), []byte(wellFormedDecisionRecordJSON), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	gate := backend.WorkflowExitGate{Type: "decision_record", Path: "<artifact_dir>/decision-record.json"}
	gateCtx := backend.ExitGateContext{FromState: "implementation", ArtifactDir: dir, BeadID: "kb-1"}

	entries, err := readDecisionRecords(gate, gateCtx, "kb-1")
	if err != nil {
		t.Fatalf("readDecisionRecords: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	for name, got := range map[string]string{
		"decision":          entries[0].Decision,
		"optionsConsidered": entries[0].OptionsConsidered,
		"tradeOffs":         entries[0].TradeOffs,
		"rationale":         entries[0].Rationale,
	} {
		if got == "" {
			t.Errorf("field %q is empty, want content", name)
		}
	}
}

// TestReadDecisionRecords_MultipleDecisionsAllExtracted proves a record
// carrying several decisions comes back with every one of them, in order -
// the read side of the same "nothing dropped" guarantee the gate itself
// provides (criterion 4).
func TestReadDecisionRecords_MultipleDecisionsAllExtracted(t *testing.T) {
	dir := t.TempDir()
	second := backend.DecisionRecordEntry{
		Title: "Second decision", Decision: "Use TOML for the export manifest.",
		OptionsConsidered: "1. JSON.\n2. TOML.", TradeOffs: "TOML reads better by hand.",
		Rationale: "Matches the config file already in the repo.",
	}
	content := mustMarshalDecisionRecordJSON(wellFormedDecisionEntry, second)
	if err := os.WriteFile(filepath.Join(dir, "decision-record.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	gate := backend.WorkflowExitGate{Type: "decision_record", Path: "<artifact_dir>/decision-record.json"}
	gateCtx := backend.ExitGateContext{FromState: "implementation", ArtifactDir: dir, BeadID: "kb-1"}

	entries, err := readDecisionRecords(gate, gateCtx, "kb-1")
	if err != nil {
		t.Fatalf("readDecisionRecords: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 - a decision was dropped on re-read", len(entries))
	}
	if entries[1].Title != "Second decision" {
		t.Errorf("entries[1].Title = %q, want %q", entries[1].Title, "Second decision")
	}
}

// --- WriteDecisionRecordNode: linked to both bead and epic via the typed
// edge constant (criterion 2), and a write failure halts loudly rather than
// dropping the reasoning on the floor (criterion 7). ---

func TestWriteDecisionRecordNode_LinksBeadAndEpic(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	bead := BeadRef{ID: "kb-child-1", Title: "child bead", TrackerKind: "br", RepoPath: "/repo"}
	epic := BeadRef{ID: "kb-epic-1", Title: "epic bead", TrackerKind: "br", RepoPath: "/repo"}

	runID := seedRunWithBeads(t, g, "epic bead", []BeadRef{bead, epic})

	ids, err := WriteDecisionRecordNode(ctx, g, []backend.DecisionRecordEntry{wellFormedDecisionEntry}, bead, epic, runID)
	if err != nil {
		t.Fatalf("WriteDecisionRecordNode: %v", err)
	}
	id := ids[0]

	var got *nodes.Decision
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = nodes.GetDecision(ctx, tx, id)
		return err
	}); err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if len(got.Tags) != 1 || got.Tags[0] != PhaseThreeDecisionTag {
		t.Errorf("Tags = %v, want [%q]", got.Tags, PhaseThreeDecisionTag)
	}

	var in []edges.Edge
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		in, err = edges.Incoming(ctx, tx, id, edges.WithType(edges.EdgeTypeHasDecision))
		return err
	}); err != nil {
		t.Fatalf("edges.Incoming: %v", err)
	}

	srcs := map[string]bool{}
	for _, e := range in {
		srcs[e.Src] = true
	}
	if !srcs[bead.ID] {
		t.Errorf("no has_decision edge from bead %s to decision %s", bead.ID, id)
	}
	if !srcs[epic.ID] {
		t.Errorf("no has_decision edge from epic %s to decision %s", epic.ID, id)
	}
	// The run is the third source: a decision is linked to the run that
	// produced it by an edge, which is what scopes ComposeRunReport's
	// traversal to one run instead of to a bead's whole history.
	if !srcs[runID] {
		t.Errorf("no has_decision edge from run %s to decision %s", runID, id)
	}
	if len(in) != 3 {
		t.Errorf("expected exactly 3 incoming has_decision edges, got %d: %+v", len(in), in)
	}

	// Criterion 6: the node id is the bead's own tracker id, and a
	// traversal starting from that id (not from the decision) reaches the
	// decision - the direction a caller actually has when it only knows a
	// bead id and wants what was decided while working it.
	var out []edges.Edge
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		out, err = edges.Outgoing(ctx, tx, bead.ID, edges.WithType(edges.EdgeTypeHasDecision))
		return err
	}); err != nil {
		t.Fatalf("edges.Outgoing: %v", err)
	}
	if len(out) != 1 || out[0].Dst != id {
		t.Errorf("traversal from bead id %s = %+v, want exactly one has_decision edge to %s", bead.ID, out, id)
	}

	var beadRef *nodes.BeadReference
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		beadRef, err = nodes.GetBeadReference(ctx, tx, bead.ID)
		return err
	}); err != nil {
		t.Fatalf("GetBeadReference(bead): %v", err)
	}
	if beadRef.Title != bead.Title || beadRef.TrackerKind != bead.TrackerKind || beadRef.Repository != bead.RepoPath {
		t.Errorf("bead reference node = %+v, want Title=%q TrackerKind=%q Repository=%q", beadRef, bead.Title, bead.TrackerKind, bead.RepoPath)
	}

	var epicRef *nodes.BeadReference
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		epicRef, err = nodes.GetBeadReference(ctx, tx, epic.ID)
		return err
	}); err != nil {
		t.Fatalf("GetBeadReference(epic): %v", err)
	}
	if epicRef.Title != epic.Title {
		t.Errorf("epic reference Title = %q, want %q", epicRef.Title, epic.Title)
	}
}

// TestWriteDecisionRecordNode_RetryConvergesOnOneNode covers the case a
// bare "write once" implementation gets wrong: the graph write succeeds and
// commits, but the caller's own next step (advancing the bead's tracker
// state) fails for an unrelated reason, and the whole run is retried from
// the top. The retry re-reads the same decision-record.json and calls
// WriteDecisionRecordNode again with identical sections, bead and epic. A
// writer that mints a fresh random ID every call would leave two Decision
// nodes and four has_decision edges behind for one real decision - and,
// since this bead, a second reference node per retry too; this one must
// converge on exactly one of each (criterion 3).
func TestWriteDecisionRecordNode_RetryConvergesOnOneNode(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	bead := BeadRef{ID: "kb-retry-1", Title: "child bead", TrackerKind: "br", RepoPath: "/repo"}
	epic := BeadRef{ID: "kb-epic-retry-1", Title: "epic bead", TrackerKind: "br", RepoPath: "/repo"}

	decisions := []backend.DecisionRecordEntry{wellFormedDecisionEntry}

	// The retry is the SAME run retrying, which is the shape the convergence
	// is for. A different run is deliberately a different Decision node -
	// decisionRecordNodeID folds the run id into the hash - so seeding a
	// second run here would be testing the opposite property.
	runID := seedRunWithBeads(t, g, "epic bead", []BeadRef{bead, epic})

	firstIDs, err := WriteDecisionRecordNode(ctx, g, decisions, bead, epic, runID)
	if err != nil {
		t.Fatalf("WriteDecisionRecordNode (first attempt): %v", err)
	}
	secondIDs, err := WriteDecisionRecordNode(ctx, g, decisions, bead, epic, runID)
	if err != nil {
		t.Fatalf("WriteDecisionRecordNode (retry): %v", err)
	}
	firstID, secondID := firstIDs[0], secondIDs[0]

	if firstID != secondID {
		t.Fatalf("retry minted a different node: first=%s second=%s", firstID, secondID)
	}

	var decisionCount int
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE type = 'decision'`).Scan(&decisionCount)
	}); err != nil {
		t.Fatalf("counting decision nodes: %v", err)
	}
	if decisionCount != 1 {
		t.Errorf("decision node count = %d after 2 identical writes, want 1", decisionCount)
	}

	var refCount int
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE type = 'bead_reference'`).Scan(&refCount)
	}); err != nil {
		t.Fatalf("counting bead_reference nodes: %v", err)
	}
	if refCount != 2 {
		t.Errorf("bead_reference node count = %d after 2 identical writes, want 2 (one bead, one epic), not one per attempt", refCount)
	}

	var in []edges.Edge
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		in, err = edges.Incoming(ctx, tx, firstID, edges.WithType(edges.EdgeTypeHasDecision))
		return err
	}); err != nil {
		t.Fatalf("edges.Incoming: %v", err)
	}
	if len(in) != 3 {
		t.Errorf("has_decision edge count = %d after 2 identical writes, want 3 (one run, one bead, one epic)", len(in))
	}
}

func TestWriteDecisionRecordNode_SameBeadAndEpicWritesOneEdge(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	bead := BeadRef{ID: "kb-standalone-1", Title: "standalone bead", TrackerKind: "bd", RepoPath: "/repo"}

	runID := seedRunWithBeads(t, g, "standalone bead", []BeadRef{bead})

	ids, err := WriteDecisionRecordNode(ctx, g, []backend.DecisionRecordEntry{wellFormedDecisionEntry}, bead, bead, runID)
	if err != nil {
		t.Fatalf("WriteDecisionRecordNode: %v", err)
	}
	id := ids[0]

	var in []edges.Edge
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		in, err = edges.Incoming(ctx, tx, id, edges.WithType(edges.EdgeTypeHasDecision))
		return err
	}); err != nil {
		t.Fatalf("edges.Incoming: %v", err)
	}
	// The bead-and-epic collapse to one edge; the run's own edge is always
	// its own, so a standalone bead run still leaves exactly two.
	if len(in) != 2 {
		t.Errorf("expected 2 edges (run + the collapsed bead/epic) when bead and epic are the same node, got %d: %+v", len(in), in)
	}

	var refCount int
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE type = 'bead_reference'`).Scan(&refCount)
	}); err != nil {
		t.Fatalf("counting bead_reference nodes: %v", err)
	}
	if refCount != 1 {
		t.Errorf("bead_reference node count = %d when bead and epic are the same tracker id, want 1", refCount)
	}
}

// TestWriteDecisionRecordNode_NeverSeenBeadSucceeds is criterion 2: the
// end-to-end failure this bead exists to remove. Before this change, calling
// WriteDecisionRecordNode for a real bead id that had never been mirrored
// into the graph failed with "edges.Create: src node ... graph: not found",
// because orchestrator bead/epic ids were never mirrored into this graph as
// nodes anywhere in this codebase (nodes.Task and nodes.Project are a
// distinct, human-authored concept - see task.go's own doc comment). That
// failure was harmless only because the decision_record gate was not yet
// wired to the workflow profile the live rig actually runs; once it is, this
// path runs on every real gate pass and the failure stops being harmless.
// The graph here starts completely empty - no stand-in of any kind - which
// is the exact shape of a bead's first decision record in production.
func TestWriteDecisionRecordNode_NeverSeenBeadSucceeds(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	bead := BeadRef{ID: "kb-never-seen-1", Title: "never seen before", TrackerKind: "br", RepoPath: "/repo"}
	epic := BeadRef{ID: "kb-never-seen-epic-1", Title: "epic, also never seen", TrackerKind: "br", RepoPath: "/repo"}

	runID := seedRunWithBeads(t, g, "epic, also never seen", []BeadRef{bead, epic})

	ids, err := WriteDecisionRecordNode(ctx, g, []backend.DecisionRecordEntry{wellFormedDecisionEntry}, bead, epic, runID)
	if err != nil {
		t.Fatalf("WriteDecisionRecordNode: %v", err)
	}
	if len(ids) != 1 || ids[0] == "" {
		t.Fatalf("expected exactly one non-empty decision node id, got %v", ids)
	}
}

// TestWriteDecisionRecordNode_MultipleDecisionsEachGetsOwnNodeAndEdges is
// the graph-writing side of criterion 4 (nothing dropped when a record
// carries several decisions - the specific failure mode the withdrawn
// prefix-matching approach would have introduced: heading-key collisions
// silently keeping only the last decision). Three entries with genuinely
// different content must become three distinct Decision nodes, each linked
// to the run, the bead and the epic.
func TestWriteDecisionRecordNode_MultipleDecisionsEachGetsOwnNodeAndEdges(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	bead := BeadRef{ID: "kb-multi-1", Title: "child bead", TrackerKind: "br", RepoPath: "/repo"}
	epic := BeadRef{ID: "kb-multi-epic-1", Title: "epic bead", TrackerKind: "br", RepoPath: "/repo"}
	runID := seedRunWithBeads(t, g, "epic bead", []BeadRef{bead, epic})

	decisions := []backend.DecisionRecordEntry{
		{Title: "First", Decision: "Decision one.", OptionsConsidered: "A or B.", TradeOffs: "A is faster.", Rationale: "A wins."},
		{Title: "Second", Decision: "Decision two.", OptionsConsidered: "C or D.", TradeOffs: "D is simpler.", Rationale: "D wins."},
		{Title: "Third", Decision: "Decision three.", OptionsConsidered: "E or F.", TradeOffs: "E is safer.", Rationale: "E wins."},
	}

	ids, err := WriteDecisionRecordNode(ctx, g, decisions, bead, epic, runID)
	if err != nil {
		t.Fatalf("WriteDecisionRecordNode: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("got %d ids, want 3 - a decision was dropped", len(ids))
	}
	if ids[0] == ids[1] || ids[1] == ids[2] || ids[0] == ids[2] {
		t.Fatalf("expected 3 distinct decision ids, got %v", ids)
	}

	var decisionCount int
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE type = 'decision'`).Scan(&decisionCount)
	}); err != nil {
		t.Fatalf("counting decision nodes: %v", err)
	}
	if decisionCount != 3 {
		t.Errorf("decision node count = %d, want 3", decisionCount)
	}

	for i, id := range ids {
		var got *nodes.Decision
		if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
			var err error
			got, err = nodes.GetDecision(ctx, tx, id)
			return err
		}); err != nil {
			t.Fatalf("GetDecision(%s): %v", id, err)
		}
		if got.Title != decisions[i].Title {
			t.Errorf("decision %d Title = %q, want %q", i, got.Title, decisions[i].Title)
		}

		var in []edges.Edge
		if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
			var err error
			in, err = edges.Incoming(ctx, tx, id, edges.WithType(edges.EdgeTypeHasDecision))
			return err
		}); err != nil {
			t.Fatalf("edges.Incoming(%s): %v", id, err)
		}
		if len(in) != 3 {
			t.Errorf("decision %d has %d incoming has_decision edges, want 3 (run, bead, epic)", i, len(in))
		}
	}
}

// TestEnsureBeadReferenceNode_MissingFactsFailsLoud pins that a bead never
// seen before, but missing one of the sourceable facts a reference node
// requires, halts with a KERNL DISPATCH FAILURE naming what is missing
// rather than substituting a placeholder (criterion in "What to build" #3).
func TestEnsureBeadReferenceNode_MissingFactsFailsLoud(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return ensureBeadReferenceNode(ctx, tx, BeadRef{ID: "kb-incomplete-1"}, nodes.Author{Name: "test"})
	})
	if err == nil {
		t.Fatal("expected an error: Title, TrackerKind and RepoPath are all empty")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error %q does not carry the KERNL DISPATCH FAILURE marker", err.Error())
	}
	for _, want := range []string{"title", "tracker kind", "repository path"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name missing %q", err.Error(), want)
		}
	}
}

// --- trackerKindFromCommand: a bead reference node's tracker_kind is
// permanent (the node is never updated), so an unrecognized tracker must
// fail loud rather than be recorded. ---

func TestTrackerKindFromCommand_RejectsUnsupportedKind(t *testing.T) {
	// "kno" is knots' real binary name (see backend.knownMemoryManagers) -
	// a genuinely supported tracker for everything else in this codebase,
	// and exactly why this must be an explicit allow-list rather than "any
	// non-empty first token": a plausible-looking tracker command must not
	// silently pass through into an immutable node.
	_, err := trackerKindFromCommand("kno -C '/repo'")
	if err == nil {
		t.Fatal("expected an error: knots is not a supported bead reference tracker kind")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error %q does not carry the KERNL DISPATCH FAILURE marker", err.Error())
	}
	if !strings.Contains(err.Error(), "kno") {
		t.Errorf("error %q does not name the rejected kind %q", err.Error(), "kno")
	}
}

func TestTrackerKindFromCommand_RejectsEmptyCommand(t *testing.T) {
	_, err := trackerKindFromCommand("")
	if err == nil {
		t.Fatal("expected an error: an empty tracker command names no kind at all")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error %q does not carry the KERNL DISPATCH FAILURE marker", err.Error())
	}
}

func TestTrackerKindFromCommand_AcceptsBrAndBd(t *testing.T) {
	for _, cmd := range []string{"br --db '/repo/.beads/beads.db'", "bd -C '/repo'"} {
		kind, err := trackerKindFromCommand(cmd)
		if err != nil {
			t.Errorf("trackerKindFromCommand(%q): %v", cmd, err)
		}
		want := strings.Fields(cmd)[0]
		if kind != want {
			t.Errorf("trackerKindFromCommand(%q) = %q, want %q", cmd, kind, want)
		}
	}
}

// TestEnsureBeadReferenceNode_ConcurrentCreatesConvergeOnOneRow is finding 2:
// every child bead of the same epic calls ensureBeadReferenceNode for the
// same epic id, each through its own *graph.Graph, the way
// recordDecisionIfGateType actually opens one per call in production. A
// check-then-insert implementation races across those separate connections -
// two callers can both observe absence before either commits, and only one
// of their inserts then succeeds, failing the other's whole decision-record
// write for a reason that has nothing to do with what that sibling did.
//
// This spawns genuinely concurrent writers (separate goroutines, separate
// *graph.Graph instances, released together) rather than a sequential loop,
// because a sequential loop cannot observe the interleaving the race
// requires: two callers' SELECT both running before either callers' INSERT
// commits.
func TestEnsureBeadReferenceNode_ConcurrentCreatesConvergeOnOneRow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "graph.db")

	// Apply the schema once, sequentially, before the concurrent writers
	// start - concurrent first-opens would otherwise race the migration
	// runner itself, a different (and uninteresting) race than the one
	// this test exists to exercise.
	seedGraph, err := graph.Open(context.Background(), graph.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("graph.Open (seed): %v", err)
	}
	if err := seedGraph.Close(); err != nil {
		t.Fatalf("closing seed graph: %v", err)
	}

	ref := BeadRef{ID: "kb-epic-concurrent-1", Title: "epic bead", TrackerKind: "br", RepoPath: "/repo"}

	const writers = 16
	graphs := make([]*graph.Graph, writers)
	for i := range graphs {
		g, err := graph.Open(context.Background(), graph.Config{Path: dbPath})
		if err != nil {
			t.Fatalf("graph.Open (writer %d): %v", i, err)
		}
		graphs[i] = g
		t.Cleanup(func() { _ = g.Close() })
	}

	var wg sync.WaitGroup
	errs := make([]error, writers)
	start := make(chan struct{})
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = graphs[i].DoWrite(context.Background(), func(tx *graph.WriteTx) error {
				return ensureBeadReferenceNode(context.Background(), tx, ref, nodes.Author{Name: "test"})
			})
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("writer %d: %v", i, err)
		}
	}

	verify, err := graph.Open(context.Background(), graph.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("graph.Open (verify): %v", err)
	}
	defer verify.Close()

	var count int
	if err := verify.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id = ?`, ref.ID).Scan(&count)
	}); err != nil {
		t.Fatalf("counting nodes: %v", err)
	}
	if count != 1 {
		t.Errorf("node count for %s after %d concurrent creators = %d, want 1", ref.ID, writers, count)
	}
}

// TestWriteDecisionRecordNode_PartialFailureRollsBackEverything is criterion
// 4: a failure part-way through the transaction must leave nothing behind.
// The bead's own reference node is created first and would succeed on its
// own; the epic's reference node is deliberately missing its Title, so
// ensureBeadReferenceNode fails loud on it a moment later, inside the same
// transaction. A reference node for the bead surviving without its epic, or
// without the Decision it was supposed to explain, would be worse than the
// original failure - it would look like a completed link.
func TestWriteDecisionRecordNode_PartialFailureRollsBackEverything(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	bead := BeadRef{ID: "kb-partial-1", Title: "bead", TrackerKind: "bd", RepoPath: "/repo"}
	epic := BeadRef{ID: "kb-epic-partial-1", TrackerKind: "bd", RepoPath: "/repo"} // Title missing on purpose

	// The run is seeded against an unrelated bead on purpose. Seeding it with
	// this test's own bead would create that bead's reference node in an
	// earlier, already-committed transaction, and the survivor count below
	// would then be counting that rather than a failed rollback.
	runID := seedRunWithBeads(t, g, "partial", []BeadRef{
		{ID: "kb-unrelated-partial-1", Title: "unrelated", TrackerKind: "bd", RepoPath: "/repo"},
	})

	_, err := WriteDecisionRecordNode(ctx, g, []backend.DecisionRecordEntry{wellFormedDecisionEntry}, bead, epic, runID)
	if err == nil {
		t.Fatal("expected an error: the epic BeadRef is missing a title")
	}

	var count int
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id IN (?, ?)`, bead.ID, epic.ID).Scan(&count)
	}); err != nil {
		t.Fatalf("counting nodes: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 nodes surviving the rollback (bead ref, epic ref, decision), found %d", count)
	}

	var decisionCount int
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE type = 'decision'`).Scan(&decisionCount)
	}); err != nil {
		t.Fatalf("counting decision nodes: %v", err)
	}
	if decisionCount != 0 {
		t.Errorf("expected no decision node to survive a rolled-back transaction, found %d", decisionCount)
	}
}

// --- recordDecisionIfGateType through the Config-derived graph path, the
// same one DriveBeadToTerminal calls in production. ---

func TestRecordDecisionIfGateType_FullPipelineSucceedsWhenNodesExist(t *testing.T) {
	vaultRoot := t.TempDir()
	cfg := &config.Config{Vault: config.VaultConfig{Root: vaultRoot}}

	graphPath, err := graphDBFilePath(cfg)
	if err != nil {
		t.Fatalf("graphDBFilePath: %v", err)
	}

	beadID, epicID := "kb-child-2", "kb-epic-2"
	be := newPersistingBackend()
	be.beads[beadID] = &backend.Bead{ID: beadID, Title: "child bead"}
	be.beads[epicID] = &backend.Bead{ID: epicID, Title: "epic bead"}

	artifactDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(artifactDir, "decision-record.json"), []byte(wellFormedDecisionRecordJSON), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	wf := backend.WorkflowDescriptor{
		ExitGates: map[string][]backend.WorkflowExitGate{
			"implementation": {{Type: "decision_record", Path: "<artifact_dir>/decision-record.json"}},
		},
	}
	runID := seedRunAtPath(t, graphPath, "epic bead", []BeadRef{
		{ID: beadID, Title: "child bead", TrackerKind: "bd", RepoPath: "/repo"},
		{ID: epicID, Title: "epic bead", TrackerKind: "bd", RepoPath: "/repo"},
	})

	gateCtx := backend.ExitGateContext{FromState: "implementation", ArtifactDir: artifactDir, BeadID: beadID}
	deps := DriveBeadDeps{Config: cfg, Backend: be, RepoPath: "/repo", TrackerCommand: "bd", RunID: runID}

	if err := recordDecisionIfGateType(context.Background(), wf, gateCtx, deps, be.beads[beadID], epicID, be.beads[epicID].Title); err != nil {
		t.Fatalf("recordDecisionIfGateType: %v", err)
	}

	g2, err := graph.Open(context.Background(), graph.Config{Path: graphPath})
	if err != nil {
		t.Fatalf("re-opening graph: %v", err)
	}
	defer g2.Close()
	var decisions []*nodes.Decision
	if err := g2.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var err error
		decisions, err = nodes.ListDecisions(context.Background(), tx, nodes.DecisionFilter{Tags: []string{PhaseThreeDecisionTag}})
		return err
	}); err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision written to %s, got %d", graphPath, len(decisions))
	}
}

// fakeHeadSHAResolver is a named fake HeadSHAResolver: a fixed answer with
// no process spawned, so a DriveBeadToTerminal test never shells out to the
// host's real git binary just to fill in a ledger/gate-context field it
// does not otherwise depend on (AGENTS.md §4: unit tests must not touch the
// host).
type fakeHeadSHAResolver struct{ sha string }

func (f fakeHeadSHAResolver) HeadSHA(worktree string) string { return f.sha }

// --- DriveBeadToTerminal wiring: the two gatePassed call sites this bead
// added, exercised end to end rather than as an isolated unit. ---

// TestDriveBeadToTerminal_FailedGateNeverWritesDecisionNode is criterion 6 at
// the orchestration boundary: recordDecisionIfGateType lives entirely inside
// the gatePassed branch in drive_bead.go, so a gate failure (here: no
// decision-record.json at all) must mean the graph is never even opened. The
// bead is left blocked, and no ".kernl-graph.db" is created under the vault
// root this test dedicates to it.
func TestDriveBeadToTerminal_FailedGateNeverWritesDecisionNode(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{
		ID:        "kb-1",
		ParentID:  "kb-epic-1",
		State:     "implementation",
		ProfileID: "autopilot_with_pr",
	}
	// Registered so the epic pre-fetch (which runs before the agent, for
	// any decision_record-gated stage, regardless of whether the record
	// file will later be found - see the loop in DriveBeadToTerminal)
	// succeeds; this test's own point is the gate failing on a missing
	// record, not the epic being unreachable.
	be.beads["kb-epic-1"] = &backend.Bead{ID: "kb-epic-1", Title: "epic bead"}

	cfg := newDriveTestConfig()
	vaultRoot := t.TempDir()
	cfg.Vault = config.VaultConfig{Root: vaultRoot}

	driver := &scriptedDriver{be: be}
	res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand:  "bd",
		StateDir:        t.TempDir(),
		VerifyCommand:   "bin/ci",
		Backend:         be,
		Driver:          driver,
		Config:          cfg,
		BeadID:          "kb-1",
		RepoPath:        "/tmp/repo",
		Worktree:        "/tmp/worktree",
		MaxStages:       16,
		HeadSHAResolver: fakeHeadSHAResolver{sha: "deadbeef"},
	})
	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}
	if res.Success {
		t.Fatalf("expected failure (gate missing decision-record.json), got success: %+v", res)
	}
	if bd, _ := be.Get("kb-1", ""); bd.State != "blocked" {
		t.Errorf("expected bead blocked by the failed gate, got state %q", bd.State)
	}

	// relatedDecisionsForPrompt (FetchRelevantDecisions's caller) does open
	// the graph unconditionally before the agent runs, on every native-flow
	// stage - but only once a graph db file already exists; a vault that has
	// never recorded a decision has nothing to read, and it stat-checks for
	// that BEFORE calling the directory-creating path resolver or
	// graph.Open (see that function's own doc comment). So the original
	// invariant still holds here: nothing upstream of the failed gate ever
	// created a db in this fresh vault, and recordDecisionIfGateType's own
	// write path never ran for a gate that failed either.
	graphPath := filepath.Join(vaultRoot, ".kernl-graph.db")
	if _, statErr := os.Stat(graphPath); statErr == nil {
		t.Errorf("graph db %s exists, but the gate never passed - recordDecisionIfGateType must not have run", graphPath)
	}
}

// TestDriveBeadToTerminal_EpicFetchFailsBeforeAgentRuns is finding 3: an
// epic that cannot be fetched must not discard a successful agent run. Here
// the epic is simply never registered in the fake backend (standing in for
// a transient tracker failure or a concurrently deleted epic) - the point
// is not the exact cause, it is that DriveBeadToTerminal discovers this
// before spawning the agent for a stage that could not have completed
// anyway, rather than after, the way a fetch made only once the gate had
// already passed would.
func TestDriveBeadToTerminal_EpicFetchFailsBeforeAgentRuns(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{
		ID:        "kb-1",
		Title:     "child bead",
		ParentID:  "kb-epic-unreachable",
		State:     "implementation",
		ProfileID: "autopilot_with_pr",
	}
	// kb-epic-unreachable is deliberately never registered.

	cfg := newDriveTestConfig()
	vaultRoot := t.TempDir()
	cfg.Vault = config.VaultConfig{Root: vaultRoot}

	// A well-formed record is in place, exactly where the gate expects it -
	// if the agent were allowed to run, the gate would pass. This is what
	// isolates the failure to the epic fetch specifically, not to a missing
	// record (already covered by
	// TestDriveBeadToTerminal_FailedGateNeverWritesDecisionNode).
	stateDir := t.TempDir()
	artifactDir := filepath.Join(stateDir, "run", "kb-epic-unreachable", "kb-1")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "decision-record.json"), []byte(wellFormedDecisionRecordJSON), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	driver := &scriptedDriver{be: be}
	res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand:  "bd",
		StateDir:        stateDir,
		VerifyCommand:   "bin/ci",
		Backend:         be,
		Driver:          driver,
		Config:          cfg,
		BeadID:          "kb-1",
		RepoPath:        "/tmp/repo",
		Worktree:        "/tmp/worktree",
		MaxStages:       16,
		HeadSHAResolver: fakeHeadSHAResolver{sha: "deadbeef"},
	})
	if err == nil {
		t.Fatal("expected an error: the epic could not be fetched")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error %q does not carry the KERNL DISPATCH FAILURE marker", err.Error())
	}
	if !strings.Contains(err.Error(), "kb-epic-unreachable") {
		t.Errorf("error %q does not name the unreachable epic", err.Error())
	}
	if res.Success {
		t.Errorf("expected failure, got success: %+v", res)
	}
	if driver.calls != 0 {
		t.Errorf("driver.calls = %d, want 0 - the agent must never run when its epic cannot be fetched, even though its record was ready and would have passed the gate", driver.calls)
	}

	graphPath := filepath.Join(vaultRoot, ".kernl-graph.db")
	if _, statErr := os.Stat(graphPath); statErr == nil {
		t.Errorf("graph db %s exists, but the epic fetch failed before any write was attempted", graphPath)
	}
}

// TestDriveBeadToTerminal_PassedDecisionRecordGateWritesQueryableNode is the
// positive counterpart: a well-formed record plus a graph that already has
// nodes for the bead and epic (see WriteDecisionRecordNode's own doc comment
// on why that precondition is not automatic today) drives the bead all the
// way to shipped, and leaves a Decision node behind, tagged and linked, in
// the exact database file the audit API reads.
func TestDriveBeadToTerminal_PassedDecisionRecordGateWritesQueryableNode(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-2"] = &backend.Bead{
		ID:        "kb-2",
		Title:     "child bead",
		ParentID:  "kb-epic-2",
		State:     "implementation",
		ProfileID: "autopilot_with_pr",
	}
	// The epic is fetched by DriveBeadToTerminal's own loop, before the
	// agent runs, to build its reference node's title (see finding 3's
	// fix in the loop, right after epicID is resolved).
	be.beads["kb-epic-2"] = &backend.Bead{ID: "kb-epic-2", Title: "epic bead"}

	cfg := newDriveTestConfig()
	vaultRoot := t.TempDir()
	cfg.Vault = config.VaultConfig{Root: vaultRoot}

	graphPath, err := graphDBFilePath(cfg)
	if err != nil {
		t.Fatalf("graphDBFilePath: %v", err)
	}

	stateDir := t.TempDir()
	artifactDir := filepath.Join(stateDir, "run", "kb-epic-2", "kb-2")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "decision-record.json"), []byte(wellFormedDecisionRecordJSON), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	runID := seedRunAtPath(t, graphPath, "epic bead", []BeadRef{
		{ID: "kb-2", Title: "child bead", TrackerKind: "bd", RepoPath: "/tmp/repo"},
		{ID: "kb-epic-2", Title: "epic bead", TrackerKind: "bd", RepoPath: "/tmp/repo"},
	})

	driver := &scriptedDriver{be: be}
	res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand:  "bd",
		StateDir:        stateDir,
		VerifyCommand:   "bin/ci",
		Backend:         be,
		Driver:          driver,
		Config:          cfg,
		BeadID:          "kb-2",
		RepoPath:        "/tmp/repo",
		Worktree:        "/tmp/worktree",
		MaxStages:       16,
		RunID:           runID,
		HeadSHAResolver: fakeHeadSHAResolver{sha: "deadbeef"},
	})
	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}

	g2, err := graph.Open(context.Background(), graph.Config{Path: graphPath})
	if err != nil {
		t.Fatalf("re-opening graph: %v", err)
	}
	defer g2.Close()
	var decisions []*nodes.Decision
	if err := g2.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var err error
		decisions, err = nodes.ListDecisions(context.Background(), tx, nodes.DecisionFilter{Tags: []string{PhaseThreeDecisionTag}})
		return err
	}); err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision written by the real orchestration path, got %d", len(decisions))
	}
}

// TestDriveBeadToTerminal_SubprocessDecisionRecordGateWritesQueryableNode
// covers the subprocess branch's own recordDecisionIfGateType call
// (drive_bead.go's subprocess flow, mirroring the native flow's), which
// nothing previously exercised: a test that never sets AgentStateStore or a
// "subprocess"-kind stage cannot reach that branch at all, so deleting the
// call there would still leave every other test in this file green. This
// one registers a real subprocess-kind workflow (the same
// backend.RegisterWorkflow pattern drive_bead_test.go's own subprocess
// tests use) with a decision_record gate on its one action state, runs a
// real (but trivial) subprocess script, and asserts a Decision node lands
// in the graph exactly as the native-flow test proves for the native path.
func TestDriveBeadToTerminal_SubprocessDecisionRecordGateWritesQueryableNode(t *testing.T) {
	scriptPath := createTestPythonScript(t, `#!/usr/bin/env python3
import sys
import json

req = json.load(sys.stdin)
print(json.dumps({"context_payload": req.get("context_payload", "")}))
`)

	backend.ClearWorkflowRegistry()
	t.Cleanup(backend.ClearWorkflowRegistry)
	customWf := backend.WorkflowDescriptor{
		ID:             "subprocess-decision-record",
		InitialState:   "ready_for_implementation",
		States:         []string{"ready_for_implementation", "implementation", "shipped"},
		TerminalStates: []string{"shipped"},
		Transitions: []backend.WorkflowTransition{
			{From: "ready_for_implementation", To: "implementation"},
			{From: "implementation", To: "shipped"},
		},
		QueueStates:  []string{"ready_for_implementation"},
		ActionStates: []string{"implementation"},
		QueueActions: map[string]string{"ready_for_implementation": "implementation"},
		ExitGates: map[string][]backend.WorkflowExitGate{
			"implementation": {{Type: "decision_record", Path: "<artifact_dir>/decision-record.json"}},
		},
		Stages: map[string]backend.StageContract{
			"implementation": {
				Role: "subprocess",
				Kind: "subprocess",
				Subprocess: &backend.SubprocessSpec{
					Command: []string{scriptPath},
				},
			},
		},
	}
	backend.RegisterWorkflow(customWf)

	be := newPersistingBackend()
	be.beads["kb-sub-1"] = &backend.Bead{
		ID:        "kb-sub-1",
		Title:     "subprocess bead",
		ParentID:  "kb-epic-sub-1",
		State:     "ready_for_implementation",
		ProfileID: "subprocess-decision-record",
	}
	// The epic is fetched by DriveBeadToTerminal's own loop, before the
	// agent runs, to build its reference node's title (see finding 3's
	// fix in the loop, right after epicID is resolved).
	be.beads["kb-epic-sub-1"] = &backend.Bead{ID: "kb-epic-sub-1", Title: "epic bead"}

	cfg := newDriveTestConfig()
	vaultRoot := t.TempDir()
	cfg.Vault = config.VaultConfig{Root: vaultRoot}

	graphPath, err := graphDBFilePath(cfg)
	if err != nil {
		t.Fatalf("graphDBFilePath: %v", err)
	}

	stateDir := t.TempDir()
	artifactDir := filepath.Join(stateDir, "run", "kb-epic-sub-1", "kb-sub-1")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "decision-record.json"), []byte(wellFormedDecisionRecordJSON), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	storeDir := t.TempDir()
	store, err := workflow.NewAgentStateStore(storeDir)
	if err != nil {
		t.Fatalf("NewAgentStateStore: %v", err)
	}
	if err := store.Save("kb-sub-1", workflow.AgentRuntime{ContextPayload: "initial"}); err != nil {
		t.Fatalf("store.Save: %v", err)
	}

	runID := seedRunAtPath(t, graphPath, "epic bead", []BeadRef{
		{ID: "kb-sub-1", Title: "sub bead", TrackerKind: "bd", RepoPath: "/tmp/repo"},
		{ID: "kb-epic-sub-1", Title: "epic bead", TrackerKind: "bd", RepoPath: "/tmp/repo"},
	})

	driver := &scriptedDriver{be: be}
	res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand:  "bd",
		StateDir:        stateDir,
		VerifyCommand:   "bin/ci",
		Backend:         be,
		Driver:          driver,
		Config:          cfg,
		BeadID:          "kb-sub-1",
		RepoPath:        "/tmp/repo",
		Worktree:        t.TempDir(),
		AgentStateStore: store,
		MaxStages:       16,
		RunID:           runID,
		HeadSHAResolver: fakeHeadSHAResolver{sha: "deadbeef"},
	})
	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}
	if bd, _ := be.Get("kb-sub-1", ""); bd.State != "shipped" {
		t.Fatalf("expected final state shipped, got %q", bd.State)
	}

	g2, err := graph.Open(context.Background(), graph.Config{Path: graphPath})
	if err != nil {
		t.Fatalf("re-opening graph: %v", err)
	}
	defer g2.Close()
	var decisions []*nodes.Decision
	if err := g2.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var err error
		decisions, err = nodes.ListDecisions(context.Background(), tx, nodes.DecisionFilter{Tags: []string{PhaseThreeDecisionTag}})
		return err
	}); err != nil {
		t.Fatalf("ListDecisions: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision written by the subprocess orchestration path, got %d", len(decisions))
	}
}
