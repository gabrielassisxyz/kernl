package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

type epicAbortTestBackend struct {
	beads      []backend.Bead
	closeCalls []closeCall
}

type closeCall struct {
	id     string
	reason string
}

func (b *epicAbortTestBackend) ListWorkflows(repoPath string) ([]backend.WorkflowDescriptor, error) {
	return nil, nil
}
func (b *epicAbortTestBackend) List(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
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
func (b *epicAbortTestBackend) ListReady(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *epicAbortTestBackend) Get(id string, repoPath string) (*backend.Bead, error) {
	for i := range b.beads {
		if b.beads[i].ID == id {
			cp := b.beads[i]
			return &cp, nil
		}
	}
	return nil, nil
}
func (b *epicAbortTestBackend) Create(input backend.CreateBeadInput, repoPath string) (*backend.Bead, error) {
	return nil, nil
}
func (b *epicAbortTestBackend) Update(id string, input backend.UpdateBeadInput, repoPath string) error {
	return nil
}
func (b *epicAbortTestBackend) Delete(id string, repoPath string) error { return nil }
func (b *epicAbortTestBackend) Close(id string, reason string, repoPath string) (*backend.TerminalState, error) {
	b.closeCalls = append(b.closeCalls, closeCall{id: id, reason: reason})
	return nil, nil
}
func (b *epicAbortTestBackend) MarkTerminal(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (b *epicAbortTestBackend) Reopen(id string, reason string, repoPath string) error { return nil }
func (b *epicAbortTestBackend) Rewind(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (b *epicAbortTestBackend) Search(query string, filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *epicAbortTestBackend) Query(expression string, options *backend.BeadQueryOptions, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *epicAbortTestBackend) AddDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (b *epicAbortTestBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (b *epicAbortTestBackend) ListDependencies(id string, repoPath string, options *backend.DependencyListOptions) ([]backend.BeadDependency, error) {
	return nil, nil
}
func (b *epicAbortTestBackend) BuildTakePrompt(beadID string, options *backend.TakePromptOptions, repoPath string) (*backend.TakePromptResult, error) {
	return nil, nil
}
func (b *epicAbortTestBackend) BuildPollPrompt(options *backend.PollPromptOptions, repoPath string) (*backend.PollPromptResult, error) {
	return nil, nil
}
func (b *epicAbortTestBackend) Comment(id string, body string, repoPath string) error { return nil }
func (b *epicAbortTestBackend) Capabilities() backend.BackendCapabilities {
	return backend.BackendCapabilities{}
}

func TestEpicAbortRequiresEpicID(t *testing.T) {
	a := &app.App{Config: &config.Config{Registry: config.RegistryConfig{Repos: []config.RepoEntry{{Path: "/test"}}}}}
	err := runEpicAbort(a, nil, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "requires an epic ID") {
		t.Fatalf("want requires-an-epic-id error, got %v", err)
	}
}

func TestEpicAbortRequiresRepo(t *testing.T) {
	a := &app.App{Config: &config.Config{}}
	err := runEpicAbort(a, []string{"e1"}, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "no repos registered") {
		t.Fatalf("want no-repos error, got %v", err)
	}
}

func TestEpicAbortClosesChildrenThenEpic(t *testing.T) {
	be := &epicAbortTestBackend{
		beads: []backend.Bead{
			{ID: "e1", Type: "epic", Title: "demo epic"},
			{ID: "c1", Type: "task", ParentID: "e1"},
			{ID: "c2", Type: "task", ParentID: "e1"},
		},
	}
	repoPath := t.TempDir()
	wtRoot := t.TempDir()
	agentDir := t.TempDir()
	t.Setenv("HOME", agentDir)

	a := &app.App{
		Backend: be,
		Config:  &config.Config{Registry: config.RegistryConfig{Repos: []config.RepoEntry{{Path: repoPath}}}, Orchestrator: config.OrchestratorConfig{WorktreeRoot: wtRoot}},
	}

	var outLines []string
	err := runEpicAbort(a, []string{"e1", "--yes"}, func(s string) { outLines = append(outLines, s) })
	if err != nil {
		t.Fatalf("runEpicAbort: %v", err)
	}

	expected := []closeCall{
		{id: "c1", reason: "aborted"},
		{id: "c2", reason: "aborted"},
		{id: "e1", reason: "aborted"},
	}
	if len(be.closeCalls) != len(expected) {
		t.Fatalf("expected %d Close calls, got %d: %+v", len(expected), len(be.closeCalls), be.closeCalls)
	}
	for i, exp := range expected {
		if be.closeCalls[i] != exp {
			t.Errorf("Close call %d: expected %+v, got %+v", i, exp, be.closeCalls[i])
		}
	}

	if len(outLines) != 5 {
		t.Fatalf("expected 5 output lines, got %d", len(outLines))
	}
}

func TestEpicAbort_CleansWorktreesAndAgentState(t *testing.T) {
	be := &epicAbortTestBackend{
		beads: []backend.Bead{
			{ID: "e1", Type: "epic", Title: "demo epic"},
			{ID: "c1", Type: "task", ParentID: "e1"},
			{ID: "c2", Type: "task", ParentID: "e1"},
		},
	}
	repoPath := t.TempDir()
	wtRoot := t.TempDir()
	agentDir := filepath.Join(t.TempDir(), ".kernl", "agentstate")
	_ = os.MkdirAll(agentDir, 0o755)
	t.Setenv("HOME", filepath.Dir(filepath.Dir(agentDir)))

	// Seed worktree paths.
	_ = os.MkdirAll(filepath.Join(wtRoot, "e1", "c1"), 0o755)
	_ = os.MkdirAll(filepath.Join(wtRoot, "e1", "c2"), 0o755)

	// Seed agent state files.
	store, err := workflow.NewAgentStateStore(agentDir)
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Save("c1", workflow.AgentRuntime{})
	_ = store.Save("c2", workflow.AgentRuntime{})
	_ = store.Save("e1", workflow.AgentRuntime{})

	a := &app.App{
		Backend: be,
		Config: &config.Config{Registry: config.RegistryConfig{Repos: []config.RepoEntry{{Path: repoPath}}},
			Orchestrator: config.OrchestratorConfig{WorktreeRoot: wtRoot}},
	}

	err = runEpicAbort(a, []string{"e1", "--yes"}, func(string) {})
	if err != nil {
		t.Fatalf("runEpicAbort: %v", err)
	}

	if _, serr := os.Stat(filepath.Join(wtRoot, "e1")); !os.IsNotExist(serr) {
		t.Fatalf("expected worktree dir %s to be removed", filepath.Join(wtRoot, "e1"))
	}
	for _, id := range []string{"c1", "c2", "e1"} {
		if _, serr := os.Stat(filepath.Join(agentDir, id+".json")); !os.IsNotExist(serr) {
			t.Fatalf("expected agent state for %s to be purged", id)
		}
	}
}

func TestEpicAbortWithoutYesRefusesAndTeaches(t *testing.T) {
	be := &epicAbortTestBackend{
		beads: []backend.Bead{
			{ID: "e1", Type: "epic", Title: "demo epic"},
			{ID: "c1", Type: "task", ParentID: "e1"},
		},
	}
	a := &app.App{
		Backend: be,
		Config:  &config.Config{Registry: config.RegistryConfig{Repos: []config.RepoEntry{{Path: t.TempDir()}}}},
	}
	err := runEpicAbort(a, []string{"e1"}, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("abort without --yes must refuse and name --yes, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--dry-run") {
		t.Errorf("refusal must offer the safe alternative --dry-run, got: %v", err)
	}
	if len(be.closeCalls) != 0 {
		t.Fatalf("nothing may be closed without --yes, got %+v", be.closeCalls)
	}
	if exitCode(err) != 2 {
		t.Errorf("missing --yes is a usage error, got exit %d", exitCode(err))
	}
}

func TestEpicAbortDryRunPreviewsWithoutClosing(t *testing.T) {
	be := &epicAbortTestBackend{
		beads: []backend.Bead{
			{ID: "e1", Type: "epic", Title: "demo epic"},
			{ID: "c1", Type: "task", ParentID: "e1"},
			{ID: "c2", Type: "task", ParentID: "e1"},
		},
	}
	a := &app.App{
		Backend: be,
		Config:  &config.Config{Registry: config.RegistryConfig{Repos: []config.RepoEntry{{Path: t.TempDir()}}}},
	}
	var lines []string
	if err := runEpicAbort(a, []string{"--dry-run", "e1"}, func(s string) { lines = append(lines, s) }); err != nil {
		t.Fatalf("dry-run must succeed, got: %v", err)
	}
	if len(be.closeCalls) != 0 {
		t.Fatalf("dry-run must not close anything, got %+v", be.closeCalls)
	}
	if len(lines) == 0 || !strings.Contains(lines[0], "dry-run") {
		t.Fatalf("dry-run must print a preview, got %v", lines)
	}
}
