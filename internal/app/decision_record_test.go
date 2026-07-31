package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

const wellFormedDecisionRecord = "## Decision\n\n" +
	"Use edges.EdgeTypeHasDecision for the bead/epic link.\n\n" +
	"## Options Considered\n\n" +
	"1. A bare string literal.\n2. A new typed constant.\n\n" +
	"## Trade-offs\n\n" +
	"A typed constant is one more name to learn, but closes the set.\n\n" +
	"## Rationale\n\n" +
	"Matches the existing closed set (related, depends_on, blocks, part_of, links_to).\n"

// --- decisionFromRecordSections / SplitDecisionBody: the four parts stay
// individually recoverable (criterion 1). ---

func TestDecisionFromRecordSections_FourPartsIndividuallyRecoverable(t *testing.T) {
	sections := backend.DecisionRecordSectionBodies(wellFormedDecisionRecord)
	if len(sections) != 4 {
		t.Fatalf("fixture produced %d sections, want 4: %v", len(sections), sections)
	}

	d := decisionFromRecordSections(sections, time.Now())

	if !strings.Contains(d.Context, "Use edges.EdgeTypeHasDecision") {
		t.Errorf("Context = %q, want the decision section's full text", d.Context)
	}
	if d.Title == "" {
		t.Error("Title is empty, want the decision section's first line")
	}
	if d.Outcome != strings.TrimSpace(sections["rationale"]) {
		t.Errorf("Outcome = %q, want exactly the rationale section: %q", d.Outcome, sections["rationale"])
	}

	options, tradeOffs, ok := SplitDecisionBody(d.Body)
	if !ok {
		t.Fatalf("SplitDecisionBody could not split Body built by buildDecisionBody: %q", d.Body)
	}
	if options != strings.TrimSpace(sections["options_considered"]) {
		t.Errorf("recovered options = %q, want %q", options, sections["options_considered"])
	}
	if tradeOffs != strings.TrimSpace(sections["trade_offs"]) {
		t.Errorf("recovered trade-offs = %q, want %q", tradeOffs, sections["trade_offs"])
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

// TestDecisionFromRecordSections_ImpactOnUseIsNilNotWritten pins that mapping
// a freshly-parsed record never sets ImpactOnUse - it is written later by
// the run's composer, not at decision time (see AGENTS.md and Decision's own
// doc comment). Inverting this (setting it to a non-nil default) would
// silently mark every record as already composed on day one.
func TestDecisionFromRecordSections_ImpactOnUseIsNilNotWritten(t *testing.T) {
	sections := backend.DecisionRecordSectionBodies(wellFormedDecisionRecord)
	d := decisionFromRecordSections(sections, time.Now())
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
		ExitGates: map[string]backend.WorkflowExitGate{
			"implementation": {Type: "commit_marker", Path: "stage: implementation"},
		},
	}
	gateCtx := backend.ExitGateContext{FromState: "implementation"}

	// cfg is deliberately nil: a no-op must return before ever touching it,
	// so passing nil doubles as proof the function never even tries to open
	// a graph for a gate type it does not own.
	if err := recordDecisionIfGateType(context.Background(), wf, gateCtx, nil, "kb-1", "kb-1"); err != nil {
		t.Fatalf("recordDecisionIfGateType: %v", err)
	}
}

func TestRecordDecisionIfGateType_NoOpWhenNoGateForState(t *testing.T) {
	wf := backend.WorkflowDescriptor{ExitGates: map[string]backend.WorkflowExitGate{}}
	gateCtx := backend.ExitGateContext{FromState: "implementation"}

	if err := recordDecisionIfGateType(context.Background(), wf, gateCtx, nil, "kb-1", "kb-1"); err != nil {
		t.Fatalf("recordDecisionIfGateType: %v", err)
	}
}

// --- readDecisionRecordSections: re-read failures halt loudly, they never
// silently skip (part of criterion 7). ---

func TestReadDecisionRecordSections_FileMissingFails(t *testing.T) {
	gate := backend.WorkflowExitGate{Type: "decision_record", Path: "<artifact_dir>/decision-record.md"}
	gateCtx := backend.ExitGateContext{FromState: "implementation", ArtifactDir: t.TempDir(), BeadID: "kb-1"}

	_, err := readDecisionRecordSections(gate, gateCtx, "kb-1")
	if err == nil {
		t.Fatal("expected an error for a decision record that does not exist on disk")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error %q does not carry the KERNL DISPATCH FAILURE marker", err.Error())
	}
}

func TestReadDecisionRecordSections_SectionEmptyOnRereadFails(t *testing.T) {
	dir := t.TempDir()
	// Missing the "## Rationale" section entirely - simulates the record
	// changing, or the gate and this read disagreeing, between the gate's
	// own read and this one.
	incomplete := "## Decision\n\nUse X.\n\n## Options Considered\n\nX or Y.\n\n## Trade-offs\n\nX is simpler.\n"
	if err := os.WriteFile(filepath.Join(dir, "decision-record.md"), []byte(incomplete), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	gate := backend.WorkflowExitGate{Type: "decision_record", Path: "<artifact_dir>/decision-record.md"}
	gateCtx := backend.ExitGateContext{FromState: "implementation", ArtifactDir: dir, BeadID: "kb-1"}

	_, err := readDecisionRecordSections(gate, gateCtx, "kb-1")
	if err == nil {
		t.Fatal("expected an error for a record missing the rationale section on re-read")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error %q does not carry the KERNL DISPATCH FAILURE marker", err.Error())
	}
	if !strings.Contains(err.Error(), "rationale") {
		t.Errorf("error %q does not name the missing section", err.Error())
	}
}

func TestReadDecisionRecordSections_WellFormedRecordExtractsAllFour(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "decision-record.md"), []byte(wellFormedDecisionRecord), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	gate := backend.WorkflowExitGate{Type: "decision_record", Path: "<artifact_dir>/decision-record.md"}
	gateCtx := backend.ExitGateContext{FromState: "implementation", ArtifactDir: dir, BeadID: "kb-1"}

	sections, err := readDecisionRecordSections(gate, gateCtx, "kb-1")
	if err != nil {
		t.Fatalf("readDecisionRecordSections: %v", err)
	}
	for _, key := range decisionRecordRequiredKeys {
		if sections[key] == "" {
			t.Errorf("section %q is empty, want content", key)
		}
	}
}

// --- WriteDecisionRecordNode: linked to both bead and epic via the typed
// edge constant (criterion 2), and a write failure halts loudly rather than
// dropping the reasoning on the floor (criterion 7). ---

func TestWriteDecisionRecordNode_LinksBeadAndEpic(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	beadID, epicID := seedStandInNodes(t, g, "kb-child-1", "kb-epic-1")

	sections := backend.DecisionRecordSectionBodies(wellFormedDecisionRecord)
	id, err := WriteDecisionRecordNode(ctx, g, sections, beadID, epicID)
	if err != nil {
		t.Fatalf("WriteDecisionRecordNode: %v", err)
	}

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
	if !srcs[beadID] {
		t.Errorf("no has_decision edge from bead %s to decision %s", beadID, id)
	}
	if !srcs[epicID] {
		t.Errorf("no has_decision edge from epic %s to decision %s", epicID, id)
	}
	if len(in) != 2 {
		t.Errorf("expected exactly 2 incoming has_decision edges, got %d: %+v", len(in), in)
	}
}

// TestWriteDecisionRecordNode_RetryConvergesOnOneNode covers the case a
// bare "write once" implementation gets wrong: the graph write succeeds and
// commits, but the caller's own next step (advancing the bead's tracker
// state) fails for an unrelated reason, and the whole run is retried from
// the top. The retry re-reads the same decision-record.md and calls
// WriteDecisionRecordNode again with identical sections, beadID and epicID.
// A writer that mints a fresh random ID every call would leave two Decision
// nodes and four has_decision edges behind for one real decision; this one
// must converge on exactly one of each.
func TestWriteDecisionRecordNode_RetryConvergesOnOneNode(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	beadID, epicID := seedStandInNodes(t, g, "kb-retry-1", "kb-epic-retry-1")

	sections := backend.DecisionRecordSectionBodies(wellFormedDecisionRecord)

	firstID, err := WriteDecisionRecordNode(ctx, g, sections, beadID, epicID)
	if err != nil {
		t.Fatalf("WriteDecisionRecordNode (first attempt): %v", err)
	}
	secondID, err := WriteDecisionRecordNode(ctx, g, sections, beadID, epicID)
	if err != nil {
		t.Fatalf("WriteDecisionRecordNode (retry): %v", err)
	}

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

	var in []edges.Edge
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		in, err = edges.Incoming(ctx, tx, firstID, edges.WithType(edges.EdgeTypeHasDecision))
		return err
	}); err != nil {
		t.Fatalf("edges.Incoming: %v", err)
	}
	if len(in) != 2 {
		t.Errorf("has_decision edge count = %d after 2 identical writes, want 2 (one bead, one epic)", len(in))
	}
}

func TestWriteDecisionRecordNode_SameBeadAndEpicWritesOneEdge(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	beadID, _ := seedStandInNodes(t, g, "kb-standalone-1", "")

	sections := backend.DecisionRecordSectionBodies(wellFormedDecisionRecord)
	id, err := WriteDecisionRecordNode(ctx, g, sections, beadID, beadID)
	if err != nil {
		t.Fatalf("WriteDecisionRecordNode: %v", err)
	}

	var in []edges.Edge
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		in, err = edges.Incoming(ctx, tx, id, edges.WithType(edges.EdgeTypeHasDecision))
		return err
	}); err != nil {
		t.Fatalf("edges.Incoming: %v", err)
	}
	if len(in) != 1 {
		t.Errorf("expected 1 edge when bead and epic are the same node, got %d: %+v", len(in), in)
	}
}

// TestWriteDecisionRecordNode_FailsLoudWhenBeadIsNotAGraphNode documents a
// real gap: orchestrator bead/epic IDs (br/bd tracker IDs) are not mirrored
// into this graph as nodes anywhere in this codebase (nodes.Task and
// nodes.Project are a distinct, human-authored concept - see task.go's own
// doc comment). edges.Create requires both ends of an edge to already exist
// as node rows, so linking a Decision to a real, un-mirrored bead ID fails.
// That failure must surface as a halting KERNL DISPATCH FAILURE (criterion
// 7), not vanish - this test pins that it does, and stands as a marker for
// whoever picks up bridging beads into the graph next.
func TestWriteDecisionRecordNode_FailsLoudWhenBeadIsNotAGraphNode(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	sections := backend.DecisionRecordSectionBodies(wellFormedDecisionRecord)
	_, err := WriteDecisionRecordNode(ctx, g, sections, "kb-does-not-exist", "kb-does-not-exist")
	if err == nil {
		t.Fatal("expected an error: neither the bead nor the epic exists as a graph node")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error %q does not carry the KERNL DISPATCH FAILURE marker", err.Error())
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
	g, err := graph.Open(context.Background(), graph.Config{Path: graphPath})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	beadID, epicID := seedStandInNodes(t, g, "kb-child-2", "kb-epic-2")
	if err := g.Close(); err != nil {
		t.Fatalf("closing seed graph: %v", err)
	}

	artifactDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(artifactDir, "decision-record.md"), []byte(wellFormedDecisionRecord), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	wf := backend.WorkflowDescriptor{
		ExitGates: map[string]backend.WorkflowExitGate{
			"implementation": {Type: "decision_record", Path: "<artifact_dir>/decision-record.md"},
		},
	}
	gateCtx := backend.ExitGateContext{FromState: "implementation", ArtifactDir: artifactDir, BeadID: beadID}

	if err := recordDecisionIfGateType(context.Background(), wf, gateCtx, cfg, beadID, epicID); err != nil {
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

// seedStandInNodes creates task nodes carrying the given IDs to stand in for
// a bead and (optionally) an epic, purely so edges.Create's src/dst
// existence check has something to point at - the same technique
// internal/api/audit_test.go already uses for the pre-existing audit-log
// edge tests. If epicID is "", only the bead stand-in is created and epicID
// is returned equal to beadID (the standalone-bead case).
func seedStandInNodes(t *testing.T, g *graph.Graph, beadID, epicID string) (string, string) {
	t.Helper()
	ctx := context.Background()
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if _, err := nodes.CreateTask(ctx, tx, nodes.Task{ID: beadID, Title: "bead stand-in"}, nodes.Author{Name: "test"}); err != nil {
			return err
		}
		if epicID != "" && epicID != beadID {
			if _, err := nodes.CreateTask(ctx, tx, nodes.Task{ID: epicID, Title: "epic stand-in"}, nodes.Author{Name: "test"}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seeding stand-in nodes: %v", err)
	}
	if epicID == "" {
		return beadID, beadID
	}
	return beadID, epicID
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
// decision-record.md at all) must mean the graph is never even opened. The
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
		t.Fatalf("expected failure (gate missing decision-record.md), got success: %+v", res)
	}
	if bd, _ := be.Get("kb-1", ""); bd.State != "blocked" {
		t.Errorf("expected bead blocked by the failed gate, got state %q", bd.State)
	}

	graphPath := filepath.Join(vaultRoot, ".kernl-graph.db")
	if _, statErr := os.Stat(graphPath); statErr == nil {
		t.Errorf("graph db %s exists, but the gate never passed - recordDecisionIfGateType must not have run", graphPath)
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
		ParentID:  "kb-epic-2",
		State:     "implementation",
		ProfileID: "autopilot_with_pr",
	}

	cfg := newDriveTestConfig()
	vaultRoot := t.TempDir()
	cfg.Vault = config.VaultConfig{Root: vaultRoot}

	graphPath, err := graphDBFilePath(cfg)
	if err != nil {
		t.Fatalf("graphDBFilePath: %v", err)
	}
	g, err := graph.Open(context.Background(), graph.Config{Path: graphPath})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	seedStandInNodes(t, g, "kb-2", "kb-epic-2")
	if err := g.Close(); err != nil {
		t.Fatalf("closing seed graph: %v", err)
	}

	stateDir := t.TempDir()
	artifactDir := filepath.Join(stateDir, "run", "kb-epic-2", "kb-2")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "decision-record.md"), []byte(wellFormedDecisionRecord), 0o644); err != nil {
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
		BeadID:          "kb-2",
		RepoPath:        "/tmp/repo",
		Worktree:        "/tmp/worktree",
		MaxStages:       16,
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
		ExitGates: map[string]backend.WorkflowExitGate{
			"implementation": {Type: "decision_record", Path: "<artifact_dir>/decision-record.md"},
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
		ParentID:  "kb-epic-sub-1",
		State:     "ready_for_implementation",
		ProfileID: "subprocess-decision-record",
	}

	cfg := newDriveTestConfig()
	vaultRoot := t.TempDir()
	cfg.Vault = config.VaultConfig{Root: vaultRoot}

	graphPath, err := graphDBFilePath(cfg)
	if err != nil {
		t.Fatalf("graphDBFilePath: %v", err)
	}
	g, err := graph.Open(context.Background(), graph.Config{Path: graphPath})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	seedStandInNodes(t, g, "kb-sub-1", "kb-epic-sub-1")
	if err := g.Close(); err != nil {
		t.Fatalf("closing seed graph: %v", err)
	}

	stateDir := t.TempDir()
	artifactDir := filepath.Join(stateDir, "run", "kb-epic-sub-1", "kb-sub-1")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "decision-record.md"), []byte(wellFormedDecisionRecord), 0o644); err != nil {
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
