package app

import (
	"context"
	"strconv"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// driveBlockedBead runs DriveBeadToTerminal over a bead that is already
// blocked, which is the entry branch every one of these tests exercises.
func driveBlockedBead(t *testing.T, be *persistingBackend) RunBeadResult {
	t.Helper()
	res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand: "bd",
		StateDir:       t.TempDir(),
		VerifyCommand:  "bin/ci",
		Backend:        be,
		Driver:         &scriptedDriver{be: be},
		Config:         newDriveTestConfig(),
		BeadID:         "kb-1",
		RepoPath:       "/tmp/repo",
		Worktree:       "/tmp/worktree",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return res
}

// The subprocess branch is the third of the three block sites, and the only
// one outside review_decision_gate.go. A cause that is recorded by two of
// three sites is the half-fix this label exists to prevent, so it gets its
// own assertion rather than being assumed from the shared helper.
func TestBlockBeadWithCause_SubprocessIsRecorded(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "implementation", Labels: []string{"wf:state:implementation"}}

	blockBeadWithCause(be, "kb-1", "/tmp/repo", BlockedCauseSubprocess)

	bd, _ := be.Get("kb-1", "/tmp/repo")
	if bd.State != "blocked" {
		t.Errorf("State = %q, want blocked", bd.State)
	}
	if got := BlockedCauseFromLabels(bd.Labels); got != BlockedCauseSubprocess {
		t.Errorf("cause = %q, want subprocess (labels %v)", got, bd.Labels)
	}
	if !HasLabel(bd.Labels, "wf:state:implementation") {
		t.Errorf("labels = %v, want wf:state:implementation untouched - it is the only record of which stage blocked", bd.Labels)
	}
}

// A bead blocked twice carries one cause, not a pile of them: the second
// block replaces the first, so BlockedCauseFromLabels can never have to pick
// between two answers.
func TestBlockBeadWithCause_ReplacesAStaleCause(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{
		ID:     "kb-1",
		State:  "implementation",
		Labels: []string{"wf:state:implementation", "wf:blocked:subprocess"},
	}

	blockBeadWithCause(be, "kb-1", "/tmp/repo", BlockedCauseJudgment)

	bd, _ := be.Get("kb-1", "/tmp/repo")
	if HasLabel(bd.Labels, "wf:blocked:subprocess") {
		t.Errorf("labels = %v, want the stale subprocess cause gone, not accumulated", bd.Labels)
	}
	if got := BlockedCauseFromLabels(bd.Labels); got != BlockedCauseJudgment {
		t.Errorf("cause = %q, want judgment", got)
	}
}

// A mechanical block is the case a re-run is allowed to retry on its own:
// the bead goes back to the stage its wf:state label names, and the run
// carries on instead of returning immediately as it did for every cause
// before the label existed.
func TestDriveBeadToTerminal_MechanicalBlockResumesItsStage(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{
		ID:     "kb-1",
		State:  "blocked",
		Labels: []string{"wf:state:implementation", "wf:blocked:subprocess"},
	}

	res := driveBlockedBead(t, be)

	if res.FinalState == "blocked" {
		t.Fatalf("a mechanical block must be retried, not returned as-is: %+v", res)
	}
	bd, _ := be.Get("kb-1", "/tmp/repo")
	if bd.State == "blocked" {
		t.Errorf("State = blocked, want the bead moved off the block")
	}
	if HasLabel(bd.Labels, "wf:blocked:subprocess") {
		t.Errorf("labels = %v, want the cause cleared once the block was resumed", bd.Labels)
	}
}

// A judgment block is the one cause that genuinely means a human is needed.
// Resuming it automatically would ignore the very decision it is asking for,
// so the run returns without touching the bead, and names the cause.
func TestDriveBeadToTerminal_JudgmentBlockIsNeverResumed(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{
		ID:     "kb-1",
		State:  "blocked",
		Labels: []string{"wf:state:implementation", "wf:blocked:judgment"},
	}
	writesBefore := be.writes

	res := driveBlockedBead(t, be)

	if res.Success || res.FinalState != "blocked" {
		t.Errorf("a judgment block must stay blocked, got %+v", res)
	}
	if res.BlockedAtState != "implementation" {
		t.Errorf("BlockedAtState = %q, want the stage recovered from wf:state", res.BlockedAtState)
	}
	if res.GateFailureReason != string(BlockedCauseJudgment) {
		t.Errorf("GateFailureReason = %q, want the cause named", res.GateFailureReason)
	}
	if be.writes != writesBefore {
		t.Errorf("a judgment block must not be written to at all, got %d writes", be.writes-writesBefore)
	}
}

// A bead blocked by hand carries no wf:blocked:* label at all. It is treated
// exactly like a judgment block, which is the conservative direction: the
// one thing worse than failing to resume a bead automatically is resuming
// one whose block nobody in this codebase recorded.
func TestDriveBeadToTerminal_BlockWithNoRecordedCauseIsNeverResumed(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{
		ID:     "kb-1",
		State:  "blocked",
		Labels: []string{"wf:state:implementation"},
	}
	writesBefore := be.writes

	res := driveBlockedBead(t, be)

	if res.Success || res.FinalState != "blocked" {
		t.Errorf("an unlabelled block must stay blocked, got %+v", res)
	}
	if be.writes != writesBefore {
		t.Errorf("an unlabelled block must not be written to at all, got %d writes", be.writes-writesBefore)
	}
}

// A resume spends one unit of the budget, and StopBeforeState pins the bead
// at the stage it was just resumed into so the count can be read there.
//
// Running on past it would prove nothing about the budget: the bead would
// sail through the rest of the workflow on a driver that always succeeds,
// and a later claim would clear wf:blocked-retries - correctly, because a
// stage that finally worked ends the mechanical problem the budget was
// counting.
func TestDriveBeadToTerminal_MechanicalResumeSpendsOneRetry(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{
		ID:     "kb-1",
		State:  "blocked",
		Labels: []string{"wf:state:implementation", "wf:blocked:gate"},
	}

	if _, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand:  "bd",
		StateDir:        t.TempDir(),
		VerifyCommand:   "bin/ci",
		Backend:         be,
		Driver:          &scriptedDriver{be: be},
		Config:          newDriveTestConfig(),
		BeadID:          "kb-1",
		RepoPath:        "/tmp/repo",
		Worktree:        "/tmp/worktree",
		StopBeforeState: "implementation",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bd, _ := be.Get("kb-1", "/tmp/repo")
	if bd.State != "implementation" {
		t.Fatalf("State = %q, want the bead resumed into implementation", bd.State)
	}
	if got := blockedRetryCountFromLabels(bd.Labels); got != 1 {
		t.Errorf("retry count = %d, want 1 after one automatic resume (labels %v)", got, bd.Labels)
	}
}

// The count only bounds anything if it survives the NEXT block. A stage that
// fails mechanically again lands back in blockBeadWithCause, which must
// replace the cause without disturbing wf:blocked-retries - otherwise every
// re-run reads a fresh count of zero and the budget never runs out.
func TestBlockBeadWithCause_PreservesTheRetryCount(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{
		ID:     "kb-1",
		State:  "implementation",
		Labels: []string{"wf:state:implementation", blockedRetryLabelPrefix + "1"},
	}

	blockBeadWithCause(be, "kb-1", "/tmp/repo", BlockedCauseSubprocess)

	bd, _ := be.Get("kb-1", "/tmp/repo")
	if got := blockedRetryCountFromLabels(bd.Labels); got != 1 {
		t.Errorf("retry count = %d, want the spent retry still counted after re-blocking (labels %v)", got, bd.Labels)
	}
}

// Past the budget the bead stops being retried and waits for a human, which
// is the whole point of bounding it: a stage that fails mechanically every
// time is a real, repeating problem, and a third blind attempt does not fix
// it.
func TestDriveBeadToTerminal_ExhaustedBudgetStopsRetrying(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{
		ID:    "kb-1",
		State: "blocked",
		Labels: []string{
			"wf:state:implementation",
			"wf:blocked:gate",
			blockedRetryLabelPrefix + strconv.Itoa(mechanicalBlockRetryLimit),
		},
	}
	writesBefore := be.writes

	res := driveBlockedBead(t, be)

	if res.Success || res.FinalState != "blocked" {
		t.Errorf("an exhausted budget must leave the bead blocked, got %+v", res)
	}
	if be.writes != writesBefore {
		t.Errorf("an exhausted budget must not keep rewriting the bead, got %d writes", be.writes-writesBefore)
	}
}
