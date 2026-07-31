package backend

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend/workflows"
)

func TestEpicProfile_LifecycleShape(t *testing.T) {
	wf := BuiltinProfileDescriptor("epic")

	if wf.InitialState != "ready_for_integration" {
		t.Fatalf("epic InitialState = %q, want ready_for_integration", wf.InitialState)
	}

	has := func(list []string, s string) bool {
		for _, x := range list {
			if x == s {
				return true
			}
		}
		return false
	}

	if !has(wf.States, "awaiting_pr_review") {
		t.Errorf("epic States missing awaiting_pr_review: %v", wf.States)
	}
	for _, banned := range []string{"shipment_review", "ready_for_shipment_review", "shipped", "ready_for_implementation", "implementation"} {
		if has(wf.States, banned) {
			t.Errorf("epic States should not contain %q: %v", banned, wf.States)
		}
	}

	if !has(wf.TerminalStates, "awaiting_pr_review") {
		t.Errorf("epic TerminalStates missing awaiting_pr_review: %v", wf.TerminalStates)
	}

	// Forward walk: integration -> integration_review -> shipment -> awaiting_pr_review.
	steps := []struct{ from, want string }{
		{"ready_for_integration", "integration"},
		{"integration", "ready_for_integration_review"},
		{"ready_for_integration_review", "integration_review"},
		{"integration_review", "ready_for_shipment"},
		{"ready_for_shipment", "shipment"},
		{"shipment", "awaiting_pr_review"},
	}
	for _, s := range steps {
		got, ok := ForwardTransitionTarget(s.from, wf)
		if !ok || got != s.want {
			t.Errorf("ForwardTransitionTarget(%q) = %q,%v; want %q,true", s.from, got, ok, s.want)
		}
	}

	// ready_for_integration must be agent-claimable into the integration action.
	rt := DeriveWorkflowRuntimeState(wf, "ready_for_integration")
	if !rt.IsAgentClaimable || rt.NextActionState != "integration" {
		t.Errorf("ready_for_integration runtime = %+v; want claimable into integration", rt)
	}

	// awaiting_pr_review is a human/terminal handoff - not agent-claimable.
	rtEnd := DeriveWorkflowRuntimeState(wf, "awaiting_pr_review")
	if rtEnd.IsAgentClaimable {
		t.Errorf("awaiting_pr_review should not be agent-claimable: %+v", rtEnd)
	}

	// Exit gates wired on the three epic action stages. "integration" is
	// epic's own implementation-side state, so it carries both its existing
	// commit_marker gate and the decision_record gate - the criterion this
	// bead exists to satisfy (see TestEpicProfile_CarriesDecisionRecordGate
	// below for the dedicated assertion).
	integrationGates := wf.ExitGates["integration"]
	if len(integrationGates) != 2 || integrationGates[0].Type != "commit_marker" || integrationGates[0].Path != "stage: integration" {
		t.Errorf("integration gates = %+v; want [commit_marker/stage: integration, decision_record/...]", integrationGates)
	}
	if g := wf.ExitGates["integration_review"]; len(g) != 1 || g[0].Type != "artifact_verdict" {
		t.Errorf("integration_review gates = %+v; want exactly one artifact_verdict", g)
	}
	if g := wf.ExitGates["shipment"]; len(g) != 1 || g[0].Type != "description_contains" || g[0].Path != "pr_url:" {
		t.Errorf("shipment gates = %+v; want exactly one description_contains/pr_url:", g)
	}
}

// TestEpicProfile_CarriesDecisionRecordGate proves epic's own
// implementation-side state ("integration") carries the decision_record gate
// alongside its pre-existing commit_marker gate, and that the stage contract
// tells the agent to write the same file the gate reads - the criterion the
// exit-gate cardinality change exists to satisfy (see also
// TestWorkerProfile_CarriesDecisionRecordGate for the worker half).
func TestEpicProfile_CarriesDecisionRecordGate(t *testing.T) {
	wf := BuiltinProfileDescriptor("epic")

	gates := wf.ExitGates["integration"]
	var hasCommitMarker, hasDecisionRecord bool
	for _, g := range gates {
		switch g.Type {
		case "commit_marker":
			hasCommitMarker = true
			if g.Path != "stage: integration" {
				t.Errorf("commit_marker gate path = %q; want %q", g.Path, "stage: integration")
			}
		case "decision_record":
			hasDecisionRecord = true
			if g.Path != "<artifact_dir>/decision-record.md" {
				t.Errorf("decision_record gate path = %q; want %q", g.Path, "<artifact_dir>/decision-record.md")
			}
		}
	}
	if !hasCommitMarker {
		t.Errorf("epic integration gates = %+v; missing commit_marker", gates)
	}
	if !hasDecisionRecord {
		t.Errorf("epic integration gates = %+v; missing decision_record", gates)
	}

	stage, ok := wf.Stages["integration"]
	if !ok || stage.DecisionRecord.Path != "<artifact_dir>/decision-record.md" {
		t.Errorf("epic integration stage DecisionRecord = %+v, ok=%v; want <artifact_dir>/decision-record.md", stage.DecisionRecord, ok)
	}
}

func TestWorkerProfile_StopsAtAwaitingIntegration(t *testing.T) {
	wf := BuiltinProfileDescriptor("worker")

	if wf.InitialState != "ready_for_implementation" {
		t.Fatalf("worker InitialState = %q, want ready_for_implementation", wf.InitialState)
	}

	has := func(list []string, s string) bool {
		for _, x := range list {
			if x == s {
				return true
			}
		}
		return false
	}

	if !has(wf.States, "awaiting_integration") {
		t.Errorf("worker States missing awaiting_integration: %v", wf.States)
	}
	for _, banned := range []string{"integration", "shipment", "shipped", "ready_for_integration"} {
		if has(wf.States, banned) {
			t.Errorf("worker States should not contain %q: %v", banned, wf.States)
		}
	}
	if !has(wf.TerminalStates, "awaiting_integration") {
		t.Errorf("worker TerminalStates missing awaiting_integration: %v", wf.TerminalStates)
	}

	// The worker hands off after implementation_review.
	got, ok := ForwardTransitionTarget("implementation_review", wf)
	if !ok || got != "awaiting_integration" {
		t.Errorf("worker ForwardTransitionTarget(implementation_review) = %q,%v; want awaiting_integration", got, ok)
	}

	// Exit gates stop a bead from advancing when it produced no real output:
	// implementation needs a marker commit AND a decision record (the
	// criterion this bead exists to satisfy - see
	// TestWorkerProfile_CarriesDecisionRecordGate below), the review needs a
	// PASS verdict.
	implGates := wf.ExitGates["implementation"]
	if len(implGates) != 2 || implGates[0].Type != "commit_marker" || implGates[0].Path != "stage: implementation" {
		t.Errorf("implementation gates = %+v; want [commit_marker/stage: implementation, decision_record/...]", implGates)
	}
	if g := wf.ExitGates["implementation_review"]; len(g) != 1 || g[0].Type != "artifact_verdict" || g[0].Path != "<artifact_dir>/implementation-review.md" {
		t.Errorf("implementation_review gates = %+v; want exactly one artifact_verdict/<artifact_dir>/implementation-review.md", g)
	}

	// Autopilot (standalone) must still flow past implementation_review toward shipment, not stop.
	ap := BuiltinProfileDescriptor("autopilot")
	apNext, _ := ForwardTransitionTarget("implementation_review", ap)
	if apNext == "awaiting_integration" {
		t.Errorf("autopilot must not hand off to awaiting_integration (worker-only)")
	}
}

// TestWorkerProfile_CarriesDecisionRecordGate proves worker's "implementation"
// state carries the decision_record gate alongside its pre-existing
// commit_marker gate, and that the stage contract (inherited from the shared
// CanonicalStageContracts, unmodified) tells the agent to write the same file
// the gate reads - the criterion the exit-gate cardinality change exists to
// satisfy (see also TestEpicProfile_CarriesDecisionRecordGate for the epic
// half).
func TestWorkerProfile_CarriesDecisionRecordGate(t *testing.T) {
	wf := BuiltinProfileDescriptor("worker")

	gates := wf.ExitGates["implementation"]
	var hasCommitMarker, hasDecisionRecord bool
	for _, g := range gates {
		switch g.Type {
		case "commit_marker":
			hasCommitMarker = true
			if g.Path != "stage: implementation" {
				t.Errorf("commit_marker gate path = %q; want %q", g.Path, "stage: implementation")
			}
		case "decision_record":
			hasDecisionRecord = true
			if g.Path != "<artifact_dir>/decision-record.md" {
				t.Errorf("decision_record gate path = %q; want %q", g.Path, "<artifact_dir>/decision-record.md")
			}
		}
	}
	if !hasCommitMarker {
		t.Errorf("worker implementation gates = %+v; missing commit_marker", gates)
	}
	if !hasDecisionRecord {
		t.Errorf("worker implementation gates = %+v; missing decision_record", gates)
	}

	stage, ok := wf.Stages["implementation"]
	if !ok || stage.DecisionRecord.Path != "<artifact_dir>/decision-record.md" {
		t.Errorf("worker implementation stage DecisionRecord = %+v, ok=%v; want <artifact_dir>/decision-record.md", stage.DecisionRecord, ok)
	}
}

func TestEpicProfile_AutopilotUnaffected(t *testing.T) {
	// The epic-only transition must not leak into other profiles.
	wf := BuiltinProfileDescriptor("autopilot")
	got, _ := ForwardTransitionTarget("shipment", wf)
	if got == "awaiting_pr_review" {
		t.Errorf("autopilot shipment must not advance to awaiting_pr_review (epic-only)")
	}
}

func TestEvaluateExitGate_EpicTypes(t *testing.T) {
	wf := BuiltinProfileDescriptor("epic")
	dir := t.TempDir()
	artifactDir := t.TempDir()

	// shipment / description_contains
	if ok, _ := EvaluateExitGate(wf, ExitGateContext{FromState: "shipment", WorktreePath: dir, ArtifactDir: artifactDir, BeadID: "kernl-e1", BeadDescription: "merge_outcome: success\npr_url: https://x/pr/1\n"}); !ok {
		t.Error("shipment gate should pass when description has pr_url:")
	}
	if ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "shipment", WorktreePath: dir, ArtifactDir: artifactDir, BeadID: "kernl-e1", BeadDescription: "merge_outcome: success\n"}); ok || reason == "" {
		t.Errorf("shipment gate should fail without pr_url: (ok=%v reason=%q)", ok, reason)
	}

	// integration_review / artifact_verdict - the artifact lives in
	// ArtifactDir (outside the worktree), not under the worktree's own
	// .kernl/<bead_id>/ as it did before the artifact directory moved.
	reviewFile := filepath.Join(artifactDir, "integration-review.md")
	if err := os.WriteFile(reviewFile, []byte("looks good\n\nVERDICT: PASS"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "integration_review", WorktreePath: dir, ArtifactDir: artifactDir, BeadID: "kernl-e1"}); !ok {
		t.Errorf("integration_review gate should pass on VERDICT: PASS (reason=%q)", reason)
	}
	if err := os.WriteFile(reviewFile, []byte("needs work\n\nVERDICT: FAIL"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, _ := EvaluateExitGate(wf, ExitGateContext{FromState: "integration_review", WorktreePath: dir, ArtifactDir: artifactDir, BeadID: "kernl-e1"}); ok {
		t.Error("integration_review gate should fail on VERDICT: FAIL")
	}

	// integration_review / artifact_verdict must not fall back to the
	// worktree (or the filesystem root) when ArtifactDir is unset - that
	// would silently write and read kernl's control files back inside the
	// target repository again.
	if ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "integration_review", WorktreePath: dir, ArtifactDir: "", BeadID: "kernl-e1"}); ok || !strings.Contains(reason, "artifact_dir_unset") {
		t.Errorf("integration_review gate must fail with artifact_dir_unset when ArtifactDir is empty; got ok=%v reason=%q", ok, reason)
	}
}

// TestEvaluateExitGate_Total proves EvaluateExitGate always passes when there
// is nothing for it to check: a state with no declared gate at all, a state
// declaring an empty gate list, a gate with an empty type, and the legacy
// agent_exit_zero type. None of these should ever require WorktreePath,
// ArtifactDir or BaseSHA to be set.
func TestEvaluateExitGate_Total(t *testing.T) {
	// "autopilot" declares no ExitGates at all - every state on it is the
	// "no gate for this state" case.
	autopilot := BuiltinProfileDescriptor("autopilot")
	if ok, reason := EvaluateExitGate(autopilot, ExitGateContext{FromState: "implementation", BeadID: "kb-1"}); !ok {
		t.Errorf("a state with no declared gate must pass, got ok=%v reason=%q", ok, reason)
	}

	wf := WorkflowDescriptor{
		ExitGates: map[string][]WorkflowExitGate{
			"empty_list":       {},
			"empty_type":       {{Type: ""}},
			"legacy_exit_zero": {{Type: "agent_exit_zero"}},
		},
	}
	if ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "empty_list", BeadID: "kb-1"}); !ok {
		t.Errorf("an empty gate list must pass, got ok=%v reason=%q", ok, reason)
	}
	if ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "empty_type", BeadID: "kb-1"}); !ok {
		t.Errorf("an empty gate type must pass, got ok=%v reason=%q", ok, reason)
	}
	if ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "legacy_exit_zero", BeadID: "kb-1"}); !ok {
		t.Errorf("agent_exit_zero must pass, got ok=%v reason=%q", ok, reason)
	}
}

// TestEvaluateExitGate_MultipleGatesPerState proves the core capability this
// bead adds: a state declaring more than one gate requires ALL of them to
// pass, a failure names which one failed, and failing two at once reports
// both instead of only the first (acceptance criteria 1 and 2).
func TestEvaluateExitGate_MultipleGatesPerState(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}

	multiGateWf := WorkflowDescriptor{
		ExitGates: map[string][]WorkflowExitGate{
			"implementation": {
				{Type: "commit_marker", Path: "stage: implementation"},
				{Type: "decision_record", Path: "<artifact_dir>/decision-record.md"},
			},
		},
	}

	writeDecisionRecord := func(t *testing.T, artifactDir string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(artifactDir, "decision-record.md"), []byte(fullDecisionRecord()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	commitMarker := func(t *testing.T, dir string) string {
		t.Helper()
		initGitRepo(t, dir)
		return gitCommit(t, dir, "base: before dispatch")
	}
	markerCommit := func(t *testing.T, dir string) {
		t.Helper()
		if out, err := exec.Command("git", "-C", dir, "commit", "--allow-empty", "-m", "stage: implementation: did the work").CombinedOutput(); err != nil {
			t.Fatalf("git commit: %v\n%s", err, out)
		}
	}

	t.Run("both pass advances", func(t *testing.T) {
		dir := t.TempDir()
		artifactDir := t.TempDir()
		baseSHA := commitMarker(t, dir)
		markerCommit(t, dir)
		writeDecisionRecord(t, artifactDir)

		ok, reason := EvaluateExitGate(multiGateWf, ExitGateContext{FromState: "implementation", WorktreePath: dir, ArtifactDir: artifactDir, BeadID: "kb-1", BaseSHA: baseSHA})
		if !ok {
			t.Fatalf("expected both gates to pass, got reason=%q", reason)
		}
	})

	t.Run("commit_marker fails alone names commit_marker", func(t *testing.T) {
		dir := t.TempDir()
		artifactDir := t.TempDir()
		baseSHA := commitMarker(t, dir)
		writeDecisionRecord(t, artifactDir)

		ok, reason := EvaluateExitGate(multiGateWf, ExitGateContext{FromState: "implementation", WorktreePath: dir, ArtifactDir: artifactDir, BeadID: "kb-1", BaseSHA: baseSHA})
		if ok {
			t.Fatal("expected failure when commit_marker is missing")
		}
		if !strings.Contains(reason, "commit_marker_missing") {
			t.Errorf("reason must name commit_marker as the failing gate, got %q", reason)
		}
		if strings.Contains(reason, "decision_record_missing_sections") || strings.Contains(reason, "artifact_missing") {
			t.Errorf("decision_record passed and must not appear in the failure reason, got %q", reason)
		}
	})

	t.Run("decision_record fails alone names decision_record", func(t *testing.T) {
		dir := t.TempDir()
		artifactDir := t.TempDir()
		baseSHA := commitMarker(t, dir)
		markerCommit(t, dir)
		// No decision record written.

		ok, reason := EvaluateExitGate(multiGateWf, ExitGateContext{FromState: "implementation", WorktreePath: dir, ArtifactDir: artifactDir, BeadID: "kb-1", BaseSHA: baseSHA})
		if ok {
			t.Fatal("expected failure when decision_record is missing")
		}
		if !strings.Contains(reason, "artifact_missing") {
			t.Errorf("reason must name the decision_record failure, got %q", reason)
		}
		if strings.Contains(reason, "commit_marker_missing") {
			t.Errorf("commit_marker passed and must not appear in the failure reason, got %q", reason)
		}
	})

	t.Run("both fail reports both", func(t *testing.T) {
		dir := t.TempDir()
		artifactDir := t.TempDir()
		baseSHA := commitMarker(t, dir)
		// Neither the marker commit nor the decision record is produced.

		ok, reason := EvaluateExitGate(multiGateWf, ExitGateContext{FromState: "implementation", WorktreePath: dir, ArtifactDir: artifactDir, BeadID: "kb-1", BaseSHA: baseSHA})
		if ok {
			t.Fatal("expected failure when both gates are unmet")
		}
		if !strings.Contains(reason, "commit_marker_missing") {
			t.Errorf("reason must report the commit_marker failure, got %q", reason)
		}
		if !strings.Contains(reason, "artifact_missing") {
			t.Errorf("reason must report the decision_record failure, got %q", reason)
		}
	})
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "--initial-branch", "main"},
		{"config", "user.email", "t@t"},
		{"config", "user.name", "t"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func gitCommit(t *testing.T, dir, message string) string {
	t.Helper()
	if out, err := exec.Command("git", "-C", dir, "commit", "--allow-empty", "-m", message).CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// commitMarkerOnlyWorkflow builds a minimal WorkflowDescriptor carrying only
// the "implementation" state's commit_marker gate, so the commit_marker
// tests below stay focused on that one gate's own behavior. worker's
// builtin "implementation" state also carries a decision_record gate now
// (see TestWorkerProfile_CarriesDecisionRecordGate); reusing the full
// builtin profile here would make these tests fail on a missing decision
// record that has nothing to do with what each one is testing.
func commitMarkerOnlyWorkflow() WorkflowDescriptor {
	return WorkflowDescriptor{
		ExitGates: map[string][]WorkflowExitGate{
			"implementation": {{Type: "commit_marker", Path: "stage: implementation"}},
		},
	}
}

// TestEvaluateExitGate_CommitMarkerScopedToBaseSHA proves the commit_marker
// gate only sees commits produced after BaseSHA, not the marker sitting in
// an ancestor commit - the bug recorded in the kernl-gc7j post-mortem, where
// a marker already present in the branch's history (a sibling merge, the
// base branch's own log) satisfied a gate the current stage never earned.
func TestEvaluateExitGate_CommitMarkerScopedToBaseSHA(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	wf := commitMarkerOnlyWorkflow()

	dir := t.TempDir()
	initGitRepo(t, dir)
	// The marker sits only in an ancestor commit - a sibling epic stage, or
	// the base branch's own history - never produced by this stage.
	gitCommit(t, dir, "stage: implementation: an ancestor's marker")
	baseSHA := gitCommit(t, dir, "base: unrelated work after the marker")

	if ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: dir, BeadID: "kb-1", BaseSHA: baseSHA}); ok {
		t.Errorf("commit_marker must not pass on an ancestor marker outside BaseSHA..HEAD (reason=%q)", reason)
	}

	// The stage now produces its own marker commit after BaseSHA.
	if out, err := exec.Command("git", "-C", dir, "commit", "--allow-empty", "-m", "stage: implementation: did the work").CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
	if ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: dir, BeadID: "kb-1", BaseSHA: baseSHA}); !ok {
		t.Errorf("commit_marker should pass once the stage's own commit carries the marker (reason=%q)", reason)
	}
}

// TestEvaluateExitGate_CommitMarkerRequiresBaseSHA proves an empty BaseSHA
// fails the gate instead of silently scanning the whole branch history -
// exactly the fallback that reintroduces the ancestor-commit bug.
func TestEvaluateExitGate_CommitMarkerRequiresBaseSHA(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	wf := commitMarkerOnlyWorkflow()

	dir := t.TempDir()
	initGitRepo(t, dir)
	gitCommit(t, dir, "stage: implementation: did the work")

	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: dir, BeadID: "kb-1", BaseSHA: ""})
	if ok {
		t.Fatal("commit_marker must not pass with no BaseSHA, even though the marker is on HEAD")
	}
	if !strings.Contains(reason, "unscoped") {
		t.Errorf("reason should say the scan was never scoped, got %q", reason)
	}
}

// TestEvaluateExitGate_CommitMarkerUnreadableBaseSHA proves an unreachable
// BaseSHA is reported as unreadable, not as a missing marker - the two mean
// different things and the caller (and its operator) needs to tell them apart.
func TestEvaluateExitGate_CommitMarkerUnreadableBaseSHA(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	wf := commitMarkerOnlyWorkflow()

	dir := t.TempDir()
	initGitRepo(t, dir)
	gitCommit(t, dir, "stage: implementation: did the work")

	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: dir, BeadID: "kb-1", BaseSHA: "0000000000000000000000000000000000dead"})
	if ok {
		t.Fatal("commit_marker must not pass when BaseSHA cannot be resolved")
	}
	if !strings.Contains(reason, "unreadable") {
		t.Errorf("reason should say the base SHA was unreadable, got %q", reason)
	}
}

// TestEvaluateExitGate_CommitMarkerRejectsRewrittenHistory proves that a
// BaseSHA which resolves fine but is no longer an ancestor of HEAD does not
// admit a marker that only exists on the history HEAD was moved onto.
// `git log <base>..HEAD` means "reachable from HEAD, not reachable from
// base" - it does not require base to be an ancestor, so if the agent resets
// or rebases onto an unrelated line of history that already contains the
// marker (a sibling epic stage's commit, this repository's own log), that
// range still evaluates and the marker satisfies a gate the stage never
// earned. This is the original ancestor-commit defect reached through a
// second door: BaseSHA was captured correctly, but HEAD moved out from
// under it.
func TestEvaluateExitGate_CommitMarkerRejectsRewrittenHistory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	wf := commitMarkerOnlyWorkflow()

	dir := t.TempDir()
	initGitRepo(t, dir)
	baseSHA := gitCommit(t, dir, "base: captured before dispatch")

	// The agent rewrites history: an unrelated orphan line already carries
	// the marker as an ancestor commit, and HEAD is reset onto its tip -
	// baseSHA is left behind on a branch HEAD no longer descends from.
	if out, err := exec.Command("git", "-C", dir, "checkout", "--orphan", "rewritten").CombinedOutput(); err != nil {
		t.Fatalf("git checkout --orphan: %v\n%s", err, out)
	}
	gitCommit(t, dir, "stage: implementation: an unrelated ancestor's marker")
	gitCommit(t, dir, "further work on the rewritten line")

	if ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: dir, BeadID: "kb-1", BaseSHA: baseSHA}); ok {
		t.Errorf("commit_marker must not pass when BaseSHA is not an ancestor of HEAD (reason=%q)", reason)
	} else if !strings.Contains(reason, "history_rewritten") {
		t.Errorf("reason should say the base SHA is no longer an ancestor, got %q", reason)
	}
}

func TestCanonicalYAML_Parity(t *testing.T) {
	// Write workflows.CanonicalYAML to a temporary file.
	dir := t.TempDir()
	path := filepath.Join(dir, "canonical.yaml")
	if err := os.WriteFile(path, workflows.CanonicalYAML, 0644); err != nil {
		t.Fatalf("failed to write canonical.yaml: %v", err)
	}

	// Load via LoadWorkflowYAML.
	loadedWf, err := LoadWorkflowYAML(path)
	if err != nil {
		t.Fatalf("failed to LoadWorkflowYAML: %v", err)
	}

	// Retrieve native autopilot_with_pr profile descriptor.
	builtinWf := BuiltinProfileDescriptor("autopilot_with_pr")

	// 1. Assert equality on States.
	if !reflect.DeepEqual(loadedWf.States, builtinWf.States) {
		t.Errorf("States mismatch:\nloaded:  %v\nbuiltin: %v", loadedWf.States, builtinWf.States)
	}

	// 2. Assert equality on Transitions as a set.
	loadedTrans := make(map[string]bool)
	for _, tr := range loadedWf.Transitions {
		loadedTrans[tr.From+"->"+tr.To] = true
	}
	builtinTrans := make(map[string]bool)
	for _, tr := range builtinWf.Transitions {
		builtinTrans[tr.From+"->"+tr.To] = true
	}
	if len(loadedTrans) != len(builtinTrans) {
		t.Errorf("Transitions length mismatch: loaded %d, builtin %d", len(loadedTrans), len(builtinTrans))
	}
	for k := range loadedTrans {
		if !builtinTrans[k] {
			t.Errorf("transition %q in loaded YAML but not in builtin autopilot_with_pr", k)
		}
	}
	for k := range builtinTrans {
		if !loadedTrans[k] {
			t.Errorf("transition %q in builtin autopilot_with_pr but not in loaded YAML", k)
		}
	}

	// 3. Assert equality on ExitGates.
	if len(loadedWf.ExitGates) != len(builtinWf.ExitGates) {
		t.Errorf("ExitGates length mismatch: loaded %d, builtin %d", len(loadedWf.ExitGates), len(builtinWf.ExitGates))
	}
	for k, loadedGate := range loadedWf.ExitGates {
		builtinGate, ok := builtinWf.ExitGates[k]
		if !ok {
			t.Errorf("ExitGate for %q in loaded YAML but not in builtin autopilot_with_pr", k)
			continue
		}
		if !reflect.DeepEqual(loadedGate, builtinGate) {
			t.Errorf("ExitGate for %q mismatch:\nloaded:  %+v\nbuiltin: %+v", k, loadedGate, builtinGate)
		}
	}

	// 4. Assert equality on Owners.
	if len(loadedWf.Owners) != len(builtinWf.Owners) {
		t.Errorf("Owners length mismatch: loaded %d, builtin %d", len(loadedWf.Owners), len(builtinWf.Owners))
	}
	for k, loadedOwner := range loadedWf.Owners {
		builtinOwner, ok := builtinWf.Owners[k]
		if !ok {
			t.Errorf("Owner for %q in loaded YAML but not in builtin autopilot_with_pr", k)
			continue
		}
		if loadedOwner != builtinOwner {
			t.Errorf("Owner for %q mismatch: loaded %v, builtin %v", k, loadedOwner, builtinOwner)
		}
	}

	// 5. Assert equality on Stages.
	if len(loadedWf.Stages) != len(builtinWf.Stages) {
		t.Errorf("Stages length mismatch: loaded %d, builtin %d", len(loadedWf.Stages), len(builtinWf.Stages))
	}
	for k, loadedStage := range loadedWf.Stages {
		builtinStage, ok := builtinWf.Stages[k]
		if !ok {
			t.Errorf("Stage %q in loaded YAML but not in builtin autopilot_with_pr", k)
			continue
		}
		if !reflect.DeepEqual(loadedStage, builtinStage) {
			t.Errorf("Stage %q mismatch:\nloaded:  %+v\nbuiltin: %+v", k, loadedStage, builtinStage)
		}
	}
}
