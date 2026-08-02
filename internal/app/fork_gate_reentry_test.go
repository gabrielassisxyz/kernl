package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// forkReentryDriver is a named, counted-call fake BeadDriver (AGENTS.md §4):
// its FIRST invocation hands a fork over exactly the way a real implementer
// would (writing fork.md into this bead's own artifact directory instead of
// committing), and every later invocation just records the args it was
// given so the test can inspect what the prompt carried.
type forkReentryDriver struct {
	stateDir     string
	epicID       string
	beadID       string
	handoverBody string
	calls        int
	// argsByCall captures in.Args for every invocation, 1-indexed by call
	// number, so the test can assert on exactly the SECOND run's prompt
	// without guessing which index that is.
	argsByCall map[int][]string
}

func (d *forkReentryDriver) RunBead(_ context.Context, in RunBeadInput) (RunBeadResult, error) {
	d.calls++
	if d.argsByCall == nil {
		d.argsByCall = map[int][]string{}
	}
	d.argsByCall[d.calls] = in.Args

	if d.calls == 1 {
		artifactDir, err := ArtifactDirPath(d.stateDir, d.epicID, d.beadID)
		if err != nil {
			return RunBeadResult{}, err
		}
		if err := os.MkdirAll(artifactDir, 0o755); err != nil {
			return RunBeadResult{}, err
		}
		if err := os.WriteFile(filepath.Join(artifactDir, "fork.md"), []byte(d.handoverBody), 0o644); err != nil {
			return RunBeadResult{}, err
		}
	}
	return RunBeadResult{FinalState: "ok", Success: true, SessionID: "ses_test"}, nil
}

// forkReentryWorkflow is the minimal custom workflow this test needs: one
// action state ("implementation") gated ONLY by decision_record - no
// commit_marker, so the test needs no real git worktree - registered
// through the same backend.RegisterWorkflow path examples/custom-workflow
// itself uses (see custom_workflow_e2e_test.go).
func forkReentryWorkflow(id string) backend.WorkflowDescriptor {
	return backend.WorkflowDescriptor{
		ID:             id,
		InitialState:   "ready_for_implementation",
		States:         []string{"ready_for_implementation", "implementation", "done"},
		TerminalStates: []string{"done"},
		Transitions: []backend.WorkflowTransition{
			{From: "ready_for_implementation", To: "implementation"},
			{From: "implementation", To: "done"},
		},
		RetakeState:  "ready_for_implementation",
		Owners:       map[string]backend.ActionOwnerKind{"implementation": backend.ActionOwnerAgent},
		ActionStates: []string{"implementation"},
		QueueStates:  []string{"ready_for_implementation"},
		QueueActions: map[string]string{
			"ready_for_implementation": "implementation",
		},
		ExitGates: map[string][]backend.WorkflowExitGate{
			"implementation": {{Type: "decision_record", Path: "<artifact_dir>/decision-record.md"}},
		},
	}
}

// TestDriveBeadToTerminal_ForkHandover_ReentersSameStageWithDAAnswerInPrompt
// is the mechanism proof the plan's §0 demanded: not just that
// DriveBeadToTerminal returns without error, but that the SAME stage
// genuinely runs a second time (a counted-call driver proves it), and that
// the DA's answer is actually present in the SECOND invocation's own prompt -
// a version that silently never re-ran anything, or one that re-ran WITHOUT
// carrying the answer, would both pass a test that only checked the return
// value.
//
// The bead sits at "implementation" (an ACTION state, not a queue state)
// when the handover is found. Reading DriveBeadToTerminal's own claiming
// logic (runtime.IsAgentClaimable, backend.ForwardTransitionTarget) shows
// IsAgentClaimable is true only for a QUEUE state whose owner is an agent
// (backend.DeriveWorkflowRuntimeState: "resolved.Phase == StepPhaseQueued
// && ownerKind == ActionOwnerAgent") - an ACTIVE state like "implementation"
// is never claimable. So a plain `continue`, with no Backend.Update and no
// Backend.Rewind at all, is enough: the next iteration re-fetches the bead
// (still "implementation"), finds it not claimable, and dispatches the SAME
// stage again. This is why handleForkGate's own Reenter path never touches
// bead state - unlike rewindAfterReviewRejection, which crosses FROM
// implementation_review back down TO implementation and genuinely needs
// Backend.Rewind to cross that boundary.
func TestDriveBeadToTerminal_ForkHandover_ReentersSameStageWithDAAnswerInPrompt(t *testing.T) {
	const profileID = "fork_reentry_test"
	backend.ClearWorkflowRegistry()
	backend.RegisterWorkflow(forkReentryWorkflow(profileID))
	defer backend.ClearWorkflowRegistry()

	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "ready_for_implementation", ProfileID: profileID}

	stateDir := t.TempDir()
	driver := &forkReentryDriver{stateDir: stateDir, epicID: "kb-1", beadID: "kb-1", handoverBody: forkHandoverRecord}
	da := decidingDA("relevance-first")
	cfg := forkGateTestConfig(t)

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
		MaxStages:      8,
	})
	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}

	// The mechanism proof: exactly two invocations of the SAME stage.
	if driver.calls != 2 {
		t.Fatalf("expected exactly 2 agent invocations (proves the stage re-ran), got %d; res=%+v", driver.calls, res)
	}
	if da.calls != 1 {
		t.Errorf("expected the DA to be consulted exactly once, got %d calls", da.calls)
	}

	secondPrompt := strings.Join(driver.argsByCall[2], "\n")
	if !strings.Contains(secondPrompt, "CHOSEN: relevance-first") {
		t.Errorf("the SECOND invocation's prompt must carry the DA's own answer:\n%s", secondPrompt)
	}
	if !strings.Contains(secondPrompt, "not open for re-litigation") {
		t.Errorf("the SECOND invocation's prompt must tell the implementer the choice is settled:\n%s", secondPrompt)
	}

	firstPrompt := strings.Join(driver.argsByCall[1], "\n")
	if strings.Contains(firstPrompt, "CHOSEN: relevance-first") {
		t.Errorf("the FIRST invocation ran before any fork was ever decided - it must not already carry an answer:\n%s", firstPrompt)
	}
	if !strings.Contains(firstPrompt, "may be handed over instead of decided alone") {
		t.Errorf("the FIRST invocation's prompt must tell the implementer it may hand a fork over (a DA is configured and decision_record is armed):\n%s", firstPrompt)
	}

	// The SECOND invocation never wrote decision-record.md (this fake only
	// exercises the handover on its first call), so its own exit gate
	// genuinely fails and the bead blocks there - proof that the re-entered
	// attempt is a REAL second attempt at the SAME stage, evaluated by the
	// SAME ordinary exit gate, not a second copy of the first success.
	if res.Success || res.FinalState != "blocked" || res.BlockedAtState != "implementation" {
		t.Errorf("res = %+v, want the second (real, gate-checked) attempt to block at implementation for missing decision-record.md", res)
	}
}

// firstForkHandoverRecord and secondForkHandoverRecord are two genuinely
// DISTINCT forks (different question, different options) - proving finding 5
// requires two REAL decided forks in one call, not the same one written
// twice.
const firstForkHandoverRecord = `## Fork

which ranking backend to use

## Options Considered

bm25; embeddings

## What Would Have To Agree

nothing outside this bead
`

const secondForkHandoverRecord = `## Fork

which field-naming convention for the new endpoint

## Options Considered

camelCase; snake_case

## What Would Have To Agree

nothing outside this bead
`

// sequentialDA is a named fake (AGENTS.md §4) answering with a DIFFERENT,
// pre-scripted decision each time it is consulted, in order - needed here
// because decidingDA always answers the same CHOSEN option, and this test
// needs two DISTINCT forks each decided with their own chosen option.
type sequentialDA struct {
	answers []string
	calls   int
}

func (d *sequentialDA) Consult(_ context.Context, _ string) (string, error) {
	i := d.calls
	d.calls++
	if i >= len(d.answers) {
		return "", fmt.Errorf("sequentialDA: no answer configured for call %d", i+1)
	}
	return d.answers[i], nil
}

// forkReentryTwiceDriver is a named, counted-call fake BeadDriver (AGENTS.md
// §4) whose first TWO invocations each hand over a genuinely distinct fork -
// exactly the case forkHandoverLimit's own doc comment says must not be
// mistaken for a repeated objection: "an implementer can legitimately meet
// more than one genuine fork while writing a single stage's code". Every
// later invocation just records its own args, like forkReentryDriver does.
type forkReentryTwiceDriver struct {
	stateDir   string
	epicID     string
	beadID     string
	calls      int
	argsByCall map[int][]string
}

func (d *forkReentryTwiceDriver) RunBead(_ context.Context, in RunBeadInput) (RunBeadResult, error) {
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
		if err := os.WriteFile(filepath.Join(artifactDir, "fork.md"), []byte(firstForkHandoverRecord), 0o644); err != nil {
			return RunBeadResult{}, err
		}
	case 2:
		if err := os.WriteFile(filepath.Join(artifactDir, "fork.md"), []byte(secondForkHandoverRecord), 0o644); err != nil {
			return RunBeadResult{}, err
		}
	}
	return RunBeadResult{FinalState: "ok", Success: true, SessionID: "ses_test"}, nil
}

// TestDriveBeadToTerminal_TwoForksInOneCall_BothAnswersSurviveIntoTheNextPrompt
// is the full-loop mechanism proof for finding 5: two DISTINCT forks decided
// in the same call, proving the prompt of the run AFTER the second one
// carries BOTH answers, not just the most recent.
func TestDriveBeadToTerminal_TwoForksInOneCall_BothAnswersSurviveIntoTheNextPrompt(t *testing.T) {
	const profileID = "fork_reentry_twice_test"
	backend.ClearWorkflowRegistry()
	backend.RegisterWorkflow(forkReentryWorkflow(profileID))
	defer backend.ClearWorkflowRegistry()

	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "ready_for_implementation", ProfileID: profileID}

	stateDir := t.TempDir()
	driver := &forkReentryTwiceDriver{stateDir: stateDir, epicID: "kb-1", beadID: "kb-1"}
	da := &sequentialDA{answers: []string{
		"FORK: DECIDE\nCHOSEN: bm25\nsiblings already assume this format.\n",
		"FORK: DECIDE\nCHOSEN: camelCase\nmatches the wire contract already used.\n",
	}}
	cfg := forkGateTestConfig(t)

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
		MaxStages:      8,
	})
	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}

	// Three invocations: the first fork, the second fork, and a real third
	// attempt at the same stage.
	if driver.calls != 3 {
		t.Fatalf("expected exactly 3 agent invocations, got %d; res=%+v", driver.calls, res)
	}
	if da.calls != 2 {
		t.Errorf("expected the DA to be consulted exactly twice (once per genuine fork), got %d calls", da.calls)
	}

	thirdPrompt := strings.Join(driver.argsByCall[3], "\n")
	if !strings.Contains(thirdPrompt, "CHOSEN: bm25") {
		t.Errorf("the THIRD invocation's prompt must still carry the FIRST fork's own answer:\n%s", thirdPrompt)
	}
	if !strings.Contains(thirdPrompt, "CHOSEN: camelCase") {
		t.Errorf("the THIRD invocation's prompt must carry the SECOND fork's own answer:\n%s", thirdPrompt)
	}

	// The third attempt never wrote decision-record.md, so its own exit gate
	// genuinely fails and the bead blocks there - proof this is a REAL third
	// attempt, evaluated by the ordinary exit gate, not a replay of anything
	// earlier.
	if res.Success || res.FinalState != "blocked" || res.BlockedAtState != "implementation" {
		t.Errorf("res = %+v, want the third (real, gate-checked) attempt to block at implementation for missing decision-record.md", res)
	}
}
