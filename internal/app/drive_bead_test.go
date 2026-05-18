package app

import (
	"context"
	"sync"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
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
		Backend:  be,
		Driver:   driver,
		Config:   newDriveTestConfig(),
		BeadID:   "kb-1",
		RepoPath: "/tmp/repo",
		Worktree: "/tmp/worktree",
		MaxStages: 16,
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
		Backend:  be,
		Driver:   driver,
		Config:   newDriveTestConfig(),
		BeadID:   "kb-1",
		RepoPath: "/tmp/repo",
		Worktree: "/tmp/worktree",
		MaxStages: 16,
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
		Backend:  be,
		Driver:   driver,
		Config:   newDriveTestConfig(),
		BeadID:   "kb-1",
		RepoPath: "/tmp/repo",
		Worktree: "/tmp/worktree",
		MaxStages: 16,
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
		Backend:  be,
		Driver:   driver,
		Config:   newDriveTestConfig(),
		BeadID:   "kb-1",
		RepoPath: "/tmp/repo",
		Worktree: "/tmp/worktree",
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
		Backend:  be,
		Driver:   driver,
		Config:   newDriveTestConfig(),
		BeadID:   "kb-1",
		RepoPath: "/tmp/repo",
		Worktree: "/tmp/worktree",
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
		Backend:  be,
		Driver:   driver,
		Config:   newDriveTestConfig(),
		BeadID:   "kb-1",
		RepoPath: "/tmp/repo",
		Worktree: "/tmp/worktree",
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
		Backend:  be,
		Driver:   driver,
		Config:   newDriveTestConfig(),
		BeadID:   "kb-1",
		RepoPath: "/tmp/repo",
		Worktree: "/tmp/worktree",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Error("expected Success=false for blocked bead")
	}
}
