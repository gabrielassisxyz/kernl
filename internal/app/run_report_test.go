package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// fakeImpactComposer is a named fake ImpactComposer (AGENTS.md §4: named
// fakes, not inline anonymous mocks). It records every call it received, so
// a test can assert the composer was (or, for the already-composed and
// no-composer-configured cases, was NOT) invoked at all.
type fakeImpactComposer struct {
	response string
	err      error
	calls    []DecisionImpact
}

func (f *fakeImpactComposer) ComposeImpact(ctx context.Context, in DecisionImpact) (string, error) {
	f.calls = append(f.calls, in)
	if f.err != nil {
		return "", f.err
	}
	return f.response, nil
}

// seedRunWithBeads starts a workflow run with the given beads and returns
// its id - the same StartWorkflowRun call sites already exercise, reused
// here rather than re-deriving a second way to create a run node.
func seedRunWithBeads(t *testing.T, g *graph.Graph, title string, beads []BeadRef) string {
	t.Helper()
	runID, err := StartWorkflowRun(context.Background(), g, StartWorkflowRunInput{
		EntryPoint:     "epic run",
		Title:          title,
		WorkflowName:   "worker",
		Beads:          beads,
		RepoPath:       "/repo",
		TrackerCommand: "br --db '/repo/.beads/beads.db'",
		StartedAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("StartWorkflowRun: %v", err)
	}
	return runID
}

// writeTestDecision records a decision against bead/epic using the same
// WriteDecisionRecordNode path recordDecisionIfGateType calls in
// production, so a test decision has the exact shape (Body split into
// options/trade-offs, ImpactOnUse nil) a real one would.
func writeTestDecision(t *testing.T, g *graph.Graph, bead, epic BeadRef, record string) string {
	t.Helper()
	sections := backend.DecisionRecordSectionBodies(record)
	id, err := WriteDecisionRecordNode(context.Background(), g, sections, bead, epic)
	if err != nil {
		t.Fatalf("WriteDecisionRecordNode: %v", err)
	}
	return id
}

const secondDecisionRecord = "## Decision\n\n" +
	"Use TOML for the export manifest.\n\n" +
	"## Options Considered\n\n" +
	"1. JSON.\n2. TOML.\n\n" +
	"## Trade-offs\n\n" +
	"TOML reads better hand-edited; JSON has wider tooling.\n\n" +
	"## Rationale\n\n" +
	"No existing precedent either way; TOML matches the config file already in the repo.\n"

func baseReportInput(g *graph.Graph, stateDir, runID, epicID string) ComposeRunReportInput {
	started := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	return ComposeRunReportInput{
		Graph:      g,
		RunID:      runID,
		EntryPoint: "epic run",
		RepoPath:   "/repo",
		BaseBranch: "master",
		Status:     "completed",
		StartedAt:  started,
		FinishedAt: started.Add(5 * time.Minute),
		Beads: []BeadRunOutcome{
			{ID: "kb-epic-1", Title: "epic bead", FinalState: "shipped"},
			{ID: "kb-child-1", Title: "child bead", FinalState: "awaiting_integration"},
		},
		StateDir: stateDir,
		EpicID:   epicID,
	}
}

// --- criterion: the traversal finds a decision two edges out from the run,
// and does not pick up a decision belonging to a different run. ---

func TestComposeRunReport_FindsOwnDecisionNotAnotherRuns(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()

	epicA := BeadRef{ID: "kb-epic-a", Title: "epic A", TrackerKind: "br", RepoPath: "/repo"}
	childA := BeadRef{ID: "kb-child-a", Title: "child A", TrackerKind: "br", RepoPath: "/repo"}
	runA := seedRunWithBeads(t, g, "epic A", []BeadRef{epicA, childA})
	writeTestDecision(t, g, childA, epicA, wellFormedDecisionRecord)

	epicB := BeadRef{ID: "kb-epic-b", Title: "epic B", TrackerKind: "br", RepoPath: "/repo"}
	childB := BeadRef{ID: "kb-child-b", Title: "child B", TrackerKind: "br", RepoPath: "/repo"}
	seedRunWithBeads(t, g, "epic B", []BeadRef{epicB, childB})
	writeTestDecision(t, g, childB, epicB, secondDecisionRecord)

	in := baseReportInput(g, stateDir, runA, "kb-epic-a")
	path, err := ComposeRunReport(context.Background(), in)
	if err != nil {
		t.Fatalf("ComposeRunReport: %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	report := string(body)
	if !strings.Contains(report, "Use edges.EdgeTypeHasDecision") {
		t.Errorf("report is missing run A's own decision:\n%s", report)
	}
	if strings.Contains(report, "Use TOML for the export manifest") {
		t.Errorf("report leaked run B's decision:\n%s", report)
	}
}

// --- criterion: ImpactOnUse is written back to the node and the report
// carries it. ---

func TestComposeRunReport_WritesImpactBackAndIntoReport(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()

	epic := BeadRef{ID: "kb-epic-2", Title: "epic", TrackerKind: "br", RepoPath: "/repo"}
	child := BeadRef{ID: "kb-child-2", Title: "child", TrackerKind: "br", RepoPath: "/repo"}
	runID := seedRunWithBeads(t, g, "epic", []BeadRef{epic, child})
	decisionID := writeTestDecision(t, g, child, epic, wellFormedDecisionRecord)

	composer := &fakeImpactComposer{response: "Callers of the affected API now see a typed constant instead of a bare string."}
	in := baseReportInput(g, stateDir, runID, "kb-epic-2")
	in.Composer = composer

	path, err := ComposeRunReport(context.Background(), in)
	if err != nil {
		t.Fatalf("ComposeRunReport: %v", err)
	}
	if len(composer.calls) != 1 {
		t.Fatalf("composer called %d times, want 1", len(composer.calls))
	}
	// WriteDecisionRecordNode links both the child and its epic to the same
	// decision (see findRunDecisions' own doc comment on dedup), so either
	// title is a correct answer - the traversal order between two ran_bead
	// edges created in the same transaction is not a behavior this test
	// pins down.
	if bt := composer.calls[0].BeadTitle; bt != "epic" && bt != "child" {
		t.Errorf("composer input BeadTitle = %q, want %q or %q", bt, "epic", "child")
	}

	var got *nodes.Decision
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var err error
		got, err = nodes.GetDecision(context.Background(), tx, decisionID)
		return err
	}); err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if got.ImpactOnUse == nil || *got.ImpactOnUse != composer.response {
		t.Errorf("ImpactOnUse = %v, want %q", got.ImpactOnUse, composer.response)
	}

	report, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(report), composer.response) {
		t.Errorf("report does not carry the composed impact:\n%s", report)
	}
}

// --- criterion: a decision that already had a non-nil ImpactOnUse is not
// re-composed and not overwritten. ---

func TestComposeRunReport_AlreadyComposedNeverOverwritten(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()

	epic := BeadRef{ID: "kb-epic-3", Title: "epic", TrackerKind: "br", RepoPath: "/repo"}
	child := BeadRef{ID: "kb-child-3", Title: "child", TrackerKind: "br", RepoPath: "/repo"}
	runID := seedRunWithBeads(t, g, "epic", []BeadRef{epic, child})
	decisionID := writeTestDecision(t, g, child, epic, wellFormedDecisionRecord)

	existing := "a previous run's composer already wrote this."
	if err := g.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		d, err := nodes.GetDecision(context.Background(), tx.AsReadTx(), decisionID)
		if err != nil {
			return err
		}
		d.ImpactOnUse = &existing
		return nodes.UpdateDecision(context.Background(), tx, *d, runRecordAuthor)
	}); err != nil {
		t.Fatalf("seeding an already-composed ImpactOnUse: %v", err)
	}

	// A composer that errors on any call: if resolveImpactField called it at
	// all for this decision, the test below would see the "awaiting" text
	// instead of the pre-seeded one, and would fail for that reason too -
	// but calls is checked explicitly to name the actual violation.
	composer := &fakeImpactComposer{err: context.DeadlineExceeded}
	in := baseReportInput(g, stateDir, runID, "kb-epic-3")
	in.Composer = composer

	path, err := ComposeRunReport(context.Background(), in)
	if err != nil {
		t.Fatalf("ComposeRunReport: %v", err)
	}
	if len(composer.calls) != 0 {
		t.Errorf("composer called %d times for an already-composed decision, want 0", len(composer.calls))
	}

	var got *nodes.Decision
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var err error
		got, err = nodes.GetDecision(context.Background(), tx, decisionID)
		return err
	}); err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if got.ImpactOnUse == nil || *got.ImpactOnUse != existing {
		t.Errorf("ImpactOnUse = %v, want unchanged %q", got.ImpactOnUse, existing)
	}

	report, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(report), existing) {
		t.Errorf("report does not carry the pre-existing impact:\n%s", report)
	}
}

// --- criterion: a composer that returns an error leaves ImpactOnUse nil
// (not ""), still writes the report, and the report says the field is
// awaiting the composer. ---

func TestComposeRunReport_ComposerErrorLeavesImpactNilAndReportSaysAwaiting(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()

	epic := BeadRef{ID: "kb-epic-4", Title: "epic", TrackerKind: "br", RepoPath: "/repo"}
	child := BeadRef{ID: "kb-child-4", Title: "child", TrackerKind: "br", RepoPath: "/repo"}
	runID := seedRunWithBeads(t, g, "epic", []BeadRef{epic, child})
	decisionID := writeTestDecision(t, g, child, epic, wellFormedDecisionRecord)

	composer := &fakeImpactComposer{err: context.DeadlineExceeded}
	in := baseReportInput(g, stateDir, runID, "kb-epic-4")
	in.Composer = composer

	path, err := ComposeRunReport(context.Background(), in)
	if err != nil {
		t.Fatalf("ComposeRunReport: %v", err)
	}

	var got *nodes.Decision
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var err error
		got, err = nodes.GetDecision(context.Background(), tx, decisionID)
		return err
	}); err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if got.ImpactOnUse != nil {
		t.Errorf("ImpactOnUse = %q, want nil after a composer error - never \"\"", *got.ImpactOnUse)
	}

	report, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(report), "awaiting the composer") {
		t.Errorf("report does not say field 4 is awaiting the composer:\n%s", report)
	}
}

// --- criterion: LLM.IsSet() == false (modeled here as a nil Composer, which
// is what the CLI wiring passes when llm.provider is empty) takes the same
// path without calling the composer at all. ---

func TestComposeRunReport_NilComposerNeverCalledStillAwaits(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()

	epic := BeadRef{ID: "kb-epic-5", Title: "epic", TrackerKind: "br", RepoPath: "/repo"}
	child := BeadRef{ID: "kb-child-5", Title: "child", TrackerKind: "br", RepoPath: "/repo"}
	runID := seedRunWithBeads(t, g, "epic", []BeadRef{epic, child})
	decisionID := writeTestDecision(t, g, child, epic, wellFormedDecisionRecord)

	in := baseReportInput(g, stateDir, runID, "kb-epic-5")
	in.Composer = nil

	path, err := ComposeRunReport(context.Background(), in)
	if err != nil {
		t.Fatalf("ComposeRunReport: %v", err)
	}

	var got *nodes.Decision
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var err error
		got, err = nodes.GetDecision(context.Background(), tx, decisionID)
		return err
	}); err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if got.ImpactOnUse != nil {
		t.Errorf("ImpactOnUse = %q, want nil when no composer is configured", *got.ImpactOnUse)
	}

	report, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(report), "awaiting the composer") {
		t.Errorf("report does not say field 4 is awaiting the composer:\n%s", report)
	}
	if !strings.Contains(string(report), "llm.provider") {
		t.Errorf("report does not name llm.provider as the fix:\n%s", report)
	}
}

// --- criterion: a run with no decisions still produces a report. ---

func TestComposeRunReport_NoDecisionsStillProducesReport(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()

	epic := BeadRef{ID: "kb-epic-6", Title: "epic", TrackerKind: "br", RepoPath: "/repo"}
	child := BeadRef{ID: "kb-child-6", Title: "child", TrackerKind: "br", RepoPath: "/repo"}
	runID := seedRunWithBeads(t, g, "epic", []BeadRef{epic, child})

	in := baseReportInput(g, stateDir, runID, "kb-epic-6")
	path, err := ComposeRunReport(context.Background(), in)
	if err != nil {
		t.Fatalf("ComposeRunReport: %v", err)
	}

	report, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(report), "No decisions were recorded") {
		t.Errorf("report does not say no decisions were recorded:\n%s", report)
	}
	// The header/summary facts still land even with zero decisions.
	if !strings.Contains(string(report), "kb-epic-6") && !strings.Contains(string(report), runID) {
		t.Errorf("report is missing header facts:\n%s", report)
	}
}

// --- criterion: the report lands under the run root and a hostile epicID
// cannot escape it. ---

func TestComposeRunReport_ReportUnderRunRoot(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()

	epic := BeadRef{ID: "kb-epic-7", Title: "epic", TrackerKind: "br", RepoPath: "/repo"}
	runID := seedRunWithBeads(t, g, "epic", []BeadRef{epic})

	in := baseReportInput(g, stateDir, runID, "kb-epic-7")
	path, err := ComposeRunReport(context.Background(), in)
	if err != nil {
		t.Fatalf("ComposeRunReport: %v", err)
	}

	wantDir := filepath.Join(stateDir, "run", "kb-epic-7")
	if filepath.Dir(path) != wantDir {
		t.Errorf("report dir = %s, want %s", filepath.Dir(path), wantDir)
	}
	if filepath.Base(path) != "report.md" {
		t.Errorf("report file = %s, want report.md", filepath.Base(path))
	}
}

func TestComposeRunReport_HostileEpicIDRejected(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()

	epic := BeadRef{ID: "kb-epic-8", Title: "epic", TrackerKind: "br", RepoPath: "/repo"}
	runID := seedRunWithBeads(t, g, "epic", []BeadRef{epic})

	in := baseReportInput(g, stateDir, runID, "../../../etc")
	_, err := ComposeRunReport(context.Background(), in)
	if err == nil {
		t.Fatal("expected an error for an epic id that is not a single path segment")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error %q does not carry the KERNL DISPATCH FAILURE marker", err.Error())
	}

	// Nothing must have escaped stateDir/run.
	entries, _ := os.ReadDir(filepath.Join(stateDir, "run"))
	for _, e := range entries {
		if e.Name() == "etc" {
			t.Errorf("a directory named %q was created under %s/run - the hostile id was not rejected before use", e.Name(), stateDir)
		}
	}
}
