package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/epic"
	"github.com/gabrielassisxyz/kernl/internal/session"
	"github.com/gabrielassisxyz/kernl/internal/shipment"
)

type testBackend struct {
	mu    sync.Mutex
	state map[string]string
	// parents, depCount and profiles are optional per-bead overrides for the
	// fields refuseEpicManagedBead inspects. Nil (the common case for a
	// standalone bead) means the zero value: no parent, no dependencies, no
	// explicit profile - reading a nil map in Go returns the zero value
	// rather than panicking, so tests that never set these are unaffected.
	parents  map[string]string
	depCount map[string]int
	profiles map[string]string
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
	bead := &backend.Bead{ID: id, State: s, ParentID: b.parents[id], ProfileID: b.profiles[id]}
	if n := b.depCount[id]; n > 0 {
		bead.Dependencies = make([]backend.BeadDependency, n)
	}
	return bead, nil
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

// withResolvableShipment stubs resolveBeadShipmentDestination to succeed,
// mirroring withoutGit(t) for the worktree manager's git executor: it lets a
// hermetic test reach a full, successful dispatch without shelling out to
// git for a remote a bare t.TempDir() does not have.
func withResolvableShipment(t *testing.T) {
	t.Helper()
	previous := resolveBeadShipmentDestination
	resolveBeadShipmentDestination = func(repoPath string, cfg config.ShipmentConfig) (shipment.Destination, error) {
		return shipment.Destination{RemoteName: "origin", RemoteURL: "git@github.com:example/example.git", RepoSlug: "example/example"}, nil
	}
	t.Cleanup(func() { resolveBeadShipmentDestination = previous })
}

// withUnresolvableShipment stubs resolveBeadShipmentDestination to fail the
// way a repository with no declared shipment remote would, without shelling
// out to git to produce that failure.
func withUnresolvableShipment(t *testing.T) {
	t.Helper()
	previous := resolveBeadShipmentDestination
	resolveBeadShipmentDestination = func(repoPath string, cfg config.ShipmentConfig) (shipment.Destination, error) {
		return shipment.Destination{}, fmt.Errorf("KERNL DISPATCH FAILURE: no shipment destination declared for %s - Fix: set registry.repos[].shipment.remote and .allowedRemotes in kernl.yaml", repoPath)
	}
	t.Cleanup(func() { resolveBeadShipmentDestination = previous })
}

// testAppForBeadRun builds the App bead run needs to reach dispatch in a
// hermetic test: a repo that declares its own tracker and verify command (so
// nothing is auto-detected from a bare temp dir), a worktree root of its own,
// and a StateDir of its own rather than the operator's real ~/.kernl.
// withoutGit(t) is epic_test.go's fixture for the worktree manager's no-git
// mode - bead run refuses a repo path that is not a git repository in
// production, exactly like epic run, so a plain t.TempDir() needs it too.
// withResolvableShipment(t) is the analogous fixture for the up-front
// shipment destination check; a test that wants the real (unstubbed)
// behavior calls withUnresolvableShipment(t) afterward to override it.
func testAppForBeadRun(t *testing.T, be *testBackend) *app.App {
	t.Helper()
	withoutGit(t)
	withResolvableShipment(t)
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
	a.Driver = app.NewSessionDriver(app.DriverDeps{Backend: be, Spawn: spawner.Spawn, SCM: testSCM(), LogDir: t.TempDir()})

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
// back": the fake driver must never be called at all, and
// workflow.NewAgentStateStore - the first write in runBeadDispatch - must
// never run either, so no agentstate directory appears under StateDir.
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
	if _, statErr := os.Stat(filepath.Join(a.StateDir, "agentstate")); !os.IsNotExist(statErr) {
		t.Errorf("dry-run must not create the agentstate directory (workflow.NewAgentStateStore is the first write past the dry-run return), stat err=%v", statErr)
	}
}

// TestRunBeadDispatchDryRunRefusesAnEpicChild proves --dry-run validates
// rather than rubber-stamping: a dry run against a bead a real run would
// refuse must refuse too, not report success for a run that would fail the
// moment --dry-run was dropped.
func TestRunBeadDispatchDryRunRefusesAnEpicChild(t *testing.T) {
	be := &testBackend{
		state:   map[string]string{"kb-12": "ready_for_implementation"},
		parents: map[string]string{"kb-12": "kb-epic"},
	}
	driver := &fakeBeadDriver{}
	a := testAppForBeadRun(t, be)
	repoEntry := a.Config.Registry.Repos[0]

	_, err := runBeadDispatch(a, driver, "kb-12", repoEntry, true)
	if err == nil {
		t.Fatal("expected dry-run to refuse a bead that belongs to an epic, the same as a real run would")
	}
	if len(driver.calls) != 0 {
		t.Fatalf("must refuse before dispatching, got %d driver calls", len(driver.calls))
	}
}

// TestRunBeadDispatchDryRunRefusesUnresolvedShipmentDestination is the
// dry-run counterpart of TestRunBeadDispatchRefusesUnresolvedShipmentDestinationUpFront:
// resolveBeadShipmentDestination is a read (a git remote lookup), so a dry
// run performs it too and must refuse the same way a real run would.
func TestRunBeadDispatchDryRunRefusesUnresolvedShipmentDestination(t *testing.T) {
	be := &testBackend{state: map[string]string{"kb-13": "ready_for_implementation"}}
	driver := &fakeBeadDriver{}
	a := testAppForBeadRun(t, be)
	withUnresolvableShipment(t)
	repoEntry := a.Config.Registry.Repos[0]

	_, err := runBeadDispatch(a, driver, "kb-13", repoEntry, true)
	if err == nil {
		t.Fatal("expected dry-run to refuse an unresolvable shipment destination, the same as a real run would")
	}
	if len(driver.calls) != 0 {
		t.Fatalf("must refuse before dispatching, got %d driver calls", len(driver.calls))
	}
}

// seedExistingWorktree recreates what a previous real bead run leaves behind:
// a worktree at the exact path wm.Add(beadID, beadID, nil) would use, with a
// file in it standing in for committed or uncommitted work. It uses the same
// no-git fallback bead run itself uses in these tests (gitRun nil), so the
// path matches exactly what runBeadDispatch would compute.
func seedExistingWorktree(t *testing.T, a *app.App, beadID string) string {
	t.Helper()
	noopUpdateDesc := func(string, func(string) string) error { return nil }
	wm := epic.NewWorktreeManager(a.Config.Orchestrator.WorktreeRoot, a.Config.Registry.Repos[0].Path, "", nil, noopUpdateDesc)
	path, err := wm.Add(beadID, beadID, nil)
	if err != nil {
		t.Fatalf("seeding an existing worktree for %s: %v", beadID, err)
	}
	marker := filepath.Join(path, "work-from-a-previous-run.txt")
	if err := os.WriteFile(marker, []byte("do not discard me"), 0o644); err != nil {
		t.Fatal(err)
	}
	return marker
}

// TestRunBeadDispatchDryRunNeverTouchesAnExistingWorktree is the regression
// test for finding 2: wm.Add auto-cleans (git worktree remove --force, then
// os.RemoveAll) any path already there before recreating it. Reaching that
// call at all on the --dry-run path would silently discard whatever a
// previous real run left in the worktree, while dispatching no agent and
// writing no tracker state - the single most destructive thing this flag
// could do, under the name people reach for specifically to avoid side
// effects.
//
// The existing-worktree check is a read (os.Stat), so a dry run runs it too -
// the same as a real run would - and must refuse for the same reason: a dry
// run that answered "success" for an input a real run would refuse answers
// nothing. Either way, the worktree itself must come out untouched.
func TestRunBeadDispatchDryRunNeverTouchesAnExistingWorktree(t *testing.T) {
	be := &testBackend{state: map[string]string{"kb-7": "ready_for_implementation"}}
	driver := &fakeBeadDriver{}
	a := testAppForBeadRun(t, be)
	repoEntry := a.Config.Registry.Repos[0]
	marker := seedExistingWorktree(t, a, "kb-7")

	_, err := runBeadDispatch(a, driver, "kb-7", repoEntry, true)
	if err == nil {
		t.Fatal("expected dry-run to refuse an existing worktree, the same as a real run would")
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("dry-run discarded the existing worktree: %v", statErr)
	}
	if len(driver.calls) != 0 {
		t.Fatalf("dry-run must not dispatch an agent, got %d driver calls", len(driver.calls))
	}
}

// TestRunBeadDispatchRefusesWhenAWorktreeAlreadyExists is the regression test
// for finding 4: a non-dry-run re-run of a bead whose worktree already exists
// must refuse rather than let wm.Add force-recreate (and thereby discard) it.
// bead run has no epic-level runstate to tell a safe-to-discard leftover from
// a previous run's committed work, so it does not guess.
func TestRunBeadDispatchRefusesWhenAWorktreeAlreadyExists(t *testing.T) {
	be := &testBackend{state: map[string]string{"kb-8": "ready_for_implementation"}}
	driver := &fakeBeadDriver{}
	a := testAppForBeadRun(t, be)
	repoEntry := a.Config.Registry.Repos[0]
	marker := seedExistingWorktree(t, a, "kb-8")

	_, err := runBeadDispatch(a, driver, "kb-8", repoEntry, false)
	if err == nil {
		t.Fatal("expected a refusal rather than a silent force-recreate of the existing worktree")
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("refusal must leave the existing worktree untouched: %v", statErr)
	}
	if len(driver.calls) != 0 {
		t.Fatalf("must refuse before dispatching, got %d driver calls", len(driver.calls))
	}
}

// TestRunBeadDispatchRefusesUnresolvedShipmentDestinationUpFront is the
// regression test for finding 1: a bead that is not currently AT the
// shipment state can still be driven all the way to it within this same
// call, so the shipment destination must be verified before any dispatch
// starts, not only when the bead happens to already be sitting at shipment.
func TestRunBeadDispatchRefusesUnresolvedShipmentDestinationUpFront(t *testing.T) {
	be := &testBackend{state: map[string]string{"kb-6": "ready_for_implementation"}}
	driver := &fakeBeadDriver{}
	a := testAppForBeadRun(t, be)
	withUnresolvableShipment(t)
	repoEntry := a.Config.Registry.Repos[0]

	_, err := runBeadDispatch(a, driver, "kb-6", repoEntry, false)
	if err == nil {
		t.Fatal("expected a refusal when the shipment destination cannot be resolved, even though the bead has not reached shipment yet")
	}
	if len(driver.calls) != 0 {
		t.Fatalf("must refuse before dispatching any agent, got %d driver calls", len(driver.calls))
	}
	entries, _ := os.ReadDir(a.Config.Orchestrator.WorktreeRoot)
	if len(entries) != 0 {
		t.Errorf("must refuse before creating any worktree, found %v", entries)
	}
}

// TestRunBeadDispatchRefusesAnEpicChild is the regression test for finding 3:
// bead run has no epic branch or dependency graph to drive a child bead
// correctly, so it must refuse rather than silently build a lesser tree.
func TestRunBeadDispatchRefusesAnEpicChild(t *testing.T) {
	be := &testBackend{
		state:   map[string]string{"kb-9": "ready_for_implementation"},
		parents: map[string]string{"kb-9": "kb-epic"},
	}
	driver := &fakeBeadDriver{}
	a := testAppForBeadRun(t, be)
	repoEntry := a.Config.Registry.Repos[0]

	_, err := runBeadDispatch(a, driver, "kb-9", repoEntry, false)
	if err == nil {
		t.Fatal("expected a refusal for a bead that belongs to an epic")
	}
	if !strings.Contains(err.Error(), "kernl epic run") {
		t.Errorf("refusal must name kernl epic run as the fix, got: %v", err)
	}
	if len(driver.calls) != 0 {
		t.Fatalf("must refuse before dispatching, got %d driver calls", len(driver.calls))
	}
}

// TestRunBeadDispatchRefusesABeadWithDependencies covers the second half of
// finding 3: a bead can be dependency-managed without a parent epic label
// having been set on it yet, and bead run has no DAG to merge those
// dependency branches from either way.
func TestRunBeadDispatchRefusesABeadWithDependencies(t *testing.T) {
	be := &testBackend{
		state:    map[string]string{"kb-10": "ready_for_implementation"},
		depCount: map[string]int{"kb-10": 1},
	}
	driver := &fakeBeadDriver{}
	a := testAppForBeadRun(t, be)
	repoEntry := a.Config.Registry.Repos[0]

	_, err := runBeadDispatch(a, driver, "kb-10", repoEntry, false)
	if err == nil {
		t.Fatal("expected a refusal for a bead with declared dependencies")
	}
	if len(driver.calls) != 0 {
		t.Fatalf("must refuse before dispatching, got %d driver calls", len(driver.calls))
	}
}

// TestRunBeadDispatchRefusesAnEpicProfileBead is the regression test for
// finding 5: the epic workflow profile's integration and shipment stages need
// the epic-specific BuildPrompt and verified shipment plan that only epic run
// assembles - the generic stage-contract prompt bead run would otherwise emit
// names neither the child branches integration needs nor the remote shipment
// needs.
func TestRunBeadDispatchRefusesAnEpicProfileBead(t *testing.T) {
	be := &testBackend{
		state:    map[string]string{"kb-11": "ready_for_integration"},
		profiles: map[string]string{"kb-11": "epic"},
	}
	driver := &fakeBeadDriver{}
	a := testAppForBeadRun(t, be)
	repoEntry := a.Config.Registry.Repos[0]

	_, err := runBeadDispatch(a, driver, "kb-11", repoEntry, false)
	if err == nil {
		t.Fatal("expected a refusal for a bead running the epic workflow profile")
	}
	if !strings.Contains(err.Error(), "kernl epic run") {
		t.Errorf("refusal must name kernl epic run as the fix, got: %v", err)
	}
	if len(driver.calls) != 0 {
		t.Fatalf("must refuse before dispatching, got %d driver calls", len(driver.calls))
	}
}

// TestParseBeadRunArgsRejectsExtraPositionals is the regression test for
// finding 6: a second positional argument used to be silently discarded, so
// `kernl bead run kb-1 kb-2` dispatched kb-1 and said nothing about kb-2.
func TestParseBeadRunArgsRejectsExtraPositionals(t *testing.T) {
	_, _, err := parseBeadRunArgs([]string{"kb-1", "kb-2"})
	if err == nil {
		t.Fatal("expected a usage error for a second positional argument")
	}
	if exitCode(err) != 2 {
		t.Errorf("an extra positional is a usage error, got exit %d", exitCode(err))
	}
}

func TestParseBeadRunArgsAcceptsDryRunOnEitherSide(t *testing.T) {
	if id, dry, err := parseBeadRunArgs([]string{"--dry-run", "kb-1"}); err != nil || id != "kb-1" || !dry {
		t.Fatalf("got id=%q dry=%v err=%v", id, dry, err)
	}
	if id, dry, err := parseBeadRunArgs([]string{"kb-1", "--dry-run"}); err != nil || id != "kb-1" || !dry {
		t.Fatalf("got id=%q dry=%v err=%v", id, dry, err)
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
