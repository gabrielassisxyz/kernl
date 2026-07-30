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

	// Exit gates wired on the three epic action stages.
	if g := wf.ExitGates["integration"]; g.Type != "commit_marker" || g.Path != "stage: integration" {
		t.Errorf("integration gate = %+v; want commit_marker/stage: integration", g)
	}
	if g := wf.ExitGates["integration_review"]; g.Type != "artifact_verdict" {
		t.Errorf("integration_review gate = %+v; want artifact_verdict", g)
	}
	if g := wf.ExitGates["shipment"]; g.Type != "description_contains" || g.Path != "pr_url:" {
		t.Errorf("shipment gate = %+v; want description_contains/pr_url:", g)
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
	// implementation needs a marker commit, the review needs a PASS verdict.
	if g := wf.ExitGates["implementation"]; g.Type != "commit_marker" || g.Path != "stage: implementation" {
		t.Errorf("implementation gate = %+v; want commit_marker/stage: implementation", g)
	}
	if g := wf.ExitGates["implementation_review"]; g.Type != "artifact_verdict" || g.Path != ".kernl/<bead_id>/implementation-review.md" {
		t.Errorf("implementation_review gate = %+v; want artifact_verdict/.kernl/<bead_id>/implementation-review.md", g)
	}

	// Autopilot (standalone) must still flow past implementation_review toward shipment, not stop.
	ap := BuiltinProfileDescriptor("autopilot")
	apNext, _ := ForwardTransitionTarget("implementation_review", ap)
	if apNext == "awaiting_integration" {
		t.Errorf("autopilot must not hand off to awaiting_integration (worker-only)")
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

	// shipment / description_contains
	if ok, _ := EvaluateExitGate(wf, ExitGateContext{FromState: "shipment", WorktreePath: dir, BeadID: "kernl-e1", BeadDescription: "merge_outcome: success\npr_url: https://x/pr/1\n"}); !ok {
		t.Error("shipment gate should pass when description has pr_url:")
	}
	if ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "shipment", WorktreePath: dir, BeadID: "kernl-e1", BeadDescription: "merge_outcome: success\n"}); ok || reason == "" {
		t.Errorf("shipment gate should fail without pr_url: (ok=%v reason=%q)", ok, reason)
	}

	// integration_review / artifact_verdict
	reviewDir := filepath.Join(dir, ".kernl", "kernl-e1")
	if err := os.MkdirAll(reviewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reviewFile := filepath.Join(reviewDir, "integration-review.md")
	if err := os.WriteFile(reviewFile, []byte("looks good\n\nVERDICT: PASS"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "integration_review", WorktreePath: dir, BeadID: "kernl-e1"}); !ok {
		t.Errorf("integration_review gate should pass on VERDICT: PASS (reason=%q)", reason)
	}
	if err := os.WriteFile(reviewFile, []byte("needs work\n\nVERDICT: FAIL"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, _ := EvaluateExitGate(wf, ExitGateContext{FromState: "integration_review", WorktreePath: dir, BeadID: "kernl-e1"}); ok {
		t.Error("integration_review gate should fail on VERDICT: FAIL")
	}
}

// TestEvaluateExitGate_Total proves EvaluateExitGate always passes when there
// is nothing for it to check: a state with no declared gate, a gate with an
// empty type, and the legacy agent_exit_zero type. None of these should ever
// require WorktreePath or BaseSHA to be set.
func TestEvaluateExitGate_Total(t *testing.T) {
	// "autopilot" declares no ExitGates at all - every state on it is the
	// "no gate for this state" case.
	autopilot := BuiltinProfileDescriptor("autopilot")
	if ok, reason := EvaluateExitGate(autopilot, ExitGateContext{FromState: "implementation", BeadID: "kb-1"}); !ok {
		t.Errorf("a state with no declared gate must pass, got ok=%v reason=%q", ok, reason)
	}

	wf := WorkflowDescriptor{
		ExitGates: map[string]WorkflowExitGate{
			"empty_type":       {Type: ""},
			"legacy_exit_zero": {Type: "agent_exit_zero"},
		},
	}
	if ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "empty_type", BeadID: "kb-1"}); !ok {
		t.Errorf("an empty gate type must pass, got ok=%v reason=%q", ok, reason)
	}
	if ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "legacy_exit_zero", BeadID: "kb-1"}); !ok {
		t.Errorf("agent_exit_zero must pass, got ok=%v reason=%q", ok, reason)
	}
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

// TestEvaluateExitGate_CommitMarkerScopedToBaseSHA proves the commit_marker
// gate only sees commits produced after BaseSHA, not the marker sitting in
// an ancestor commit - the bug recorded in the kernl-gc7j post-mortem, where
// a marker already present in the branch's history (a sibling merge, the
// base branch's own log) satisfied a gate the current stage never earned.
func TestEvaluateExitGate_CommitMarkerScopedToBaseSHA(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	wf := BuiltinProfileDescriptor("worker")

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
	wf := BuiltinProfileDescriptor("worker")

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
	wf := BuiltinProfileDescriptor("worker")

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
