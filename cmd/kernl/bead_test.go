package main

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/session"
)

type testBackend struct {
	mu    sync.Mutex
	state map[string]string
}

func (b *testBackend) ListWorkflows(repoPath string) ([]backend.WorkflowDescriptor, error) {
	return nil, nil
}
func (b *testBackend) List(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *testBackend) ListReady(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *testBackend) Get(id string, repoPath string) (*backend.Bead, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.state[id]
	if !ok {
		return nil, nil
	}
	return &backend.Bead{ID: id, State: s}, nil
}
func (b *testBackend) Create(input backend.CreateBeadInput, repoPath string) (*backend.Bead, error) {
	return nil, nil
}
func (b *testBackend) Update(id string, input backend.UpdateBeadInput, repoPath string) error {
	if input.State == "" {
		return nil
	}
	// Mutating on State is what lets DriveBeadToTerminal's claim-then-dispatch
	// loop actually advance in a hermetic test - a no-op Update left the bead
	// reporting the same state forever, which the loop keeps re-claiming until
	// it exceeds its max-stages guard.
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state[id] = input.State
	return nil
}
func (b *testBackend) Delete(id string, repoPath string) error { return nil }
func (b *testBackend) Close(id string, reason string, repoPath string) (*backend.TerminalState, error) {
	return nil, nil
}
func (b *testBackend) MarkTerminal(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (b *testBackend) Reopen(id string, reason string, repoPath string) error { return nil }
func (b *testBackend) Rewind(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (b *testBackend) Search(query string, filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *testBackend) Query(expression string, options *backend.BeadQueryOptions, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *testBackend) AddDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (b *testBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (b *testBackend) ListDependencies(id string, repoPath string, options *backend.DependencyListOptions) ([]backend.BeadDependency, error) {
	return nil, nil
}
func (b *testBackend) BuildTakePrompt(beadID string, options *backend.TakePromptOptions, repoPath string) (*backend.TakePromptResult, error) {
	return nil, nil
}
func (b *testBackend) BuildPollPrompt(options *backend.PollPromptOptions, repoPath string) (*backend.PollPromptResult, error) {
	return nil, nil
}
func (b *testBackend) Comment(id string, body string, repoPath string) error { return nil }
func (b *testBackend) Capabilities() backend.BackendCapabilities {
	return backend.BackendCapabilities{}
}

type testProcess struct {
	exitErr error
}

func (p *testProcess) Wait() error { return p.exitErr }
func (p *testProcess) Kill() error { return nil }

type testSpawner struct {
	spawned bool
}

func (s *testSpawner) Spawn(ctx context.Context, cmd string, args []string, cwd string, env []string) (app.Process, io.Reader, io.Reader, error) {
	s.spawned = true
	return &testProcess{}, strings.NewReader(""), strings.NewReader(""), nil
}

type testProvider struct{}

func (p *testProvider) GetSessionEntry(id string) (session.SessionInfo, bool) {
	return session.SessionInfo{}, false
}
func (p *testProvider) ListSessionIDs() []session.SessionInfo          { return nil }
func (p *testProvider) PushEvent(id string, evt session.TerminalEvent) {}

func testSCM() *session.SessionConnectionManager {
	return session.NewSessionConnectionManager(&testProvider{}, nil)
}

// fakeBeadDriver stands in for a.Driver in tests that only need to observe
// what bead run assembles before dispatch - the prompt, the worktree, the
// argv - without spawning a real agent process. app.RunBeadInput carries no
// exit code or diagnostics of its own to fake; recording every call is
// enough for the assertions that need it.
type fakeBeadDriver struct {
	calls []app.RunBeadInput
}

func (f *fakeBeadDriver) RunBead(ctx context.Context, input app.RunBeadInput) (app.RunBeadResult, error) {
	f.calls = append(f.calls, input)
	return app.RunBeadResult{FinalState: "ready_for_review", Success: true}, nil
}

// testAppForBeadRun builds the App bead run needs to reach dispatch in a
// hermetic test: a repo that declares its own tracker and verify command (so
// nothing is auto-detected from a bare temp dir), a worktree root of its own,
// and a StateDir of its own rather than the operator's real ~/.kernl.
// withoutGit(t) is epic_test.go's fixture for the worktree manager's no-git
// mode - bead run refuses a repo path that is not a git repository in
// production, exactly like epic run, so a plain t.TempDir() needs it too.
func testAppForBeadRun(t *testing.T, be *testBackend) *app.App {
	t.Helper()
	withoutGit(t)
	return &app.App{
		Backend:  be,
		StateDir: t.TempDir(),
		Config: &config.Config{
			Settings: config.Settings{
				Agents: map[string]config.AgentConfig{
					"opencode": {Command: "opencode", Args: []string{"run", "--format", "json"}, Label: "opencode"},
				},
				// The full autopilot pool set: with a fake driver that always
				// succeeds, DriveBeadToTerminal advances the bead through every
				// agent-claimable stage of the default workflow, not just the
				// first one.
				Pools: map[string]config.PoolConfig{
					"implementation":        {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
					"planning":              {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
					"plan_review":           {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
					"implementation_review": {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
					"integration":           {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
					"integration_review":    {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
					"shipment":              {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
					"shipment_review":       {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
				},
			},
			// verifyCommand/memoryManager rather than detection from disk: this
			// is a bare t.TempDir(), not a repository.
			Registry:     config.RegistryConfig{Repos: []config.RepoEntry{{Path: t.TempDir(), VerifyCommand: "true", MemoryManager: "beads"}}},
			Orchestrator: config.OrchestratorConfig{WorktreeRoot: t.TempDir()},
		},
	}
}

func TestRunBeadCmdInvokesDriver(t *testing.T) {
	be := &testBackend{state: map[string]string{"kb-1": "ready_for_implementation"}}
	spawner := &testSpawner{}
	a := testAppForBeadRun(t, be)
	a.Driver = app.NewSessionDriver(app.DriverDeps{Backend: be, Spawn: spawner.Spawn, SCM: testSCM()})

	err := runBeadWithApp(a, []string{"run", "kb-1"})
	if err != nil {
		t.Fatalf("bead run did not drive kb-1: err=%v", err)
	}
	if !spawner.spawned {
		t.Fatal("bead run did not spawn the agent process")
	}
}

// TestRunBeadDispatchAssemblesPromptWorktreeAndArgv is the acceptance test for
// routing bead run through app.DriveBeadToTerminal instead of calling
// a.Driver.RunBead directly: the old code spawned the agent with no prompt,
// no isolated worktree and none of the per-dialect flags DriveBeadToTerminal
// builds. A named fake driver captures exactly what reaches it, so the
// assertions hold regardless of what a real agent process would do with it.
func TestRunBeadDispatchAssemblesPromptWorktreeAndArgv(t *testing.T) {
	be := &testBackend{state: map[string]string{"kb-2": "ready_for_implementation"}}
	driver := &fakeBeadDriver{}
	a := testAppForBeadRun(t, be)
	repoEntry := a.Config.Registry.Repos[0]

	res, err := runBeadDispatch(a, driver, "kb-2", repoEntry, false)
	if err != nil {
		t.Fatalf("runBeadDispatch: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}
	// The fake driver always succeeds, so a full autopilot run advances the
	// bead through every remaining stage in one call - the assertions below
	// are about the FIRST dispatch, the one for the bead's own starting state.
	if len(driver.calls) == 0 {
		t.Fatal("expected the driver to be invoked at least once")
	}
	got := driver.calls[0]

	if len(got.Args) == 0 || strings.TrimSpace(got.Args[len(got.Args)-1]) == "" {
		t.Fatalf("expected a non-empty stage prompt as the trailing positional, got args=%v", got.Args)
	}

	if got.Cwd == "" {
		t.Fatal("expected the dispatched agent to run in an isolated worktree, got empty Cwd")
	}
	if info, err := os.Stat(got.Cwd); err != nil || !info.IsDir() {
		t.Fatalf("expected the worktree at %s to exist as a directory: %v", got.Cwd, err)
	}

	dirFlagAt := -1
	for i, arg := range got.Args {
		if arg == "--dir" {
			dirFlagAt = i
		}
	}
	if dirFlagAt == -1 || dirFlagAt+1 >= len(got.Args) || got.Args[dirFlagAt+1] != got.Cwd {
		t.Errorf("expected opencode's --dir <worktree> among the per-dialect flags, got args=%v", got.Args)
	}
}

// TestRunBeadDispatchDryRunStopsBeforeDispatch proves --dry-run means the run
// stops before entering the stage, not "spawn the agent and ask it to hold
// back": the fake driver must never be called at all.
func TestRunBeadDispatchDryRunStopsBeforeDispatch(t *testing.T) {
	be := &testBackend{state: map[string]string{"kb-3": "ready_for_implementation"}}
	driver := &fakeBeadDriver{}
	a := testAppForBeadRun(t, be)
	repoEntry := a.Config.Registry.Repos[0]

	res, err := runBeadDispatch(a, driver, "kb-3", repoEntry, true)
	if err != nil {
		t.Fatalf("dry-run must not error: %v", err)
	}
	if len(driver.calls) != 0 {
		t.Fatalf("dry-run must not dispatch an agent, got %d driver calls", len(driver.calls))
	}
	if res.FinalState != "ready_for_implementation" {
		t.Errorf("dry-run must stop at the bead's own current state, got %q", res.FinalState)
	}
}

// TestRefuseUnverifiedShipmentBlocksShipmentStage covers the second door this
// verb opens into the shipment stage: bead run must refuse a bead about to
// dispatch shipment work in a repository that never declared where it may
// publish, the same as epic run does before dispatching anything.
func TestRefuseUnverifiedShipmentBlocksShipmentStage(t *testing.T) {
	be := &testBackend{state: map[string]string{"kb-4": "shipment"}}
	a := &app.App{Backend: be}
	repoEntry := config.RepoEntry{Path: t.TempDir()}

	if err := refuseUnverifiedShipment(a, "kb-4", repoEntry); err == nil {
		t.Fatal("expected a refusal for a repo with no declared shipment destination")
	}
}

func TestRefuseUnverifiedShipmentAllowsNonShipmentStage(t *testing.T) {
	be := &testBackend{state: map[string]string{"kb-5": "ready_for_implementation"}}
	a := &app.App{Backend: be}
	repoEntry := config.RepoEntry{Path: t.TempDir()}

	if err := refuseUnverifiedShipment(a, "kb-5", repoEntry); err != nil {
		t.Fatalf("a non-shipment stage must not be refused: %v", err)
	}
}

func TestRunBeadMissingSubcommand(t *testing.T) {
	a := &app.App{}
	err := runBeadWithApp(a, []string{})
	if err == nil {
		t.Fatal("expected error for missing subcommand")
	}
}

func TestRunBeadMissingID(t *testing.T) {
	a := &app.App{}
	err := runBeadWithApp(a, []string{"run"})
	if err == nil {
		t.Fatal("expected error for missing bead ID")
	}
}
