package main

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

type epicAbortTestBackend struct {
	beads []backend.Bead
	closeCalls []closeCall
}

type closeCall struct {
	id     string
	reason string
}

func (b *epicAbortTestBackend) ListWorkflows(repoPath string) ([]backend.WorkflowDescriptor, error) { return nil, nil }
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
func (b *epicAbortTestBackend) ListReady(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) { return nil, nil }
func (b *epicAbortTestBackend) Get(id string, repoPath string) (*backend.Bead, error) {
	for i := range b.beads {
		if b.beads[i].ID == id {
			cp := b.beads[i]
			return &cp, nil
		}
	}
	return nil, nil
}
func (b *epicAbortTestBackend) Create(input backend.CreateBeadInput, repoPath string) (*backend.Bead, error) { return nil, nil }
func (b *epicAbortTestBackend) Update(id string, input backend.UpdateBeadInput, repoPath string) error { return nil }
func (b *epicAbortTestBackend) Delete(id string, repoPath string) error { return nil }
func (b *epicAbortTestBackend) Close(id string, reason string, repoPath string) (*backend.TerminalState, error) {
	b.closeCalls = append(b.closeCalls, closeCall{id: id, reason: reason})
	return nil, nil
}
func (b *epicAbortTestBackend) MarkTerminal(id string, targetState string, reason string, repoPath string) error { return nil }
func (b *epicAbortTestBackend) Reopen(id string, reason string, repoPath string) error { return nil }
func (b *epicAbortTestBackend) Rewind(id string, targetState string, reason string, repoPath string) error { return nil }
func (b *epicAbortTestBackend) Search(query string, filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) { return nil, nil }
func (b *epicAbortTestBackend) Query(expression string, options *backend.BeadQueryOptions, repoPath string) ([]backend.Bead, error) { return nil, nil }
func (b *epicAbortTestBackend) AddDependency(blockerID string, blockedID string, repoPath string) error { return nil }
func (b *epicAbortTestBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error { return nil }
func (b *epicAbortTestBackend) ListDependencies(id string, repoPath string, options *backend.DependencyListOptions) ([]backend.BeadDependency, error) { return nil, nil }
func (b *epicAbortTestBackend) BuildTakePrompt(beadID string, options *backend.TakePromptOptions, repoPath string) (*backend.TakePromptResult, error) { return nil, nil }
func (b *epicAbortTestBackend) BuildPollPrompt(options *backend.PollPromptOptions, repoPath string) (*backend.PollPromptResult, error) { return nil, nil }
func (b *epicAbortTestBackend) Comment(id string, body string, repoPath string) error { return nil }
func (b *epicAbortTestBackend) Capabilities() backend.BackendCapabilities { return backend.BackendCapabilities{} }

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
	a := &app.App{
		Backend: be,
		Config:  &config.Config{Registry: config.RegistryConfig{Repos: []config.RepoEntry{{Path: "/test"}}}},
	}

	var outLines []string
	 err := runEpicAbort(a, []string{"e1"}, func(s string) { outLines = append(outLines, s) })
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

	if len(outLines) != 3 {
		t.Fatalf("expected 3 output lines, got %d", len(outLines))
	}
}
