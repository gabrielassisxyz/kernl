package app

import (
	"context"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// A stage with nothing to verify with must not be dispatched. Rule 4 of the
// prompt renders the verify command into a code block, so an empty one tells
// the agent to run nothing and then declare itself done - which is precisely
// what stopped being a hardcoded Go command in order to prevent.
func TestDriveBeadToTerminal_RefusesAnEmptyVerifyCommand(t *testing.T) {
	for _, verifyCommand := range []string{"", "   \t\n"} {
		be := newPersistingBackend()
		be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "ready_for_implementation"}
		driver := &scriptedDriver{be: be}

		_, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
			TrackerCommand: "bd",
			StateDir:       t.TempDir(),
			VerifyCommand:  verifyCommand,
			Backend:        be,
			Driver:         driver,
			Config:         newDriveTestConfig(),
			BeadID:         "kb-1",
			RepoPath:       "/tmp/repo",
			Worktree:       "/tmp/worktree",
		})

		if err == nil {
			t.Fatalf("verify command %q: expected a loud refusal rather than a stage with nothing to run", verifyCommand)
		}
		if !strings.Contains(err.Error(), "VerifyCommand") {
			t.Errorf("verify command %q: the error must name the field that fixes it, got: %v", verifyCommand, err)
		}
		if driver.calls != 0 {
			t.Errorf("verify command %q: no agent may be spawned, got %d dispatch(es)", verifyCommand, driver.calls)
		}
	}
}

// Containment for the one stage that acts outside the machine has to be
// structural: the agent is never spawned. A prompt asking it not to publish is
// not a control, as the run that motivated this proved by publishing anyway.
func TestDriveBeadToTerminal_StopBeforeStateDoesNotDispatchThatStage(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "ready_for_shipment"}

	driver := &scriptedDriver{be: be}
	res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand:  "bd",
		StateDir:        t.TempDir(),
		VerifyCommand:   "bin/ci",
		Backend:         be,
		Driver:          driver,
		Config:          newDriveTestConfig(),
		BeadID:          "kb-1",
		RepoPath:        "/tmp/repo",
		Worktree:        "/tmp/worktree",
		StopBeforeState: "shipment",
	})

	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}
	if !res.Success {
		t.Errorf("stopping short is a clean stop, got Success=false (FinalState=%q)", res.FinalState)
	}
	if driver.calls != 0 {
		t.Errorf("expected zero agent calls, got %d", driver.calls)
	}
	// The bead must not be claimed into the stage it never ran: a bead sitting
	// in "shipment" with no work done is the stranded state a resume has to be
	// reset out of by hand.
	if got := be.beads["kb-1"].State; got != "ready_for_shipment" {
		t.Errorf("bead advanced to %q; it must stay at ready_for_shipment so a later run resumes", got)
	}
	if res.FinalState != "ready_for_shipment" {
		t.Errorf("FinalState = %q, want ready_for_shipment", res.FinalState)
	}
}

// A run resumed after being killed mid-stage finds the bead already inside the
// stopped state; it must still refuse to dispatch it.
func TestDriveBeadToTerminal_StopBeforeStateAlsoStopsWhenAlreadyInIt(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "shipment"}

	driver := &scriptedDriver{be: be}
	res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand:  "bd",
		StateDir:        t.TempDir(),
		VerifyCommand:   "bin/ci",
		Backend:         be,
		Driver:          driver,
		Config:          newDriveTestConfig(),
		BeadID:          "kb-1",
		RepoPath:        "/tmp/repo",
		Worktree:        "/tmp/worktree",
		StopBeforeState: "shipment",
	})

	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}
	if driver.calls != 0 {
		t.Errorf("expected zero agent calls, got %d", driver.calls)
	}
	if res.FinalState != "shipment" {
		t.Errorf("FinalState = %q, want shipment", res.FinalState)
	}
}

// An unset StopBeforeState must not accidentally match a workflow that has no
// forward transition target.
func TestDriveBeadToTerminal_EmptyStopBeforeStateDrivesNormally(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "ready_for_implementation"}

	driver := &scriptedDriver{be: be}
	res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand: "bd",
		StateDir:       t.TempDir(),
		VerifyCommand:  "bin/ci",
		Backend:        be,
		Driver:         driver,
		Config:         newDriveTestConfig(),
		BeadID:         "kb-1",
		RepoPath:       "/tmp/repo",
		Worktree:       "/tmp/worktree",
	})

	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}
	if driver.calls == 0 {
		t.Error("expected the loop to dispatch normally when StopBeforeState is unset")
	}
	if !res.Success {
		t.Errorf("expected success, got FinalState=%q", res.FinalState)
	}
}
