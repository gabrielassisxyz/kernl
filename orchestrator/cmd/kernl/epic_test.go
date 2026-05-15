package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
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
