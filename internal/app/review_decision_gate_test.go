package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

// writeImplementationReviewArtifact writes content to the well-known
// implementation-review.md path handleGateFailure reads back, following the
// same resolution readRejectedReview's own callers already use.
func writeImplementationReviewArtifact(t *testing.T, dir, beadID, content string) {
	t.Helper()
	path := backend.ResolveArtifactPath("<artifact_dir>/implementation-review.md", beadID, dir)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("seeding implementation-review.md: %v", err)
	}
}

// --- handleGateFailure: the non-rejection and unclassified/fixup paths -
// today's behavior, unchanged, and the DA must never be touched for any of
// them. ---

func TestHandleGateFailure_NonDeliberateRejectionJustBlocks(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "implementation", Labels: []string{"wf:state:implementation"}}
	da := decidingDA("x")

	got, err := handleGateFailure(context.Background(), gateFailureContext{
		Deps:        DriveBeadDeps{Backend: be, RepoPath: "/repo", BeadID: "kb-1", DA: da},
		WF:          workerWorkflow(),
		ActiveState: "implementation",
		GateReason:  "artifact_missing: /tmp/plan.md",
	})
	if err != nil {
		t.Fatalf("handleGateFailure: %v", err)
	}
	if got.Reenter {
		t.Fatalf("expected the bead to block, not reenter, got %+v", got)
	}
	if got.Result.FinalState != "blocked" || got.Result.GateFailureReason != "artifact_missing: /tmp/plan.md" {
		t.Errorf("Result = %+v, want an ordinary blocked gate failure naming the raw gate reason", got.Result)
	}
	if len(be.comments) != 1 || !strings.Contains(be.comments[0].Body, "gate_failed:") {
		t.Errorf("comments = %+v, want one comment prefixed gate_failed:", be.comments)
	}
	if da.calls != 0 {
		t.Errorf("the DA must never be asked about a non-rejection gate failure, got %d calls", da.calls)
	}
	bd, _ := be.Get("kb-1", "/repo")
	if BlockedCauseFromLabels(bd.Labels) != BlockedCauseGate {
		t.Errorf("labels = %v, want a wf:blocked:gate label recording the cause", bd.Labels)
	}
	if !HasLabel(bd.Labels, "wf:state:implementation") {
		t.Errorf("labels = %v, want the pre-existing wf:state:implementation label left untouched", bd.Labels)
	}
}

func TestHandleGateFailure_UnclassifiedRejectionRewindsAndNeverAsksTheDA(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "implementation_review"}
	dir := t.TempDir()
	writeImplementationReviewArtifact(t, dir, "kb-1", unclassifiedRejectionRecord)
	da := decidingDA("x")

	got, err := handleGateFailure(context.Background(), gateFailureContext{
		Deps:        DriveBeadDeps{Backend: be, RepoPath: "/repo", BeadID: "kb-1", DA: da},
		WF:          workerWorkflow(),
		ActiveState: "implementation_review",
		ArtifactDir: dir,
		GateReason:  "verdict_reject: " + filepath.Join(dir, "implementation-review.md"),
	})
	if err != nil {
		t.Fatalf("handleGateFailure: %v", err)
	}
	if !got.Reenter {
		t.Fatalf("expected an unclassified rejection to rewind (Reenter=true), got %+v", got)
	}
	if got.ReviewRewinds != 1 {
		t.Errorf("ReviewRewinds = %d, want 1", got.ReviewRewinds)
	}
	if da.calls != 0 {
		t.Errorf("the DA must never be asked about an unclassified rejection - this is the no-regression test - got %d calls", da.calls)
	}
	if bd, _ := be.Get("kb-1", ""); bd.State != "ready_for_implementation" {
		t.Errorf("bead state = %q, want it rewound to the workflow's retake state", bd.State)
	}
}

func TestHandleGateFailure_FixupClassifiedRewindsAndNeverAsksTheDA(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "implementation_review"}
	dir := t.TempDir()
	writeImplementationReviewArtifact(t, dir, "kb-1", fixupClassifiedRejectionRecord)
	da := decidingDA("x")

	got, err := handleGateFailure(context.Background(), gateFailureContext{
		Deps:        DriveBeadDeps{Backend: be, RepoPath: "/repo", BeadID: "kb-1", DA: da},
		WF:          workerWorkflow(),
		ActiveState: "implementation_review",
		ArtifactDir: dir,
		GateReason:  "verdict_reject: " + filepath.Join(dir, "implementation-review.md"),
	})
	if err != nil {
		t.Fatalf("handleGateFailure: %v", err)
	}
	if !got.Reenter {
		t.Fatalf("expected a fixup-classified rejection to rewind exactly like an unclassified one, got %+v", got)
	}
	if da.calls != 0 {
		t.Errorf("the DA must never be asked about a fixup classification, got %d calls", da.calls)
	}
}

// --- handleGateFailure: the decision-classified path ---

func decisionGateTestConfig(t *testing.T) *config.Config {
	t.Helper()
	return forkGateTestConfig(t)
}

func TestHandleGateFailure_DecisionClassified_ReachesTheDAOnceAndRewinds(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "implementation_review"}
	dir := t.TempDir()
	writeImplementationReviewArtifact(t, dir, "kb-1", decisionClassifiedRejectionRecord)
	da := decidingDA("reverted")

	got, err := handleGateFailure(context.Background(), gateFailureContext{
		Deps:        DriveBeadDeps{Backend: be, RepoPath: "/repo", BeadID: "kb-1", DA: da, Config: decisionGateTestConfig(t)},
		WF:          workerWorkflow(),
		ActiveState: "implementation_review",
		ArtifactDir: dir,
		GateReason:  "verdict_reject: " + filepath.Join(dir, "implementation-review.md"),
	})
	if err != nil {
		t.Fatalf("handleGateFailure: %v", err)
	}
	if da.calls != 1 {
		t.Fatalf("expected the DA to be consulted exactly once, got %d calls", da.calls)
	}
	if !got.Reenter {
		t.Fatalf("expected a decided review-raised decision to rewind, got %+v", got)
	}
	if got.ForkGateCalls != 1 {
		t.Errorf("ForkGateCalls = %d, want 1 - the shared budget must be spent", got.ForkGateCalls)
	}
	if got.ReviewRewinds != 1 {
		t.Errorf("ReviewRewinds = %d, want 1", got.ReviewRewinds)
	}
	answer := readForkAnswerArtifact("kb-1", dir)
	if !strings.Contains(answer, "CHOSEN: reverted") {
		t.Errorf("fork-answer.md = %q, want the DA's chosen option recorded", answer)
	}
	if len(be.comments) == 0 {
		t.Fatal("expected at least one comment")
	}
	found := false
	for _, c := range be.comments {
		if strings.Contains(c.Body, "review_decision_decided") && strings.Contains(c.Body, "reverted") {
			found = true
		}
	}
	if !found {
		t.Errorf("comments = %+v, want one naming review_decision_decided and the chosen option", be.comments)
	}
}

// TestHandleGateFailure_DecisionClassified_AlreadyRoutedEscalatesWithoutAskingTheDAAgain
// is the direct-unit proof of finding 1 of the fork/decision-gate hardening
// pass: readRejectedReview deliberately keeps a stale REJECT readable
// forever, so a marker recording "this exact text was already routed" - not
// the review text itself - is what must stop the SAME rejection from being
// asked of the DA twice. See TestDriveBeadToTerminal_ReviewRaisedDecision_StaleUnrewrittenReviewIsNotRoutedTwice
// for the full-loop mechanism proof (a reviewer stage that genuinely exits
// zero without rewriting its own review).
func TestHandleGateFailure_DecisionClassified_AlreadyRoutedEscalatesWithoutAskingTheDAAgain(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "implementation_review"}
	dir := t.TempDir()
	writeImplementationReviewArtifact(t, dir, "kb-1", decisionClassifiedRejectionRecord)
	if err := markReviewDecisionRouted("kb-1", dir, decisionClassifiedRejectionRecord); err != nil {
		t.Fatalf("seeding the already-routed marker: %v", err)
	}
	da := decidingDA("reverted")

	got, err := handleGateFailure(context.Background(), gateFailureContext{
		Deps:        DriveBeadDeps{Backend: be, RepoPath: "/repo", BeadID: "kb-1", DA: da, Config: decisionGateTestConfig(t)},
		WF:          workerWorkflow(),
		ActiveState: "implementation_review",
		ArtifactDir: dir,
		GateReason:  "verdict_reject: " + filepath.Join(dir, "implementation-review.md"),
	})
	if err != nil {
		t.Fatalf("handleGateFailure: %v", err)
	}
	if got.Reenter {
		t.Fatalf("expected an escalation (Reenter=false) for a rejection already routed once, got %+v", got)
	}
	if da.calls != 0 {
		t.Errorf("the DA must not be asked again about a rejection it already answered, got %d calls", da.calls)
	}
	if !strings.Contains(got.Result.GateFailureReason, string(ForkCauseReviewDecisionAlreadyRouted)) {
		t.Errorf("GateFailureReason = %q, want it to name %q", got.Result.GateFailureReason, ForkCauseReviewDecisionAlreadyRouted)
	}
}

// TestHandleGateFailure_DecisionClassified_RewindNotPossibleEscalatesWithoutAskingTheDA
// is the direct-unit proof of finding 4: with the rewind budget already
// spent, a decision-classified rejection must escalate WITHOUT the DA ever
// being consulted - asking it for an answer that cannot reach anyone would
// waste a real consultation.
func TestHandleGateFailure_DecisionClassified_RewindNotPossibleEscalatesWithoutAskingTheDA(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "implementation_review"}
	dir := t.TempDir()
	writeImplementationReviewArtifact(t, dir, "kb-1", decisionClassifiedRejectionRecord)
	da := decidingDA("reverted")

	got, err := handleGateFailure(context.Background(), gateFailureContext{
		Deps:          DriveBeadDeps{Backend: be, RepoPath: "/repo", BeadID: "kb-1", DA: da, Config: decisionGateTestConfig(t)},
		WF:            workerWorkflow(),
		ActiveState:   "implementation_review",
		ArtifactDir:   dir,
		GateReason:    "verdict_reject: " + filepath.Join(dir, "implementation-review.md"),
		ReviewRewinds: implementationReviewRewindLimit,
	})
	if err != nil {
		t.Fatalf("handleGateFailure: %v", err)
	}
	if got.Reenter {
		t.Fatalf("expected an escalation once the rewind budget is spent, got %+v", got)
	}
	if da.calls != 0 {
		t.Errorf("the DA must not be asked for an answer that could not be rewound to any stage, got %d calls", da.calls)
	}
	if !strings.Contains(got.Result.GateFailureReason, string(ForkCauseRewindNotPossible)) {
		t.Errorf("GateFailureReason = %q, want it to name %q", got.Result.GateFailureReason, ForkCauseRewindNotPossible)
	}
}

func TestHandleGateFailure_DecisionClassified_OpenDependentsEscalatesWithoutAskingTheDA(t *testing.T) {
	be := &forkScopeFakeBackend{children: []backend.Bead{
		{ID: "kb-1", State: "implementation_review"},
		{ID: "kb-2", Dependencies: []backend.BeadDependency{{TargetID: "kb-1", Type: "blocks"}}},
	}}
	dir := t.TempDir()
	writeImplementationReviewArtifact(t, dir, "kb-1", decisionClassifiedRejectionRecord)
	da := decidingDA("reverted")

	got, err := handleGateFailure(context.Background(), gateFailureContext{
		Deps:        DriveBeadDeps{Backend: be, RepoPath: "/repo", BeadID: "kb-1", DA: da, Config: decisionGateTestConfig(t)},
		WF:          workerWorkflow(),
		ActiveState: "implementation_review",
		ArtifactDir: dir,
		EpicID:      "ep-1",
		GateReason:  "verdict_reject: " + filepath.Join(dir, "implementation-review.md"),
	})
	if err != nil {
		t.Fatalf("handleGateFailure: %v", err)
	}
	if got.Reenter {
		t.Fatalf("expected an escalation (Reenter=false), got %+v", got)
	}
	if da.calls != 0 {
		t.Errorf("the DA must not be asked when a sibling already depends on this bead, got %d calls", da.calls)
	}
	if got.Result.FinalState != "blocked" || !strings.Contains(got.Result.GateFailureReason, "review_decision_escalated") {
		t.Errorf("Result = %+v, want a blocked bead naming review_decision_escalated", got.Result)
	}
}

func TestHandleGateFailure_DecisionClassified_SharedBudgetSpentEscalatesWithoutAskingTheDA(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "implementation_review"}
	dir := t.TempDir()
	writeImplementationReviewArtifact(t, dir, "kb-1", decisionClassifiedRejectionRecord)
	da := decidingDA("reverted")

	got, err := handleGateFailure(context.Background(), gateFailureContext{
		Deps:          DriveBeadDeps{Backend: be, RepoPath: "/repo", BeadID: "kb-1", DA: da},
		WF:            workerWorkflow(),
		ActiveState:   "implementation_review",
		ArtifactDir:   dir,
		GateReason:    "verdict_reject: " + filepath.Join(dir, "implementation-review.md"),
		ForkGateCalls: forkHandoverLimit,
	})
	if err != nil {
		t.Fatalf("handleGateFailure: %v", err)
	}
	if got.Reenter {
		t.Fatalf("expected an escalation once the shared budget is spent, got %+v", got)
	}
	if da.calls != 0 {
		t.Errorf("the DA must not be asked once the shared fork/decision budget is spent, got %d calls", da.calls)
	}
	if !strings.Contains(got.Result.GateFailureReason, string(ForkCauseBudgetSpent)) {
		t.Errorf("GateFailureReason = %q, want it to name %q", got.Result.GateFailureReason, ForkCauseBudgetSpent)
	}
}

func TestHandleGateFailure_DecisionClassified_DAEscalates(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "implementation_review", Labels: []string{"wf:state:implementation_review"}}
	dir := t.TempDir()
	writeImplementationReviewArtifact(t, dir, "kb-1", decisionClassifiedRejectionRecord)
	da := &countingDA{answer: "FORK: ESCALATE\nthis genuinely needs the operator's own judgment.\n"}

	got, err := handleGateFailure(context.Background(), gateFailureContext{
		Deps:        DriveBeadDeps{Backend: be, RepoPath: "/repo", BeadID: "kb-1", DA: da, Config: decisionGateTestConfig(t)},
		WF:          workerWorkflow(),
		ActiveState: "implementation_review",
		ArtifactDir: dir,
		GateReason:  "verdict_reject: " + filepath.Join(dir, "implementation-review.md"),
	})
	if err != nil {
		t.Fatalf("handleGateFailure: %v", err)
	}
	if got.Reenter {
		t.Fatalf("expected an escalation, got %+v", got)
	}
	if da.calls != 1 {
		t.Errorf("expected the DA to be consulted exactly once, got %d calls", da.calls)
	}
	if !strings.Contains(got.Result.GateFailureReason, "review_decision_escalated") {
		t.Errorf("GateFailureReason = %q, want it to name review_decision_escalated", got.Result.GateFailureReason)
	}
	bd, _ := be.Get("kb-1", "/repo")
	if BlockedCauseFromLabels(bd.Labels) != BlockedCauseJudgment {
		t.Errorf("labels = %v, want a wf:blocked:judgment label - this is a genuine escalation, the only cause meaning a human is needed", bd.Labels)
	}
	if !HasLabel(bd.Labels, "wf:state:implementation_review") {
		t.Errorf("labels = %v, want the pre-existing wf:state:implementation_review label left untouched", bd.Labels)
	}
}

func TestHandleGateFailure_DecisionClassified_NoDAConfiguredEscalatesWithoutAskingAnything(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "implementation_review"}
	dir := t.TempDir()
	writeImplementationReviewArtifact(t, dir, "kb-1", decisionClassifiedRejectionRecord)

	got, err := handleGateFailure(context.Background(), gateFailureContext{
		Deps:        DriveBeadDeps{Backend: be, RepoPath: "/repo", BeadID: "kb-1", Config: decisionGateTestConfig(t)},
		WF:          workerWorkflow(),
		ActiveState: "implementation_review",
		ArtifactDir: dir,
		GateReason:  "verdict_reject: " + filepath.Join(dir, "implementation-review.md"),
	})
	if err != nil {
		t.Fatalf("handleGateFailure: %v", err)
	}
	if got.Reenter {
		t.Fatalf("expected an escalation with no DA configured, got %+v", got)
	}
	if !strings.Contains(got.Result.GateFailureReason, "no_da_configured") {
		t.Errorf("GateFailureReason = %q, want it to name no_da_configured", got.Result.GateFailureReason)
	}
}

// --- The full-loop mechanism proof: DriveBeadToTerminal genuinely re-runs
// the implementation stage a second time, carrying the DA's own answer. ---

const decisionRecordFixture = `## Decision

Use approach X for the retry policy.

## Options Considered

X (per-bead budget) and Y (per-epic budget).

## Trade-offs

X is simpler to reason about; Y shares the budget across an epic's beads.

## Rationale

X wins because nothing here yet needs an epic-wide budget.
`

// reviewDecisionReentryDriver is a named, counted-call fake BeadDriver
// (AGENTS.md §4) driving three real stage attempts in sequence:
//  1. implementation - writes a normal decision record: no fork, no
//     rejection yet, this attempt simply succeeds.
//  2. implementation_review - rejects the work and classifies the
//     rejection as needing an explicit approved decision.
//  3. implementation, re-entered after the DA decided - this attempt
//     REMOVES the first attempt's decision record rather than writing a new
//     one, so its own exit gate genuinely fails - proving this is a REAL
//     second attempt at the stage, evaluated by the ordinary gate, not a
//     replay of the first success. The same proof forkReentryDriver
//     (fork_gate_reentry_test.go) establishes for the proactive fork gate.
type reviewDecisionReentryDriver struct {
	stateDir   string
	epicID     string
	beadID     string
	reviewBody string
	calls      int
	// argsByCall captures in.Args for every invocation, 1-indexed by call
	// number, so the test can assert on exactly the THIRD run's prompt
	// without guessing which index that is.
	argsByCall map[int][]string
}

func (d *reviewDecisionReentryDriver) RunBead(_ context.Context, in RunBeadInput) (RunBeadResult, error) {
	d.calls++
	if d.argsByCall == nil {
		d.argsByCall = map[int][]string{}
	}
	d.argsByCall[d.calls] = in.Args

	artifactDir, err := ArtifactDirPath(d.stateDir, d.epicID, d.beadID)
	if err != nil {
		return RunBeadResult{}, err
	}
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return RunBeadResult{}, err
	}

	switch d.calls {
	case 1:
		if err := os.WriteFile(filepath.Join(artifactDir, "decision-record.md"), []byte(decisionRecordFixture), 0o644); err != nil {
			return RunBeadResult{}, err
		}
	case 2:
		if err := os.WriteFile(filepath.Join(artifactDir, "implementation-review.md"), []byte(d.reviewBody), 0o644); err != nil {
			return RunBeadResult{}, err
		}
	case 3:
		_ = os.Remove(filepath.Join(artifactDir, "decision-record.md"))
	}
	return RunBeadResult{FinalState: "ok", Success: true, SessionID: "ses_test"}, nil
}

// reviewDecisionReentryWorkflow is the minimal custom workflow this test
// needs: implementation (gated by decision_record) -> implementation_review
// (gated by artifact_verdict, the same gate type production's "worker"
// profile carries on that state) -> done. Registered the same way
// forkReentryWorkflow (and examples/custom-workflow) already is.
func reviewDecisionReentryWorkflow(id string) backend.WorkflowDescriptor {
	return backend.WorkflowDescriptor{
		ID:           id,
		InitialState: "ready_for_implementation",
		States: []string{
			"ready_for_implementation", "implementation",
			"ready_for_implementation_review", "implementation_review", "done",
		},
		TerminalStates: []string{"done"},
		Transitions: []backend.WorkflowTransition{
			{From: "ready_for_implementation", To: "implementation"},
			{From: "implementation", To: "ready_for_implementation_review"},
			{From: "ready_for_implementation_review", To: "implementation_review"},
			{From: "implementation_review", To: "done"},
		},
		RetakeState: "ready_for_implementation",
		Owners: map[string]backend.ActionOwnerKind{
			"implementation":        backend.ActionOwnerAgent,
			"implementation_review": backend.ActionOwnerAgent,
		},
		ActionStates: []string{"implementation", "implementation_review"},
		QueueStates:  []string{"ready_for_implementation", "ready_for_implementation_review"},
		QueueActions: map[string]string{
			"ready_for_implementation":        "implementation",
			"ready_for_implementation_review": "implementation_review",
		},
		ExitGates: map[string][]backend.WorkflowExitGate{
			"implementation":        {{Type: "decision_record", Path: "<artifact_dir>/decision-record.md"}},
			"implementation_review": {{Type: "artifact_verdict", Path: "<artifact_dir>/implementation-review.md"}},
		},
	}
}

// TestDriveBeadToTerminal_ReviewRaisedDecision_ReachesTheDAOnceAndCarriesTheAnswerIntoTheNextRun
// is the mechanism proof for the reviewer backstop (plan §3.3's last
// bullet): not just that DriveBeadToTerminal returns without error, but that
// the SAME implementation stage genuinely runs a second time (a
// counted-call driver proves it), and the DA's own answer is actually
// present in that SECOND invocation's own prompt. See
// fork_gate_reentry_test.go's own doc comment for why only checking the
// return value would miss both a version that silently never re-ran
// anything, and one that re-ran WITHOUT carrying the answer.
func TestDriveBeadToTerminal_ReviewRaisedDecision_ReachesTheDAOnceAndCarriesTheAnswerIntoTheNextRun(t *testing.T) {
	const profileID = "review_decision_reentry_test"
	backend.ClearWorkflowRegistry()
	backend.RegisterWorkflow(reviewDecisionReentryWorkflow(profileID))
	defer backend.ClearWorkflowRegistry()

	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", Title: "retry policy budget", State: "ready_for_implementation", ProfileID: profileID}

	stateDir := t.TempDir()
	driver := &reviewDecisionReentryDriver{
		stateDir:   stateDir,
		epicID:     "kb-1",
		beadID:     "kb-1",
		reviewBody: decisionClassifiedRejectionRecord,
	}
	da := decidingDA("reverted")
	cfg := forkGateTestConfig(t)

	// The first attempt genuinely satisfies the decision_record gate
	// (unlike fork_gate_reentry_test.go's own driver, which never writes
	// one), so recordDecisionIfGateType actually runs and needs a REAL
	// workflow_run node to link the decision to - seedRunAtPath creates one
	// in the same graph db file the pipeline itself will open from cfg.
	graphPath, err := graphDBFilePath(cfg)
	if err != nil {
		t.Fatalf("graphDBFilePath: %v", err)
	}
	runID := seedRunAtPath(t, graphPath, "retry policy budget", []BeadRef{
		{ID: "kb-1", Title: "retry policy budget", TrackerKind: "bd", RepoPath: "/tmp/repo"},
	})

	res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand: "bd",
		StateDir:       stateDir,
		VerifyCommand:  "bin/ci",
		Backend:        be,
		Driver:         driver,
		Config:         cfg,
		BeadID:         "kb-1",
		RepoPath:       "/tmp/repo",
		Worktree:       "/tmp/worktree",
		DA:             da,
		MaxStages:      16,
		RunID:          runID,
	})
	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}

	// The mechanism proof: exactly three invocations (implementation,
	// implementation_review, implementation again), and the DA consulted
	// exactly once.
	if driver.calls != 3 {
		t.Fatalf("expected exactly 3 agent invocations, got %d; res=%+v", driver.calls, res)
	}
	if da.calls != 1 {
		t.Errorf("expected the DA to be consulted exactly once, got %d calls", da.calls)
	}

	thirdPrompt := strings.Join(driver.argsByCall[3], "\n")
	if !strings.Contains(thirdPrompt, "CHOSEN: reverted") {
		t.Errorf("the THIRD invocation's prompt must carry the DA's own answer:\n%s", thirdPrompt)
	}
	if !strings.Contains(thirdPrompt, "not open for re-litigation") {
		t.Errorf("the THIRD invocation's prompt must tell the implementer the choice is settled:\n%s", thirdPrompt)
	}

	firstPrompt := strings.Join(driver.argsByCall[1], "\n")
	if strings.Contains(firstPrompt, "CHOSEN: reverted") {
		t.Errorf("the FIRST invocation ran before any decision was ever handed to the DA - it must not already carry an answer:\n%s", firstPrompt)
	}

	// The THIRD invocation never wrote its own decision-record.md (this
	// fake deliberately removes the first attempt's one instead), so its
	// own exit gate genuinely fails and the bead blocks there - proof this
	// is a REAL third attempt, evaluated by the real exit gate, not a
	// rerun of the first success.
	if res.Success || res.FinalState != "blocked" || res.BlockedAtState != "implementation" {
		t.Errorf("res = %+v, want the third (real, gate-checked) attempt to block at implementation for a missing decision-record.md", res)
	}
}

// reviewDecisionStaleRefireDriver is a named, counted-call fake BeadDriver
// (AGENTS.md §4) that reaches the exact shape finding 1 of the
// fork/decision-gate hardening pass fixes: unlike reviewDecisionReentryDriver
// above (whose third call deliberately breaks the DIFFERENT gate,
// decision_record, to prove a real re-run), this driver's implementation
// stage succeeds cleanly every time it runs, so the bead genuinely reaches
// implementation_review a SECOND time - where this driver writes NOTHING,
// leaving the IDENTICAL rejection its first review pass already wrote (and
// already routed to the DA) stale on disk:
//
//  1. implementation - writes a decision record: succeeds.
//  2. implementation_review - writes the decision-classified rejection:
//     REJECTs, routing it to the DA.
//  3. implementation, re-entered with the DA's own answer - writes a decision
//     record again: succeeds, so the bead reaches implementation_review a
//     second time.
//  4. implementation_review, a second time - writes nothing at all. The
//     stale implementation-review.md from call 2 is still on disk, still
//     ending "VERDICT: REJECT" with the identical decision classification.
type reviewDecisionStaleRefireDriver struct {
	stateDir   string
	epicID     string
	beadID     string
	reviewBody string
	calls      int
}

func (d *reviewDecisionStaleRefireDriver) RunBead(_ context.Context, in RunBeadInput) (RunBeadResult, error) {
	d.calls++
	artifactDir, err := ArtifactDirPath(d.stateDir, d.epicID, d.beadID)
	if err != nil {
		return RunBeadResult{}, err
	}
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return RunBeadResult{}, err
	}

	switch d.calls {
	case 1, 3:
		if err := os.WriteFile(filepath.Join(artifactDir, "decision-record.md"), []byte(decisionRecordFixture), 0o644); err != nil {
			return RunBeadResult{}, err
		}
	case 2:
		if err := os.WriteFile(filepath.Join(artifactDir, "implementation-review.md"), []byte(d.reviewBody), 0o644); err != nil {
			return RunBeadResult{}, err
		}
		// case 4 deliberately writes nothing - the bug shape: a reviewer stage
		// that exits zero without rewriting its own review.
	}
	return RunBeadResult{FinalState: "ok", Success: true, SessionID: "ses_test"}, nil
}

// TestDriveBeadToTerminal_ReviewRaisedDecision_StaleUnrewrittenReviewIsNotRoutedTwice
// is the full-loop mechanism proof for finding 1: a reviewer stage that
// exits zero WITHOUT rewriting its own review leaves the exit gate reading
// the SAME "VERDICT: REJECT" a second time. Before this fix, handleGateFailure
// re-parsed that stale file, saw the identical `decision` classification, and
// routed it to the DA again - possibly getting a different answer to a
// question already answered, and certainly consulting the DA twice for one
// genuine rejection. The DA must be consulted EXACTLY ONCE across this
// entire run, and the second, stale gate failure must escalate instead.
func TestDriveBeadToTerminal_ReviewRaisedDecision_StaleUnrewrittenReviewIsNotRoutedTwice(t *testing.T) {
	const profileID = "review_decision_stale_refire_test"
	backend.ClearWorkflowRegistry()
	backend.RegisterWorkflow(reviewDecisionReentryWorkflow(profileID))
	defer backend.ClearWorkflowRegistry()

	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", Title: "retry policy budget", State: "ready_for_implementation", ProfileID: profileID}

	stateDir := t.TempDir()
	driver := &reviewDecisionStaleRefireDriver{
		stateDir:   stateDir,
		epicID:     "kb-1",
		beadID:     "kb-1",
		reviewBody: decisionClassifiedRejectionRecord,
	}
	da := decidingDA("reverted")
	cfg := forkGateTestConfig(t)

	graphPath, err := graphDBFilePath(cfg)
	if err != nil {
		t.Fatalf("graphDBFilePath: %v", err)
	}
	runID := seedRunAtPath(t, graphPath, "retry policy budget", []BeadRef{
		{ID: "kb-1", Title: "retry policy budget", TrackerKind: "bd", RepoPath: "/tmp/repo"},
	})

	res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand: "bd",
		StateDir:       stateDir,
		VerifyCommand:  "bin/ci",
		Backend:        be,
		Driver:         driver,
		Config:         cfg,
		BeadID:         "kb-1",
		RepoPath:       "/tmp/repo",
		Worktree:       "/tmp/worktree",
		DA:             da,
		MaxStages:      16,
		RunID:          runID,
	})
	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}

	// Four real attempts: implementation, implementation_review (routes to
	// the DA), implementation again (carrying the answer), and
	// implementation_review a second time - left exactly as stale as the
	// first review, on purpose.
	if driver.calls != 4 {
		t.Fatalf("expected exactly 4 agent invocations, got %d; res=%+v", driver.calls, res)
	}
	// The mechanism proof this test exists for.
	if da.calls != 1 {
		t.Errorf("expected the DA to be consulted exactly once across the whole run, got %d calls - a stale, unrewritten review must never be routed twice", da.calls)
	}

	if res.Success || res.FinalState != "blocked" || res.BlockedAtState != "implementation_review" {
		t.Errorf("res = %+v, want the bead to end blocked at implementation_review - the second, stale rejection has nowhere new to go", res)
	}
	if !strings.Contains(res.GateFailureReason, string(ForkCauseReviewDecisionAlreadyRouted)) {
		t.Errorf("GateFailureReason = %q, want it to name %q", res.GateFailureReason, ForkCauseReviewDecisionAlreadyRouted)
	}
}
