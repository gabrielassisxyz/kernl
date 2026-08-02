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

// workflowWithDecisionRecordGate is the minimal WorkflowDescriptor shape
// forkHandoverArmed cares about: a "decision_record" exit gate declared on
// "implementation" - the same gate the "worker" profile carries in
// production (internal/backend/state_machine.go's canonicalImplementationExitGates).
func workflowWithDecisionRecordGate() backend.WorkflowDescriptor {
	return backend.WorkflowDescriptor{
		ExitGates: map[string][]backend.WorkflowExitGate{
			"implementation": {{Type: "decision_record", Path: "<artifact_dir>/decision-record.json"}},
		},
	}
}

// --- forkHandoverArmed ---

func TestForkHandoverArmed_RequiresBothDAAndDecisionRecordGate(t *testing.T) {
	wf := workflowWithDecisionRecordGate()
	da := decidingDA("relevance-first")

	if !forkHandoverArmed(wf, "implementation", da) {
		t.Error("expected armed: a DA is configured and the state carries a decision_record gate")
	}
	if forkHandoverArmed(wf, "implementation", nil) {
		t.Error("expected NOT armed with no DA configured - the gate must not fire with nothing to ask")
	}
	if forkHandoverArmed(wf, "implementation_review", da) {
		t.Error("expected NOT armed: implementation_review carries no decision_record gate - a reviewer stage must never be told it can hand over a fork")
	}
}

// --- read/consume handover, write/read answer ---

func TestReadForkHandoverArtifact_AbsentWhenNeverWritten(t *testing.T) {
	dir := t.TempDir()
	if _, present := readForkHandoverArtifact("kb-1", dir); present {
		t.Error("expected present=false when fork.md was never written - the normal case for every ordinary attempt")
	}
}

func TestReadForkHandoverArtifact_ReadsWhatWasWritten(t *testing.T) {
	dir := t.TempDir()
	path := resolvedForkHandoverPath("kb-1", dir)
	if err := os.WriteFile(path, []byte(forkHandoverRecord), 0o644); err != nil {
		t.Fatalf("seeding fork.md: %v", err)
	}

	content, present := readForkHandoverArtifact("kb-1", dir)
	if !present {
		t.Fatal("expected present=true")
	}
	if content != forkHandoverRecord {
		t.Errorf("content = %q, want the exact bytes written", content)
	}
}

func TestConsumeForkHandover_ArchivesRatherThanDeletes(t *testing.T) {
	dir := t.TempDir()
	path := resolvedForkHandoverPath("kb-1", dir)
	if err := os.WriteFile(path, []byte(forkHandoverRecord), 0o644); err != nil {
		t.Fatalf("seeding fork.md: %v", err)
	}

	if err := consumeForkHandover("kb-1", dir, 1); err != nil {
		t.Fatalf("consumeForkHandover: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("fork.md must no longer exist at its original path after being consumed, stat err = %v", err)
	}
	archived, err := os.ReadFile(path + ".answered.1")
	if err != nil {
		t.Fatalf("expected an archived copy of the handover to survive: %v", err)
	}
	if string(archived) != forkHandoverRecord {
		t.Errorf("archived content = %q, want the original handover preserved for the run report", string(archived))
	}
}

// TestConsumeForkHandover_UniqueNamePerAttempt proves a second handover in
// the same call does not clobber the first one's archived record.
func TestConsumeForkHandover_UniqueNamePerAttempt(t *testing.T) {
	dir := t.TempDir()
	path := resolvedForkHandoverPath("kb-1", dir)

	if err := os.WriteFile(path, []byte("first fork"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := consumeForkHandover("kb-1", dir, 1); err != nil {
		t.Fatalf("first consume: %v", err)
	}
	if err := os.WriteFile(path, []byte("second fork"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := consumeForkHandover("kb-1", dir, 2); err != nil {
		t.Fatalf("second consume: %v", err)
	}

	first, err := os.ReadFile(path + ".answered.1")
	if err != nil || string(first) != "first fork" {
		t.Errorf("first archived handover = %q, %v, want %q preserved", first, err, "first fork")
	}
	second, err := os.ReadFile(path + ".answered.2")
	if err != nil || string(second) != "second fork" {
		t.Errorf("second archived handover = %q, %v, want %q preserved", second, err, "second fork")
	}
}

func TestConsumeForkHandover_MissingFileFailsLoud(t *testing.T) {
	dir := t.TempDir()
	err := consumeForkHandover("kb-1", dir, 1)
	if err == nil {
		t.Fatal("expected an error consuming a handover that was never written")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error must carry the greppable marker: %v", err)
	}
}

func TestForkAnswerArtifact_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	if got := readForkAnswerArtifact("kb-1", dir); got != "" {
		t.Errorf("expected no answer before one is written, got %q", got)
	}

	decision := ForkDecision{Action: ForkActionDecided, ChosenOption: "bm25", Reason: "the epic's siblings already assume this format"}
	if err := writeForkAnswerArtifact("kb-1", dir, decision); err != nil {
		t.Fatalf("writeForkAnswerArtifact: %v", err)
	}

	got := readForkAnswerArtifact("kb-1", dir)
	if !strings.Contains(got, "CHOSEN: bm25") || !strings.Contains(got, "the epic's siblings already assume this format") {
		t.Errorf("readForkAnswerArtifact() = %q, want it to carry the chosen option and the reason", got)
	}
}

// TestWriteForkAnswerArtifact_AppendsRatherThanOverwrites is the
// artifact-level proof of finding 5 of the fork/decision-gate hardening
// pass: deciding a SECOND fork in the same call must not erase the first
// one's answer. forkHandoverLimit is deliberately greater than one (see that
// constant's own doc comment) precisely because an implementer can meet more
// than one genuine fork while writing a single stage's code - and
// os.WriteFile here used to erase the first answer the moment a second fork
// was decided, so the next re-entered attempt's prompt carried only the
// second answer and silently re-decided the first fork alone.
func TestWriteForkAnswerArtifact_AppendsRatherThanOverwrites(t *testing.T) {
	dir := t.TempDir()
	first := ForkDecision{Action: ForkActionDecided, ChosenOption: "bm25", Reason: "siblings already assume this format"}
	second := ForkDecision{Action: ForkActionDecided, ChosenOption: "camelCase", Reason: "matches the wire contract already used"}

	if err := writeForkAnswerArtifact("kb-1", dir, first); err != nil {
		t.Fatalf("first writeForkAnswerArtifact: %v", err)
	}
	if err := writeForkAnswerArtifact("kb-1", dir, second); err != nil {
		t.Fatalf("second writeForkAnswerArtifact: %v", err)
	}

	got := readForkAnswerArtifact("kb-1", dir)
	if !strings.Contains(got, "CHOSEN: bm25") {
		t.Errorf("readForkAnswerArtifact() = %q, want the FIRST decided fork's answer to survive a second decision", got)
	}
	if !strings.Contains(got, "CHOSEN: camelCase") {
		t.Errorf("readForkAnswerArtifact() = %q, want the SECOND decided fork's own answer", got)
	}
	firstIdx := strings.Index(got, "CHOSEN: bm25")
	secondIdx := strings.Index(got, "CHOSEN: camelCase")
	if firstIdx < 0 || secondIdx < 0 || firstIdx > secondIdx {
		t.Errorf("readForkAnswerArtifact() = %q, want the two answers in the order they were decided", got)
	}
}

// --- handleForkGate ---

func forkGateTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := newDriveTestConfig()
	// Vault.Root pinned to a scratch directory so relatedDecisionsForPrompt's
	// own graph lookup (reused here via decideForkAttempt) never touches the
	// operator's real ~/.kernl-graph.db - AGENTS.md §4's hermetic-test rule.
	cfg.Vault.Root = t.TempDir()
	return cfg
}

func TestHandleForkGate_NotArmedWithNoDA_ProceedsNormally(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "implementation"}
	dir := t.TempDir()
	// A handover is present, but with no DA configured the gate is not
	// armed at all - the implementer was never told it could hand one over,
	// so this must behave exactly as if nothing were there.
	if err := os.WriteFile(resolvedForkHandoverPath("kb-1", dir), []byte(forkHandoverRecord), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := handleForkGate(context.Background(), forkGateAttemptContext{
		Deps:        DriveBeadDeps{Backend: be, RepoPath: "/repo", BeadID: "kb-1", Config: forkGateTestConfig(t)},
		WF:          workflowWithDecisionRecordGate(),
		Bead:        be.beads["kb-1"],
		ActiveState: "implementation",
		ArtifactDir: dir,
	})
	if err != nil {
		t.Fatalf("handleForkGate: %v", err)
	}
	if got.Applied {
		t.Errorf("expected Applied=false with no DA configured, got %+v", got)
	}
	if _, err := os.Stat(resolvedForkHandoverPath("kb-1", dir)); err != nil {
		t.Errorf("an unarmed gate must not touch the handover file at all: stat err = %v", err)
	}
}

func TestHandleForkGate_ArmedButNoHandoverPresent_ProceedsNormally(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "implementation"}
	dir := t.TempDir()

	got, err := handleForkGate(context.Background(), forkGateAttemptContext{
		Deps:        DriveBeadDeps{Backend: be, RepoPath: "/repo", BeadID: "kb-1", Config: forkGateTestConfig(t), DA: decidingDA("x")},
		WF:          workflowWithDecisionRecordGate(),
		Bead:        be.beads["kb-1"],
		ActiveState: "implementation",
		ArtifactDir: dir,
	})
	if err != nil {
		t.Fatalf("handleForkGate: %v", err)
	}
	if got.Applied {
		t.Errorf("expected Applied=false - the implementer simply committed, as usual; got %+v", got)
	}
}

func TestHandleForkGate_Decided_WritesAnswerConsumesHandoverAndReenters(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "implementation"}
	dir := t.TempDir()
	stateDir := t.TempDir()
	if err := os.WriteFile(resolvedForkHandoverPath("kb-1", dir), []byte(forkHandoverRecord), 0o644); err != nil {
		t.Fatal(err)
	}
	da := decidingDA("relevance-first")

	got, err := handleForkGate(context.Background(), forkGateAttemptContext{
		Deps:        DriveBeadDeps{Backend: be, RepoPath: "/repo", BeadID: "kb-1", StateDir: stateDir, Config: forkGateTestConfig(t), DA: da},
		WF:          workflowWithDecisionRecordGate(),
		Bead:        be.beads["kb-1"],
		ActiveState: "implementation",
		ArtifactDir: dir,
		EpicID:      "kb-1",
		LedgerInput: StageAttemptInput{AgentID: "opencode", Dialect: "opencode", BeadID: "kb-1", Stage: "implementation"},
	})
	if err != nil {
		t.Fatalf("handleForkGate: %v", err)
	}
	if !got.Applied || !got.Reenter {
		t.Fatalf("expected Applied=true, Reenter=true for a decided fork, got %+v", got)
	}
	if da.calls != 1 {
		t.Errorf("expected the DA to be consulted exactly once, got %d calls", da.calls)
	}

	if _, err := os.Stat(resolvedForkHandoverPath("kb-1", dir)); !os.IsNotExist(err) {
		t.Errorf("the handover must be consumed once decided, stat err = %v", err)
	}
	answer := readForkAnswerArtifact("kb-1", dir)
	if !strings.Contains(answer, "CHOSEN: relevance-first") {
		t.Errorf("answer artifact = %q, want the DA's chosen option recorded", answer)
	}

	if len(be.comments) != 1 || !strings.Contains(be.comments[0].Body, "relevance-first") {
		t.Errorf("comments = %+v, want one comment naming the chosen option", be.comments)
	}

	ledgerPath, lerr := resolveAttemptLedgerPath(stateDir, "kb-1")
	if lerr != nil {
		t.Fatalf("resolveAttemptLedgerPath: %v", lerr)
	}
	recs := readLedgerLines(t, ledgerPath)
	if len(recs) != 1 {
		t.Fatalf("expected exactly one ledger row for the fork-gate attempt, got %d", len(recs))
	}
	if recs[0].GatePassed {
		t.Error("a fork-gate attempt must record GatePassed=false - it did not pass the ordinary exit gate, it was decided by the fork gate instead")
	}
	if recs[0].GateFailureReason == nil || !strings.Contains(*recs[0].GateFailureReason, "fork_gate") {
		t.Errorf("GateFailureReason = %v, want it to name the fork gate", recs[0].GateFailureReason)
	}
}

func TestHandleForkGate_DAEscalates_BlocksAndComments(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "implementation"}
	dir := t.TempDir()
	stateDir := t.TempDir()
	if err := os.WriteFile(resolvedForkHandoverPath("kb-1", dir), []byte(forkHandoverRecord), 0o644); err != nil {
		t.Fatal(err)
	}
	// persistingBackend.List always answers with nil (no children), so this
	// proves handleForkGate's own escalation plumbing (block, comment,
	// consume, ledger) using the DA's own escalating verdict - the
	// open-dependents-only escalation is already covered, without any DA at
	// all, by fork_handover_test.go's own DecideForkAction unit tests.
	da := &countingDA{answer: "FORK: ESCALATE\nthis needs the operator - the two options commit to incompatible storage formats.\n"}

	got, err := handleForkGate(context.Background(), forkGateAttemptContext{
		Deps:        DriveBeadDeps{Backend: be, RepoPath: "/repo", BeadID: "kb-1", StateDir: stateDir, Config: forkGateTestConfig(t), DA: da},
		WF:          workflowWithDecisionRecordGate(),
		Bead:        be.beads["kb-1"],
		ActiveState: "implementation",
		ArtifactDir: dir,
		EpicID:      "kb-1",
		LedgerInput: StageAttemptInput{AgentID: "opencode", Dialect: "opencode", BeadID: "kb-1", Stage: "implementation"},
	})
	if err != nil {
		t.Fatalf("handleForkGate: %v", err)
	}
	if !got.Applied || got.Reenter {
		t.Fatalf("expected Applied=true, Reenter=false for an escalation, got %+v", got)
	}
	if got.Result.FinalState != "blocked" || got.Result.Success {
		t.Errorf("Result = %+v, want a blocked, unsuccessful result", got.Result)
	}
	if got.Result.BlockedAtState != "implementation" {
		t.Errorf("BlockedAtState = %q, want %q", got.Result.BlockedAtState, "implementation")
	}
	if da.calls != 1 {
		t.Errorf("expected the DA to be consulted exactly once, got %d calls", da.calls)
	}
	if bd, _ := be.Get("kb-1", ""); bd.State != "blocked" {
		t.Errorf("bead state = %q, want blocked", bd.State)
	}
	if len(be.comments) != 1 || !strings.Contains(be.comments[0].Body, "fork_gate_escalated") {
		t.Errorf("comments = %+v, want one comment naming the fork gate escalation", be.comments)
	}
	if _, err := os.Stat(resolvedForkHandoverPath("kb-1", dir)); !os.IsNotExist(err) {
		t.Errorf("an escalated handover must still be consumed, so a later retry cannot refire it, stat err = %v", err)
	}
}

// TestHandleForkGate_BudgetSpent_EscalatesWithoutAskingTheDA proves the
// per-call budget (forkHandoverLimit) escalates a fork WITHOUT ever
// consulting the DA once it is spent.
func TestHandleForkGate_BudgetSpent_EscalatesWithoutAskingTheDA(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "implementation"}
	dir := t.TempDir()
	stateDir := t.TempDir()
	if err := os.WriteFile(resolvedForkHandoverPath("kb-1", dir), []byte(forkHandoverRecord), 0o644); err != nil {
		t.Fatal(err)
	}
	da := decidingDA("relevance-first")

	got, err := handleForkGate(context.Background(), forkGateAttemptContext{
		Deps:        DriveBeadDeps{Backend: be, RepoPath: "/repo", BeadID: "kb-1", StateDir: stateDir, Config: forkGateTestConfig(t), DA: da},
		WF:          workflowWithDecisionRecordGate(),
		Bead:        be.beads["kb-1"],
		ActiveState: "implementation",
		ArtifactDir: dir,
		EpicID:      "kb-1",
		CallsUsed:   forkHandoverLimit,
		LedgerInput: StageAttemptInput{AgentID: "opencode", Dialect: "opencode", BeadID: "kb-1", Stage: "implementation"},
	})
	if err != nil {
		t.Fatalf("handleForkGate: %v", err)
	}
	if !got.Applied || got.Reenter {
		t.Fatalf("expected an escalation once the budget is spent, got %+v", got)
	}
	if da.calls != 0 {
		t.Errorf("the DA must not be asked once this call's fork-handover budget is spent, got %d calls", da.calls)
	}
	if !strings.Contains(got.Result.GateFailureReason, string(ForkCauseBudgetSpent)) {
		t.Errorf("GateFailureReason = %q, want it to name %q", got.Result.GateFailureReason, ForkCauseBudgetSpent)
	}
}

// TestHandleForkGate_ScopeMeasurementFailure_FailsLoudNotAsEscalation proves
// a genuine infrastructure failure (the tracker could not be asked which
// siblings depend on this bead) propagates as a real error - it is not this
// fork's own escalation, and must not be silently folded into one.
func TestHandleForkGate_ScopeMeasurementFailure_FailsLoudNotAsEscalation(t *testing.T) {
	be := &forkScopeFakeBackend{listErr: errFakeListBroke}
	dir := t.TempDir()
	if err := os.WriteFile(resolvedForkHandoverPath("kb-1", dir), []byte(forkHandoverRecord), 0o644); err != nil {
		t.Fatal(err)
	}
	da := decidingDA("relevance-first")

	_, err := handleForkGate(context.Background(), forkGateAttemptContext{
		Deps:        DriveBeadDeps{Backend: be, RepoPath: "/repo", BeadID: "kb-1", StateDir: t.TempDir(), Config: forkGateTestConfig(t), DA: da},
		WF:          workflowWithDecisionRecordGate(),
		Bead:        &backend.Bead{ID: "kb-1", State: "implementation"},
		ActiveState: "implementation",
		ArtifactDir: dir,
		EpicID:      "kb-1",
	})
	if err == nil {
		t.Fatal("expected a genuine error when scope facts cannot be measured, got nil")
	}
	if da.calls != 0 {
		t.Errorf("the DA must not be asked when its own scope facts could not be measured, got %d calls", da.calls)
	}
}

// TestForkHandoverLimit_IsGreaterThanOne documents the load-bearing property
// of the constant: a limit of one would turn a second, genuinely distinct
// fork in the same stage into a block (see forkHandoverLimit's own doc
// comment).
func TestForkHandoverLimit_IsGreaterThanOne(t *testing.T) {
	if forkHandoverLimit <= 1 {
		t.Fatalf("forkHandoverLimit = %d, want > 1 - an implementer can legitimately meet more than one genuine fork in a stage", forkHandoverLimit)
	}
}

func TestResolvedForkPaths_ExpandArtifactDirPlaceholder(t *testing.T) {
	dir := filepath.Join("/home/user/.kernl/run/epic-1", "kb-1")
	if got := resolvedForkHandoverPath("kb-1", dir); got != dir+"/fork.md" {
		t.Errorf("resolvedForkHandoverPath = %q, want %q", got, dir+"/fork.md")
	}
	if got := resolvedForkAnswerPath("kb-1", dir); got != dir+"/fork-answer.md" {
		t.Errorf("resolvedForkAnswerPath = %q, want %q", got, dir+"/fork-answer.md")
	}
}
