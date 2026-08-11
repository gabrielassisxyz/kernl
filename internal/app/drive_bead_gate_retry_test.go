package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDriveBead_GateFailureRetriedInRunLeavesBothAttemptsInTheLedger is the
// outcome this step exists for. A stage that fails its exit gate for a
// mechanical reason used to end the call - and with it the whole epic run,
// since one unsuccessful bead fails the run fast (internal/epic/dispatch.go's
// recordResult) - leaving a person to type `kernl epic run` again for the
// resume that was always allowed. It is now spent inside the run.
//
// The ledger is where the repair has to remain visible: two rows for one
// stage, the first naming what the gate refused, the second passing. Nothing
// collapses them, which is why this step needs no per-attempt history of its
// own.
func TestDriveBead_GateFailureRetriedInRunLeavesBothAttemptsInTheLedger(t *testing.T) {
	be, deps, worktree, stateDir := newWorkerDriveFixture(t, "kernl-gr1")
	deps.Driver = &reentryDriver{be: be, beadID: deps.BeadID, worktree: worktree, stateDir: stateDir}

	res, err := DriveBeadToTerminal(context.Background(), deps)
	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}
	if !res.Success {
		t.Fatalf("a stage whose retry satisfies the gate must finish the run; got %+v\n%s",
			res, describeGateEvidence(t, stateDir, deps.BeadID))
	}

	rows := ledgerRowsForStage(t, stateDir, deps.BeadID, "implementation")
	if len(rows) != 2 {
		t.Fatalf("want two implementation attempts in the ledger (the failure and its repair), got %d", len(rows))
	}
	if rows[0].GatePassed || rows[0].GateFailureReason == nil ||
		!strings.HasPrefix(*rows[0].GateFailureReason, "artifact_missing") {
		t.Errorf("the first row must keep the failure that triggered the retry; got %+v", rows[0])
	}
	if !rows[1].GatePassed {
		t.Errorf("the second row must record the repair; got %+v", rows[1])
	}
}

// TestDriveBead_GateRetryBudgetIsSpentNotRenewed is the test that would catch
// an in-run counter kept in memory instead of on the bead. The budget belongs
// to the block, is written as wf:blocked-retries:*, and a second call has to
// find it already spent - otherwise a stage that fails forever earns two more
// dispatches every time anyone re-runs the epic, which is exactly what that
// label was introduced to stop.
func TestDriveBead_GateRetryBudgetIsSpentNotRenewed(t *testing.T) {
	_, deps, worktree, _ := newWorkerDriveFixture(t, "kernl-gr2")
	driver := &markerOnlyDriver{worktree: worktree}
	deps.Driver = driver

	first, _ := DriveBeadToTerminal(context.Background(), deps)
	if first.Success || first.BlockedAtState != "implementation" {
		t.Fatalf("a stage that never satisfies its gate must end blocked; got %+v", first)
	}
	if want := 1 + mechanicalBlockRetryLimit; driver.dispatches != want {
		t.Fatalf("want %d dispatches (the attempt plus its retries), got %d", want, driver.dispatches)
	}
	if !strings.HasPrefix(first.GateFailureReason, "artifact_missing") {
		t.Errorf("the blocked result must still name what the gate refused, not just the cause; got %q",
			first.GateFailureReason)
	}

	spent := driver.dispatches
	second, _ := DriveBeadToTerminal(context.Background(), deps)
	if driver.dispatches != spent {
		t.Fatalf("a second call must not renew the budget: %d dispatches before, %d after", spent, driver.dispatches)
	}
	if second.Success {
		t.Fatalf("the bead must stay blocked for a person; got %+v", second)
	}
}

// TestDriveBead_RetryPromptCarriesThePriorGateFailure covers the half of this
// step that needed nothing built: the retake's prompt already tells the agent
// what the gate refused, through LastGateFailure and renderPriorGateFailure.
// An automatic retry that did not carry it would re-dispatch an agent with no
// idea why it is being asked again, which is a dispatch spent restating the
// failure rather than repairing it.
func TestDriveBead_RetryPromptCarriesThePriorGateFailure(t *testing.T) {
	be, deps, worktree, stateDir := newWorkerDriveFixture(t, "kernl-gr3")
	driver := &promptCapturingDriver{inner: &reentryDriver{
		be: be, beadID: deps.BeadID, worktree: worktree, stateDir: stateDir,
	}}
	deps.Driver = driver

	if _, err := DriveBeadToTerminal(context.Background(), deps); err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}
	if len(driver.prompts) < 2 {
		t.Fatalf("precondition: the stage must have been dispatched twice, got %d", len(driver.prompts))
	}
	if strings.Contains(driver.prompts[0], "artifact_missing") {
		t.Errorf("the FIRST dispatch has no prior failure to carry:\n%s", driver.prompts[0])
	}
	if !strings.Contains(driver.prompts[1], "artifact_missing") {
		t.Errorf("the retry's prompt must name what the gate refused:\n%s", driver.prompts[1])
	}
}

// TestDriveBead_RetriesDoNotExhaustTheStageCeiling pins the arithmetic
// maxStages needed once retries became possible. Its error text tells an
// operator to check the workflow for cycles, so a run that spent its
// iterations on legitimate retries must not end there - a ceiling that fires
// for two unrelated reasons cannot say which one it hit.
func TestDriveBead_RetriesDoNotExhaustTheStageCeiling(t *testing.T) {
	// The ceiling is measured, not guessed: a run of this same workflow that
	// never fails a gate is exactly what maxStages is meant to cover. Any
	// iteration a retry adds on top of that has to come out of the retry
	// allowance instead, so handing the retrying run this number is what makes
	// the test able to fail at all. A generous ceiling would pass either way.
	//
	// The +1 is the iteration that observes the terminal state and returns:
	// it runs the loop but dispatches nothing, so deps.Log - which fires once
	// per dispatched stage - does not see it.
	ceiling := dispatchesOfAClearRun(t, "kernl-gr4a") + 1

	be, deps, worktree, stateDir := newWorkerDriveFixture(t, "kernl-gr4b")
	deps.Driver = &failEveryStageOnceDriver{
		be: be, beadID: deps.BeadID, worktree: worktree, stateDir: stateDir,
		failFirst: true, dispatchesByState: map[string]int{},
	}
	deps.MaxStages = ceiling

	res, err := DriveBeadToTerminal(context.Background(), deps)
	if err != nil {
		t.Fatalf("a retry at every stage must not spend the ceiling a clear run needs (%d): %v\n%s",
			ceiling, err, describeGateEvidence(t, stateDir, deps.BeadID))
	}
	if !res.Success || res.FinalState != "awaiting_integration" {
		t.Fatalf("every stage retried once must still reach the end of the workflow; got %+v\n%s",
			res, describeGateEvidence(t, stateDir, deps.BeadID))
	}
}

// dispatchesOfAClearRun drives the same worker workflow with a driver that
// satisfies every gate first time, and reports how many stages it dispatched.
func dispatchesOfAClearRun(t *testing.T, beadID string) int {
	t.Helper()
	be, deps, worktree, stateDir := newWorkerDriveFixture(t, beadID)
	deps.Driver = &failEveryStageOnceDriver{
		be: be, beadID: deps.BeadID, worktree: worktree, stateDir: stateDir,
		failFirst: false, dispatchesByState: map[string]int{},
	}
	dispatched := 0
	deps.Log = func(int, string) { dispatched++ }

	res, err := DriveBeadToTerminal(context.Background(), deps)
	if err != nil || !res.Success {
		t.Fatalf("the baseline run must succeed: %v %+v", err, res)
	}
	if dispatched == 0 {
		t.Fatal("the baseline dispatched nothing, so it cannot bound anything")
	}
	return dispatched
}

func TestMechanicalResumeAllowed(t *testing.T) {
	retries := func(n int) string { return fmt.Sprintf("%s%d", blockedRetryLabelPrefix, n) }
	cases := []struct {
		name    string
		labels  []string
		state   string
		allowed bool
	}{
		{
			name:    "a fresh mechanical block resumes",
			labels:  []string{"wf:state:implementation", blockedCauseLabelPrefix + string(BlockedCauseGate)},
			state:   "implementation",
			allowed: true,
		},
		{
			name:    "a subprocess block is mechanical too",
			labels:  []string{"wf:state:integration", blockedCauseLabelPrefix + string(BlockedCauseSubprocess)},
			state:   "integration",
			allowed: true,
		},
		{
			// The one cause that means a person is needed. It must never be
			// reachable by an automatic retry, whatever the budget says.
			name:    "a judgment block never resumes",
			labels:  []string{"wf:state:implementation", blockedCauseLabelPrefix + string(BlockedCauseJudgment)},
			state:   "implementation",
			allowed: false,
		},
		{
			// A bead blocked by hand carries no cause label, and is read the
			// same way as a judgment block.
			name:    "no recorded cause never resumes",
			labels:  []string{"wf:state:implementation"},
			state:   "implementation",
			allowed: false,
		},
		{
			name:    "a spent budget stops resuming",
			labels:  []string{"wf:state:implementation", blockedCauseLabelPrefix + string(BlockedCauseGate), retries(mechanicalBlockRetryLimit)},
			state:   "implementation",
			allowed: false,
		},
		{
			name:    "one retry short of the limit still resumes",
			labels:  []string{"wf:state:implementation", blockedCauseLabelPrefix + string(BlockedCauseGate), retries(mechanicalBlockRetryLimit - 1)},
			state:   "implementation",
			allowed: true,
		},
		{
			// Without the stale wf:state label there is no stage to go back
			// to, so resuming would dispatch the bead at "blocked".
			name:    "an unrecoverable stage never resumes",
			labels:  []string{blockedCauseLabelPrefix + string(BlockedCauseGate)},
			state:   "",
			allowed: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, allowed := mechanicalResumeAllowed(tc.labels)
			if state != tc.state || allowed != tc.allowed {
				t.Fatalf("mechanicalResumeAllowed(%v) = (%q, %v), want (%q, %v)",
					tc.labels, state, allowed, tc.state, tc.allowed)
			}
		})
	}
}

// promptCapturingDriver records the prompt each dispatch was given - the last
// of BuildStageArgs' arguments - and otherwise behaves exactly like the driver
// it wraps.
type promptCapturingDriver struct {
	inner   *reentryDriver
	prompts []string
}

func (d *promptCapturingDriver) RunBead(ctx context.Context, in RunBeadInput) (RunBeadResult, error) {
	if len(in.Args) > 0 {
		d.prompts = append(d.prompts, in.Args[len(in.Args)-1])
	} else {
		d.prompts = append(d.prompts, "")
	}
	return d.inner.RunBead(ctx, in)
}

// failEveryStageOnceDriver fails the exit gate of every stage it is dispatched
// at exactly once, by producing nothing on the first dispatch and the stage's
// artifact on the second. It is the worst realistic case for the iteration
// ceiling: every stage costs its retry.
//
// With failFirst false it satisfies every gate on the first dispatch instead,
// which is the same workflow with no retries in it - the baseline the ceiling
// is measured from.
type failEveryStageOnceDriver struct {
	be       *epicFakeBackend
	beadID   string
	worktree string
	stateDir string

	failFirst         bool
	dispatchesByState map[string]int
}

func (d *failEveryStageOnceDriver) RunBead(_ context.Context, _ RunBeadInput) (RunBeadResult, error) {
	bd, err := d.be.Get(d.beadID, "")
	if err != nil || bd == nil {
		return RunBeadResult{Success: false}, fmt.Errorf("bead %s not found: %w", d.beadID, err)
	}
	dir := filepath.Join(d.stateDir, "run", d.beadID, d.beadID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return RunBeadResult{Success: false}, fmt.Errorf("artifact dir: %w", err)
	}

	d.dispatchesByState[bd.State]++
	withholdArtifact := d.failFirst && d.dispatchesByState[bd.State] == 1

	switch bd.State {
	case "implementation":
		if d.dispatchesByState[bd.State] == 1 {
			// The commit lands on the first dispatch either way: the stage
			// epoch is what lets a retry pass commit_marker on it without
			// committing again.
			if err := commitRealChange(d.worktree, "work.txt", "did the work\n", "stage: implementation: did the work"); err != nil {
				return RunBeadResult{Success: false}, fmt.Errorf("implementation commit: %w", err)
			}
		}
		if withholdArtifact {
			return RunBeadResult{FinalState: "ok", Success: true, SessionID: "ses"}, nil
		}
		if err := os.WriteFile(filepath.Join(dir, "decision-record.json"), []byte(fakeDecisionRecord), 0o644); err != nil {
			return RunBeadResult{Success: false}, fmt.Errorf("decision record: %w", err)
		}
	case "implementation_review":
		if withholdArtifact {
			return RunBeadResult{FinalState: "ok", Success: true, SessionID: "ses"}, nil
		}
		if err := os.WriteFile(filepath.Join(dir, "implementation-review.md"),
			[]byte("code matches the plan\n\nVERDICT: PASS"), 0o644); err != nil {
			return RunBeadResult{Success: false}, fmt.Errorf("review artifact: %w", err)
		}
	}
	return RunBeadResult{FinalState: "ok", Success: true, SessionID: "ses"}, nil
}

// ledgerRowsForStage reads back every attempt this run recorded for one bead
// and stage, in the order they were written.
func ledgerRowsForStage(t *testing.T, stateDir, beadID, stage string) []StageAttemptRecord {
	t.Helper()
	path, err := resolveAttemptLedgerPath(stateDir, beadID)
	if err != nil {
		t.Fatalf("resolveAttemptLedgerPath: %v", err)
	}
	var rows []StageAttemptRecord
	for _, rec := range readLedgerLines(t, path) {
		if rec.BeadID == beadID && rec.Stage == stage {
			rows = append(rows, rec)
		}
	}
	return rows
}
