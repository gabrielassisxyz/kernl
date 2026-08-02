package app

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/dispatch"
)

// settlingDA answers an open-design question by settling the shape. Distinct
// from decidingDA, whose answer is in the fork handover's own vocabulary -
// feeding one to the other's parser is exactly the confusion the two token
// sets exist to prevent, so the tests keep them apart too.
func settlingDA(shape string) *countingDA {
	return &countingDA{answer: "DESIGN: DECIDE\nSHAPE: " + shape + "\nthe operator's notes already settle this direction.\n"}
}

func escalatingOpenDesignDA() *countingDA {
	return &countingDA{answer: "DESIGN: ESCALATE\nranking these two costs against each other is the operator's call.\n"}
}

// openDesignBead is a bead the classifier marks DepthGate, via a marker its
// own description carries. Asserted below rather than assumed: if
// dispatch's marker list changes, this fixture must fail loudly rather than
// quietly stop testing the gate.
func openDesignBead() *backend.Bead {
	return &backend.Bead{
		ID:          "ep-1.4",
		State:       "implementation",
		Title:       "Decide how conversion is counted",
		Description: "The shape of this is an open question - whether unmarked requests count as friction is not yet decided.",
	}
}

func settledBead() *backend.Bead {
	return &backend.Bead{
		ID:          "ep-1.5",
		Type:        "bug",
		State:       "implementation",
		Title:       "The loader raises before the default path is applied",
		Description: "A missing config file crashes the loader.",
		Acceptance:  "A missing config file falls back to the default path.",
	}
}

func depthGateDeps(t *testing.T, be *persistingBackend, da DA) DriveBeadDeps {
	t.Helper()
	return DriveBeadDeps{
		Backend:  be,
		RepoPath: "/repo",
		BeadID:   "ep-1.4",
		DA:       da,
		Config:   decisionGateTestConfig(t),
	}
}

func TestClassifyDepth_TheFixturesThisFileReliesOn(t *testing.T) {
	if got := dispatch.ClassifyDepth(*openDesignBead()).Depth; got != dispatch.DepthGate {
		t.Fatalf("openDesignBead must classify as %q for this file's tests to mean anything, got %q", dispatch.DepthGate, got)
	}
	if got := dispatch.ClassifyDepth(*settledBead()).Depth; got == dispatch.DepthGate {
		t.Fatalf("settledBead must NOT classify as %q, got %q", dispatch.DepthGate, got)
	}
}

// TestHandleDepthGate_OpenDesignBeadIsSettledByTheDABeforeDispatch is the
// acceptance test for the defect this gate closes: nothing read the depth
// classifier, so a bead the classifier itself marks as a decision gate was
// handed to an implementer that chose an approach nobody committed to.
func TestHandleDepthGate_OpenDesignBeadIsSettledByTheDABeforeDispatch(t *testing.T) {
	be := newPersistingBackend()
	bead := openDesignBead()
	be.beads[bead.ID] = bead
	dir := t.TempDir()
	da := settlingDA("count unmarked requests as friction, and say so in the report")

	got, err := handleDepthGate(context.Background(), depthGateDeps(t, be, da), bead, "ep-1", dir, "implementation", 0)
	if err != nil {
		t.Fatalf("handleDepthGate: %v", err)
	}
	if got.Blocked {
		t.Fatalf("a settled shape must let the bead be dispatched, got %+v", got.Result)
	}
	if da.calls != 1 {
		t.Fatalf("the DA must be consulted exactly once, got %d", da.calls)
	}
	if got.ForkGateCalls != 1 {
		t.Errorf("a real consultation must consume the shared fork/decision budget, got %d", got.ForkGateCalls)
	}

	// The settled shape has to reach the implementer, and the only path
	// that carries it is the fork-answer artifact the stage prompt reads.
	answer := readForkAnswerArtifact(bead.ID, dir)
	if !strings.Contains(answer, "count unmarked requests as friction") {
		t.Errorf("the settled shape must be written where the implementer reads it, got %q", answer)
	}
}

func TestHandleDepthGate_TheDAsEscalationBlocksForTheOperator(t *testing.T) {
	be := newPersistingBackend()
	bead := openDesignBead()
	be.beads[bead.ID] = bead
	dir := t.TempDir()
	da := escalatingOpenDesignDA()

	got, err := handleDepthGate(context.Background(), depthGateDeps(t, be, da), bead, "ep-1", dir, "implementation", 0)
	if err != nil {
		t.Fatalf("handleDepthGate: %v", err)
	}
	if !got.Blocked {
		t.Fatal("a bead the DA declined to settle must not be dispatched")
	}
	bd, _ := be.Get(bead.ID, "/repo")
	if BlockedCauseFromLabels(bd.Labels) != BlockedCauseJudgment {
		t.Errorf("labels = %v, want wf:blocked:judgment - this is a genuine escalation", bd.Labels)
	}
	if readForkAnswerArtifact(bead.ID, dir) != "" {
		t.Error("an escalation settles nothing, so it must write no answer for an implementer to act on")
	}
}

// TestHandleDepthGate_NoDAConfiguredBlocksRatherThanDispatching pins the
// degenerate case that makes this one code path rather than two: da.agent is
// unset by default, and with nothing to ask, the gate falls back to exactly
// the block it would otherwise route around.
func TestHandleDepthGate_NoDAConfiguredBlocksRatherThanDispatching(t *testing.T) {
	be := newPersistingBackend()
	bead := openDesignBead()
	be.beads[bead.ID] = bead
	dir := t.TempDir()

	got, err := handleDepthGate(context.Background(), depthGateDeps(t, be, nil), bead, "ep-1", dir, "implementation", 0)
	if err != nil {
		t.Fatalf("handleDepthGate: %v", err)
	}
	if !got.Blocked {
		t.Fatal("with no DA configured there is nothing to settle the design, so the bead must not be dispatched")
	}
	if !strings.Contains(got.Result.GateFailureReason, string(ForkCauseNoDAConfigured)) {
		t.Errorf("GateFailureReason = %q, want it to name %q", got.Result.GateFailureReason, ForkCauseNoDAConfigured)
	}
}

// TestHandleDepthGate_ASettledBeadIsNeverPutToTheDA is the other half of the
// contract: this gate must be invisible to every bead that is ordinary work.
func TestHandleDepthGate_ASettledBeadIsNeverPutToTheDA(t *testing.T) {
	be := newPersistingBackend()
	bead := settledBead()
	be.beads[bead.ID] = bead
	dir := t.TempDir()
	da := settlingDA("should never be asked")

	deps := depthGateDeps(t, be, da)
	deps.BeadID = bead.ID
	got, err := handleDepthGate(context.Background(), deps, bead, "ep-1", dir, "implementation", 0)
	if err != nil {
		t.Fatalf("handleDepthGate: %v", err)
	}
	if got.Blocked {
		t.Fatal("a bead with no open-design language must be dispatched untouched")
	}
	if da.calls != 0 {
		t.Errorf("the DA must not be asked about ordinary work, got %d calls", da.calls)
	}
	if got.ForkGateCalls != 0 {
		t.Errorf("a gate that did not consult must not consume the budget, got %d", got.ForkGateCalls)
	}
}

// TestHandleDepthGate_AskedAtMostOncePerBead covers the resumed-run shape:
// the in-memory budget resets when a bead is picked back up by a separate
// `kernl epic run`, so only the marker on disk stops the same question being
// paid for twice.
func TestHandleDepthGate_AskedAtMostOncePerBead(t *testing.T) {
	be := newPersistingBackend()
	bead := openDesignBead()
	be.beads[bead.ID] = bead
	dir := t.TempDir()
	da := settlingDA("count unmarked requests as friction")

	if _, err := handleDepthGate(context.Background(), depthGateDeps(t, be, da), bead, "ep-1", dir, "implementation", 0); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if da.calls != 1 {
		t.Fatalf("setup: want exactly one consultation before the resume, got %d", da.calls)
	}

	// A separate run: the counter is back at zero, the marker is not.
	got, err := handleDepthGate(context.Background(), depthGateDeps(t, be, da), bead, "ep-1", dir, "implementation", 0)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if da.calls != 1 {
		t.Errorf("a bead whose design was already put to the DA must not be asked again, got %d calls", da.calls)
	}
	if got.Blocked {
		t.Error("the answer from the first call still stands, so the bead must still be dispatchable")
	}
}

func TestHandleDepthGate_SpentSharedBudgetEscalatesWithoutAskingTheDA(t *testing.T) {
	be := newPersistingBackend()
	bead := openDesignBead()
	be.beads[bead.ID] = bead
	dir := t.TempDir()
	da := settlingDA("should never be asked")

	got, err := handleDepthGate(context.Background(), depthGateDeps(t, be, da), bead, "ep-1", dir, "implementation", forkHandoverLimit)
	if err != nil {
		t.Fatalf("handleDepthGate: %v", err)
	}
	if !got.Blocked {
		t.Fatal("with the shared budget spent there is nowhere to put another consultation")
	}
	if da.calls != 0 {
		t.Errorf("the DA must not be consulted once the shared budget is spent, got %d", da.calls)
	}
}

func TestDepthGateAlreadyRouted_AnUnreadableArtifactDirIsNotReadAsAFreshBead(t *testing.T) {
	dir := t.TempDir()
	// A directory where the marker file should be: Stat succeeds, so this
	// is not the interesting case; make the path itself unreadable instead
	// by pointing at a file used as a directory component.
	blocker := dir + "/blocker"
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if _, err := depthGateAlreadyRouted("ep-1.4", blocker); err == nil {
		t.Error("an artifact directory that cannot be read must surface, not be reported as 'never asked'")
	}
}

func TestParseOpenDesignAnswer_RejectsEverythingThatIsNotAnAnswer(t *testing.T) {
	cases := []struct {
		name   string
		answer string
	}{
		{"empty", "   "},
		{"no verdict line", "I think we should probably index by recency.\n"},
		{"fork vocabulary is not this question's", "FORK: DECIDE\nCHOSEN: recency-first\nbecause it is simpler.\n"},
		{"decide with no shape line", "DESIGN: DECIDE\nbecause it is simpler.\n"},
		{"decide with an empty shape", "DESIGN: DECIDE\nSHAPE:\nbecause it is simpler.\n"},
		{"a shape with no reasoning under it", "DESIGN: DECIDE\nSHAPE: index by recency\n"},
		{"escalate with no reason", "DESIGN: ESCALATE\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseOpenDesignAnswer(tc.answer); err == nil {
				t.Errorf("want a refusal - this gate must fail toward stopping, never toward dispatching")
			}
		})
	}
}

func TestParseOpenDesignAnswer_ReadsASettledShapeAndItsReason(t *testing.T) {
	got, err := ParseOpenDesignAnswer("DESIGN: DECIDE\nSHAPE: count unmarked requests as friction\nthe operator's own notes already take this position.\n")
	if err != nil {
		t.Fatalf("ParseOpenDesignAnswer: %v", err)
	}
	if got.Action != ForkActionDecided {
		t.Errorf("Action = %v, want ForkActionDecided", got.Action)
	}
	if got.Cause != ForkCauseOpenDesignDecided {
		t.Errorf("Cause = %q, want %q - a settled design must be distinguishable from a fork an implementer handed over", got.Cause, ForkCauseOpenDesignDecided)
	}
	if got.ChosenOption != "count unmarked requests as friction" {
		t.Errorf("ChosenOption = %q", got.ChosenOption)
	}
	if !strings.Contains(got.Reason, "already take this position") {
		t.Errorf("Reason = %q, want the DA's own reasoning preserved", got.Reason)
	}
}
