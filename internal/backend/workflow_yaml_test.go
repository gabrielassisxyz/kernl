package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadWorkflow_StagesBlockParses(t *testing.T) {
	yaml := `
id: test_wf
label: "Test workflow with stages"
stages:
  planning:
    role: "Decompose the bead into a plan."
    inputs:
      - bead.title
      - bead.description
    output_artifact:
      path: "<artifact_dir>/plan.md"
    forbidden_paths:
      - "**/*.go"
      - "**/*.ts"
`

	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	wf, err := LoadWorkflowYAML(path)
	if err != nil {
		t.Fatalf("LoadWorkflowYAML: %v", err)
	}
	if wf.ID != "test_wf" {
		t.Errorf("expected id=test_wf, got %q", wf.ID)
	}
	contract, ok := wf.Stages["planning"]
	if !ok {
		t.Fatal("expected planning stage contract")
	}
	if contract.Role != "Decompose the bead into a plan." {
		t.Errorf("expected role text, got %q", contract.Role)
	}
	if len(contract.Inputs) != 2 {
		t.Errorf("expected 2 inputs, got %d", len(contract.Inputs))
	}
	if contract.OutputArtifact.Path != "<artifact_dir>/plan.md" {
		t.Errorf("expected artifact path, got %q", contract.OutputArtifact.Path)
	}
	if len(contract.ForbiddenPaths) != 2 {
		t.Errorf("expected 2 forbidden paths, got %d", len(contract.ForbiddenPaths))
	}
}

func TestLoadWorkflow_StagesBlockOptional(t *testing.T) {
	yaml := `
id: minimal_wf
label: "Minimal workflow"
`

	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	wf, err := LoadWorkflowYAML(path)
	if err != nil {
		t.Fatalf("LoadWorkflowYAML: %v", err)
	}
	if wf.ID != "minimal_wf" {
		t.Errorf("expected id=minimal_wf, got %q", wf.ID)
	}
	if len(wf.Stages) != 0 {
		t.Errorf("expected empty stages map, got %d entries", len(wf.Stages))
	}
}

func TestLoadWorkflow_UnknownFieldUnderStageRejects(t *testing.T) {
	yamlText := `
id: bad_wf
stages:
  planning:
    role: "Some role"
    invalid_field: "foo"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(yamlText), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadWorkflowYAML(path)
	if err == nil {
		t.Fatal("expected error due to unknown field 'invalid_field' under stages.planning")
	}
}

func TestLoadWorkflow_SubprocessStageHappy(t *testing.T) {
	yaml := `
id: test_wf
stages:
  planning:
    role: "Decompose the bead into a plan."
    inputs:
      - bead.title
  subprocess_stage:
    kind: "subprocess"
    subprocess:
      command: ["python3", "run.py"]
      timeout: "5m"
`

	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	wf, err := LoadWorkflowYAML(path)
	if err != nil {
		t.Fatalf("LoadWorkflowYAML: %v", err)
	}

	plan, ok := wf.Stages["planning"]
	if !ok {
		t.Fatal("expected planning stage")
	}
	if plan.Kind != "" && plan.Kind != "native" {
		t.Errorf("expected empty or native kind, got %q", plan.Kind)
	}
	if plan.Subprocess != nil {
		t.Errorf("expected nil subprocess for native stage, got %v", plan.Subprocess)
	}

	sub, ok := wf.Stages["subprocess_stage"]
	if !ok {
		t.Fatal("expected subprocess_stage")
	}
	if sub.Kind != "subprocess" {
		t.Errorf("expected kind=subprocess, got %q", sub.Kind)
	}
	if sub.Subprocess == nil {
		t.Fatal("expected non-nil subprocess spec")
	}
	if len(sub.Subprocess.Command) != 2 || sub.Subprocess.Command[0] != "python3" || sub.Subprocess.Command[1] != "run.py" {
		t.Errorf("unexpected command: %v", sub.Subprocess.Command)
	}
	if sub.Subprocess.Timeout != "5m" {
		t.Errorf("expected timeout=5m, got %q", sub.Subprocess.Timeout)
	}
}

func TestLoadWorkflow_BothNativeAndSubprocessRejects(t *testing.T) {
	// Native stage with subprocess block
	yaml1 := `
id: test_wf
stages:
  bad_stage:
    role: "Role text"
    subprocess:
      command: ["python3"]
`
	// Subprocess stage with role
	yaml2 := `
id: test_wf
stages:
  bad_stage:
    kind: "subprocess"
    role: "Role text"
    subprocess:
      command: ["python3"]
`

	for i, y := range []string{yaml1, yaml2} {
		dir := t.TempDir()
		path := filepath.Join(dir, fmt.Sprintf("workflow_%d.yaml", i))
		if err := os.WriteFile(path, []byte(y), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := LoadWorkflowYAML(path)
		if err == nil {
			t.Errorf("case %d: expected error when both native and subprocess fields are set", i)
		} else if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE: bad_stage") {
			t.Errorf("case %d: expected stage name 'bad_stage' in dispatch error: %v", i, err)
		}
	}
}

func TestLoadWorkflow_SubprocessMissingScriptRejects(t *testing.T) {
	yaml := `
id: test_wf
stages:
  bad_stage:
    kind: "subprocess"
`

	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadWorkflowYAML(path)
	if err == nil {
		t.Fatal("expected error when subprocess stage is missing script/command")
	} else if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE: bad_stage") {
		t.Errorf("expected stage name 'bad_stage' in dispatch error: %v", err)
	}
}

// TestLoadWorkflow_LegacyInWorktreeArtifactPathRejects proves a workflow
// definition that still names the pre-fix ".kernl/<bead_id>/..." location
// fails at load time instead of being silently honored - honoring it would
// mean every workflow written from an unmigrated example keeps writing
// kernl's own control files into the target repository's commits, which is
// exactly the defect the artifact directory move closed.
func TestLoadWorkflow_LegacyInWorktreeArtifactPathRejects(t *testing.T) {
	cases := map[string]string{
		"output_artifact.path": `
id: legacy_wf
stages:
  planning:
    role: "Plan"
    output_artifact:
      path: ".kernl/<bead_id>/plan.md"
`,
		"inputs entry": `
id: legacy_wf
stages:
  plan_review:
    role: "Review"
    inputs:
      - ".kernl/<bead_id>/plan.md"
`,
		"exit gate path": `
id: legacy_wf
exit_gates:
  implementation_review:
    - type: "artifact_verdict"
      path: ".kernl/<bead_id>/implementation-review.md"
`,
		"decision_record.path": `
id: legacy_wf
stages:
  implementation:
    role: "Implement"
    decision_record:
      path: ".kernl/<bead_id>/decision-record.md"
`,
		"decision_record exit gate path": `
id: legacy_wf
exit_gates:
  implementation:
    - type: "decision_record"
      path: ".kernl/<bead_id>/decision-record.md"
`,
	}

	for name, yamlText := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "workflow.yaml")
			if err := os.WriteFile(path, []byte(yamlText), 0644); err != nil {
				t.Fatal(err)
			}

			_, err := LoadWorkflowYAML(path)
			if err == nil {
				t.Fatal("expected LoadWorkflowYAML to reject a legacy in-worktree .kernl/ artifact path")
			}
			if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
				t.Errorf("error must carry the KERNL DISPATCH FAILURE marker, got: %v", err)
			}
			if !strings.Contains(err.Error(), "<artifact_dir>") {
				t.Errorf("error must name <artifact_dir> as the fix, got: %v", err)
			}
		})
	}
}

// TestLoadWorkflow_DecisionRecordPathMustStayBeneathArtifactDir proves a
// decision_record path is rejected unless it is anchored at, and stays
// confined beneath, <artifact_dir> - unlike OutputArtifact/artifact_verdict
// paths, which may still legitimately resolve relative to the worktree, a
// decision_record path has no such fallback and exists solely to keep this
// control file out of the target repository.
func TestLoadWorkflow_DecisionRecordPathMustStayBeneathArtifactDir(t *testing.T) {
	cases := map[string]string{
		"stage decision_record.path not anchored at all": `
id: unanchored_wf
stages:
  implementation:
    role: "Implement"
    decision_record:
      path: "decision-record.md"
`,
		"exit gate decision_record path not anchored at all": `
id: unanchored_wf
exit_gates:
  implementation:
    - type: "decision_record"
      path: "decision-record.md"
`,
		"stage decision_record.path escapes via ..": `
id: escaping_wf
stages:
  implementation:
    role: "Implement"
    decision_record:
      path: "<artifact_dir>/../../etc/passwd"
`,
		"exit gate decision_record path escapes via ..": `
id: escaping_wf
exit_gates:
  implementation:
    - type: "decision_record"
      path: "<artifact_dir>/../../etc/passwd"
`,
	}

	for name, yamlText := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "workflow.yaml")
			if err := os.WriteFile(path, []byte(yamlText), 0644); err != nil {
				t.Fatal(err)
			}

			_, err := LoadWorkflowYAML(path)
			if err == nil {
				t.Fatal("expected LoadWorkflowYAML to reject a decision_record path not confined beneath <artifact_dir>")
			}
			if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
				t.Errorf("error must carry the KERNL DISPATCH FAILURE marker, got: %v", err)
			}
			if !strings.Contains(err.Error(), "<artifact_dir>") {
				t.Errorf("error must name <artifact_dir> as the anchor, got: %v", err)
			}
		})
	}
}

// TestLoadWorkflow_DecisionRecordPathProperlyAnchoredAccepted is the
// positive case for the test above: a path anchored at, and confined
// beneath, <artifact_dir> loads without error.
func TestLoadWorkflow_DecisionRecordPathProperlyAnchoredAccepted(t *testing.T) {
	yamlText := `
id: anchored_wf
stages:
  implementation:
    role: "Implement"
    decision_record:
      path: "<artifact_dir>/decision-record.md"
exit_gates:
  implementation:
    - type: "decision_record"
      path: "<artifact_dir>/decision-record.md"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(yamlText), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadWorkflowYAML(path); err != nil {
		t.Fatalf("expected a properly anchored decision_record path to load, got: %v", err)
	}
}

// TestLoadWorkflow_DecisionRecordPathMismatchBetweenStageAndGateRejects
// proves the exact scenario the review reported: a stage tells the agent to
// write one file while the exit gate reads a different one, so a competent
// implementer that writes exactly what it was told still fails the gate.
func TestLoadWorkflow_DecisionRecordPathMismatchBetweenStageAndGateRejects(t *testing.T) {
	yamlText := `
id: mismatched_wf
exit_gates:
  implementation:
    - type: decision_record
      path: "<artifact_dir>/gate-record.md"
stages:
  implementation:
    role: "Implement"
    decision_record:
      path: "<artifact_dir>/prompt-record.md"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(yamlText), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadWorkflowYAML(path)
	if err == nil {
		t.Fatal("expected LoadWorkflowYAML to reject a decision_record path mismatch between the stage and its exit gate")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error must carry the KERNL DISPATCH FAILURE marker, got: %v", err)
	}
	if !strings.Contains(err.Error(), "gate-record.md") || !strings.Contains(err.Error(), "prompt-record.md") {
		t.Errorf("error must name both disagreeing paths, got: %v", err)
	}
}

// TestLoadWorkflow_DecisionRecordPathMatchBetweenStageAndGateAccepted is the
// positive case: identical paths on the stage and the gate load cleanly.
func TestLoadWorkflow_DecisionRecordPathMatchBetweenStageAndGateAccepted(t *testing.T) {
	yamlText := `
id: matched_wf
exit_gates:
  implementation:
    - type: decision_record
      path: "<artifact_dir>/decision-record.md"
stages:
  implementation:
    role: "Implement"
    decision_record:
      path: "<artifact_dir>/decision-record.md"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(yamlText), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadWorkflowYAML(path); err != nil {
		t.Fatalf("expected matching stage/gate decision_record paths to load, got: %v", err)
	}
}

func TestLoadWorkflow_UnknownFieldUnderSubprocessRejects(t *testing.T) {
	yamlText := `
id: bad_wf
stages:
  planning:
    kind: "subprocess"
    subprocess:
      command: ["python3"]
      invalid_field: "foo"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(yamlText), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadWorkflowYAML(path)
	if err == nil {
		t.Fatal("expected error due to unknown field 'invalid_field' under stages.planning.subprocess")
	}
}

// TestLoadWorkflow_LegacySingleGateExitGatesShapeRejects proves a workflow
// YAML still using the pre-list exit_gates shape (a single gate object
// directly under the state key) fails loud at load, naming the offending
// state and showing the list shape that fixes it - not just failing with
// yaml.v3's default "cannot unmarshal !!map into []backend.WorkflowExitGate",
// which names a line number but never the state (acceptance criterion 4).
func TestLoadWorkflow_LegacySingleGateExitGatesShapeRejects(t *testing.T) {
	yamlText := `
id: old_shape_wf
exit_gates:
  implementation:
    type: commit_marker
    path: "stage: implementation"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(yamlText), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadWorkflowYAML(path)
	if err == nil {
		t.Fatal("expected LoadWorkflowYAML to reject the old single-gate exit_gates shape")
	}
	msg := err.Error()
	if !strings.Contains(msg, "KERNL DISPATCH FAILURE") {
		t.Errorf("error must carry the KERNL DISPATCH FAILURE marker, got: %v", err)
	}
	if !strings.Contains(msg, "exit_gates.implementation") {
		t.Errorf("error must name the offending state 'implementation', got: %v", err)
	}
	if !strings.Contains(msg, `- type: "commit_marker"`) || !strings.Contains(msg, `path: "stage: implementation"`) {
		t.Errorf("error must show the list shape that fixes it, got: %v", err)
	}

	assertSuggestedFixParses(t, msg)
}

// assertSuggestedFixParses extracts the YAML snippet after "Fix: write it
// as" from a rejectLegacyExitGatesShape error and feeds it back through the
// real YAML parser. A suggested fix that itself fails to parse is not a fix:
// the value driving this precheck (a gate's own path, e.g. commit_marker's
// "stage: implementation", which contains ": ") previously produced
// unquoted output that yaml.v3 rejects with "mapping values are not allowed
// here" when copied verbatim.
func assertSuggestedFixParses(t *testing.T, errMsg string) {
	t.Helper()
	const marker = "Fix: write it as\n"
	idx := strings.Index(errMsg, marker)
	if idx == -1 {
		t.Fatalf("error must contain the %q marker ahead of the suggested fix, got: %s", marker, errMsg)
	}
	snippet := errMsg[idx+len(marker):]

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(snippet), &parsed); err != nil {
		t.Fatalf("the suggested fix does not parse as YAML: %v\nsnippet:\n%s", err, snippet)
	}
	if _, ok := parsed["exit_gates"]; !ok {
		t.Errorf("parsed suggested fix missing 'exit_gates', got: %+v", parsed)
	}
}

// TestLoadWorkflow_LegacyShapeViaYAMLAliasRejects proves the old-shape
// precheck sees through a YAML alias, not just a literal mapping node. A
// state whose value is "*legacy" - an alias to an old-shape gate object
// anchored elsewhere in the same document - carries no Kind of its own to
// classify; without resolving it first, the precheck would skip straight
// past it and fall through to yaml.v3's own unreadable decode error, which
// is exactly the message this file exists to replace.
func TestLoadWorkflow_LegacyShapeViaYAMLAliasRejects(t *testing.T) {
	yamlText := `
id: aliased_wf
exit_gates:
  planning:
    - &legacy
      type: commit_marker
      path: marker
  implementation: *legacy
`
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(yamlText), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadWorkflowYAML(path)
	if err == nil {
		t.Fatal("expected LoadWorkflowYAML to reject the old shape reached through a YAML alias")
	}
	msg := err.Error()
	if !strings.Contains(msg, "KERNL DISPATCH FAILURE") {
		t.Errorf("error must carry the KERNL DISPATCH FAILURE marker, got: %v", err)
	}
	if !strings.Contains(msg, "exit_gates.implementation") {
		t.Errorf("error must name the aliased state 'implementation', got: %v", err)
	}
	if strings.Contains(msg, "cannot unmarshal") {
		t.Errorf("error must be the actionable precheck message, not yaml.v3's raw decode error, got: %v", err)
	}
}

// TestLoadWorkflow_ExitGatesListShapeWithMultipleGatesParses proves a state
// declaring more than one gate in the new list shape loads correctly and
// both gates are preserved in order.
func TestLoadWorkflow_ExitGatesListShapeWithMultipleGatesParses(t *testing.T) {
	yamlText := `
id: multi_gate_wf
exit_gates:
  implementation:
    - type: commit_marker
    - type: decision_record
      path: "<artifact_dir>/decision-record.md"
stages:
  implementation:
    role: "Implement"
    decision_record:
      path: "<artifact_dir>/decision-record.md"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(yamlText), 0644); err != nil {
		t.Fatal(err)
	}

	wf, err := LoadWorkflowYAML(path)
	if err != nil {
		t.Fatalf("LoadWorkflowYAML: %v", err)
	}
	gates := wf.ExitGates["implementation"]
	if len(gates) != 2 {
		t.Fatalf("expected 2 exit gates on 'implementation', got %d: %+v", len(gates), gates)
	}
	if gates[0].Type != "commit_marker" || gates[0].Path != "" {
		t.Errorf("gate 0 = %+v; want commit_marker with no path", gates[0])
	}
	if gates[1].Type != "decision_record" || gates[1].Path != "<artifact_dir>/decision-record.md" {
		t.Errorf("gate 1 = %+v; want decision_record/<artifact_dir>/decision-record.md", gates[1])
	}
}

// TestLoadWorkflow_CommitMarkerPathRejects proves a workflow YAML whose
// commit_marker gate still sets a path fails loud at load, naming the fix -
// the gate stopped reading commit message text (a leak of kernl's own
// "stage: X" vocabulary into every target repository's history), so a path
// left over from before that change would otherwise be silently ignored
// instead of telling its author the check it configured no longer applies.
func TestLoadWorkflow_CommitMarkerPathRejects(t *testing.T) {
	yamlText := `
id: stale_marker_wf
exit_gates:
  implementation:
    - type: commit_marker
      path: "stage: implementation"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(yamlText), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadWorkflowYAML(path)
	if err == nil {
		t.Fatal("expected LoadWorkflowYAML to reject a commit_marker gate that still sets a path")
	}
	msg := err.Error()
	if !strings.Contains(msg, "KERNL DISPATCH FAILURE") {
		t.Errorf("error must carry the KERNL DISPATCH FAILURE marker, got: %v", err)
	}
	if !strings.Contains(msg, "implementation") {
		t.Errorf("error must name the offending state, got: %v", err)
	}
	if !strings.Contains(msg, "remove the path field") {
		t.Errorf("error must say what to do about it, got: %v", err)
	}
}

// TestLoadWorkflow_ExampleCustomWorkflowLoadsUnderListShape proves the one
// user-facing workflow file in the repository, migrated to the new exit_gates
// list shape, still loads (acceptance criterion 5).
func TestLoadWorkflow_ExampleCustomWorkflowLoadsUnderListShape(t *testing.T) {
	wf, err := LoadWorkflowYAML("../../examples/custom-workflow/custom.yaml")
	if err != nil {
		t.Fatalf("expected examples/custom-workflow/custom.yaml to load under the new list shape, got: %v", err)
	}
	if len(wf.ExitGates["qa"]) != 1 || wf.ExitGates["qa"][0].Type != "artifact_verdict" {
		t.Errorf("qa exit gate = %+v; want exactly one artifact_verdict", wf.ExitGates["qa"])
	}
}
