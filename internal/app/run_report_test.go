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

// seedRunAtPath is seedRunWithBeads for the tests that drive a bead through
// the real pipeline: those let recordDecisionIfGateType open the graph itself
// from the config, so the run node has to be created in that same database
// file rather than in an in-memory handle the pipeline will never see.
func seedRunAtPath(t *testing.T, dbPath, title string, beads []BeadRef) string {
	t.Helper()
	g, err := graph.Open(context.Background(), graph.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("graph.Open (seed run): %v", err)
	}
	defer g.Close()
	return seedRunWithBeads(t, g, title, beads)
}

// writeTestDecision records a decision against bead/epic using the same
// WriteDecisionRecordNode path recordDecisionIfGateType calls in
// production, so a test decision has the exact shape (Body split into
// options/trade-offs, ImpactOnUse nil) a real one would.
func writeTestDecision(t *testing.T, g *graph.Graph, runID string, bead, epic BeadRef, record string) string {
	t.Helper()
	sections := backend.DecisionRecordSectionBodies(record)
	id, err := WriteDecisionRecordNode(context.Background(), g, sections, bead, epic, runID)
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
		Status:     "completed",
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

// The two runs drive THE SAME bead, which is the only shape that reproduces
// the leak this scoping exists to prevent. bead_reference nodes are
// persistent and shared across every run that touches a tracker id, so
// has_decision edges accumulate on one forever; a traversal that reached
// decisions through the bead would hand the second run every decision the
// first one recorded. Giving the two runs different bead ids passes either
// way and proves nothing.
func TestComposeRunReport_FindsOwnDecisionNotAnotherRuns(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()

	epic := BeadRef{ID: "kb-epic-a", Title: "epic A", TrackerKind: "br", RepoPath: "/repo"}
	child := BeadRef{ID: "kb-child-a", Title: "child A", TrackerKind: "br", RepoPath: "/repo"}

	firstRun := seedRunWithBeads(t, g, "epic A", []BeadRef{epic, child})
	writeTestDecision(t, g, firstRun, child, epic, wellFormedDecisionRecord)

	// A resume, or a re-dispatch, of the same epic over the same bead.
	secondRun := seedRunWithBeads(t, g, "epic A", []BeadRef{epic, child})
	writeTestDecision(t, g, secondRun, child, epic, secondDecisionRecord)

	in := baseReportInput(g, stateDir, secondRun, "kb-epic-a")
	path, err := ComposeRunReport(context.Background(), in)
	if err != nil {
		t.Fatalf("ComposeRunReport: %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	report := string(body)
	if !strings.Contains(report, "Use TOML for the export manifest") {
		t.Errorf("report is missing the second run's own decision:\n%s", report)
	}
	if strings.Contains(report, "Use edges.EdgeTypeHasDecision") {
		t.Errorf("report leaked the first run's decision over the same bead:\n%s", report)
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
	decisionID := writeTestDecision(t, g, runID, child, epic, wellFormedDecisionRecord)

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

// --- criterion: the repository's own docs reach the composer as context,
// which is the whole point of Unit A (see AGENTS.md's plan reference and
// prompt.RenderImpactOnUse's own doc comment) - the Oracle has no other way
// to know what this repository is for. ---

func TestComposeRunReport_ThreadsRepositoryContextToTheComposer(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()
	repoPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("archeion crawls sitemaps and dedupes robots.txt fetches."), 0o644); err != nil {
		t.Fatal(err)
	}

	epic := BeadRef{ID: "kb-epic-ctx", Title: "epic", TrackerKind: "br", RepoPath: repoPath}
	child := BeadRef{ID: "kb-child-ctx", Title: "child", TrackerKind: "br", RepoPath: repoPath}
	runID, err := StartWorkflowRun(context.Background(), g, StartWorkflowRunInput{
		EntryPoint:     "epic run",
		Title:          "epic",
		WorkflowName:   "worker",
		Beads:          []BeadRef{epic, child},
		RepoPath:       repoPath,
		TrackerCommand: "br --db '" + repoPath + "/.beads/beads.db'",
		StartedAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("StartWorkflowRun: %v", err)
	}
	writeTestDecision(t, g, runID, child, epic, wellFormedDecisionRecord)

	composer := &fakeImpactComposer{response: "callers see a typed constant now."}
	in := baseReportInput(g, stateDir, runID, "kb-epic-ctx")
	in.Composer = composer

	if _, err := ComposeRunReport(context.Background(), in); err != nil {
		t.Fatalf("ComposeRunReport: %v", err)
	}
	if len(composer.calls) != 1 {
		t.Fatalf("composer called %d times, want 1", len(composer.calls))
	}
	got := composer.calls[0].RepositoryContext
	if !strings.Contains(got, "archeion crawls sitemaps") {
		t.Errorf("RepositoryContext = %q, want the repository's own README content", got)
	}
	if !strings.Contains(got, "README.md") {
		t.Errorf("RepositoryContext = %q, want the file named under its own heading", got)
	}
	// ROADMAP.md is the second DefaultContextDocs entry and this test repo
	// has none - an omitted section would be indistinguishable from the
	// assembler never having run at all.
	if !strings.Contains(got, "ROADMAP.md") {
		t.Errorf("RepositoryContext = %q, want the missing default doc named explicitly", got)
	}
}

// A repository that declares registry.repos[].contextDocs uses exactly that
// list, in order, instead of DefaultContextDocs.
func TestComposeRunReport_ConfiguredContextDocsOverrideTheDefault(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()
	repoPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("the default doc, not configured"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoPath, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "docs", "ORIENTATION.md"), []byte("the configured doc"), 0o644); err != nil {
		t.Fatal(err)
	}

	epic := BeadRef{ID: "kb-epic-ctx2", Title: "epic", TrackerKind: "br", RepoPath: repoPath}
	runID, err := StartWorkflowRun(context.Background(), g, StartWorkflowRunInput{
		EntryPoint:     "epic run",
		Title:          "epic",
		WorkflowName:   "worker",
		Beads:          []BeadRef{epic},
		RepoPath:       repoPath,
		TrackerCommand: "br --db '" + repoPath + "/.beads/beads.db'",
		StartedAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("StartWorkflowRun: %v", err)
	}
	writeTestDecision(t, g, runID, epic, epic, wellFormedDecisionRecord)

	composer := &fakeImpactComposer{response: "callers see a typed constant now."}
	in := baseReportInput(g, stateDir, runID, "kb-epic-ctx2")
	in.Composer = composer
	in.ContextDocs = []string{"docs/ORIENTATION.md"}

	if _, err := ComposeRunReport(context.Background(), in); err != nil {
		t.Fatalf("ComposeRunReport: %v", err)
	}
	got := composer.calls[0].RepositoryContext
	if !strings.Contains(got, "the configured doc") {
		t.Errorf("RepositoryContext = %q, want the configured doc's content", got)
	}
	if strings.Contains(got, "the default doc, not configured") {
		t.Errorf("RepositoryContext = %q, want README.md excluded once contextDocs is configured", got)
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
	decisionID := writeTestDecision(t, g, runID, child, epic, wellFormedDecisionRecord)

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
	decisionID := writeTestDecision(t, g, runID, child, epic, wellFormedDecisionRecord)

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
	decisionID := writeTestDecision(t, g, runID, child, epic, wellFormedDecisionRecord)

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

// --- criterion: the pull request URL the shipment stage wrote lands in the
// report, and stays silent (no empty field, no placeholder) when there is
// none. ---

func TestComposeRunReport_PRURLAppearsInHeaderWhenSet(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()

	epic := BeadRef{ID: "kb-epic-8", Title: "epic", TrackerKind: "br", RepoPath: "/repo"}
	runID := seedRunWithBeads(t, g, "epic", []BeadRef{epic})

	in := baseReportInput(g, stateDir, runID, "kb-epic-8")
	in.PRURL = "https://github.com/gabrielassisxyz/clarity-data-workflow/pull/30"
	path, err := ComposeRunReport(context.Background(), in)
	if err != nil {
		t.Fatalf("ComposeRunReport: %v", err)
	}

	report, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(report), in.PRURL) {
		t.Errorf("report is missing the pull request URL:\n%s", report)
	}
}

func TestComposeRunReport_AbsentPRURLStaysSilent(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()

	epic := BeadRef{ID: "kb-epic-9", Title: "epic", TrackerKind: "br", RepoPath: "/repo"}
	runID := seedRunWithBeads(t, g, "epic", []BeadRef{epic})

	in := baseReportInput(g, stateDir, runID, "kb-epic-9")
	in.PRURL = ""
	path, err := ComposeRunReport(context.Background(), in)
	if err != nil {
		t.Fatalf("ComposeRunReport: %v", err)
	}

	report, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(strings.ToLower(string(report)), "pull request") {
		t.Errorf("report mentions a pull request with none set - want silence, not an empty field or placeholder:\n%s", report)
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

// --- criterion: an unknown run id fails loud rather than producing a valid
// empty report. ---

// A run id that names nothing resolves to zero outgoing edges, which is
// indistinguishable from a real run that recorded no decisions. Without an
// existence check the composer writes a cheerful "no decisions" report for a
// run that never happened, and the caller has no way to tell the two apart.
func TestComposeRunReport_UnknownRunIDFailsLoud(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()

	in := baseReportInput(g, stateDir, "run-that-does-not-exist", "kb-epic-8")
	_, err := ComposeRunReport(context.Background(), in)
	if err == nil {
		t.Fatal("expected an error for a run id that names no workflow run")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error must carry the fail-loud marker, got: %v", err)
	}
	if !strings.Contains(err.Error(), "run-that-does-not-exist") {
		t.Errorf("error must name the run id it could not resolve, got: %v", err)
	}
}

// --- criterion: a fix-up bead's own decision is sorted first and labeled,
// ahead of a decision the original beads recorded - the one point in a run
// the operator had no prior context, so the one place attention is worth
// the most (orchestrator-autonomy decision model §7). ---

func TestComposeRunReport_FixupDecisionSortsFirstAndIsLabeled(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	stateDir := t.TempDir()

	epic := BeadRef{ID: "kb-epic-fx", Title: "epic", TrackerKind: "br", RepoPath: "/repo"}
	originalChild := BeadRef{ID: "kb-child-fx", Title: "original child", TrackerKind: "br", RepoPath: "/repo"}
	fixupChild := BeadRef{ID: "kb-fixup-fx", Title: "fix-up child", TrackerKind: "br", RepoPath: "/repo", IsFixup: true}

	runID := seedRunWithBeads(t, g, "epic", []BeadRef{epic, originalChild, fixupChild})
	// Written in this order on purpose: the original bead's decision first,
	// the fix-up bead's decision second - insertion order alone would put
	// the original one first in the report; only the sort makes the fix-up
	// one lead.
	writeTestDecision(t, g, runID, originalChild, epic, wellFormedDecisionRecord)
	writeTestDecision(t, g, runID, fixupChild, epic, secondDecisionRecord)

	in := baseReportInput(g, stateDir, runID, "kb-epic-fx")
	path, err := ComposeRunReport(context.Background(), in)
	if err != nil {
		t.Fatalf("ComposeRunReport: %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	report := string(body)

	fixupIdx := strings.Index(report, "Use TOML for the export manifest")
	originalIdx := strings.Index(report, "Use edges.EdgeTypeHasDecision")
	if fixupIdx == -1 || originalIdx == -1 {
		t.Fatalf("both decisions must appear in the report:\n%s", report)
	}
	if fixupIdx >= originalIdx {
		t.Errorf("the fix-up bead's decision must be listed before the original bead's, got fixup at %d and original at %d:\n%s", fixupIdx, originalIdx, report)
	}
	if !strings.Contains(report, "recorded by a fix-up bead the operator never saw created") {
		t.Errorf("the fix-up decision must be labeled as such, report:\n%s", report)
	}
	// The label must sit with the fix-up decision, not leak onto the
	// original one - checked by position, not just presence.
	labelIdx := strings.Index(report, "recorded by a fix-up bead")
	if labelIdx == -1 || labelIdx < fixupIdx || labelIdx > originalIdx {
		t.Errorf("the fix-up label must appear between the fix-up decision's own heading and the original decision's, got label at %d (fixup=%d, original=%d)", labelIdx, fixupIdx, originalIdx)
	}
}

// --- criterion: a whitespace-only completion is a failure, not a
// deliberate "nothing to add". ---

func TestNonEmptyCompletion_RejectsWhitespaceOnly(t *testing.T) {
	for _, in := range []string{"", "   ", "\n", " \t\n "} {
		if _, err := nonEmptyCompletion(in); err == nil {
			t.Errorf("nonEmptyCompletion(%q) returned no error; a whitespace-only answer must not become a deliberate empty ImpactOnUse", in)
		}
	}
	got, err := nonEmptyCompletion("  a real answer\n")
	if err != nil {
		t.Fatalf("nonEmptyCompletion(real answer): %v", err)
	}
	if got != "a real answer" {
		t.Errorf("nonEmptyCompletion trimmed to %q, want %q", got, "a real answer")
	}
}

func TestResolveContextDocs(t *testing.T) {
	if got := resolveContextDocs(nil); strings.Join(got, ",") != strings.Join(DefaultContextDocs, ",") {
		t.Errorf("resolveContextDocs(nil) = %v, want DefaultContextDocs %v", got, DefaultContextDocs)
	}
	if got := resolveContextDocs([]string{}); strings.Join(got, ",") != strings.Join(DefaultContextDocs, ",") {
		t.Errorf("resolveContextDocs([]) = %v, want DefaultContextDocs %v", got, DefaultContextDocs)
	}
	configured := []string{"docs/ORIENTATION.md"}
	if got := resolveContextDocs(configured); strings.Join(got, ",") != strings.Join(configured, ",") {
		t.Errorf("resolveContextDocs(%v) = %v, want the configured list unchanged", configured, got)
	}
}
