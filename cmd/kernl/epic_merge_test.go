package main

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

type mergeTestBackend struct {
	beads   map[string]backend.Bead
	updates []struct {
		ID    string
		Input backend.UpdateBeadInput
	}
}

func newMergeTestBackend() *mergeTestBackend {
	return &mergeTestBackend{beads: map[string]backend.Bead{}}
}

func (b *mergeTestBackend) add(bead backend.Bead) {
	b.beads[bead.ID] = bead
}

func (b *mergeTestBackend) ListWorkflows(repoPath string) ([]backend.WorkflowDescriptor, error) {
	return nil, nil
}
func (b *mergeTestBackend) List(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
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
func (b *mergeTestBackend) ListReady(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *mergeTestBackend) Get(id string, repoPath string) (*backend.Bead, error) {
	if bd, ok := b.beads[id]; ok {
		cp := bd
		return &cp, nil
	}
	return nil, nil
}
func (b *mergeTestBackend) Create(input backend.CreateBeadInput, repoPath string) (*backend.Bead, error) {
	return nil, nil
}
func (b *mergeTestBackend) Update(id string, input backend.UpdateBeadInput, repoPath string) error {
	b.updates = append(b.updates, struct {
		ID    string
		Input backend.UpdateBeadInput
	}{ID: id, Input: input})

	bd := b.beads[id]
	if input.State != "" {
		bd.State = input.State
	}
	if input.Description != "" {
		bd.Description = input.Description
	}
	b.beads[id] = bd
	return nil
}
func (b *mergeTestBackend) Delete(id string, repoPath string) error                { return nil }
func (b *mergeTestBackend) Close(id string, reason string, repoPath string) (*backend.TerminalState, error) {
	return nil, nil
}
func (b *mergeTestBackend) MarkTerminal(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (b *mergeTestBackend) Reopen(id string, reason string, repoPath string) error { return nil }
func (b *mergeTestBackend) Rewind(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (b *mergeTestBackend) Search(query string, filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *mergeTestBackend) Query(expression string, options *backend.BeadQueryOptions, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *mergeTestBackend) AddDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (b *mergeTestBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (b *mergeTestBackend) ListDependencies(id string, repoPath string, options *backend.DependencyListOptions) ([]backend.BeadDependency, error) {
	return nil, nil
}
func (b *mergeTestBackend) BuildTakePrompt(beadID string, options *backend.TakePromptOptions, repoPath string) (*backend.TakePromptResult, error) {
	return nil, nil
}
func (b *mergeTestBackend) BuildPollPrompt(options *backend.PollPromptOptions, repoPath string) (*backend.PollPromptResult, error) {
	return nil, nil
}
func (b *mergeTestBackend) Comment(id string, body string, repoPath string) error { return nil }
func (b *mergeTestBackend) Capabilities() backend.BackendCapabilities {
	return backend.BackendCapabilities{}
}

type mergeTestMM struct {
	dispatched []string
}

func (m *mergeTestMM) TryTrigger(epicID string) error      { return nil }
func (m *mergeTestMM) RouteOutcome(epicID string) error     { return nil }
func (m *mergeTestMM) DispatchMerger(epicID string) error {
	m.dispatched = append(m.dispatched, epicID)
	return nil
}

func mergeTestApp(t *testing.T, be *mergeTestBackend, mm *mergeTestMM) *app.App {
	t.Helper()
	return &app.App{
		Backend:      be,
		Config:       &config.Config{Registry: config.RegistryConfig{Repos: []config.RepoEntry{{Path: "/test"}}}},
		MergeManager: mm,
	}
}

func TestEpicMergeNotBlockedError(t *testing.T) {
	be := newMergeTestBackend()
	be.add(backend.Bead{ID: "e1", Type: "epic", State: "open"})
	mm := &mergeTestMM{}
	a := mergeTestApp(t, be, mm)

	err := runEpicMerge(a, []string{"e1"}, func(string) {})
	if err == nil {
		t.Fatal("expected error when epic is not blocked")
	}
	if !strings.Contains(err.Error(), "expected blocked") {
		t.Errorf("error must mention expected blocked, got: %v", err)
	}
}

func TestEpicMergeNoRecoverySignalError(t *testing.T) {
	be := newMergeTestBackend()
	be.add(backend.Bead{ID: "e1", Type: "epic", State: "blocked", Description: "no fields here"})
	mm := &mergeTestMM{}
	a := mergeTestApp(t, be, mm)

	err := runEpicMerge(a, []string{"e1"}, func(string) {})
	if err == nil {
		t.Fatal("expected error when no recovery signal in description")
	}
	if !strings.Contains(err.Error(), "no recovery signal") {
		t.Errorf("error must mention recovery signal, got: %v", err)
	}
}

func TestEpicMergeBlockedWithMergeConflictAtNoRecoverableOutcome_NoError(t *testing.T) {
	be := newMergeTestBackend()
	be.add(backend.Bead{ID: "e1", Type: "epic", State: "blocked",
		Description: "epic_branch: feat/e1\nmerge_conflict_at: src/main.go\n"})
	mm := &mergeTestMM{}
	a := mergeTestApp(t, be, mm)

	err := runEpicMerge(a, []string{"e1"}, func(string) {})
	if err != nil {
		t.Fatalf("expected no error when merge_conflict_at present: %v", err)
	}
}

func TestEpicMergeBlockedWithMergeOutcomeMergeConflictOnly_NoRecoverySignalError(t *testing.T) {
	be := newMergeTestBackend()
	be.add(backend.Bead{ID: "e1", Type: "epic", State: "blocked",
		Description: "merge_outcome: merge_conflict\n"})
	mm := &mergeTestMM{}
	a := mergeTestApp(t, be, mm)

	err := runEpicMerge(a, []string{"e1"}, func(string) {})
	if err == nil {
		t.Fatal("expected error when merge_conflict outcome but no merge_conflict_at")
	}
	if !strings.Contains(err.Error(), "no recovery signal") {
		t.Errorf("error must be no recovery signal, got: %v", err)
	}
}

func TestEpicMergeBlockedWithRecoverableOutcome_Success(t *testing.T) {
	cases := []string{
		"push_failed",
		"pr_create_failed",
	}
	for _, outcome := range cases {
		t.Run(outcome, func(t *testing.T) {
			be := newMergeTestBackend()
			be.add(backend.Bead{ID: "e1", Type: "epic", State: "blocked",
				Description: "merge_outcome: " + outcome + "\npr_url: https://x/pr/1\n"})
			mm := &mergeTestMM{}
			a := mergeTestApp(t, be, mm)

			err := runEpicMerge(a, []string{"e1"}, func(string) {})
			if err != nil {
				t.Fatalf("expected no error for recoverable outcome %q: %v", outcome, err)
			}
			if len(mm.dispatched) != 1 || mm.dispatched[0] != "e1" {
				t.Fatalf("expected DispatchMerger for e1, dispatched: %v", mm.dispatched)
			}
		})
	}
}

func TestEpicMergeChildInProgressError(t *testing.T) {
	be := newMergeTestBackend()
	be.add(backend.Bead{ID: "e1", Type: "epic", State: "blocked",
		Description: "merge_conflict_at: src/main.go\n"})
	be.add(backend.Bead{ID: "c1", Type: "task", State: "awaiting_integration", ParentID: "e1"})
	be.add(backend.Bead{ID: "c2", Type: "task", State: "in_progress", ParentID: "e1"})
	mm := &mergeTestMM{}
	a := mergeTestApp(t, be, mm)

	err := runEpicMerge(a, []string{"e1"}, func(string) {})
	if err == nil {
		t.Fatal("expected error when child is in_progress")
	}
	if !strings.Contains(err.Error(), "child") || !strings.Contains(err.Error(), "in_progress") {
		t.Errorf("error must mention child and in_progress, got: %v", err)
	}
}

func TestEpicMergeHappyPath(t *testing.T) {
	be := newMergeTestBackend()
	be.add(backend.Bead{ID: "e1", Type: "epic", State: "blocked",
		Description: "epic_branch: feat/e1\nmerge_conflict_at: src/main.go\nmerge_outcome: push_failed\n"})
	be.add(backend.Bead{ID: "c1", Type: "task", State: "awaiting_integration", ParentID: "e1"})
	be.add(backend.Bead{ID: "c2", Type: "task", State: "closed", ParentID: "e1"})
	mm := &mergeTestMM{}
	a := mergeTestApp(t, be, mm)

	var out strings.Builder
	err := runEpicMerge(a, []string{"e1"}, func(s string) { out.WriteString(s) })
	if err != nil {
		t.Fatalf("happy path must not error: %v", err)
	}

	if len(mm.dispatched) != 1 || mm.dispatched[0] != "e1" {
		t.Fatalf("DispatchMerger must be called for e1: %v", mm.dispatched)
	}

	epic := be.beads["e1"]
	if epic.State != "in_progress" {
		t.Errorf("epic state must be in_progress, got %q", epic.State)
	}
	desc := epic.Description
	if workflow.GetMergeConflictAt(desc) != "" {
		t.Error("merge_conflict_at must be cleared from description")
	}
	if workflow.GetMergeOutcome(desc) != "" {
		t.Error("merge_outcome must be cleared from description")
	}
}

func TestEpicMergeIdempotenceAlreadyInProgressError(t *testing.T) {
	be := newMergeTestBackend()
	be.add(backend.Bead{ID: "e1", Type: "epic", State: "in_progress",
		Description: "merge_conflict_at: src/main.go\n"})
	mm := &mergeTestMM{}
	a := mergeTestApp(t, be, mm)

	err := runEpicMerge(a, []string{"e1"}, func(string) {})
	if err == nil {
		t.Fatal("expected error when epic already in_progress")
	}
	if !strings.Contains(err.Error(), "already in_progress") {
		t.Errorf("error must mention already in_progress, got: %v", err)
	}
}
