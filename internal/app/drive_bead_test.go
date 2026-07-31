package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

type persistingBackend struct {
	mu       sync.Mutex
	beads    map[string]*backend.Bead
	writes   int
	comments []struct {
		ID   string
		Body string
	}
}

func newPersistingBackend() *persistingBackend {
	return &persistingBackend{beads: make(map[string]*backend.Bead)}
}

func (b *persistingBackend) Get(id string, _ string) (*backend.Bead, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if bd, ok := b.beads[id]; ok {
		cp := *bd
		cp.Labels = append([]string(nil), bd.Labels...)
		return &cp, nil
	}
	return nil, nil
}

func (b *persistingBackend) Update(id string, in backend.UpdateBeadInput, _ string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	bd, ok := b.beads[id]
	if !ok {
		return nil
	}
	if in.State != "" {
		bd.State = in.State
	}
	if in.SetLabels != nil {
		bd.Labels = append([]string(nil), in.SetLabels...)
	}
	b.writes++
	return nil
}

func (b *persistingBackend) List(_ *backend.BeadListFilters, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *persistingBackend) ListReady(_ *backend.BeadListFilters, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *persistingBackend) Create(_ backend.CreateBeadInput, _ string) (*backend.Bead, error) {
	return nil, nil
}
func (b *persistingBackend) Delete(_ string, _ string) error { return nil }
func (b *persistingBackend) Close(_ string, _ string, _ string) (*backend.TerminalState, error) {
	return nil, nil
}
func (b *persistingBackend) MarkTerminal(_, _, _, _ string) error { return nil }
func (b *persistingBackend) Reopen(_, _, _ string) error          { return nil }
func (b *persistingBackend) Rewind(_, _, _, _ string) error       { return nil }
func (b *persistingBackend) Search(_ string, _ *backend.BeadListFilters, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *persistingBackend) Query(_ string, _ *backend.BeadQueryOptions, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *persistingBackend) AddDependency(_, _, _ string) error    { return nil }
func (b *persistingBackend) RemoveDependency(_, _, _ string) error { return nil }
func (b *persistingBackend) ListDependencies(_, _ string, _ *backend.DependencyListOptions) ([]backend.BeadDependency, error) {
	return nil, nil
}
func (b *persistingBackend) BuildTakePrompt(_ string, _ *backend.TakePromptOptions, _ string) (*backend.TakePromptResult, error) {
	return nil, nil
}
func (b *persistingBackend) BuildPollPrompt(_ *backend.PollPromptOptions, _ string) (*backend.PollPromptResult, error) {
	return nil, nil
}
func (b *persistingBackend) ListWorkflows(_ string) ([]backend.WorkflowDescriptor, error) {
	return nil, nil
}
func (b *persistingBackend) Comment(id string, body string, _ string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.comments = append(b.comments, struct {
		ID   string
		Body string
	}{ID: id, Body: body})
	return nil
}
func (b *persistingBackend) Capabilities() backend.BackendCapabilities {
	return backend.BackendCapabilities{}
}

type scriptedDriver struct {
	be    *persistingBackend
	calls int
}

func (s *scriptedDriver) RunBead(ctx context.Context, in RunBeadInput) (RunBeadResult, error) {
	s.calls++
	return RunBeadResult{FinalState: "ok", Success: true, SessionID: "ses_test"}, nil
}

func newDriveTestConfig() *config.Config {
	return &config.Config{
		Settings: config.Settings{
			Agents: map[string]config.AgentConfig{
				"opencode": {Command: "opencode", Args: []string{"run"}},
			},
			Pools: map[string]config.PoolConfig{
				"planning":              {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
				"plan_review":           {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
				"implementation":        {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
				"implementation_review": {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
				"integration":           {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
				"integration_review":    {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
				"shipment":              {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
				"shipment_review":       {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
			},
		},
	}
}

func TestDriveBead_OrchestratorAdvancesAfterAgentSuccess(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{
		ID:    "kb-1",
		State: "planning",
	}

	driver := &scriptedDriver{be: be}

	_, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand: "bd",
		StateDir:       t.TempDir(),
		VerifyCommand:  "bin/ci",
		Backend:        be,
		Driver:         driver,
		Config:         newDriveTestConfig(),
		BeadID:         "kb-1",
		RepoPath:       "/tmp/repo",
		Worktree:       "/tmp/worktree",
		MaxStages:      16,
	})

	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}
	bd, _ := be.Get("kb-1", "")
	if bd.State != "shipped" {
		t.Errorf("expected final state shipped, got %q", bd.State)
	}
}

func TestDriveBead_GateDefaultIsAgentExitZero(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{
		ID:        "kb-1",
		State:     "implementation",
		ProfileID: "autopilot",
	}

	driver := &scriptedDriver{be: be}

	_, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand: "bd",
		StateDir:       t.TempDir(),
		VerifyCommand:  "bin/ci",
		Backend:        be,
		Driver:         driver,
		Config:         newDriveTestConfig(),
		BeadID:         "kb-1",
		RepoPath:       "/tmp/repo",
		Worktree:       "/tmp/worktree",
		MaxStages:      16,
	})

	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}
	bd, _ := be.Get("kb-1", "")
	if bd.State != "shipped" {
		t.Errorf("expected final state shipped (default agent_exit_zero), got %q", bd.State)
	}
}

func TestDriveBead_AgentBdUpdateAttemptDoesNotDoubleAdvance(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{
		ID:        "kb-1",
		State:     "planning",
		ProfileID: "autopilot",
	}

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
		MaxStages:      16,
	})

	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}
	if !res.Success {
		t.Error("expected success after idempotent advancement")
	}
	if bd, _ := be.Get("kb-1", ""); bd.State != "shipped" {
		t.Errorf("expected final state shipped, got %q", bd.State)
	}
}

func TestDriveBeadToTerminal_LoopsThroughMultipleStages(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{
		ID:    "kb-1",
		State: "ready_for_implementation",
	}

	driver := &scriptedDriver{be: be}

	var stages []string
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
		Log: func(stage int, state string) {
			stages = append(stages, state)
		},
	})

	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}
	if !res.Success {
		t.Errorf("expected success, got Success=false, FinalState=%q", res.FinalState)
	}
	if res.FinalState != "shipped" {
		t.Errorf("expected FinalState=shipped, got %q", res.FinalState)
	}
	if driver.calls < 4 {
		t.Errorf("expected at least 4 agent calls, got %d", driver.calls)
	}
}

func TestDriveBeadToTerminal_TerminalStateShortCircuits(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "shipped"}

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
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success || res.FinalState != "shipped" {
		t.Errorf("expected success at shipped, got %+v", res)
	}
	if driver.calls != 0 {
		t.Errorf("expected zero agent calls for terminal bead, got %d", driver.calls)
	}
}

func TestDriveBeadToTerminal_AwaitingIntegrationIsGate(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "awaiting_integration"}

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
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success || res.FinalState != "awaiting_integration" {
		t.Errorf("expected success at awaiting_integration, got %+v", res)
	}
	if driver.calls != 0 {
		t.Errorf("expected zero agent calls for gate state, got %d", driver.calls)
	}
}

func TestDriveBeadToTerminal_BlockedReturnsFalse(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "blocked"}

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
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Error("expected Success=false for blocked bead")
	}
}

func TestDriveBead_StageCommentRecorded(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{
		ID:        "kb-1",
		State:     "planning",
		ProfileID: "autopilot",
	}

	driver := &scriptedDriver{be: be}

	_, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand: "bd",
		StateDir:       t.TempDir(),
		VerifyCommand:  "bin/ci",
		Backend:        be,
		Driver:         driver,
		Config:         newDriveTestConfig(),
		BeadID:         "kb-1",
		RepoPath:       "/tmp/repo",
		Worktree:       "/tmp/worktree",
		MaxStages:      16,
	})
	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}

	be.mu.Lock()
	comments := be.comments
	be.mu.Unlock()

	if len(comments) == 0 {
		t.Fatal("expected Comment to be called after stage advancement")
	}

	for i, c := range comments {
		if c.ID != "kb-1" {
			t.Errorf("comment %d: expected bead ID kb-1, got %q", i, c.ID)
		}
		for _, field := range []string{"stage:", "agent:", "session_id:", "artifact:", "commit:", "duration:"} {
			if !strings.Contains(c.Body, field) {
				t.Errorf("comment %d: expected body to contain %q, got:\n%s", i, field, c.Body)
			}
		}
	}
}

func TestDriveBeadToTerminal_UnknownStateTriggersDispatchFailure(t *testing.T) {
	// A bead in an unrecognized state (e.g. from a future bd version) must not
	// be silently re-queued as "ready_for_implementation". The normalizer
	// passes the raw status through and the dispatcher must fail loud.
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "limbo"}

	driver := &scriptedDriver{be: be}
	_, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
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

	if err == nil {
		t.Fatal("expected KERNL DISPATCH FAILURE for unknown state, got nil error")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("expected error to contain KERNL DISPATCH FAILURE, got: %v", err)
	}
	if driver.calls != 0 {
		t.Errorf("expected zero agent calls for unroutable state, got %d", driver.calls)
	}
}

func createTestPythonScript(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "script.py")
	err := os.WriteFile(path, []byte(content), 0755)
	if err != nil {
		t.Fatalf("failed to create script: %v", err)
	}
	return path
}

func TestDriveBead_SubprocessWorkflow_MixedAndAccumulation(t *testing.T) {
	scriptPath := createTestPythonScript(t, `#!/usr/bin/env python3
import sys
import json
import os

req = json.load(sys.stdin)
epic_id = req.get("epic_id", "")
bead_id = req.get("bead_id", "")
worktree_path = req.get("worktree_path", "")
context_payload = req.get("context_payload", "")

new_payload = f"epic:{epic_id}|bead:{bead_id}|prev:{context_payload}"

# Write gate artifact
with open(os.path.join(worktree_path, "gate_artifact.txt"), "w") as f:
    f.write("VERDICT: PASS")

print(json.dumps({"context_payload": new_payload}))
`)

	backend.ClearWorkflowRegistry()
	customWf := backend.WorkflowDescriptor{
		ID:           "mixed-subprocess",
		InitialState: "ready_for_planning",
		States: []string{
			"ready_for_planning", "planning",
			"ready_for_sub1", "sub1",
			"ready_for_sub2", "sub2",
			"ready_for_implementation", "implementation",
			"shipped",
		},
		TerminalStates: []string{"shipped"},
		Transitions: []backend.WorkflowTransition{
			{From: "ready_for_planning", To: "planning"},
			{From: "planning", To: "ready_for_sub1"},
			{From: "ready_for_sub1", To: "sub1"},
			{From: "sub1", To: "ready_for_sub2"},
			{From: "ready_for_sub2", To: "sub2"},
			{From: "sub2", To: "ready_for_implementation"},
			{From: "ready_for_implementation", To: "implementation"},
			{From: "implementation", To: "shipped"},
		},
		QueueStates: []string{
			"ready_for_planning",
			"ready_for_sub1",
			"ready_for_sub2",
			"ready_for_implementation",
		},
		ActionStates: []string{
			"planning",
			"sub1",
			"sub2",
			"implementation",
		},
		QueueActions: map[string]string{
			"ready_for_planning":       "planning",
			"ready_for_sub1":           "sub1",
			"ready_for_sub2":           "sub2",
			"ready_for_implementation": "implementation",
		},
		ExitGates: map[string][]backend.WorkflowExitGate{
			"sub1": {{Type: "artifact_verdict", Path: "gate_artifact.txt"}},
			"sub2": {{Type: "artifact_verdict", Path: "gate_artifact.txt"}},
		},
		Stages: map[string]backend.StageContract{
			"planning": {Role: "worker", Kind: "native"},
			"sub1": {
				Role: "subprocess",
				Kind: "subprocess",
				Subprocess: &backend.SubprocessSpec{
					Command: []string{scriptPath},
				},
			},
			"sub2": {
				Role: "subprocess",
				Kind: "subprocess",
				Subprocess: &backend.SubprocessSpec{
					Command: []string{scriptPath},
				},
			},
			"implementation": {Role: "worker", Kind: "native"},
		},
	}
	backend.RegisterWorkflow(customWf)

	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{
		ID:        "kb-1",
		ParentID:  "parent-epic-id",
		State:     "ready_for_planning",
		ProfileID: "mixed-subprocess",
	}

	driver := &scriptedDriver{be: be}

	// Create AgentStateStore
	storeDir := t.TempDir()
	store, err := workflow.NewAgentStateStore(storeDir)
	if err != nil {
		t.Fatalf("failed to create agent state store: %v", err)
	}

	// Save initial state with empty payload
	err = store.Save("kb-1", workflow.AgentRuntime{ContextPayload: "initial"})
	if err != nil {
		t.Fatalf("failed to save initial runtime: %v", err)
	}

	worktreeDir := t.TempDir()
	_, err = DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand:  "bd",
		StateDir:        t.TempDir(),
		VerifyCommand:   "bin/ci",
		Backend:         be,
		Driver:          driver,
		Config:          newDriveTestConfig(),
		BeadID:          "kb-1",
		RepoPath:        "/tmp/repo",
		Worktree:        worktreeDir,
		AgentStateStore: store,
		MaxStages:       16,
	})
	if err != nil {
		t.Fatalf("unexpected error during DriveBeadToTerminal: %v", err)
	}

	bd, _ := be.Get("kb-1", "")
	if bd.State != "shipped" {
		t.Fatalf("expected final state shipped, got %q", bd.State)
	}

	// Verify context_payload accumulation
	finalRuntime, err := store.Load("kb-1")
	if err != nil {
		t.Fatalf("failed to load final runtime state: %v", err)
	}

	expectedPayload := "epic:parent-epic-id|bead:kb-1|prev:epic:parent-epic-id|bead:kb-1|prev:initial"
	if finalRuntime.ContextPayload != expectedPayload {
		t.Errorf("expected context payload accumulation:\n%q\ngot:\n%q", expectedPayload, finalRuntime.ContextPayload)
	}
}

func TestDriveBead_Subprocess_EdgeCases(t *testing.T) {
	scriptPath := createTestPythonScript(t, `#!/usr/bin/env python3
import sys
import json
import os

req = json.load(sys.stdin)
epic_id = req.get("epic_id", "")
bead_id = req.get("bead_id", "")
worktree_path = req.get("worktree_path", "")
context_payload = req.get("context_payload", "")

new_payload = f"epic:{epic_id}|bead:{bead_id}"

if "write_artifact" in context_payload:
    with open(os.path.join(worktree_path, "gate_artifact.txt"), "w") as f:
        f.write("VERDICT: PASS")

print(json.dumps({"context_payload": new_payload}))
`)

	backend.ClearWorkflowRegistry()
	customWf := backend.WorkflowDescriptor{
		ID:           "subprocess-edge",
		InitialState: "ready_for_sub",
		States: []string{
			"ready_for_sub", "sub",
			"shipped",
		},
		TerminalStates: []string{"shipped"},
		Transitions: []backend.WorkflowTransition{
			{From: "ready_for_sub", To: "sub"},
			{From: "sub", To: "shipped"},
		},
		QueueStates: []string{
			"ready_for_sub",
		},
		ActionStates: []string{
			"sub",
		},
		QueueActions: map[string]string{
			"ready_for_sub": "sub",
		},
		ExitGates: map[string][]backend.WorkflowExitGate{
			"sub": {{Type: "artifact_verdict", Path: "gate_artifact.txt"}},
		},
		Stages: map[string]backend.StageContract{
			"sub": {
				Role: "subprocess",
				Kind: "subprocess",
				Subprocess: &backend.SubprocessSpec{
					Command: []string{scriptPath},
				},
			},
		},
	}
	backend.RegisterWorkflow(customWf)

	t.Run("Missing artifact blocks", func(t *testing.T) {
		be := newPersistingBackend()
		be.beads["kb-2"] = &backend.Bead{
			ID:        "kb-2",
			ParentID:  "parent-epic-id",
			State:     "ready_for_sub",
			ProfileID: "subprocess-edge",
		}

		driver := &scriptedDriver{be: be}
		storeDir := t.TempDir()
		store, _ := workflow.NewAgentStateStore(storeDir)
		_ = store.Save("kb-2", workflow.AgentRuntime{ContextPayload: "no_artifact"})

		worktreeDir := t.TempDir()
		_, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
			TrackerCommand:  "bd",
			StateDir:        t.TempDir(),
			VerifyCommand:   "bin/ci",
			Backend:         be,
			Driver:          driver,
			Config:          newDriveTestConfig(),
			BeadID:          "kb-2",
			RepoPath:        "/tmp/repo",
			Worktree:        worktreeDir,
			AgentStateStore: store,
			MaxStages:       16,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		bd, _ := be.Get("kb-2", "")
		if bd.State != "blocked" {
			t.Errorf("expected state to be blocked, got %q", bd.State)
		}
	})

	t.Run("No declared exit gate advances on exit 0", func(t *testing.T) {
		backend.ClearWorkflowRegistry()
		noGateWf := customWf
		noGateWf.ExitGates = nil
		backend.RegisterWorkflow(noGateWf)

		be := newPersistingBackend()
		be.beads["kb-3"] = &backend.Bead{
			ID:        "kb-3",
			ParentID:  "parent-epic-id",
			State:     "ready_for_sub",
			ProfileID: "subprocess-edge",
		}

		driver := &scriptedDriver{be: be}
		storeDir := t.TempDir()
		store, _ := workflow.NewAgentStateStore(storeDir)

		worktreeDir := t.TempDir()
		_, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
			TrackerCommand:  "bd",
			StateDir:        t.TempDir(),
			VerifyCommand:   "bin/ci",
			Backend:         be,
			Driver:          driver,
			Config:          newDriveTestConfig(),
			BeadID:          "kb-3",
			RepoPath:        "/tmp/repo",
			Worktree:        worktreeDir,
			AgentStateStore: store,
			MaxStages:       16,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		bd, _ := be.Get("kb-3", "")
		if bd.State != "shipped" {
			t.Errorf("expected final state shipped, got %q", bd.State)
		}
	})

	t.Run("Empty ParentID uses bead.ID", func(t *testing.T) {
		backend.ClearWorkflowRegistry()
		backend.RegisterWorkflow(customWf)

		be := newPersistingBackend()
		be.beads["kb-4"] = &backend.Bead{
			ID:        "kb-4",
			ParentID:  "",
			State:     "ready_for_sub",
			ProfileID: "subprocess-edge",
		}

		driver := &scriptedDriver{be: be}
		storeDir := t.TempDir()
		store, _ := workflow.NewAgentStateStore(storeDir)
		_ = store.Save("kb-4", workflow.AgentRuntime{ContextPayload: "write_artifact"})

		worktreeDir := t.TempDir()
		_, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
			TrackerCommand:  "bd",
			StateDir:        t.TempDir(),
			VerifyCommand:   "bin/ci",
			Backend:         be,
			Driver:          driver,
			Config:          newDriveTestConfig(),
			BeadID:          "kb-4",
			RepoPath:        "/tmp/repo",
			Worktree:        worktreeDir,
			AgentStateStore: store,
			MaxStages:       16,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		bd, _ := be.Get("kb-4", "")
		if bd.State != "shipped" {
			t.Errorf("expected final state shipped, got %q", bd.State)
		}

		finalRuntime, _ := store.Load("kb-4")
		if !strings.Contains(finalRuntime.ContextPayload, "epic:kb-4") {
			t.Errorf("expected epic_id to be derived from bead.ID 'kb-4', got %q", finalRuntime.ContextPayload)
		}
	})
}

func TestDriveBead_NilGuardRegression(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-nil"] = &backend.Bead{
		ID:        "kb-nil",
		State:     "planning",
		ProfileID: "autopilot",
	}

	driver := &scriptedDriver{be: be}
	res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand:  "bd",
		StateDir:        t.TempDir(),
		VerifyCommand:   "bin/ci",
		Backend:         be,
		Driver:          driver,
		Config:          newDriveTestConfig(),
		BeadID:          "kb-nil",
		RepoPath:        "/tmp/repo",
		Worktree:        "/tmp/worktree",
		AgentStateStore: nil,
		MaxStages:       16,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Errorf("expected success to be true, got %+v", res)
	}
	bd, _ := be.Get("kb-nil", "")
	if bd.State != "shipped" {
		t.Errorf("expected state to drive to shipped, got %q", bd.State)
	}
}

func TestDriveBead_Subprocess_FailureModes(t *testing.T) {
	backend.ClearWorkflowRegistry()

	customWf := backend.WorkflowDescriptor{
		ID:           "subprocess-failure",
		InitialState: "ready_for_sub",
		States: []string{
			"ready_for_sub", "sub",
			"shipped",
		},
		TerminalStates: []string{"shipped"},
		Transitions: []backend.WorkflowTransition{
			{From: "ready_for_sub", To: "sub"},
			{From: "sub", To: "shipped"},
		},
		QueueStates: []string{
			"ready_for_sub",
		},
		ActionStates: []string{
			"sub",
		},
		QueueActions: map[string]string{
			"ready_for_sub": "sub",
		},
		Stages: map[string]backend.StageContract{
			"sub": {
				Role: "subprocess",
				Kind: "subprocess",
				Subprocess: &backend.SubprocessSpec{
					Command: []string{"temp-script-placeholder"},
				},
			},
		},
	}

	t.Run("Crash traceback on stderr with exit code 1", func(t *testing.T) {
		crashScript := createTestPythonScript(t, `#!/usr/bin/env python3
import sys
sys.stderr.write("Traceback (most recent call last):\n  File \"script.py\", line 5, in <module>\n    raise ValueError(\"boom\")\nValueError: boom\n")
sys.exit(1)
`)
		wf := customWf
		spec := wf.Stages["sub"]
		spec.Subprocess = &backend.SubprocessSpec{
			Command: []string{crashScript},
		}
		wf.Stages["sub"] = spec

		backend.ClearWorkflowRegistry()
		backend.RegisterWorkflow(wf)

		be := newPersistingBackend()
		be.beads["kb-crash"] = &backend.Bead{
			ID:        "kb-crash",
			ParentID:  "parent-epic-id",
			State:     "ready_for_sub",
			ProfileID: "subprocess-failure",
		}

		driver := &scriptedDriver{be: be}
		storeDir := t.TempDir()
		store, _ := workflow.NewAgentStateStore(storeDir)

		res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
			TrackerCommand:  "bd",
			StateDir:        t.TempDir(),
			VerifyCommand:   "bin/ci",
			Backend:         be,
			Driver:          driver,
			Config:          newDriveTestConfig(),
			BeadID:          "kb-crash",
			RepoPath:        "/tmp/repo",
			Worktree:        t.TempDir(),
			AgentStateStore: store,
			MaxStages:       16,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Success {
			t.Fatal("expected Success=false for failed subprocess")
		}

		bd, _ := be.Get("kb-crash", "")
		if bd.State != "blocked" {
			t.Errorf("expected state to be blocked, got %q", bd.State)
		}

		be.mu.Lock()
		comments := be.comments
		be.mu.Unlock()

		if len(comments) == 0 {
			t.Fatal("expected a comment to be recorded")
		}

		commentBody := comments[len(comments)-1].Body
		if !strings.Contains(commentBody, "stage sub failed: non-zero exit") {
			t.Errorf("expected comment to contain failure cause, got %q", commentBody)
		}
		if !strings.Contains(commentBody, "ValueError: boom") {
			t.Errorf("expected comment to contain traceback, got %q", commentBody)
		}

		// Ensure it didn't write a success stage comment (which begins with "stage:")
		for _, c := range comments {
			if strings.HasPrefix(c.Body, "stage:") {
				t.Errorf("unexpected success stage comment found: %s", c.Body)
			}
		}
	})

	t.Run("Timeout failure", func(t *testing.T) {
		sleepScript := createTestPythonScript(t, `#!/usr/bin/env python3
import time
time.sleep(5)
`)
		wf := customWf
		spec := wf.Stages["sub"]
		spec.Subprocess = &backend.SubprocessSpec{
			Command: []string{sleepScript},
			Timeout: "100ms",
		}
		wf.Stages["sub"] = spec

		backend.ClearWorkflowRegistry()
		backend.RegisterWorkflow(wf)

		be := newPersistingBackend()
		be.beads["kb-timeout"] = &backend.Bead{
			ID:        "kb-timeout",
			ParentID:  "parent-epic-id",
			State:     "ready_for_sub",
			ProfileID: "subprocess-failure",
		}

		driver := &scriptedDriver{be: be}
		storeDir := t.TempDir()
		store, _ := workflow.NewAgentStateStore(storeDir)

		res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
			TrackerCommand:  "bd",
			StateDir:        t.TempDir(),
			VerifyCommand:   "bin/ci",
			Backend:         be,
			Driver:          driver,
			Config:          newDriveTestConfig(),
			BeadID:          "kb-timeout",
			RepoPath:        "/tmp/repo",
			Worktree:        t.TempDir(),
			AgentStateStore: store,
			MaxStages:       16,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Success {
			t.Fatal("expected Success=false for timeout subprocess")
		}

		bd, _ := be.Get("kb-timeout", "")
		if bd.State != "blocked" {
			t.Errorf("expected state to be blocked, got %q", bd.State)
		}

		be.mu.Lock()
		comments := be.comments
		be.mu.Unlock()

		if len(comments) == 0 {
			t.Fatal("expected a comment to be recorded")
		}

		commentBody := comments[len(comments)-1].Body
		if !strings.Contains(commentBody, "stage sub failed: timeout") {
			t.Errorf("expected comment to indicate timeout, got %q", commentBody)
		}
	})

	t.Run("Stdout output too large", func(t *testing.T) {
		largeScript := createTestPythonScript(t, `#!/usr/bin/env python3
import sys
sys.stdout.write("A" * 70000)
`)
		wf := customWf
		spec := wf.Stages["sub"]
		spec.Subprocess = &backend.SubprocessSpec{
			Command: []string{largeScript},
		}
		wf.Stages["sub"] = spec

		backend.ClearWorkflowRegistry()
		backend.RegisterWorkflow(wf)

		be := newPersistingBackend()
		be.beads["kb-large"] = &backend.Bead{
			ID:        "kb-large",
			ParentID:  "parent-epic-id",
			State:     "ready_for_sub",
			ProfileID: "subprocess-failure",
		}

		driver := &scriptedDriver{be: be}
		storeDir := t.TempDir()
		store, _ := workflow.NewAgentStateStore(storeDir)

		res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
			TrackerCommand:  "bd",
			StateDir:        t.TempDir(),
			VerifyCommand:   "bin/ci",
			Backend:         be,
			Driver:          driver,
			Config:          newDriveTestConfig(),
			BeadID:          "kb-large",
			RepoPath:        "/tmp/repo",
			Worktree:        t.TempDir(),
			AgentStateStore: store,
			MaxStages:       16,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Success {
			t.Fatal("expected Success=false for large output")
		}

		bd, _ := be.Get("kb-large", "")
		if bd.State != "blocked" {
			t.Errorf("expected state to be blocked, got %q", bd.State)
		}

		be.mu.Lock()
		comments := be.comments
		be.mu.Unlock()

		if len(comments) == 0 {
			t.Fatal("expected a comment to be recorded")
		}

		commentBody := comments[len(comments)-1].Body
		if !strings.Contains(commentBody, "stage sub failed: output too large") {
			t.Errorf("expected comment to explicitly say 'output too large', got %q", commentBody)
		}
	})

	t.Run("Stderr truncation marker in comment", func(t *testing.T) {
		largeStderrScript := createTestPythonScript(t, `#!/usr/bin/env python3
import sys
sys.stderr.write("B" * 70000)
sys.exit(1)
`)
		wf := customWf
		spec := wf.Stages["sub"]
		spec.Subprocess = &backend.SubprocessSpec{
			Command: []string{largeStderrScript},
		}
		wf.Stages["sub"] = spec

		backend.ClearWorkflowRegistry()
		backend.RegisterWorkflow(wf)

		be := newPersistingBackend()
		be.beads["kb-large-stderr"] = &backend.Bead{
			ID:        "kb-large-stderr",
			ParentID:  "parent-epic-id",
			State:     "ready_for_sub",
			ProfileID: "subprocess-failure",
		}

		driver := &scriptedDriver{be: be}
		storeDir := t.TempDir()
		store, _ := workflow.NewAgentStateStore(storeDir)

		_, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
			TrackerCommand:  "bd",
			StateDir:        t.TempDir(),
			VerifyCommand:   "bin/ci",
			Backend:         be,
			Driver:          driver,
			Config:          newDriveTestConfig(),
			BeadID:          "kb-large-stderr",
			RepoPath:        "/tmp/repo",
			Worktree:        t.TempDir(),
			AgentStateStore: store,
			MaxStages:       16,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		be.mu.Lock()
		comments := be.comments
		be.mu.Unlock()

		commentBody := comments[len(comments)-1].Body
		if !strings.Contains(commentBody, "... (truncated)") && !strings.Contains(commentBody, "truncated at 65536 bytes") {
			t.Errorf("expected comment to contain a truncation marker, got %q", commentBody)
		}
	})

	t.Run("Subsequent drive does not silently resume past blocked", func(t *testing.T) {
		be := newPersistingBackend()
		be.beads["kb-blocked-check"] = &backend.Bead{
			ID:        "kb-blocked-check",
			State:     "blocked",
			ProfileID: "subprocess-failure",
		}

		driver := &scriptedDriver{be: be}
		res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
			TrackerCommand: "bd",
			StateDir:       t.TempDir(),
			VerifyCommand:  "bin/ci",
			Backend:        be,
			Driver:         driver,
			Config:         newDriveTestConfig(),
			BeadID:         "kb-blocked-check",
			RepoPath:       "/tmp/repo",
			Worktree:       t.TempDir(),
			MaxStages:      16,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Success {
			t.Fatal("expected Success=false for blocked bead")
		}
		if res.FinalState != "blocked" {
			t.Errorf("expected FinalState=blocked, got %q", res.FinalState)
		}
		if driver.calls != 0 {
			t.Errorf("expected zero agent calls for blocked bead, got %d", driver.calls)
		}
	})
}

// TestResolveArtifactDir_OutsideWorktree proves exit-gate artifacts resolve
// outside the worktree kernl hands to the agent - the fix for PR #40 on
// archeion, where .kernl/<bead>/*.md ended up published in a target
// repository's diff because the old artifact directory was a subdirectory
// of the worktree.
func TestResolveArtifactDir_OutsideWorktree(t *testing.T) {
	worktree := filepath.Join(t.TempDir(), "worktree")
	stateDir := t.TempDir()

	dir, err := resolveArtifactDir(stateDir, "epic-1", "kb-1")
	if err != nil {
		t.Fatalf("resolveArtifactDir: %v", err)
	}
	if strings.HasPrefix(dir, worktree) {
		t.Errorf("artifact dir %q must not be inside the worktree %q", dir, worktree)
	}
	if !strings.HasPrefix(dir, stateDir) {
		t.Errorf("artifact dir %q must be under StateDir %q", dir, stateDir)
	}
	if want := filepath.Join(stateDir, "run", "epic-1", "kb-1"); dir != want {
		t.Errorf("artifact dir = %q, want %q", dir, want)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("resolveArtifactDir must create the directory it returns: %v", err)
	}
}

func TestResolveArtifactDir_FailsLoudWithoutStateDir(t *testing.T) {
	_, err := resolveArtifactDir("", "epic-1", "kb-1")
	if err == nil {
		t.Fatal("expected a loud failure when StateDir is empty")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") || !strings.Contains(err.Error(), "StateDir") {
		t.Errorf("error must name the field that fixes it, got: %v", err)
	}
}

// TestResolveArtifactDir_RefusesPathTraversal proves a bead whose id or
// parent id is not a single safe path segment is refused outright, rather
// than being cleaned by filepath.Join into a directory outside StateDir.
// epicID and beadID are tracker data - in the scenario this project is
// built for, the tracker belongs to a repository kernl does not own - so an
// unvalidated "../../../.." would collapse the artifact directory toward
// filesystem root, and the opencode external_directory rule built from it
// (see writeStageOpencodeConfig) would then grant access to everything.
func TestResolveArtifactDir_RefusesPathTraversal(t *testing.T) {
	stateDir := t.TempDir()

	unsafe := []string{
		"..", ".", "", "../../../..", "a/../../b", "sub/dir", `sub\dir`, "/abs",
	}

	for _, epicID := range unsafe {
		t.Run("epicID="+epicID, func(t *testing.T) {
			dir, err := resolveArtifactDir(stateDir, epicID, "kb-1")
			if err == nil {
				t.Fatalf("expected refusal for unsafe epic id %q, got dir %q", epicID, dir)
			}
			if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
				t.Errorf("error must carry the KERNL DISPATCH FAILURE marker, got: %v", err)
			}
			// The error quotes the id with %q, which itself escapes a
			// literal backslash - compare against the same quoting rather
			// than the raw id so a backslash-containing case isn't a false
			// negative here.
			if !strings.Contains(err.Error(), fmt.Sprintf("%q", epicID)) {
				t.Errorf("error must name the offending id %q, got: %v", epicID, err)
			}
			if dir != "" {
				t.Errorf("a refused id must not return any directory, got %q", dir)
			}
		})
	}

	for _, beadID := range unsafe {
		t.Run("beadID="+beadID, func(t *testing.T) {
			dir, err := resolveArtifactDir(stateDir, "epic-1", beadID)
			if err == nil {
				t.Fatalf("expected refusal for unsafe bead id %q, got dir %q", beadID, dir)
			}
			if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
				t.Errorf("error must carry the KERNL DISPATCH FAILURE marker, got: %v", err)
			}
			if dir != "" {
				t.Errorf("a refused id must not return any directory, got %q", dir)
			}
		})
	}

	// The worst case named in review: a genuinely malicious id pair must
	// never collapse to StateDir itself or above it.
	dir, err := resolveArtifactDir(stateDir, "../../../..", ".")
	if err == nil {
		t.Fatalf("expected refusal for a traversal pair, got dir %q", dir)
	}
}

// TestResolveArtifactDir_RefusalEmitsNoOpencodePermission proves that when
// the artifact directory is refused, DriveBeadToTerminal stops before ever
// dispatching an agent or writing a permission file - the refusal is the
// only observable effect, not a permission rule computed from a bad path.
func TestResolveArtifactDir_RefusalEmitsNoOpencodePermission(t *testing.T) {
	be := newPersistingBackend()
	be.beads["kb-traversal"] = &backend.Bead{
		ID:        "kb-traversal",
		ParentID:  "../../../../etc",
		State:     "planning",
		ProfileID: "autopilot",
	}

	driver := &scriptedDriver{be: be}
	stateDir := t.TempDir()

	_, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand: "bd",
		StateDir:       stateDir,
		VerifyCommand:  "bin/ci",
		Backend:        be,
		Driver:         driver,
		Config:         newDriveTestConfig(),
		BeadID:         "kb-traversal",
		RepoPath:       "/tmp/repo",
		Worktree:       "/tmp/worktree",
		MaxStages:      16,
	})
	if err == nil {
		t.Fatal("expected DriveBeadToTerminal to refuse a bead with an unsafe parent id")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error must carry the KERNL DISPATCH FAILURE marker, got: %v", err)
	}
	if driver.calls != 0 {
		t.Errorf("expected zero agent dispatches for a refused artifact dir, got %d", driver.calls)
	}
	// No opencode-*.json should exist anywhere under StateDir: the
	// permission grant is never computed for a refused directory.
	var foundConfig []string
	_ = filepath.WalkDir(stateDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "opencode-") {
			foundConfig = append(foundConfig, path)
		}
		return nil
	})
	if len(foundConfig) != 0 {
		t.Errorf("expected no opencode permission files to be written, found: %v", foundConfig)
	}
}

func TestEpicIDFor_FallsBackToBeadIDWithNoParent(t *testing.T) {
	standalone := &backend.Bead{ID: "kb-1"}
	if got := epicIDFor(standalone); got != "kb-1" {
		t.Errorf("epicIDFor(no parent) = %q, want kb-1", got)
	}

	child := &backend.Bead{ID: "kb-2", ParentID: "epic-1"}
	if got := epicIDFor(child); got != "epic-1" {
		t.Errorf("epicIDFor(with parent) = %q, want epic-1", got)
	}
}
