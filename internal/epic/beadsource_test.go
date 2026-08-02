package epic

import (
	"strings"
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
func (f *fakeBackend) Comment(id string, body string, repoPath string) error { return nil }
func (f *fakeBackend) Capabilities() backend.BackendCapabilities {
	return backend.BackendCapabilities{}
}

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

// A dependency on a bead outside the epic is a real and useful thing to
// express - phase 10 depends on work landed in phase 9 - and once that work is
// closed it must not stop the run. The loader used to reject any dependency
// whose target was not a child of the epic, calling it a cycle.
func TestLoadEpicDropsClosedCrossEpicDependency(t *testing.T) {
	be := &fakeBackend{beads: []backend.Bead{
		{ID: "e", Type: "epic"},
		{ID: "c1", ParentID: "e"},
		{ID: "c2", ParentID: "e", Dependencies: []backend.BeadDependency{
			{SourceID: "c2", TargetID: "other.1", Type: "blocks"},
		}},
		{ID: "other.1", ParentID: "other", State: "closed"},
	}}
	ep, err := LoadEpic(be, "e", "/repo")
	if err != nil {
		t.Fatalf("LoadEpic: %v", err)
	}
	if len(ep.Children) != 2 {
		t.Errorf("children = %d, want 2 - the external blocker is not a child", len(ep.Children))
	}
	ready := ep.DAG.ReadySet(map[string]bool{})
	if !sameSet(ready, []string{"c1", "c2"}) {
		t.Errorf("ready = %v, want [c1 c2] - a closed external blocker holds nothing back", ready)
	}
}

// The other half of the same rule: an external blocker that is still open is a
// genuine reason to refuse, and the refusal has to name where that work lives
// so the operator knows which epic to run first.
func TestLoadEpicRefusesOpenCrossEpicDependency(t *testing.T) {
	be := &fakeBackend{beads: []backend.Bead{
		{ID: "e", Type: "epic"},
		{ID: "c1", ParentID: "e", Dependencies: []backend.BeadDependency{
			{SourceID: "c1", TargetID: "other.1", Type: "blocks"},
		}},
		{ID: "other.1", ParentID: "other", State: "implementation"},
	}}
	_, err := LoadEpic(be, "e", "/repo")
	if err == nil {
		t.Fatal("expected a refusal: the external blocker is still open")
	}
	for _, want := range []string{"other.1", "other", "implementation"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "cycle") {
		t.Errorf("error %q calls an open external blocker a cycle", err)
	}
}

// A dependency the tracker has never heard of is the one case that really is a
// broken graph - and it must say so in those words, not as a cycle.
func TestLoadEpicRejectsDependencyOnBeadTheTrackerDoesNotHave(t *testing.T) {
	be := &fakeBackend{beads: []backend.Bead{
		{ID: "e", Type: "epic"},
		{ID: "c1", ParentID: "e", Dependencies: []backend.BeadDependency{
			{SourceID: "c1", TargetID: "ghost", Type: "blocks"},
		}},
	}}
	_, err := LoadEpic(be, "e", "/repo")
	if err == nil {
		t.Fatal("expected a refusal: the blocker does not exist")
	}
	if !strings.Contains(err.Error(), "unknown bead ghost") {
		t.Errorf("error %q does not name the unknown bead", err)
	}
	if strings.Contains(err.Error(), "cycle") {
		t.Errorf("error %q calls a missing bead a cycle", err)
	}
}

// Regression for the 2026-05-17 epic-run failure: bd's `list --json` output
// stores `issue_id` as the dependent and `depends_on_id` as the blocker,
// which RawDependency parses into SourceID=dependent / TargetID=blocker.
// The loader used to assert TargetID == child.ID, but in the bd convention
// SourceID is the child. Accept both shapes so the wire format works
// without bd-side normalization.
func TestLoadEpicAcceptsBdNativeDepConvention(t *testing.T) {
	be := &fakeBackend{beads: []backend.Bead{
		{ID: "e", Type: "epic"},
		{ID: "c1", ParentID: "e"},
		{ID: "c2", ParentID: "e", Dependencies: []backend.BeadDependency{
			// SourceID = the child (dependent), TargetID = the blocker  -
			// exactly what `bd list --json` returns for a c2 that depends on c1.
			{SourceID: "c2", TargetID: "c1", Type: "blocks"},
		}},
	}}
	ep, err := LoadEpic(be, "e", "/repo")
	if err != nil {
		t.Fatalf("LoadEpic: %v", err)
	}
	ready := ep.DAG.ReadySet(map[string]bool{})
	if !sameSet(ready, []string{"c1"}) {
		t.Errorf("ready = %v, want [c1] - c2 should be blocked by c1", ready)
	}
}
