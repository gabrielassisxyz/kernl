package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
      path: ".kernl/<bead_id>/plan.md"
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
	if contract.OutputArtifact.Path != ".kernl/<bead_id>/plan.md" {
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
