package backend

import (
	"fmt"
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

	// Exit gates wired on the three epic action stages. "integration" carries
	// only its pre-existing commit_marker gate - see
	// TestEpicProfile_IntegrationCarriesNoDecisionRecordGate for why it does
	// not also carry decision_record.
	integrationGates := wf.ExitGates["integration"]
	if len(integrationGates) != 1 || integrationGates[0].Type != "commit_marker" || integrationGates[0].Path != "" {
		t.Errorf("integration gates = %+v; want exactly one commit_marker with no path", integrationGates)
	}
	if g := wf.ExitGates["integration_review"]; len(g) != 1 || g[0].Type != "artifact_verdict" {
		t.Errorf("integration_review gates = %+v; want exactly one artifact_verdict", g)
	}
	if g := wf.ExitGates["shipment"]; len(g) != 1 || g[0].Type != "description_contains" || g[0].Path != "pr_url:" {
		t.Errorf("shipment gates = %+v; want exactly one description_contains/pr_url:", g)
	}
}

// TestEpicProfile_IntegrationCarriesNoDecisionRecordGate proves epic's
// "integration" state does NOT carry a decision_record gate, unlike worker's
// "implementation" (see TestWorkerProfile_CarriesDecisionRecordGate).
// decision_record checks for the four sections an implementer writes down
// before coding; "integration" is a merge stage whose real prompt
// (RenderIntegration in cmd/kernl/epic.go, which replaces the generic
// stage-contract prompt for that state) never asks the agent for one, and an
// ordinary conflict-free merge usually has no open design choice to record.
// Gating on it would force the agent to fabricate content to pass, which is
// worse than no gate at all - see canonicalImplementationExitGates.
func TestEpicProfile_IntegrationCarriesNoDecisionRecordGate(t *testing.T) {
	wf := BuiltinProfileDescriptor("epic")

	for _, g := range wf.ExitGates["integration"] {
		if g.Type == "decision_record" {
			t.Fatalf("epic integration gates = %+v; must not carry decision_record", wf.ExitGates["integration"])
		}
	}

	// The stage contract must not advertise a requirement nothing enforces
	// either - epic never overrides the shared CanonicalStageContracts, so
	// its "integration" stage carries whatever that shared entry declares.
	stage, ok := wf.Stages["integration"]
	if !ok {
		t.Fatal("epic must still declare an 'integration' stage contract")
	}
	if stage.DecisionRecord.Path != "" {
		t.Errorf("epic integration stage DecisionRecord = %+v; want empty (no decision_record requirement)", stage.DecisionRecord)
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
	// implementation needs a new commit AND a decision record (the
	// criterion this bead exists to satisfy - see
	// TestWorkerProfile_CarriesDecisionRecordGate below), the review needs a
	// PASS verdict.
	implGates := wf.ExitGates["implementation"]
	if len(implGates) != 2 || implGates[0].Type != "commit_marker" || implGates[0].Path != "" {
		t.Errorf("implementation gates = %+v; want [commit_marker with no path, decision_record/...]", implGates)
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
// satisfy. Epic does NOT carry the same pairing on its own implementation-side
// state - see TestEpicProfile_IntegrationCarriesNoDecisionRecordGate for why.
func TestWorkerProfile_CarriesDecisionRecordGate(t *testing.T) {
	wf := BuiltinProfileDescriptor("worker")

	gates := wf.ExitGates["implementation"]
	var hasCommitMarker, hasDecisionRecord bool
	for _, g := range gates {
		switch g.Type {
		case "commit_marker":
			hasCommitMarker = true
			if g.Path != "" {
				t.Errorf("commit_marker gate path = %q; want empty (the gate no longer reads a marker string)", g.Path)
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

// TestEvaluateExitGate_ArtifactVerdictRequiresExactLastLine proves the gate
// checks the trimmed LAST LINE for exact equality, not the whole document
// for a trailing substring. HasSuffix(whole document, "VERDICT: PASS") is
// true for a single line reading "NOT A VALID VERDICT: PASS" - that string
// IS its own suffix - which would pass a document carrying no real sentinel
// at all. The same shape applies to REJECT.
//
// Inversion: reading this test against evaluateSingleExitGate with the
// exact-line comparison replaced by the original
// strings.HasSuffix(trimmed, "VERDICT: PASS") check makes the first
// assertion below fail: ok comes back true instead of false, because the
// fabricated line genuinely is a suffix of itself. Confirmed by hand.
func TestEvaluateExitGate_ArtifactVerdictRequiresExactLastLine(t *testing.T) {
	wf := BuiltinProfileDescriptor("epic")
	dir := t.TempDir()
	artifactDir := t.TempDir()
	reviewFile := filepath.Join(artifactDir, "integration-review.md")

	if err := os.WriteFile(reviewFile, []byte("NOT A VALID VERDICT: PASS"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "integration_review", WorktreePath: dir, ArtifactDir: artifactDir, BeadID: "kernl-e1"}); ok {
		t.Errorf("a fabricated line ending in the PASS sentinel as a substring must not pass the gate, got ok=%v reason=%q", ok, reason)
	}

	if err := os.WriteFile(reviewFile, []byte("NOT A VALID VERDICT: REJECT"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "integration_review", WorktreePath: dir, ArtifactDir: artifactDir, BeadID: "kernl-e1"}); ok || strings.HasPrefix(reason, "verdict_reject") {
		t.Errorf("a fabricated line ending in the REJECT sentinel as a substring must read as verdict_not_pass, not a real rejection: ok=%v reason=%q", ok, reason)
	}
}

// TestEvaluateExitGate_ReviewStagesRecognizeReject proves the distinction this
// gate exists to make: a reviewer that deliberately rejects (VERDICT: REJECT)
// still fails the gate - a rejection is not a pass - but with a reason
// distinguishable from every other way this gate fails, so the caller can tell
// "the reviewer did its job and said no" apart from "the reviewer produced
// nothing coherent."
//
// Inversion: reading this test against a state_machine.go with the REJECT
// branch removed (i.e. reverting to the pre-Phase-6 two-way PASS/not-PASS
// check) makes the first assertion below fail on the "verdict_reject: "
// prefix - it would still get "verdict_not_pass: " instead, since REJECT is
// not PASS either. Confirmed by hand: with only the two-branch check, this
// case's reason reads "verdict_not_pass: <path>", not
// "verdict_reject: <path>", which is exactly the collapse Phase 6 exists to
// undo.
func TestEvaluateExitGate_ReviewStagesRecognizeReject(t *testing.T) {
	wf := BuiltinProfileDescriptor("epic")
	dir := t.TempDir()
	artifactDir := t.TempDir()
	reviewFile := filepath.Join(artifactDir, "integration-review.md")

	if err := os.WriteFile(reviewFile, []byte("this is wrong\n\nVERDICT: REJECT"), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "integration_review", WorktreePath: dir, ArtifactDir: artifactDir, BeadID: "kernl-e1"})
	if ok {
		t.Fatal("a rejection must still fail the gate - it is not a pass")
	}
	if !strings.HasPrefix(reason, "verdict_reject: ") {
		t.Errorf("reason = %q, want the verdict_reject prefix so a caller can tell a deliberate rejection apart from a generic failure", reason)
	}

	// implementation_review now has somewhere to send a rejection too: the
	// bead goes back to the workflow's retake state carrying the review, so
	// its REJECT has to arrive distinguishable for the same reason
	// integration_review's does.
	//
	// plan_review and shipment_review still have nowhere to send one, and
	// must keep reading as the generic verdict_not_pass - promoting them
	// would turn "this did not pass" into "send the work back" at stages
	// where nothing is listening for that.
	cases := map[string]string{
		"implementation_review": "verdict_reject: ",
		"plan_review":           "verdict_not_pass: ",
		"shipment_review":       "verdict_not_pass: ",
	}
	for state, wantPrefix := range cases {
		t.Run(state, func(t *testing.T) {
			otherWf := WorkflowDescriptor{
				ExitGates: map[string][]WorkflowExitGate{state: {{Type: "artifact_verdict", Path: "<artifact_dir>/review.md"}}},
			}
			path := filepath.Join(artifactDir, "review.md")
			if err := os.WriteFile(path, []byte("this is wrong\n\nVERDICT: REJECT"), 0o644); err != nil {
				t.Fatal(err)
			}
			ok, reason := EvaluateExitGate(otherWf, ExitGateContext{FromState: state, WorktreePath: dir, ArtifactDir: artifactDir, BeadID: "kernl-e1"})
			if ok {
				t.Fatalf("%s must not pass on VERDICT: REJECT", state)
			}
			if !strings.HasPrefix(reason, wantPrefix) {
				t.Errorf("%s reason = %q, want the %q prefix", state, reason, wantPrefix)
			}
		})
	}

	// A verdict gate failing for any other reason must NOT read as a
	// rejection, at either stage that acts on one: an unfinished reviewer is
	// not a reviewer saying no, and treating it as one sends work back on the
	// strength of a truncated file.
	for _, state := range []string{"integration_review", "implementation_review"} {
		t.Run(state+"/incoherent", func(t *testing.T) {
			otherWf := WorkflowDescriptor{
				ExitGates: map[string][]WorkflowExitGate{state: {{Type: "artifact_verdict", Path: "<artifact_dir>/partial.md"}}},
			}
			path := filepath.Join(artifactDir, "partial.md")
			if err := os.WriteFile(path, []byte("I was still writing when"), 0o644); err != nil {
				t.Fatal(err)
			}
			ok, reason := EvaluateExitGate(otherWf, ExitGateContext{FromState: state, WorktreePath: dir, ArtifactDir: artifactDir, BeadID: "kernl-e1"})
			if ok {
				t.Fatalf("%s must not pass on an unfinished review", state)
			}
			if !strings.HasPrefix(reason, "verdict_not_pass: ") {
				t.Errorf("%s reason = %q, want verdict_not_pass - only an explicit REJECT is a rejection", state, reason)
			}
		})
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

// TestInitBuiltinWorkflows_ValidatesArtifactPaths proves builtin profiles are
// not exempt from the artifact-path containment check that YAML-loaded
// workflows already go through in LoadWorkflowYAML. Without this,
// initBuiltinWorkflows could hand back a descriptor whose decision_record
// path escapes <artifact_dir> - the exact leak that check exists to prevent
// (archeion PR #40) - and nothing would catch it before it ran.
//
// This mutates the package-level builtinProfiles/builtinWorkflowCache to
// inject a deliberately broken profile, which is safe here (and nowhere
// else in this package) because no test in this package runs in parallel;
// both are restored via defer regardless of outcome.
func TestInitBuiltinWorkflows_ValidatesArtifactPaths(t *testing.T) {
	savedProfiles := builtinProfiles
	savedCache := builtinWorkflowCache
	defer func() {
		builtinProfiles = savedProfiles
		builtinWorkflowCache = savedCache
	}()

	builtinProfiles = []profileConfig{
		{
			ID: "broken_builtin",
			ExitGates: map[string][]WorkflowExitGate{
				"implementation": {{Type: "decision_record", Path: "<artifact_dir>/../../etc/passwd"}},
			},
		},
	}
	builtinWorkflowCache = nil

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected initBuiltinWorkflows to panic on a builtin profile with an unconfined decision_record path")
		}
		msg := fmt.Sprint(r)
		if !strings.Contains(msg, "KERNL DISPATCH FAILURE") {
			t.Errorf("panic message must carry the KERNL DISPATCH FAILURE marker, got: %v", msg)
		}
		if !strings.Contains(msg, "broken_builtin") {
			t.Errorf("panic message must name the offending profile, got: %v", msg)
		}
	}()
	initBuiltinWorkflows()
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
		gitCommitWithChange(t, dir, "stage: implementation: did the work")
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

// TestEvaluateExitGate_FailureReasonEscapesEmbeddedDelimiter proves the
// aggregate failure reason stays reversible even when a gate's own free-text
// detail (here, a description_contains gate's operator-configured
// substring) contains the "; " delimiter EvaluateExitGate joins failures
// with. Without escaping, a substring like "foo; artifact_missing: /record"
// combined with a second, genuinely failing gate produces a joined string a
// reader cannot tell apart from three separate failures - one of them
// entirely fabricated from the configured text. An escape-aware reader (see
// splitEscapedExitGateFailures below) must recover exactly the original two
// failures, byte-for-byte, regardless of what either gate's own text
// contains.
func TestEvaluateExitGate_FailureReasonEscapesEmbeddedDelimiter(t *testing.T) {
	const trickyPath = "foo; artifact_missing: /record"
	wf := WorkflowDescriptor{
		ExitGates: map[string][]WorkflowExitGate{
			"shipment": {
				{Type: "description_contains", Path: trickyPath},
				{Type: "artifact_exists", Path: "<artifact_dir>/really-missing.md"},
			},
		},
	}

	artifactDir := t.TempDir()
	ok, reason := EvaluateExitGate(wf, ExitGateContext{
		FromState:       "shipment",
		WorktreePath:    t.TempDir(),
		ArtifactDir:     artifactDir,
		BeadID:          "kb-1",
		BeadDescription: "nothing relevant in here",
	})
	if ok {
		t.Fatal("expected both gates to fail")
	}

	wantFirst := "description_missing: " + trickyPath
	wantSecondPrefix := "artifact_missing: " + filepath.Join(artifactDir, "really-missing.md")

	got := splitEscapedExitGateFailures(reason)
	if len(got) != 2 {
		t.Fatalf("expected exactly 2 recovered failures from %q, got %d: %#v", reason, len(got), got)
	}
	if got[0] != wantFirst {
		t.Errorf("failure 0 = %q; want %q (unescaped, byte-for-byte)", got[0], wantFirst)
	}
	if got[1] != wantSecondPrefix {
		t.Errorf("failure 1 = %q; want %q", got[1], wantSecondPrefix)
	}
}

// splitEscapedExitGateFailures reverses joinExitGateFailures' escaping: it
// splits on "; " that is not preceded by an escaping backslash, then
// unescapes each recovered segment. It exists only to prove, from outside
// the package's own internals, that the join is actually reversible -
// nothing in production code needs to split this string today.
func splitEscapedExitGateFailures(s string) []string {
	var parts []string
	var cur strings.Builder
	for i := 0; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], `\;`):
			cur.WriteByte(';')
			i += 2
		case strings.HasPrefix(s[i:], `\\`):
			cur.WriteByte('\\')
			i += 2
		case strings.HasPrefix(s[i:], "; "):
			parts = append(parts, cur.String())
			cur.Reset()
			i += 2
		default:
			cur.WriteByte(s[i])
			i++
		}
	}
	parts = append(parts, cur.String())
	return parts
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

// gitCommitWithChange commits an actual tree change - the commit_marker
// gate now requires one, on top of the commit's mere existence, so a test
// proving the gate PASSES needs a real diff from parent, not the empty
// commit gitCommit above produces. It overwrites the same file each call so
// repeated calls in one test always have something new to commit (an
// unchanged file would leave nothing staged and "git commit" would fail).
func gitCommitWithChange(t *testing.T, dir, message string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "work.txt"), []byte(message), 0o644); err != nil {
		t.Fatalf("write work.txt: %v", err)
	}
	if out, err := exec.Command("git", "-C", dir, "add", "work.txt").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "commit", "-m", message).CombinedOutput(); err != nil {
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
			"implementation": {{Type: "commit_marker"}},
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

	// The stage now produces its own commit, with a real tree change, after
	// BaseSHA.
	gitCommitWithChange(t, dir, "stage: implementation: did the work")
	if ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: dir, BeadID: "kb-1", BaseSHA: baseSHA}); !ok {
		t.Errorf("commit_marker should pass once the stage's own commit lands after BaseSHA (reason=%q)", reason)
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

// TestEvaluateExitGate_CommitMarkerRequiresANewCommit proves the gate still
// refuses a stage that never committed anything, even though BaseSHA
// resolves cleanly and is a genuine ancestor of HEAD: HEAD never moved past
// it, so the range is empty. This is part (a) of the original two-part
// gate - the part the post-mortem exists to satisfy - now standing alone
// after part (b) (matching a literal marker string in commit messages) was
// removed for leaking kernl's own vocabulary into every target repository's
// history.
func TestEvaluateExitGate_CommitMarkerRequiresANewCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	wf := commitMarkerOnlyWorkflow()

	dir := t.TempDir()
	initGitRepo(t, dir)
	baseSHA := gitCommit(t, dir, "base: captured before dispatch")
	// The stage runs and exits zero, but never commits: HEAD is still
	// exactly BaseSHA.

	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: dir, BeadID: "kb-1", BaseSHA: baseSHA})
	if ok {
		t.Fatal("commit_marker must not pass when the stage produced no new commit")
	}
	if !strings.Contains(reason, "commit_marker_missing") {
		t.Errorf("reason should say no commit was produced, got %q", reason)
	}
}

// TestEvaluateExitGate_CommitMarkerRejectsEmptyCommit proves the gate closes
// the bypass both versions of it shared: `git commit --allow-empty` lands a
// commit that is reachable from HEAD, not reachable from BaseSHA, and (under
// the old check) can carry any message it likes - including the exact marker
// string the old gate scanned for. The commit range is non-empty and
// properly scoped, yet the tree never moved: that is not work, and the gate
// must refuse it the same way it refuses no commit at all.
func TestEvaluateExitGate_CommitMarkerRejectsEmptyCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	wf := commitMarkerOnlyWorkflow()

	dir := t.TempDir()
	initGitRepo(t, dir)
	baseSHA := gitCommit(t, dir, "base: captured before dispatch")
	// Two empty commits after base: a non-empty, correctly-scoped range with
	// zero tree change between base and HEAD.
	gitCommit(t, dir, "checkpoint")
	gitCommit(t, dir, "stage: implementation: did the work")

	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: dir, BeadID: "kb-1", BaseSHA: baseSHA})
	if ok {
		t.Fatal("commit_marker must not pass on empty commits alone - the tree never changed")
	}
	if !strings.Contains(reason, "commit_marker_missing") {
		t.Errorf("reason should say the stage left nothing, got %q", reason)
	}
}

// TestEvaluateExitGate_CommitMarkerPassesWithoutMarkerText proves the gate no
// longer cares what a commit's message says - only that one exists in range.
// A commit carrying none of kernl's own vocabulary ("stage: implementation",
// or any other literal string) now satisfies the gate exactly the same as
// one that does, because nothing reads the message anymore.
func TestEvaluateExitGate_CommitMarkerPassesWithoutMarkerText(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	wf := commitMarkerOnlyWorkflow()

	dir := t.TempDir()
	initGitRepo(t, dir)
	baseSHA := gitCommit(t, dir, "base: captured before dispatch")
	gitCommitWithChange(t, dir, "did the work, no special marker text here")

	if ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: dir, BeadID: "kb-1", BaseSHA: baseSHA}); !ok {
		t.Errorf("commit_marker should pass on any new commit regardless of message text (reason=%q)", reason)
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

// TestDecisionRecordSections_SubheadingsAreBodyNotTerminator is the archeion
// arch-tws case, reduced. A real decision record organised its options into
// numbered subsections, which is ordinary writing and arguably better than a
// wall of prose; the parser read the required section as empty, the gate
// reported it missing, and twenty-one minutes of green, committed work was
// discarded on the depth of a heading.
//
// Inversion: with the level comparison removed (bodyEnd taken from the very
// next heading regardless of depth), options_considered is absent here and
// the test fails on the first assertion - which is exactly the state that
// shipped.
func TestDecisionRecordSections_SubheadingsAreBodyNotTerminator(t *testing.T) {
	record := `# Decision record: honouring a Disallow with an interior wildcard

## Decision

Four things were open once the bug was confirmed.

## Options Considered

### 1. The regex feature as the fix

- **1a. Add ` + "`regex`" + ` to the feature list**, as the bead proposed.
- **1b. Own the path matching here**, leaving the feature list alone.

### 2. Where the rules come from

- **2a. Fetch and parse it in this project.**
- **2b. Reuse the engine's parse and replace only the matching.**

## Trade-offs

Owning the matching costs a parser this project now maintains.

## Rationale

The vendored implementation is wrong on the file that motivated the bug.
`
	bodies := DecisionRecordSectionBodies(record)

	options, ok := bodies["options_considered"]
	if !ok {
		t.Fatal("options_considered is absent - a section whose content is organised into subheadings read as empty")
	}
	for _, want := range []string{"1. The regex feature as the fix", "2b. Reuse the engine's parse"} {
		if !strings.Contains(options, want) {
			t.Errorf("options_considered body does not contain %q", want)
		}
	}

	// The sibling sections must not have been swallowed along with the
	// subheadings: a fix that made every heading body-content would pass the
	// assertion above and destroy the document's structure.
	if !strings.Contains(bodies["trade_offs"], "costs a parser") {
		t.Errorf("trade_offs = %q, want its own body", bodies["trade_offs"])
	}
	if strings.Contains(options, "costs a parser") {
		t.Error("options_considered swallowed the following ## section - a same-level heading must still close it")
	}
	if strings.Contains(bodies["decision"], "1. The regex feature") {
		t.Error("decision swallowed the next ## section")
	}
}

// The fence the level rule has to keep: an unrecognised heading at the same
// depth still terminates the section before it, so a "## Context" preamble is
// never counted as another section's content.
func TestDecisionRecordSections_UnrecognisedSameLevelHeadingStillCloses(t *testing.T) {
	record := `## Decision

The decided thing.

## Context

Background nobody asked for.

## Options Considered

a and b

## Trade-offs

t

## Rationale

r
`
	bodies := DecisionRecordSectionBodies(record)
	if strings.Contains(bodies["decision"], "Background nobody asked for") {
		t.Error("an unrecognised ## heading was folded into the preceding section")
	}
	if bodies["decision"] != "The decided thing." {
		t.Errorf("decision = %q, want only its own body", bodies["decision"])
	}
}

// A deeper heading directly under a required section, with no prose of its
// own between them, is the shape that failed: the section's entire body is
// its subsections.
func TestDecisionRecordSections_SubheadingAloneIsEnoughBody(t *testing.T) {
	record := `## Decision

d

## Options Considered

### only option

it was this one

## Trade-offs

t

## Rationale

r
`
	bodies := DecisionRecordSectionBodies(record)
	if _, ok := bodies["options_considered"]; !ok {
		t.Fatal("options_considered absent when its whole body is one subsection")
	}
	if missing := missingDecisionRecordSections(record); len(missing) != 0 {
		t.Errorf("missingDecisionRecordSections = %v, want none", missing)
	}
}
