package backend

import (
	"os"
	"path/filepath"
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
