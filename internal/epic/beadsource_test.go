package epic

import (
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

type fakeBackend struct {
	beads []backend.Bead
	state map[string]string
}

func (f *fakeBackend) Get(id string, _ string) (*backend.Bead, error) {
	if state, ok := f.state[id]; ok {
		return &backend.Bead{ID: id, State: state}, nil
	}
	for _, b := range f.beads {
		if b.ID == id {
			return &b, nil
		}
	}
	return nil, nil
}

func (f *fakeBackend) List(filters *backend.BeadListFilters, _ string) ([]backend.Bead, error) {
	if filters == nil {
		return f.beads, nil
	}
	result := make([]backend.Bead, 0)
	for _, b := range f.beads {
		if filters.Parent != "" && b.ParentID != filters.Parent {
			continue
		}
		result = append(result, b)
	}
	return result, nil
}

func (f *fakeBackend) ListReady(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (f *fakeBackend) Create(input backend.CreateBeadInput, repoPath string) (*backend.Bead, error) {
	return nil, nil
}
func (f *fakeBackend) Update(id string, input backend.UpdateBeadInput, repoPath string) error {
	return nil
}
func (f *fakeBackend) Delete(id string, repoPath string) error { return nil }
func (f *fakeBackend) Close(id string, reason string, repoPath string) (*backend.TerminalState, error) {
	return nil, nil
}
func (f *fakeBackend) MarkTerminal(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (f *fakeBackend) Reopen(id string, reason string, repoPath string) error { return nil }
func (f *fakeBackend) Rewind(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (f *fakeBackend) Search(query string, filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (f *fakeBackend) Query(expression string, options *backend.BeadQueryOptions, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (f *fakeBackend) AddDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (f *fakeBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (f *fakeBackend) ListDependencies(id string, repoPath string, options *backend.DependencyListOptions) ([]backend.BeadDependency, error) {
	return nil, nil
}
func (f *fakeBackend) BuildTakePrompt(beadID string, options *backend.TakePromptOptions, repoPath string) (*backend.TakePromptResult, error) {
	return nil, nil
}
func (f *fakeBackend) BuildPollPrompt(options *backend.PollPromptOptions, repoPath string) (*backend.PollPromptResult, error) {
	return nil, nil
}
func (f *fakeBackend) ListWorkflows(repoPath string) ([]backend.WorkflowDescriptor, error) {
	return nil, nil
}
func (f *fakeBackend) Capabilities() backend.BackendCapabilities { return backend.BackendCapabilities{} }

func TestLoadEpicBuildsDAGFromBackend(t *testing.T) {
	be := &fakeBackend{beads: []backend.Bead{
		{ID: "e", Type: "epic"},
		{ID: "c1", ParentID: "e"},
		{ID: "c2", ParentID: "e", Dependencies: []backend.BeadDependency{{SourceID: "c1", TargetID: "c2"}}},
	}}
	ep, err := LoadEpic(be, "e", "/repo")
	if err != nil {
		t.Fatalf("LoadEpic: %v", err)
	}
	if len(ep.Children) != 2 {
		t.Errorf("children = %d, want 2", len(ep.Children))
	}
	ready := ep.DAG.ReadySet(map[string]bool{})
	if !sameSet(ready, []string{"c1"}) {
		t.Errorf("ready = %v, want [c1]", ready)
	}
}
