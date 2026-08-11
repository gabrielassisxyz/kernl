package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

// reentryDriver simulates the two dispatches a gate-blocked worker stage
// actually receives across two `epic run` invocations.
//
// The first dispatch commits the stage's work but writes no decision record,
// so the stage clears commit_marker and fails decision_record - the one
// combination that leaves a worker's "implementation" blocked with its work
// already in the worktree's history.
//
// The second supplies only the missing artifact. It deliberately does NOT
// commit again: the code it was asked to write is already there, and an
// implementer that re-commits identical work to satisfy a check is inventing
// work, which is the behaviour the commit_marker gate exists to prevent in
// the first place.
type reentryDriver struct {
	be       *epicFakeBackend
	beadID   string
	worktree string
	stateDir string

	implDispatches int
}

func (d *reentryDriver) RunBead(_ context.Context, _ RunBeadInput) (RunBeadResult, error) {
	bd, err := d.be.Get(d.beadID, "")
	if err != nil || bd == nil {
		return RunBeadResult{Success: false}, fmt.Errorf("bead %s not found: %w", d.beadID, err)
	}
	dir := filepath.Join(d.stateDir, "run", d.beadID, d.beadID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return RunBeadResult{Success: false}, fmt.Errorf("artifact dir: %w", err)
	}

	switch bd.State {
	case "implementation":
		d.implDispatches++
		if d.implDispatches == 1 {
			if err := commitRealChange(d.worktree, "work.txt", "did the work\n", "stage: implementation: did the work"); err != nil {
				return RunBeadResult{Success: false}, fmt.Errorf("implementation commit: %w", err)
			}
			return RunBeadResult{FinalState: "ok", Success: true, SessionID: "ses-1"}, nil
		}
		if err := os.WriteFile(filepath.Join(dir, "decision-record.json"), []byte(fakeDecisionRecord), 0o644); err != nil {
			return RunBeadResult{Success: false}, fmt.Errorf("decision record: %w", err)
		}
	case "implementation_review":
		if err := os.WriteFile(filepath.Join(dir, "implementation-review.md"), []byte("code matches the plan\n\nVERDICT: PASS"), 0o644); err != nil {
			return RunBeadResult{Success: false}, fmt.Errorf("review artifact: %w", err)
		}
	}
	return RunBeadResult{FinalState: "ok", Success: true, SessionID: "ses-2"}, nil
}

// TestDriveWorker_ReentryReprovesWorkAlreadyCommitted is step 5's failing
// test: a stage that is re-entered after a gate block can never pass
// commit_marker again, because BaseSHA is captured per DISPATCH
// (drive_bead.go's `baseSHA := headSHA.HeadSHA(deps.Worktree)` inside the
// loop) rather than per stage entry. The second dispatch therefore measures
// the stage's own work as history that predates it, and reports
// commit_marker_missing on a worktree that visibly carries the commit.
//
// This is the "committed anyway" defect the ledger recorded three times -
// two codex integration rows at the same SHA on 2026-07-31, one claude row
// on 2026-08-02 - reproduced without an agent. The production sequence was two
// `epic run` invocations into a surviving worktree, and the mechanical-block
// resume (BlockedCauseGate.IsMechanical) is what re-entered the stage on the
// second. That resume now happens inside one call, so the sequence this test
// drives is one DriveBeadToTerminal rather than two; what it proves is
// unchanged, because the epoch is what the re-entered dispatch reads either
// way, and the store's own tests cover it surviving a process boundary.
//
// The fix this test is red against is the stage epoch: capture BaseSHA when
// the bead first ENTERS a state, persist it outside the worktree, reuse it
// for every dispatch within that uninterrupted state, and replace it only
// when the bead transitions away. Widening the gate's range semantics
// instead - accepting any historical commit, or scanning past BaseSHA -
// would let an empty commit pass, which is the outcome the gate exists to
// refuse.
func TestDriveWorker_ReentryReprovesWorkAlreadyCommitted(t *testing.T) {
	be, deps, worktree, stateDir := newWorkerDriveFixture(t, "kernl-re1")
	driver := &reentryDriver{be: be, beadID: deps.BeadID, worktree: worktree, stateDir: stateDir}
	deps.Driver = driver

	// The first dispatch's work lands and decision_record does not, so the
	// stage blocks and is retried; the retry writes the missing artifact and
	// commits nothing, because the code it was asked to write is already
	// there.
	res, err := DriveBeadToTerminal(context.Background(), deps)
	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v\n%s", err, describeGateEvidence(t, stateDir, deps.BeadID))
	}
	if _, err := os.Stat(filepath.Join(worktree, "work.txt")); err != nil {
		t.Fatalf("precondition: the first dispatch must leave its commit in the worktree: %v", err)
	}
	if driver.implDispatches < 2 {
		t.Fatalf("precondition: implementation should have been dispatched twice, got %d", driver.implDispatches)
	}
	if !res.Success || res.FinalState != "awaiting_integration" {
		t.Fatalf("a re-entered stage must pass its commit_marker on the work already committed;\n"+
			"got %+v\n%s", res, describeGateEvidence(t, stateDir, deps.BeadID))
	}
}

// rewindingBackend gives epicFakeBackend the one behaviour a rewind test
// needs and the shared fake stubs out: a Rewind that actually returns the
// bead to the retake state. Without it the rejection leaves the bead sitting
// at implementation_review and the retake never happens, so the test would
// pass while proving nothing.
type rewindingBackend struct{ *epicFakeBackend }

func (b rewindingBackend) Rewind(id, retakeState, _, repoPath string) error {
	return b.Update(id, backend.UpdateBeadInput{State: retakeState}, repoPath)
}

// rewindDriver is the other half of the epoch's contract: a stage LEFT and
// re-entered, rather than one re-entered without ever leaving.
//
// The implementer commits its work and clears both gates, the reviewer
// rejects it, and the retake that follows commits nothing - it is the
// implementer that answers a rejection by arguing rather than by changing the
// code. That retake must fail, because the only commit in the worktree is the
// one the reviewer already refused.
type rewindDriver struct {
	be       *epicFakeBackend
	beadID   string
	worktree string
	stateDir string

	implDispatches   int
	reviewDispatches int
}

func (d *rewindDriver) RunBead(_ context.Context, _ RunBeadInput) (RunBeadResult, error) {
	bd, err := d.be.Get(d.beadID, "")
	if err != nil || bd == nil {
		return RunBeadResult{Success: false}, fmt.Errorf("bead %s not found: %w", d.beadID, err)
	}
	dir := filepath.Join(d.stateDir, "run", d.beadID, d.beadID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return RunBeadResult{Success: false}, fmt.Errorf("artifact dir: %w", err)
	}

	switch bd.State {
	case "implementation":
		d.implDispatches++
		if d.implDispatches > 1 {
			// The retake: the decision record from the first pass is still
			// on disk, so commit_marker is the only gate left to answer.
			return RunBeadResult{FinalState: "ok", Success: true, SessionID: "ses-retake"}, nil
		}
		if err := commitRealChange(d.worktree, "work.txt", "did the work\n", "stage: implementation: did the work"); err != nil {
			return RunBeadResult{Success: false}, fmt.Errorf("implementation commit: %w", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "decision-record.json"), []byte(fakeDecisionRecord), 0o644); err != nil {
			return RunBeadResult{Success: false}, fmt.Errorf("decision record: %w", err)
		}
	case "implementation_review":
		d.reviewDispatches++
		if err := os.WriteFile(filepath.Join(dir, "implementation-review.md"),
			[]byte("the code does not do what the plan says\n\nVERDICT: REJECT"), 0o644); err != nil {
			return RunBeadResult{Success: false}, fmt.Errorf("review artifact: %w", err)
		}
	}
	return RunBeadResult{FinalState: "ok", Success: true, SessionID: "ses-2"}, nil
}

// TestDriveWorker_RewindStartsANewEpoch is the boundary that keeps the fix
// above from becoming a hole: a rejected implementer that does nothing at all
// must not pass its gate on the very commit the reviewer refused, which is
// what an epoch outliving its stage entry would let it do.
//
// Two rules keep the retake a new entry, and either one alone is enough here
// (measured): the retake arrives through a claim out of the retake state, and
// the reviewer's own dispatch has already replaced the epoch with one of its
// own. So this test goes red only when BOTH are lost - it pins the outcome
// the pair exists for, while resolveStageEpochBase's unit tests pin each rule
// separately.
func TestDriveWorker_RewindStartsANewEpoch(t *testing.T) {
	be, deps, worktree, stateDir := newWorkerDriveFixture(t, "kernl-re2")
	driver := &rewindDriver{be: be, beadID: deps.BeadID, worktree: worktree, stateDir: stateDir}
	deps.Driver = driver
	deps.Backend = rewindingBackend{be}

	res, err := DriveBeadToTerminal(context.Background(), deps)
	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v\n%s", err, describeGateEvidence(t, stateDir, deps.BeadID))
	}
	if driver.reviewDispatches < 1 || driver.implDispatches < 2 {
		t.Fatalf("precondition: the review must have run and rewound to a retake; impl=%d review=%d",
			driver.implDispatches, driver.reviewDispatches)
	}
	if res.Success {
		t.Fatalf("a retake that committed nothing must not pass commit_marker on the rejected commit;\n"+
			"got %+v\n%s", res, describeGateEvidence(t, stateDir, deps.BeadID))
	}
	if res.BlockedAtState != "implementation" || !strings.HasPrefix(res.GateFailureReason, "commit_marker_missing") {
		t.Fatalf("the retake must fail its own commit_marker; got %+v\n%s",
			res, describeGateEvidence(t, stateDir, deps.BeadID))
	}
}

// TestDriveWorker_EpochBaseOutlivedByAHistoryRewriteFailsLoud pins the other
// half of "do not widen the range semantics": a persisted base is reused, so
// a worktree whose history was rewritten under the run now carries a base
// that HEAD cannot reach. That has to say so, rather than degrade into the
// generic "the stage left nothing" it would report if the base were quietly
// recaptured at the new HEAD.
func TestDriveWorker_EpochBaseOutlivedByAHistoryRewriteFailsLoud(t *testing.T) {
	be, deps, worktree, stateDir := newWorkerDriveFixture(t, "kernl-re3")
	driver := &historyRewriteDriver{
		reentryDriver: reentryDriver{be: be, beadID: deps.BeadID, worktree: worktree, stateDir: stateDir},
		t:             t,
	}
	deps.Driver = driver

	res, err := DriveBeadToTerminal(context.Background(), deps)
	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v\n%s", err, describeGateEvidence(t, stateDir, deps.BeadID))
	}
	if driver.implDispatches < 2 {
		t.Fatalf("precondition: the rewrite happens on the retry, which never ran (%d dispatches)", driver.implDispatches)
	}
	if res.Success {
		t.Fatalf("a rewritten history must not pass the gate; got %+v\n%s",
			res, describeGateEvidence(t, stateDir, deps.BeadID))
	}
	if !strings.HasPrefix(res.GateFailureReason, "commit_marker_history_rewritten") {
		t.Fatalf("the failure must name the rewrite rather than read as an idle stage; got %+v\n%s",
			res, describeGateEvidence(t, stateDir, deps.BeadID))
	}
}

// historyRewriteDriver is reentryDriver with one addition: its retry resets
// the worktree onto an unrelated line of history before writing the missing
// artifact, the way an agent that rebases or resets under the run does. The
// epoch's base survives that; HEAD's ancestry does not.
type historyRewriteDriver struct {
	reentryDriver
	t *testing.T
}

func (d *historyRewriteDriver) RunBead(ctx context.Context, in RunBeadInput) (RunBeadResult, error) {
	// Exactly once, on the first retry: the later retries this failure earns
	// must find the history already rewritten, not rewrite it again.
	if d.implDispatches == 1 {
		d.t.Helper()
		for _, args := range [][]string{
			{"checkout", "--orphan", "rewritten"},
			{"reset", "--hard"},
			{"commit", "--allow-empty", "-m", "rewritten history"},
		} {
			if out, err := exec.Command("git", append([]string{"-C", d.worktree}, args...)...).CombinedOutput(); err != nil {
				d.t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
	}
	return d.reentryDriver.RunBead(ctx, in)
}

// newWorkerDriveFixture builds the one-bead worker run every test in this file
// drives: a real git worktree with a single base commit, a fake tracker
// holding the bead at ready_for_implementation, and the graph run the
// implementation stage's decision_record gate writes its Decision node into.
// The caller supplies the driver, which is the only thing these tests differ
// in.
func newWorkerDriveFixture(t *testing.T, beadID string) (*epicFakeBackend, DriveBeadDeps, string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	worktree := t.TempDir()
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"},
		{"commit", "--allow-empty", "-m", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", worktree}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	be := newEpicFakeBackend()
	be.put(&backend.Bead{
		ID: beadID, Type: "task", Title: "Re-entered Child", State: "ready_for_implementation",
		ProfileID: "worker",
	})

	stateDir := t.TempDir()
	cfg := newDriveTestConfig()
	cfg.Vault = config.VaultConfig{Root: t.TempDir()}
	graphPath, err := graphDBFilePath(cfg)
	if err != nil {
		t.Fatalf("graphDBFilePath: %v", err)
	}
	// The worker profile's implementation stage carries a decision_record
	// exit gate, so a passing drive writes a real Decision node and needs
	// the run it belongs to to already exist.
	runID := seedRunAtPath(t, graphPath, "Re-entered Child", []BeadRef{
		{ID: beadID, Title: "Re-entered Child", TrackerKind: "bd", RepoPath: "/repo"},
	})

	return be, DriveBeadDeps{
		TrackerCommand: "bd",
		StateDir:       stateDir,
		VerifyCommand:  "bin/ci",
		Backend:        be,
		Config:         cfg,
		BeadID:         beadID,
		RepoPath:       t.TempDir(),
		Worktree:       worktree,
		RunID:          runID,
	}, worktree, stateDir
}

// describeGateEvidence renders every commit_marker evidence object the run
// wrote, so a failure of the test above reads as the diagnosis rather than as
// a bare state mismatch. This is exactly what the gate started recording for
// this purpose (backend.GateEvidence).
func describeGateEvidence(t *testing.T, stateDir, epicID string) string {
	t.Helper()
	path, err := resolveAttemptLedgerPath(stateDir, epicID)
	if err != nil {
		return "no ledger path: " + err.Error()
	}
	out := "gate evidence:\n"
	for _, rec := range readLedgerLines(t, path) {
		if rec.GateEvidence == nil {
			continue
		}
		ev := rec.GateEvidence
		reason := ""
		if rec.GateFailureReason != nil {
			reason = *rec.GateFailureReason
		}
		out += fmt.Sprintf("  attempt %d %s: base=%s head=%s range=%q treesDiffered=%v reason=%q\n",
			rec.AttemptNumber, rec.Stage, shortSHA(ev.BaseSHA), shortSHA(ev.HeadSHA),
			ev.RangeOutput, ev.TreesDiffered, reason)
	}
	return out
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
