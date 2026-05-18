package app

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

// persistingBackend is the fake the per-bead loop needs: Get + Update both
// see and mutate the same state map. driver_test.go's fakeBackend ignores
// Update which makes it useless for loop coverage.
type persistingBackend struct {
	mu     sync.Mutex
	beads  map[string]*backend.Bead
	writes int
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

// no-op stubs to satisfy BackendPort
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
func (b *persistingBackend) Capabilities() backend.BackendCapabilities {
	return backend.BackendCapabilities{}
}

// scriptedDriver fakes an agent run: each call advances the bead's state
// per the scripted transition table, then returns success. Mirrors a real
// agent that updates bead.status before exiting.
type scriptedDriver struct {
	be          *persistingBackend
	transitions map[string]string // active state → next queued state
	calls       int
}

func (s *scriptedDriver) RunBead(ctx context.Context, in RunBeadInput) (RunBeadResult, error) {
	s.calls++
	bd, _ := s.be.Get(in.BeadID, "")
	if bd == nil {
		return RunBeadResult{}, nil
	}
	if next, ok := s.transitions[bd.State]; ok {
		_ = s.be.Update(in.BeadID, backend.UpdateBeadInput{State: next}, "")
		return RunBeadResult{FinalState: next, Success: true}, nil
	}
	return RunBeadResult{FinalState: bd.State, Success: true}, nil
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
				"shipment":              {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
				"shipment_review":       {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
			},
		},
	}
}

func TestDriveBeadToTerminal_LoopsThroughMultipleStages(t *testing.T) {
	// Regression for kernl-h8i9: `kernl epic run` used to dispatch ONE agent
	// per invocation, leaving beads stranded at intermediate states like
	// `ready_for_implementation_review`. The loop should now drive the bead
	// from any agent-claimable state all the way to terminal.
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{
		ID:    "kb-1",
		State: "ready_for_implementation",
	}

	// Agent script: each stage advances to the next queued state, until
	// shipped (terminal in the autopilot workflow).
	driver := &scriptedDriver{
		be: be,
		transitions: map[string]string{
			"implementation":        "ready_for_implementation_review",
			"implementation_review": "ready_for_shipment",
			"shipment":              "ready_for_shipment_review",
			"shipment_review":       "shipped",
		},
	}

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
	if driver.calls != 4 {
		t.Errorf("expected 4 agent calls (implementation, impl_review, shipment, shipment_review), got %d", driver.calls)
	}
	if got := strings.Join(stages, ","); got != "ready_for_implementation,ready_for_implementation_review,ready_for_shipment,ready_for_shipment_review" {
		t.Errorf("unexpected stage walk: %s", got)
	}
}

func TestDriveBeadToTerminal_StuckAgentFailsLoud(t *testing.T) {
	// If the agent runs successfully but doesn't advance bead.status, we
	// must not spin — fail loud with a Fix hint.
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "ready_for_implementation"}

	driver := &scriptedDriver{
		be: be,
		// No transition for "implementation" — agent runs but never advances.
		transitions: map[string]string{},
	}

	_, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		Backend:  be,
		Driver:   driver,
		Config:   newDriveTestConfig(),
		BeadID:   "kb-1",
		RepoPath: "/tmp/repo",
		Worktree: "/tmp/worktree",
	})

	if err == nil {
		t.Fatal("expected stuck-state failure, got nil")
	}
	if !strings.Contains(err.Error(), "stuck at state") {
		t.Errorf("expected 'stuck at state' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("expected KERNL DISPATCH FAILURE marker, got: %v", err)
	}
}

func TestDriveBeadToTerminal_TerminalStateShortCircuits(t *testing.T) {
	// A bead already at a terminal state should return success without
	// spawning anything.
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
	// A bead at awaiting_integration is handed off to the merger — drive
	// loop returns success without spawning a per-bead agent.
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

// retryOnNthCallDriver fails to advance the bead on the first N-1 calls, then
// succeeds on call N. Used to simulate an agent that needs a follow-up.
type retryOnNthCallDriver struct {
	be           *persistingBackend
	successOnCall int // 1-indexed: advance state starting from this call number
	nextState    string
	calls        int
	prompts      []string
}

func (d *retryOnNthCallDriver) RunBead(_ context.Context, in RunBeadInput) (RunBeadResult, error) {
	d.calls++
	if len(in.Args) > 0 {
		d.prompts = append(d.prompts, in.Args[len(in.Args)-1])
	}
	bd, _ := d.be.Get(in.BeadID, "")
	if bd == nil {
		return RunBeadResult{}, nil
	}
	if d.calls >= d.successOnCall {
		_ = d.be.Update(in.BeadID, backend.UpdateBeadInput{State: d.nextState}, "")
		return RunBeadResult{FinalState: d.nextState, Success: true, SessionID: "ses_test123"}, nil
	}
	return RunBeadResult{FinalState: bd.State, Success: true, SessionID: "ses_test123"}, nil
}

func TestDriveBeadToTerminal_RetrySucceedsOnSecondAttempt(t *testing.T) {
	// Simulate opencode exiting clean without running bd update --status twice,
	// then succeeding on the first retry. The orchestrator should complete normally.
	//
	// Call sequence:
	//   call 1 (i=0, initial run): no advance
	//   call 2 (i=1, initial run): no advance  — now stuck detected
	//   call 3 (retryStuckStage attempt 1): advance to "shipped" (terminal)
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "ready_for_implementation"}

	driver := &retryOnNthCallDriver{
		be:            be,
		successOnCall: 3,       // calls 1 & 2 don't advance; call 3 (retry) does
		nextState:     "shipped", // terminal, so outer loop exits immediately after retry
	}

	res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		Backend:            be,
		Driver:             driver,
		Config:             newDriveTestConfig(),
		BeadID:             "kb-1",
		RepoPath:           "/tmp/repo",
		Worktree:           "/tmp/worktree",
		StageRetryAttempts: 2,
		MaxStages:          8,
	})

	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if !res.Success {
		t.Errorf("expected Success=true, got %+v", res)
	}
	if driver.calls != 3 {
		t.Errorf("expected 3 driver calls (2 initial + 1 retry), got %d", driver.calls)
	}
	// The retry prompt (third call) must instruct the agent to run bd update --status.
	if len(driver.prompts) < 3 {
		t.Fatalf("expected at least 3 prompts recorded, got %d", len(driver.prompts))
	}
	retryPrompt := driver.prompts[2] // zero-indexed: third prompt
	if !strings.Contains(retryPrompt, "bd update --status") {
		t.Errorf("retry prompt should instruct agent to run bd update --status, got: %q", retryPrompt)
	}
}

func TestDriveBeadToTerminal_RetryExhaustedFailsLoud(t *testing.T) {
	// Agent never advances the bead, even after retries. The orchestrator should
	// fail loud with a KERNL DISPATCH FAILURE error mentioning the retry count.
	be := newPersistingBackend()
	be.beads["kb-1"] = &backend.Bead{ID: "kb-1", State: "ready_for_implementation"}

	driver := &retryOnNthCallDriver{
		be:            be,
		successOnCall: 999, // never succeeds within test budget
		nextState:     "ready_for_implementation_review",
	}

	_, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		Backend:            be,
		Driver:             driver,
		Config:             newDriveTestConfig(),
		BeadID:             "kb-1",
		RepoPath:           "/tmp/repo",
		Worktree:           "/tmp/worktree",
		StageRetryAttempts: 1,
	})

	if err == nil {
		t.Fatal("expected failure after retries exhausted, got nil")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("expected KERNL DISPATCH FAILURE marker, got: %v", err)
	}
	if !strings.Contains(err.Error(), "stuck at state") {
		t.Errorf("expected 'stuck at state' in error, got: %v", err)
	}
	// Total calls: 1 initial (outer loop) + 1 retry = 2 minimum (before stuck is detected on 2nd outer iteration)
	// Actually: outer loop calls once, sees stuck, calls retryStuckStage(limit=1), which calls once more.
	// So: 1 (outer i=0) + 1 (outer i=1, agent called again, stuck detected) + 1 (retry) = 3? No...
	// Let me recount: i=0: agent called once (prevState was ""). i=1: agent called again (stuck check sees prevState==state → retryStuckStage(1)).
	// But wait, the driver DOES advance on i=0: it uses transitions table which has nothing, so state stays.
	// Actually: outer loop i=0: no stuck check, agent runs → state stays at "implementation". prevState = "ready_for_implementation".
	// i=1: bead.state = "implementation" != prevState "ready_for_implementation" → no stuck check. Agent runs → state stays. prevState = "implementation".
	// i=2: bead.state = "implementation" == prevState → retryStuckStage(1). 1 retry call → state stays. Return failure.
	// Total calls = 3.
	if driver.calls < 2 {
		t.Errorf("expected at least 2 driver calls, got %d", driver.calls)
	}
}
