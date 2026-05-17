package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/epic"
	"github.com/gabrielassisxyz/kernl/internal/session"
)

type epicTestBackend struct {
	beads []backend.Bead
}

func (b *epicTestBackend) ListWorkflows(repoPath string) ([]backend.WorkflowDescriptor, error) { return nil, nil }
func (b *epicTestBackend) List(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	var result []backend.Bead
	for _, bead := range b.beads {
		if filters.Type != "" && bead.Type != filters.Type {
			continue
		}
		if filters.Parent != "" && bead.ParentID != filters.Parent {
			continue
		}
		result = append(result, bead)
	}
	return result, nil
}
func (b *epicTestBackend) ListReady(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *epicTestBackend) Get(id string, repoPath string) (*backend.Bead, error) { return nil, nil }
func (b *epicTestBackend) Create(input backend.CreateBeadInput, repoPath string) (*backend.Bead, error) {
	return nil, nil
}
func (b *epicTestBackend) Update(id string, input backend.UpdateBeadInput, repoPath string) error {
	return nil
}
func (b *epicTestBackend) Delete(id string, repoPath string) error { return nil }
func (b *epicTestBackend) Close(id string, reason string, repoPath string) (*backend.TerminalState, error) {
	return nil, nil
}
func (b *epicTestBackend) MarkTerminal(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (b *epicTestBackend) Reopen(id string, reason string, repoPath string) error { return nil }
func (b *epicTestBackend) Rewind(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (b *epicTestBackend) Search(query string, filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *epicTestBackend) Query(expression string, options *backend.BeadQueryOptions, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *epicTestBackend) AddDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (b *epicTestBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (b *epicTestBackend) ListDependencies(id string, repoPath string, options *backend.DependencyListOptions) ([]backend.BeadDependency, error) {
	return nil, nil
}
func (b *epicTestBackend) BuildTakePrompt(beadID string, options *backend.TakePromptOptions, repoPath string) (*backend.TakePromptResult, error) {
	return nil, nil
}
func (b *epicTestBackend) BuildPollPrompt(options *backend.PollPromptOptions, repoPath string) (*backend.PollPromptResult, error) {
	return nil, nil
}
func (b *epicTestBackend) Capabilities() backend.BackendCapabilities { return backend.BackendCapabilities{} }

func captureEpicList(t *testing.T, be backend.BackendPort) string {
	t.Helper()
	var buf bytes.Buffer
	a := &app.App{
		Backend: be,
		Config:  &config.Config{Registry: config.RegistryConfig{Repos: []config.RepoEntry{{Path: "/test"}}}},
	}
	if err := runEpicList(a, &buf); err != nil {
		t.Fatalf("runEpicList: %v", err)
	}
	return buf.String()
}

func TestEpicListShowsEpicsWithChildCounts(t *testing.T) {
	be := &epicTestBackend{beads: []backend.Bead{
		{ID: "kb-0", Type: "epic", Title: "demo epic"},
		{ID: "kb-1", Type: "task", ParentID: "kb-0"},
		{ID: "kb-2", Type: "task", ParentID: "kb-0"},
	}}
	out := captureEpicList(t, be)
	if !strings.Contains(out, "kb-0") || !strings.Contains(out, "demo epic") || !strings.Contains(out, "2") {
		t.Errorf("epic list output missing id/title/child-count: %q", out)
	}
}

type epicRunTestBackend struct {
	beads []backend.Bead
}

func (b *epicRunTestBackend) ListWorkflows(repoPath string) ([]backend.WorkflowDescriptor, error) { return nil, nil }
func (b *epicRunTestBackend) List(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	var result []backend.Bead
	for _, bead := range b.beads {
		if filters.Type != "" && bead.Type != filters.Type {
			continue
		}
		if filters.Parent != "" && bead.ParentID != filters.Parent {
			continue
		}
		result = append(result, bead)
	}
	return result, nil
}
func (b *epicRunTestBackend) ListReady(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *epicRunTestBackend) Get(id string, repoPath string) (*backend.Bead, error) {
	for i := range b.beads {
		if b.beads[i].ID == id {
			cp := b.beads[i]
			return &cp, nil
		}
	}
	return nil, nil
}
func (b *epicRunTestBackend) Create(input backend.CreateBeadInput, repoPath string) (*backend.Bead, error) {
	return nil, nil
}
func (b *epicRunTestBackend) Update(id string, input backend.UpdateBeadInput, repoPath string) error {
	return nil
}
func (b *epicRunTestBackend) Delete(id string, repoPath string) error { return nil }
func (b *epicRunTestBackend) Close(id string, reason string, repoPath string) (*backend.TerminalState, error) {
	return nil, nil
}
func (b *epicRunTestBackend) MarkTerminal(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (b *epicRunTestBackend) Reopen(id string, reason string, repoPath string) error { return nil }
func (b *epicRunTestBackend) Rewind(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (b *epicRunTestBackend) Search(query string, filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *epicRunTestBackend) Query(expression string, options *backend.BeadQueryOptions, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *epicRunTestBackend) AddDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (b *epicRunTestBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (b *epicRunTestBackend) ListDependencies(id string, repoPath string, options *backend.DependencyListOptions) ([]backend.BeadDependency, error) {
	return nil, nil
}
func (b *epicRunTestBackend) BuildTakePrompt(beadID string, options *backend.TakePromptOptions, repoPath string) (*backend.TakePromptResult, error) {
	return nil, nil
}
func (b *epicRunTestBackend) BuildPollPrompt(options *backend.PollPromptOptions, repoPath string) (*backend.PollPromptResult, error) {
	return nil, nil
}
func (b *epicRunTestBackend) Capabilities() backend.BackendCapabilities { return backend.BackendCapabilities{} }

type epicRunTestProcess struct {
	exitErr error
}

func (p *epicRunTestProcess) Wait() error { return p.exitErr }
func (p *epicRunTestProcess) Kill() error { return nil }

type epicRunProvider struct{}

func (p *epicRunProvider) GetSessionEntry(id string) (session.SessionInfo, bool) {
	return session.SessionInfo{}, false
}
func (p *epicRunProvider) ListSessionIDs() []session.SessionInfo { return nil }
func (p *epicRunProvider) PushEvent(id string, evt session.TerminalEvent) {}

func epicRunSuccessSpawn(ctx context.Context, cmd string, args []string, cwd string, env []string) (app.Process, io.Reader, io.Reader, error) {
	return &epicRunTestProcess{}, strings.NewReader(""), strings.NewReader(""), nil
}

func epicRunFailSpawn(ctx context.Context, cmd string, args []string, cwd string, env []string) (app.Process, io.Reader, io.Reader, error) {
	return &epicRunTestProcess{exitErr: context.DeadlineExceeded}, strings.NewReader(""), strings.NewReader(""), nil
}

func testAppWithDiamondEpic(t *testing.T, spawnFn app.SpawnFunc) *app.App {
	t.Helper()
	be := &epicRunTestBackend{
		beads: []backend.Bead{
			{ID: "e", Type: "epic", Title: "test epic"},
			{ID: "a", Type: "task", ParentID: "e"},
			{ID: "b", Type: "task", ParentID: "e", Dependencies: []backend.BeadDependency{{SourceID: "a", TargetID: "b"}}},
			{ID: "c", Type: "task", ParentID: "e", Dependencies: []backend.BeadDependency{{SourceID: "a", TargetID: "c"}}},
			{ID: "d", Type: "task", ParentID: "e", Dependencies: []backend.BeadDependency{{SourceID: "b", TargetID: "d"}, {SourceID: "c", TargetID: "d"}}},
		},
	}
	scm := session.NewSessionConnectionManager(&epicRunProvider{}, nil)
	driver := app.NewSessionDriver(app.DriverDeps{Backend: be, Spawn: spawnFn, SCM: scm})
	pools := map[string]config.PoolConfig{
		"implementation":          {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
		"planning":                {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
		"plan_review":             {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
		"implementation_review":    {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
		"shipment":                {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
		"shipment_review":          {Agents: []config.WeightedAgent{{AgentID: "opencode", Weight: 1}}},
	}

	return &app.App{
		Backend:    be,
		Driver:     driver,
		Config: &config.Config{
			Settings: config.Settings{
				Agents: map[string]config.AgentConfig{
					"opencode": {Command: "opencode", Args: []string{"run", "--format", "json"}, Label: "opencode"},
				},
				Pools: pools,
			},
			Registry:     config.RegistryConfig{Repos: []config.RepoEntry{{Path: t.TempDir()}}},
			Orchestrator: config.OrchestratorConfig{WorktreeRoot: t.TempDir(), MaxConcurrentBeads: 5},
			Server:       config.ServerConfig{Port: 0},
		},
		EpicEvents: epic.NewEpicEventHub(),
	}
}

func TestEpicRunWiresExecutorAndServesGUI(t *testing.T) {
	fakeApp := testAppWithDiamondEpic(t, epicRunSuccessSpawn)
	var guiURLPrinted bool
	err := runEpicWithApp(fakeApp, []string{"run", "e"}, func(line string) {
		if strings.Contains(line, "GUI ") && strings.Contains(line, "http://") {
			guiURLPrinted = true
		}
	})
	if err != nil {
		t.Fatalf("epic run: %v", err)
	}
	if !guiURLPrinted {
		t.Error("epic run must print the embedded GUI URL on startup")
	}
}

func TestEpicRunBlockedPrintsNextStep(t *testing.T) {
	fakeApp := testAppWithDiamondEpic(t, epicRunFailSpawn)
	var out strings.Builder
	err := runEpicWithApp(fakeApp, []string{"run", "e"}, func(l string) { out.WriteString(l + "\n") })
	if err == nil {
		t.Fatal("expected error when bead fails")
	}
	s := out.String()
	if !strings.Contains(s, "blocked") || !strings.Contains(s, "kernl epic run e") {
		t.Errorf("blocked output must name the failed bead and the re-run command: %q", s)
	}
}
