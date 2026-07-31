package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// revertRouteBackend is a named fake distinct from testBackend (beads_test.go):
// the revert-decision route needs Get to return a mutable Description and
// Update to actually persist it, which testBackend's unconditional
// nil-returning Update does not do - and testBackend is shared by several
// other route tests in this package that do not need that behavior, so this
// stays a separate fixture rather than changing what they depend on.
type revertRouteBackend struct {
	mu    sync.Mutex
	beads map[string]*backend.Bead
}

func newRevertRouteBackend(beads ...backend.Bead) *revertRouteBackend {
	m := map[string]*backend.Bead{}
	for i := range beads {
		b := beads[i]
		m[b.ID] = &b
	}
	return &revertRouteBackend{beads: m}
}

func (b *revertRouteBackend) ListWorkflows(repoPath string) ([]backend.WorkflowDescriptor, error) {
	return nil, nil
}
func (b *revertRouteBackend) List(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *revertRouteBackend) ListReady(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *revertRouteBackend) Get(id string, repoPath string) (*backend.Bead, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	bead, ok := b.beads[id]
	if !ok {
		return nil, nil
	}
	cp := *bead
	return &cp, nil
}
func (b *revertRouteBackend) Create(input backend.CreateBeadInput, repoPath string) (*backend.Bead, error) {
	return nil, nil
}
func (b *revertRouteBackend) Update(id string, input backend.UpdateBeadInput, repoPath string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	bead, ok := b.beads[id]
	if !ok {
		return nil
	}
	if input.Description != "" {
		bead.Description = input.Description
	}
	return nil
}
func (b *revertRouteBackend) Delete(id string, repoPath string) error { return nil }
func (b *revertRouteBackend) Close(id string, reason string, repoPath string) (*backend.TerminalState, error) {
	return nil, nil
}
func (b *revertRouteBackend) MarkTerminal(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (b *revertRouteBackend) Reopen(id string, reason string, repoPath string) error { return nil }
func (b *revertRouteBackend) Rewind(id string, targetState string, reason string, repoPath string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	bead, ok := b.beads[id]
	if ok {
		bead.State = targetState
	}
	return nil
}
func (b *revertRouteBackend) Search(query string, filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *revertRouteBackend) Query(expression string, options *backend.BeadQueryOptions, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *revertRouteBackend) AddDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (b *revertRouteBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (b *revertRouteBackend) ListDependencies(id string, repoPath string, options *backend.DependencyListOptions) ([]backend.BeadDependency, error) {
	return nil, nil
}
func (b *revertRouteBackend) BuildTakePrompt(beadID string, options *backend.TakePromptOptions, repoPath string) (*backend.TakePromptResult, error) {
	return nil, nil
}
func (b *revertRouteBackend) BuildPollPrompt(options *backend.PollPromptOptions, repoPath string) (*backend.PollPromptResult, error) {
	return nil, nil
}
func (b *revertRouteBackend) Comment(id string, body string, repoPath string) error { return nil }
func (b *revertRouteBackend) Capabilities() backend.BackendCapabilities {
	return backend.BackendCapabilities{}
}

// seedRouteDecision writes a real Decision node linked to beadID via
// has_decision, through the production write path, so this route test
// exercises the same shape a real run leaves behind.
func seedRouteDecision(t *testing.T, g *graph.Graph, beadID string) string {
	t.Helper()
	ref := app.BeadRef{ID: beadID, Title: "bead " + beadID, TrackerKind: "br", RepoPath: "/repo"}
	runID, err := app.StartWorkflowRun(context.Background(), g, app.StartWorkflowRunInput{
		EntryPoint:     "epic run",
		Title:          "bead " + beadID,
		WorkflowName:   "worker",
		Beads:          []app.BeadRef{ref},
		RepoPath:       "/repo",
		TrackerCommand: "br --db '/repo/.beads/beads.db'",
		StartedAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("StartWorkflowRun: %v", err)
	}
	sections := backend.DecisionRecordSectionBodies(
		"## Decision\n\nUse a TOML export manifest.\n\n" +
			"## Options Considered\n\n1. JSON.\n2. TOML.\n\n" +
			"## Trade-offs\n\nTOML reads friendlier by hand; JSON has no ambiguity.\n\n" +
			"## Rationale\n\nMatches the rest of the config surface.\n",
	)
	id, err := app.WriteDecisionRecordNode(context.Background(), g, sections, ref, ref, runID)
	if err != nil {
		t.Fatalf("WriteDecisionRecordNode: %v", err)
	}
	return id
}

func TestRevertDecisionHandler_ReopensBeadWithConstraint(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	decisionID := seedRouteDecision(t, g, "kb-1")
	be := newRevertRouteBackend(backend.Bead{ID: "kb-1", Description: "original description", State: "blocked"})

	a := &app.App{Backend: be, Config: testCfg(), Graph: g}
	r := NewRouter(a)

	reqBody := `{"targetState":"ready_for_implementation","reason":"wrong export format"}`
	req := httptest.NewRequest("POST", "/api/beads/kb-1/revert-decision", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if got["decisionId"] != decisionID {
		t.Errorf("decisionId = %q, want %q", got["decisionId"], decisionID)
	}
	if got["targetState"] != "ready_for_implementation" {
		t.Errorf("targetState = %q, want ready_for_implementation", got["targetState"])
	}

	bead, _ := be.Get("kb-1", "/repo")
	if bead.State != "ready_for_implementation" {
		t.Errorf("bead was not rewound, state = %q", bead.State)
	}
	if !strings.Contains(bead.Description, "original description") {
		t.Errorf("original description lost: %q", bead.Description)
	}
	if !strings.Contains(bead.Description, "Do not choose this option again") {
		t.Errorf("constraint missing from description: %q", bead.Description)
	}
}

func TestRevertDecisionHandler_UnknownBeadFailsLoud(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	be := newRevertRouteBackend()
	a := &app.App{Backend: be, Config: testCfg(), Graph: g}
	r := NewRouter(a)

	req := httptest.NewRequest("POST", "/api/beads/kb-missing/revert-decision", strings.NewReader(`{"targetState":"ready_for_implementation","reason":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 500 {
		t.Fatalf("status = %d, want 500, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error body must carry the fail-loud marker, got: %s", w.Body.String())
	}
}
