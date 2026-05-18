package app

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/session"
)

type fakeBackend struct {
	mu    sync.Mutex
	state map[string]string
}

func (b *fakeBackend) ListWorkflows(repoPath string) ([]backend.WorkflowDescriptor, error) { return nil, nil }
func (b *fakeBackend) List(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *fakeBackend) ListReady(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *fakeBackend) Get(id string, repoPath string) (*backend.Bead, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.state[id]
	if !ok {
		return nil, nil
	}
	return &backend.Bead{ID: id, State: s}, nil
}
func (b *fakeBackend) Create(input backend.CreateBeadInput, repoPath string) (*backend.Bead, error) {
	return nil, nil
}
func (b *fakeBackend) Update(id string, input backend.UpdateBeadInput, repoPath string) error {
	return nil
}
func (b *fakeBackend) Delete(id string, repoPath string) error { return nil }
func (b *fakeBackend) Close(id string, reason string, repoPath string) (*backend.TerminalState, error) {
	return nil, nil
}
func (b *fakeBackend) MarkTerminal(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (b *fakeBackend) Reopen(id string, reason string, repoPath string) error { return nil }
func (b *fakeBackend) Rewind(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (b *fakeBackend) Search(query string, filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *fakeBackend) Query(expression string, options *backend.BeadQueryOptions, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *fakeBackend) AddDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (b *fakeBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (b *fakeBackend) ListDependencies(id string, repoPath string, options *backend.DependencyListOptions) ([]backend.BeadDependency, error) {
	return nil, nil
}
func (b *fakeBackend) BuildTakePrompt(beadID string, options *backend.TakePromptOptions, repoPath string) (*backend.TakePromptResult, error) {
	return nil, nil
}
func (b *fakeBackend) BuildPollPrompt(options *backend.PollPromptOptions, repoPath string) (*backend.PollPromptResult, error) {
	return nil, nil
}
func (b *fakeBackend) Capabilities() backend.BackendCapabilities { return backend.BackendCapabilities{} }

type fakeProcess struct {
	exitErr   error
	onExit    func()
	exitOnce  sync.Once
}

func (p *fakeProcess) Wait() error {
	p.exitOnce.Do(func() {
		if p.onExit != nil {
			p.onExit()
		}
	})
	return p.exitErr
}

func (p *fakeProcess) Kill() error { return nil }

type fakeSpawner struct {
	script  string
	onExit  func()
	spawned bool
}

func (s *fakeSpawner) Spawn(ctx context.Context, cmd string, args []string, cwd string, env []string) (Process, io.Reader, io.Reader, error) {
	s.spawned = true
	return &fakeProcess{onExit: s.onExit}, strings.NewReader(s.script), strings.NewReader(""), nil
}

type stubProvider struct{}

func (p *stubProvider) GetSessionEntry(id string) (session.SessionInfo, bool) {
	return session.SessionInfo{}, false
}
func (p *stubProvider) ListSessionIDs() []session.SessionInfo { return nil }
func (p *stubProvider) PushEvent(id string, evt session.TerminalEvent) {}

func newTestSCM() *session.SessionConnectionManager {
	return session.NewSessionConnectionManager(&stubProvider{}, nil)
}

func TestDriverRunBeadAdvancesViaTakeLoop(t *testing.T) {
	be := &fakeBackend{state: map[string]string{"kb-1": "ready_for_implementation"}}
	spawn := &fakeSpawner{
		script: "{\"type\":\"output\",\"content\":\"ok\"}\n{\"type\":\"session_idle\"}\n",
		onExit: func() { be.state["kb-1"] = "done" },
	}
	d := NewSessionDriver(DriverDeps{Backend: be, Spawn: spawn.Spawn, SCM: newTestSCM()})
	res, err := d.RunBead(context.Background(), RunBeadInput{BeadID: "kb-1", RepoPath: t.TempDir(), Command: "opencode", AgentName: "opencode"})
	if err != nil {
		t.Fatalf("RunBead: %v", err)
	}
	if res.FinalState != "done" {
		t.Errorf("FinalState = %q, want done", res.FinalState)
	}
	if !spawn.spawned {
		t.Error("driver must spawn the agent process")
	}
}
