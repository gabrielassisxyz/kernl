package main

import (
	"fmt"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/epic"
	"github.com/gabrielassisxyz/kernl/internal/merge"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

type mergeTestBackend struct {
	beads           []backend.Bead
	updateCalled    bool
	updateState     string
	updateDesc      string
}

func (b *mergeTestBackend) ListWorkflows(repoPath string) ([]backend.WorkflowDescriptor, error) { return nil, nil }
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
	for i := range b.beads {
		if b.beads[i].ID == id {
			cp := b.beads[i]
			return &cp, nil
		}
	}
	return nil, nil
}
func (b *mergeTestBackend) Create(input backend.CreateBeadInput, repoPath string) (*backend.Bead, error) {
	return nil, nil
}
func (b *mergeTestBackend) Update(id string, input backend.UpdateBeadInput, repoPath string) error {
	b.updateCalled = true
	b.updateState = input.State
	b.updateDesc = input.Description
	return nil
}
func (b *mergeTestBackend) Delete(id string, repoPath string) error { return nil }
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
func (b *mergeTestBackend) Capabilities() backend.BackendCapabilities { return backend.BackendCapabilities{} }

type mergeDispatchFake struct {
	dispatched bool
	err        error
}

func (f *mergeDispatchFake) TryTrigger(epicID string)       {}
func (f *mergeDispatchFake) RouteOutcome(epicID string)     {}
func (f *mergeDispatchFake) DispatchMerger(epicID string) error {
	f.dispatched = true
	return f.err
}

func mergeApp(be *mergeTestBackend, merger merge.MergeDispatchPort) *app.App {
	return &app.App{
		Backend: be,
		Merger:  merger,
		Config: &config.Config{
			Registry: config.RegistryConfig{Repos: []config.RepoEntry{{Path: "/test"}}},
		},
		EpicEvents: epic.NewEpicEventHub(),
	}
}

func TestEpicMergeNotBlocked(t *testing.T) {
	be := &mergeTestBackend{beads: []backend.Bead{
		{ID: "e1", Type: "epic", Title: "test", State: string(workflow.StatusOpen)},
		{ID: "c1", Type: "task", ParentID: "e1", State: string(workflow.StatusAwaitingIntegration)},
	}}
	merger := &mergeDispatchFake{}
	a := mergeApp(be, merger)

	err := runEpicMergeWithApp(a, []string{"e1"})
	if err == nil {
		t.Fatal("expected error when epic is not blocked")
	}
}

func TestEpicMergeBlockedNoRecoverySignal(t *testing.T) {
	be := &mergeTestBackend{beads: []backend.Bead{
		{ID: "e2", Type: "epic", Title: "test", State: string(workflow.StatusBlocked), Description: "no metadata here"},
		{ID: "c2", Type: "task", ParentID: "e2", State: string(workflow.StatusAwaitingIntegration)},
	}}
	merger := &mergeDispatchFake{}
	a := mergeApp(be, merger)

	err := runEpicMergeWithApp(a, []string{"e2"})
	if err == nil {
		t.Fatal("expected error when blocked epic has no recovery signal")
	}
}

func TestEpicMergeChildInProgress(t *testing.T) {
	be := &mergeTestBackend{beads: []backend.Bead{
		{ID: "e3", Type: "epic", Title: "test", State: string(workflow.StatusBlocked), Description: workflow.AddMetadataField("", "merge_conflict_at", "2024-01-01")},
		{ID: "c3", Type: "task", ParentID: "e3", State: string(workflow.StatusInProgress)},
	}}
	merger := &mergeDispatchFake{}
	a := mergeApp(be, merger)

	err := runEpicMergeWithApp(a, []string{"e3"})
	if err == nil {
		t.Fatal("expected error when a child is in progress")
	}
}

func TestEpicMergeHappyPath(t *testing.T) {
	desc := workflow.AddMetadataField("", "merge_conflict_at", "2024-01-01")
	be := &mergeTestBackend{beads: []backend.Bead{
		{ID: "e4", Type: "epic", Title: "test", State: string(workflow.StatusBlocked), Description: desc},
		{ID: "c4", Type: "task", ParentID: "e4", State: string(workflow.StatusAwaitingIntegration)},
		{ID: "c5", Type: "task", ParentID: "e4", State: string(workflow.StatusClosed)},
	}}
	merger := &mergeDispatchFake{}
	a := mergeApp(be, merger)

	err := runEpicMergeWithApp(a, []string{"e4"})
	if err != nil {
		t.Fatalf("expected happy path to succeed, got: %v", err)
	}
	if !merger.dispatched {
		t.Error("expected DispatchMerger to be called")
	}
	if !be.updateCalled {
		t.Error("expected Backend.Update to be called")
	}
	if be.updateState != string(workflow.StatusInProgress) {
		t.Errorf("expected update to in_progress, got %q", be.updateState)
	}
	if workflow.GetMergeConflictAt(be.updateDesc) != "" {
		t.Errorf("expected merge_conflict_at to be cleared, got %q", workflow.GetMergeConflictAt(be.updateDesc))
	}
	if workflow.GetMergeOutcome(be.updateDesc) != "" {
		t.Errorf("expected merge_outcome to be cleared, got %q", workflow.GetMergeOutcome(be.updateDesc))
	}
}

func TestEpicMergeWithPushFailedOutcome(t *testing.T) {
	desc := workflow.AddMetadataField("", "merge_outcome", string(merge.OutcomePushFailed))
	be := &mergeTestBackend{beads: []backend.Bead{
		{ID: "e5", Type: "epic", Title: "test", State: string(workflow.StatusBlocked), Description: desc},
		{ID: "c6", Type: "task", ParentID: "e5", State: string(workflow.StatusAwaitingIntegration)},
	}}
	merger := &mergeDispatchFake{}
	a := mergeApp(be, merger)

	err := runEpicMergeWithApp(a, []string{"e5"})
	if err != nil {
		t.Fatalf("expected push_failed outcome to be recoverable, got: %v", err)
	}
	if !merger.dispatched {
		t.Error("expected DispatchMerger to be called")
	}
}

func TestEpicMergeIdempotent(t *testing.T) {
	be := &mergeTestBackend{beads: []backend.Bead{
		{ID: "e6", Type: "epic", Title: "test", State: string(workflow.StatusBlocked), Description: workflow.AddMetadataField("", "merge_conflict_at", "2024-01-01")},
		{ID: "c7", Type: "task", ParentID: "e6", State: string(workflow.StatusAwaitingIntegration)},
	}}
	merger := &mergeDispatchFake{}
	a := mergeApp(be, merger)

	err1 := runEpicMergeWithApp(a, []string{"e6"})
	if err1 != nil {
		t.Fatalf("first merge should succeed: %v", err1)
	}

	be.beads[0].State = string(workflow.StatusInProgress)
	be.beads[0].Description = ""

	err2 := runEpicMergeWithApp(a, []string{"e6"})
	if err2 == nil {
		t.Fatal("expected error when epic is already in_progress")
	}
}

func TestEpicMergeDispatchFailure(t *testing.T) {
	desc := workflow.AddMetadataField("", "merge_conflict_at", "2024-01-01")
	be := &mergeTestBackend{beads: []backend.Bead{
		{ID: "e7", Type: "epic", Title: "test", State: string(workflow.StatusBlocked), Description: desc},
		{ID: "c8", Type: "task", ParentID: "e7", State: string(workflow.StatusAwaitingIntegration)},
	}}
	merger := &mergeDispatchFake{err: fmt.Errorf("dispatch failed")}
	a := mergeApp(be, merger)

	err := runEpicMergeWithApp(a, []string{"e7"})
	if err == nil {
		t.Fatal("expected error when merger dispatch fails")
	}
}
