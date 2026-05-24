package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

// Helper to copy a file
func copyTestFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func TestCustomWorkflow_E2E_AE1(t *testing.T) {
	// 1. Load the custom.yaml using the real loader path
	customYAMLPath := "../../examples/custom-workflow/custom.yaml"
	loadedWf, err := backend.LoadWorkflowYAML(customYAMLPath)
	if err != nil {
		t.Fatalf("Failed to load custom.yaml workflow: %v", err)
	}

	// 2. Register it with the backend workflow registry
	backend.ClearWorkflowRegistry()
	backend.RegisterWorkflow(loadedWf)
	defer backend.ClearWorkflowRegistry()

	// 3. Set up the persisting backend with a test bead
	be := newPersistingBackend()
	beadID := "bead-ae1"
	epicID := "epic-ae1"
	be.beads[beadID] = &backend.Bead{
		ID:        beadID,
		ParentID:  epicID,
		State:     "ready_for_planning",
		ProfileID: "custom_workflow",
	}

	driver := &scriptedDriver{be: be}

	// 4. Set up the AgentStateStore
	storeDir := t.TempDir()
	store, err := workflow.NewAgentStateStore(storeDir)
	if err != nil {
		t.Fatalf("Failed to create agent state store: %v", err)
	}

	// Save initial ContextPayload in the store
	initialPayload := "initial_payload"
	err = store.Save(beadID, workflow.AgentRuntime{ContextPayload: initialPayload})
	if err != nil {
		t.Fatalf("Failed to save initial runtime: %v", err)
	}

	// 5. Set up temporary worktree
	worktreeDir := t.TempDir()

	// Copy the stages/qa.py script to the temporary worktree so the relative path command works
	qaScriptSrc := "../../examples/custom-workflow/stages/qa.py"
	qaScriptDst := filepath.Join(worktreeDir, "examples/custom-workflow/stages/qa.py")
	if err := copyTestFile(qaScriptSrc, qaScriptDst); err != nil {
		t.Fatalf("Failed to copy qa.py script to worktree: %v", err)
	}

	// 6. Drive the bead! Since qa is stage 2 (after planning), we run up to 2 stages
	res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		Backend:         be,
		Driver:          driver,
		Config:          newDriveTestConfig(),
		BeadID:          beadID,
		RepoPath:        "/tmp/repo",
		Worktree:        worktreeDir,
		AgentStateStore: store,
		MaxStages:       2,
	})
	// We expect the loop to terminate with a "max stages exceeded" error since we capped it at 2
	// to prevent it from running the remaining workflow stages (which require further inputs/artifacts).
	if err == nil || !strings.Contains(err.Error(), "exceeded max stages") {
		t.Fatalf("Expected max stages exceeded error, got: %v", err)
	}

	// 7. Verify the bead has advanced successfully to the next stage (ready_for_implementation)
	bd, err := be.Get(beadID, "")
	if err != nil {
		t.Fatalf("Failed to get bead from backend: %v", err)
	}

	if bd.State != "ready_for_implementation" {
		t.Fatalf("Expected bead state to be 'ready_for_implementation', got %q", bd.State)
	}

	if res.Success {
		t.Errorf("Expected DriveBeadToTerminal result success to be false due to max stages cap, got true")
	}

	// 8. Verify context_payload was correctly updated in the store
	runtimeState, err := store.Load(beadID)
	if err != nil {
		t.Fatalf("Failed to load runtime state from store: %v", err)
	}

	expectedPayload := "QA_PASSED:epic-ae1:bead-ae1:initial_payload"
	if runtimeState.ContextPayload != expectedPayload {
		t.Errorf("Expected context_payload to be %q, got %q", expectedPayload, runtimeState.ContextPayload)
	}

	// 9. Verify the exit gate artifact 'qa_verdict.txt' exists and passed
	verdictPath := filepath.Join(worktreeDir, "qa_verdict.txt")
	verdictBytes, err := os.ReadFile(verdictPath)
	if err != nil {
		t.Fatalf("Expected exit gate artifact qa_verdict.txt to exist: %v", err)
	}

	verdictStr := strings.TrimSpace(string(verdictBytes))
	if verdictStr != "VERDICT: PASS" {
		t.Errorf("Expected verdict to be 'VERDICT: PASS', got %q", verdictStr)
	}
}

func TestCustomWorkflow_E2E_AE2(t *testing.T) {
	// 1. Load the custom.yaml using the real loader path
	customYAMLPath := "../../examples/custom-workflow/custom.yaml"
	loadedWf, err := backend.LoadWorkflowYAML(customYAMLPath)
	if err != nil {
		t.Fatalf("Failed to load custom.yaml workflow: %v", err)
	}

	// 2. Register it with the backend workflow registry
	backend.ClearWorkflowRegistry()
	backend.RegisterWorkflow(loadedWf)
	defer backend.ClearWorkflowRegistry()

	// 3. Set up the persisting backend with a test bead
	be := newPersistingBackend()
	beadID := "bead-ae2"
	epicID := "epic-ae2"
	be.beads[beadID] = &backend.Bead{
		ID:        beadID,
		ParentID:  epicID,
		State:     "ready_for_planning",
		ProfileID: "custom_workflow",
	}

	driver := &scriptedDriver{be: be}

	// 4. Set up the AgentStateStore
	storeDir := t.TempDir()
	store, err := workflow.NewAgentStateStore(storeDir)
	if err != nil {
		t.Fatalf("Failed to create agent state store: %v", err)
	}

	// Save "crash" ContextPayload in the store to trigger the deliberate crash in qa.py
	err = store.Save(beadID, workflow.AgentRuntime{ContextPayload: "crash_please"})
	if err != nil {
		t.Fatalf("Failed to save initial runtime: %v", err)
	}

	// 5. Set up temporary worktree
	worktreeDir := t.TempDir()

	// Copy the stages/qa.py script to the temporary worktree so the relative path command works
	qaScriptSrc := "../../examples/custom-workflow/stages/qa.py"
	qaScriptDst := filepath.Join(worktreeDir, "examples/custom-workflow/stages/qa.py")
	if err := copyTestFile(qaScriptSrc, qaScriptDst); err != nil {
		t.Fatalf("Failed to copy qa.py script to worktree: %v", err)
	}

	// 6. Drive the bead! Stage 2 is qa, where it will crash
	res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		Backend:         be,
		Driver:          driver,
		Config:          newDriveTestConfig(),
		BeadID:          beadID,
		RepoPath:        "/tmp/repo",
		Worktree:        worktreeDir,
		AgentStateStore: store,
		MaxStages:       2,
	})
	if err != nil {
		t.Fatalf("Unexpected error during DriveBeadToTerminal: %v", err)
	}

	// 7. Verify the bead has transitioned to 'blocked' status
	bd, err := be.Get(beadID, "")
	if err != nil {
		t.Fatalf("Failed to get bead from backend: %v", err)
	}

	if bd.State != "blocked" {
		t.Fatalf("Expected bead state to be 'blocked', got %q", bd.State)
	}

	if res.Success {
		t.Errorf("Expected DriveBeadToTerminal result success to be false, got true")
	}

	// 8. Verify the comment carries the Python traceback capture
	if len(be.comments) == 0 {
		t.Fatalf("Expected comments to be posted on the bead, got none")
	}

	// Find the failure comment
	var foundComment bool
	var commentBody string
	for _, c := range be.comments {
		if c.ID == beadID && strings.Contains(c.Body, "subprocess stage qa failed") {
			foundComment = true
			commentBody = c.Body
			break
		}
	}

	if !foundComment {
		t.Fatalf("Could not find the expected subprocess failure comment in comments: %+v", be.comments)
	}

	// Verify Python traceback keywords exist in the comment Body
	if !strings.Contains(commentBody, "Traceback (most recent call last):") {
		t.Errorf("Expected comment to carry Python traceback, but it did not. Comment content:\n%s", commentBody)
	}

	if !strings.Contains(commentBody, "ValueError: Deliberate crash requested via context_payload trigger") {
		t.Errorf("Expected comment to contain the raised ValueError message, but it did not. Comment content:\n%s", commentBody)
	}
}
