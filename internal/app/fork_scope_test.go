package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

var errFakeListBroke = errors.New("fake list broke")

// forkScopeFakeBackend is a named fake (AGENTS.md §4) covering only what
// BackendForkScopeInspector.OpenDependents exercises: List. Every other
// BackendPort method is a stub never called by this test, following the
// same convention as fixupFakeBackend in epic_fixup_test.go.
type forkScopeFakeBackend struct {
	children []backend.Bead
	listErr  error
}

func (b *forkScopeFakeBackend) List(_ *backend.BeadListFilters, _ string) ([]backend.Bead, error) {
	return b.children, b.listErr
}
func (b *forkScopeFakeBackend) ListReady(_ *backend.BeadListFilters, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *forkScopeFakeBackend) Get(_ string, _ string) (*backend.Bead, error) { return nil, nil }
func (b *forkScopeFakeBackend) Create(_ backend.CreateBeadInput, _ string) (*backend.Bead, error) {
	return nil, nil
}
func (b *forkScopeFakeBackend) Update(_ string, _ backend.UpdateBeadInput, _ string) error {
	return nil
}
func (b *forkScopeFakeBackend) Delete(_ string, _ string) error { return nil }
func (b *forkScopeFakeBackend) Close(_ string, _ string, _ string) (*backend.TerminalState, error) {
	return nil, nil
}
func (b *forkScopeFakeBackend) MarkTerminal(_, _, _, _ string) error { return nil }
func (b *forkScopeFakeBackend) Reopen(_, _, _ string) error          { return nil }
func (b *forkScopeFakeBackend) Rewind(_, _, _, _ string) error       { return nil }
func (b *forkScopeFakeBackend) Search(_ string, _ *backend.BeadListFilters, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *forkScopeFakeBackend) Query(_ string, _ *backend.BeadQueryOptions, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *forkScopeFakeBackend) AddDependency(_, _, _ string) error    { return nil }
func (b *forkScopeFakeBackend) RemoveDependency(_, _, _ string) error { return nil }
func (b *forkScopeFakeBackend) ListDependencies(_, _ string, _ *backend.DependencyListOptions) ([]backend.BeadDependency, error) {
	return nil, nil
}
func (b *forkScopeFakeBackend) BuildTakePrompt(_ string, _ *backend.TakePromptOptions, _ string) (*backend.TakePromptResult, error) {
	return nil, nil
}
func (b *forkScopeFakeBackend) BuildPollPrompt(_ *backend.PollPromptOptions, _ string) (*backend.PollPromptResult, error) {
	return nil, nil
}
func (b *forkScopeFakeBackend) ListWorkflows(_ string) ([]backend.WorkflowDescriptor, error) {
	return nil, nil
}
func (b *forkScopeFakeBackend) Comment(_ string, _ string, _ string) error { return nil }
func (b *forkScopeFakeBackend) Capabilities() backend.BackendCapabilities {
	return backend.BackendCapabilities{}
}

var _ backend.BackendPort = (*forkScopeFakeBackend)(nil)

func TestBackendForkScopeInspector_FindsOpenBlockingSiblings(t *testing.T) {
	be := &forkScopeFakeBackend{children: []backend.Bead{
		{ID: "ep-1.1", ClosedAt: "", Dependencies: []backend.BeadDependency{{TargetID: "ep-1.2", Type: "blocks"}}},
		{ID: "ep-1.2"},
	}}
	inspector := BackendForkScopeInspector{Backend: be}

	got, err := inspector.OpenDependents("ep-1", "ep-1.2", "/repo")
	if err != nil {
		t.Fatalf("OpenDependents: %v", err)
	}
	if len(got) != 1 || got[0] != "ep-1.1" {
		t.Errorf("got %v, want [ep-1.1]", got)
	}
}

func TestBackendForkScopeInspector_ClosedSiblingIsNotOpen(t *testing.T) {
	be := &forkScopeFakeBackend{children: []backend.Bead{
		{ID: "ep-1.1", ClosedAt: "2026-08-01T00:00:00Z", Dependencies: []backend.BeadDependency{{TargetID: "ep-1.2", Type: "blocks"}}},
		{ID: "ep-1.2"},
	}}
	inspector := BackendForkScopeInspector{Backend: be}

	got, err := inspector.OpenDependents("ep-1", "ep-1.2", "/repo")
	if err != nil {
		t.Fatalf("OpenDependents: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want none - the dependent sibling already closed", got)
	}
}

func TestBackendForkScopeInspector_ParentChildDependencyDoesNotCount(t *testing.T) {
	be := &forkScopeFakeBackend{children: []backend.Bead{
		// A parent-child edge only marks epic membership, never an
		// ordering the sibling waits on - see blocksDependencyType's own
		// doc comment.
		{ID: "ep-1.1", Dependencies: []backend.BeadDependency{{TargetID: "ep-1.2", Type: "parent-child"}}},
		{ID: "ep-1.2"},
	}}
	inspector := BackendForkScopeInspector{Backend: be}

	got, err := inspector.OpenDependents("ep-1", "ep-1.2", "/repo")
	if err != nil {
		t.Fatalf("OpenDependents: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want none - parent-child is not a blocking dependency", got)
	}
}

func TestBackendForkScopeInspector_ListFailureIsDispatchFailure(t *testing.T) {
	be := &forkScopeFakeBackend{listErr: errFakeListBroke}
	inspector := BackendForkScopeInspector{Backend: be}

	_, err := inspector.OpenDependents("ep-1", "ep-1.2", "/repo")
	if err == nil || !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Fatalf("OpenDependents() = %v, want a KERNL DISPATCH FAILURE", err)
	}
}

// --- GatherForkScopeFacts ---

// countingForkScopeInspector is a named fake (AGENTS.md §4) recording how
// often it was asked - useful for any future caller that wants to assert
// GatherForkScopeFacts measures exactly once per call.
type countingForkScopeInspector struct {
	open  []string
	err   error
	calls int
}

func (i *countingForkScopeInspector) OpenDependents(_, _, _ string) ([]string, error) {
	i.calls++
	return i.open, i.err
}

func TestGatherForkScopeFacts_CarriesMeasuredAndPassedThroughFields(t *testing.T) {
	inspector := &countingForkScopeInspector{open: []string{"ep-1.1"}}
	facts, err := GatherForkScopeFacts(GatherForkScopeFactsInput{
		EpicID:            "ep-1",
		BeadID:            "ep-1.2",
		RepoPath:          "/repo",
		RelatedDecisions:  "- **X** - Y",
		RepositoryContext: "### README.md\n\nhello\n\n",
		Inspector:         inspector,
	})
	if err != nil {
		t.Fatalf("GatherForkScopeFacts: %v", err)
	}
	if inspector.calls != 1 {
		t.Errorf("Inspector was asked %d times, want exactly once", inspector.calls)
	}
	if len(facts.OpenDependents) != 1 || facts.OpenDependents[0] != "ep-1.1" {
		t.Errorf("OpenDependents = %v, want [ep-1.1]", facts.OpenDependents)
	}
	if facts.RelatedDecisions != "- **X** - Y" || facts.RepositoryContext != "### README.md\n\nhello\n\n" {
		t.Errorf("got %+v, want the pre-rendered text carried through unchanged", facts)
	}
}

func TestGatherForkScopeFacts_NilInspectorFailsLoud(t *testing.T) {
	_, err := GatherForkScopeFacts(GatherForkScopeFactsInput{EpicID: "ep-1", BeadID: "ep-1.2"})
	if err == nil || !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Fatalf("GatherForkScopeFacts() = %v, want a KERNL DISPATCH FAILURE", err)
	}
}

func TestGatherForkScopeFacts_InspectorFailureIsReturned(t *testing.T) {
	inspector := &countingForkScopeInspector{err: errFakeListBroke}
	_, err := GatherForkScopeFacts(GatherForkScopeFactsInput{EpicID: "ep-1", BeadID: "ep-1.2", Inspector: inspector})
	if err == nil {
		t.Fatal("expected the inspector's own error to propagate")
	}
}
