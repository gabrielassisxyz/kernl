package backend

import (
	"os/exec"
	"strings"
	"testing"
)

// TestEvaluateExitGateWithEvidence_CommitMarkerEmptyRangeRecordsWhatItSaw is
// the case the investigation this bead came from could not read backwards
// from: a commit_marker failure that used to record only
// "commit_marker_missing: implementation" now also names the BaseSHA, the
// resolved HEAD, the worktree it ran in, and the (empty) range it saw.
func TestEvaluateExitGateWithEvidence_CommitMarkerEmptyRangeRecordsWhatItSaw(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	wf := commitMarkerOnlyWorkflow()

	dir := t.TempDir()
	initGitRepo(t, dir)
	baseSHA := gitCommit(t, dir, "base: captured before dispatch")
	// The stage runs and exits zero, but never commits: HEAD is still
	// exactly BaseSHA, so the range is empty.

	ok, reason, ev := EvaluateExitGateWithEvidence(wf, ExitGateContext{FromState: "implementation", WorktreePath: dir, BeadID: "kb-1", BaseSHA: baseSHA})
	if ok {
		t.Fatal("commit_marker must not pass when the stage produced no new commit")
	}
	if !strings.Contains(reason, "commit_marker_missing") {
		t.Errorf("reason should say no commit was produced, got %q", reason)
	}

	if ev.GateType != "commit_marker" {
		t.Errorf("GateType = %q, want commit_marker", ev.GateType)
	}
	if ev.BaseSHA != baseSHA {
		t.Errorf("BaseSHA = %q, want %q", ev.BaseSHA, baseSHA)
	}
	if ev.HeadSHA != baseSHA {
		t.Errorf("HeadSHA = %q, want %q (HEAD never moved)", ev.HeadSHA, baseSHA)
	}
	if ev.WorktreePath != dir {
		t.Errorf("WorktreePath = %q, want %q", ev.WorktreePath, dir)
	}
	if ev.RangeOutput != "" {
		t.Errorf("RangeOutput = %q, want empty (that emptiness is the whole finding)", ev.RangeOutput)
	}
	if ev.TreesDiffered {
		t.Error("TreesDiffered = true, want false: nothing was ever compared")
	}
}

// TestEvaluateExitGateWithEvidence_CommitMarkerPassRecordsRangeAndDiffered
// proves a passing gate carries evidence too, not only a failing one - the
// range it read and that the trees did differ.
func TestEvaluateExitGateWithEvidence_CommitMarkerPassRecordsRangeAndDiffered(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	wf := commitMarkerOnlyWorkflow()

	dir := t.TempDir()
	initGitRepo(t, dir)
	baseSHA := gitCommit(t, dir, "base: captured before dispatch")
	commitSHA := gitCommitWithChange(t, dir, "stage: implementation: did the work")

	ok, reason, ev := EvaluateExitGateWithEvidence(wf, ExitGateContext{FromState: "implementation", WorktreePath: dir, BeadID: "kb-1", BaseSHA: baseSHA})
	if !ok {
		t.Fatalf("expected the gate to pass, got reason=%q", reason)
	}

	if ev.GateType != "commit_marker" {
		t.Errorf("GateType = %q, want commit_marker", ev.GateType)
	}
	if ev.BaseSHA != baseSHA {
		t.Errorf("BaseSHA = %q, want %q", ev.BaseSHA, baseSHA)
	}
	if ev.HeadSHA != commitSHA {
		t.Errorf("HeadSHA = %q, want %q", ev.HeadSHA, commitSHA)
	}
	if ev.RangeOutput != commitSHA {
		t.Errorf("RangeOutput = %q, want %q", ev.RangeOutput, commitSHA)
	}
	if !ev.TreesDiffered {
		t.Error("TreesDiffered = false, want true: the stage's commit changed the tree")
	}
}

// TestEvaluateExitGateWithEvidence_NonCommitMarkerGateCarriesTypeOnly proves
// the "collected and empty" case: a gate type that observes none of the
// git-specific fields still carries its own GateType, so a reader can tell
// it apart from a state that declared no gate at all.
func TestEvaluateExitGateWithEvidence_NonCommitMarkerGateCarriesTypeOnly(t *testing.T) {
	wf := WorkflowDescriptor{
		ExitGates: map[string][]WorkflowExitGate{
			"implementation": {{Type: "description_contains", Path: "required marker"}},
		},
	}

	ok, reason, ev := EvaluateExitGateWithEvidence(wf, ExitGateContext{
		FromState:       "implementation",
		WorktreePath:    "/some/worktree",
		BeadID:          "kb-1",
		BeadDescription: "unrelated text",
		BaseSHA:         "some-base-sha",
	})
	if ok {
		t.Fatal("expected description_contains to fail: the marker is absent")
	}
	if !strings.Contains(reason, "description_missing") {
		t.Errorf("reason = %q, want it to name description_missing", reason)
	}

	if ev.GateType != "description_contains" {
		t.Errorf("GateType = %q, want description_contains", ev.GateType)
	}
	// This gate type observes none of the git-specific fields - they must
	// stay zero even though ctx carried non-empty BaseSHA/WorktreePath.
	if ev.BaseSHA != "" || ev.HeadSHA != "" || ev.WorktreePath != "" || ev.RangeOutput != "" || ev.TreesDiffered {
		t.Errorf("evidence carries fields description_contains never observed: %+v", ev)
	}
}

// TestEvaluateExitGateWithEvidence_NoGateDeclaredIsZeroEvidence proves the
// other half of that distinction: a state with no declared gates returns
// the zero-value GateEvidence (GateType == ""), meaning "no evidence
// collected" rather than "evidence collected and empty".
func TestEvaluateExitGateWithEvidence_NoGateDeclaredIsZeroEvidence(t *testing.T) {
	wf := WorkflowDescriptor{}

	ok, _, ev := EvaluateExitGateWithEvidence(wf, ExitGateContext{FromState: "implementation", BeadID: "kb-1"})
	if !ok {
		t.Fatal("a state with no declared gates must pass (legacy agent_exit_zero)")
	}
	if ev != (GateEvidence{}) {
		t.Errorf("evidence = %+v, want the zero value", ev)
	}
}

// TestEvaluateExitGate_WrapperMatchesWithEvidenceOutcome proves the
// signature-preserving wrapper decides exactly what
// EvaluateExitGateWithEvidence decides - it must only drop the evidence,
// never change the verdict or the reason text.
func TestEvaluateExitGate_WrapperMatchesWithEvidenceOutcome(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	wf := commitMarkerOnlyWorkflow()

	dir := t.TempDir()
	initGitRepo(t, dir)
	baseSHA := gitCommit(t, dir, "base: captured before dispatch")

	wantOK, wantReason, _ := EvaluateExitGateWithEvidence(wf, ExitGateContext{FromState: "implementation", WorktreePath: dir, BeadID: "kb-1", BaseSHA: baseSHA})
	gotOK, gotReason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: dir, BeadID: "kb-1", BaseSHA: baseSHA})

	if gotOK != wantOK || gotReason != wantReason {
		t.Errorf("EvaluateExitGate = (%v, %q), want (%v, %q)", gotOK, gotReason, wantOK, wantReason)
	}
}

// TestEvaluateExitGateWithEvidence_CommitMarkerWinsAmongSeveralGates covers the
// preference the aggregator applies when a state declares more than one gate
// (worker's `implementation` carries both commit_marker and decision_record):
// only one GateEvidence comes back, and it must be the commit_marker's, because
// that is the only gate whose evidence anyone reads. Both declaration orders are
// exercised - a "first wins" or "last wins" rule would each satisfy one of them
// and lose the evidence in the other.
func TestEvaluateExitGateWithEvidence_CommitMarkerWinsAmongSeveralGates(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}

	for _, tc := range []struct {
		name  string
		gates []WorkflowExitGate
	}{
		{"commit_marker declared first", []WorkflowExitGate{
			{Type: "commit_marker"},
			{Type: "artifact_exists", Path: "<artifact_dir>/absent.md"},
		}},
		{"commit_marker declared last", []WorkflowExitGate{
			{Type: "artifact_exists", Path: "<artifact_dir>/absent.md"},
			{Type: "commit_marker"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			initGitRepo(t, dir)
			baseSHA := gitCommit(t, dir, "base: captured before dispatch")

			wf := WorkflowDescriptor{ExitGates: map[string][]WorkflowExitGate{"implementation": tc.gates}}
			_, _, ev := EvaluateExitGateWithEvidence(wf, ExitGateContext{
				FromState: "implementation", WorktreePath: dir, ArtifactDir: t.TempDir(), BeadID: "kb-1", BaseSHA: baseSHA,
			})

			if ev.GateType != "commit_marker" {
				t.Fatalf("GateType = %q, want commit_marker: the other gate's evidence overwrote it", ev.GateType)
			}
			if ev.BaseSHA != baseSHA {
				t.Errorf("BaseSHA = %q, want %q - the commit_marker evidence was lost", ev.BaseSHA, baseSHA)
			}
			if ev.WorktreePath != dir {
				t.Errorf("WorktreePath = %q, want %q", ev.WorktreePath, dir)
			}
		})
	}
}
