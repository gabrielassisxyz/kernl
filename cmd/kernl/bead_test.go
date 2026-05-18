package main

import (
	"context"
	"io"
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

func (b *testBackend) ListWorkflows(repoPath string) ([]backend.WorkflowDescriptor, error) { return nil, nil }
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
func (b *testBackend) Capabilities() backend.BackendCapabilities { return backend.BackendCapabilities{} }

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
func (p *testProvider) ListSessionIDs() []session.SessionInfo { return nil }
func (p *testProvider) PushEvent(id string, evt session.TerminalEvent) {}

func testSCM() *session.SessionConnectionManager {
	return session.NewSessionConnectionManager(&testProvider{}, nil)
}

func TestRunBeadCmdInvokesDriver(t *testing.T) {
	be := &testBackend{state: map[string]string{"kb-1": "ready_for_implementation"}}
	spawner := &testSpawner{}
	d := app.NewSessionDriver(app.DriverDeps{Backend: be, Spawn: spawner.Spawn, SCM: testSCM()})
	a := &app.App{
		Backend: be,
		Driver:  d,
		Config: &config.Config{
			Settings: config.Settings{
				Agents: map[string]config.AgentConfig{
					"opencode": {Command: "opencode", Args: []string{"run", "--format", "json"}, Label: "opencode"},
				},
				Pools: map[string]config.PoolConfig{
					"implementation": {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
				},
			},
			Registry: config.RegistryConfig{Repos: []config.RepoEntry{{Path: t.TempDir()}}},
		},
	}

	err := runBeadWithApp(a, []string{"run", "kb-1"})
	if err != nil {
		t.Fatalf("bead run did not drive kb-1: err=%v", err)
	}
	if !spawner.spawned {
		t.Fatal("bead run did not spawn the agent process")
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
