package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// revertTestBackend is a named fake backend.BackendPort for
// RevertDecisionAndReopenBead's tests. It needs three things none of the
// package's existing fakes offer together: Get returning a mutable
// Description, Update actually persisting it (so a retry's marker check
// reads what a prior call wrote), and Rewind recording its calls and being
// able to fail exactly once - nothing before this bead needed a backend fake
// with a real read-modify-write description.
type revertTestBackend struct {
	mu             sync.Mutex
	beads          map[string]*backend.Bead
	rewindCalls    []rewindCall
	failNextRewind error
}

type rewindCall struct {
	id, targetState, reason string
}

func newRevertTestBackend(beads ...backend.Bead) *revertTestBackend {
	m := map[string]*backend.Bead{}
	for i := range beads {
		b := beads[i]
		m[b.ID] = &b
	}
	return &revertTestBackend{beads: m}
}

func (b *revertTestBackend) ListWorkflows(repoPath string) ([]backend.WorkflowDescriptor, error) {
	return nil, nil
}
func (b *revertTestBackend) List(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *revertTestBackend) ListReady(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *revertTestBackend) Get(id string, repoPath string) (*backend.Bead, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	bead, ok := b.beads[id]
	if !ok {
		return nil, nil
	}
	cp := *bead
	return &cp, nil
}
func (b *revertTestBackend) Create(input backend.CreateBeadInput, repoPath string) (*backend.Bead, error) {
	return nil, nil
}
func (b *revertTestBackend) Update(id string, input backend.UpdateBeadInput, repoPath string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	bead, ok := b.beads[id]
	if !ok {
		return fmt.Errorf("revertTestBackend.Update: no such bead %s", id)
	}
	if input.Description != "" {
		bead.Description = input.Description
	}
	return nil
}
func (b *revertTestBackend) Delete(id string, repoPath string) error { return nil }
func (b *revertTestBackend) Close(id string, reason string, repoPath string) (*backend.TerminalState, error) {
	return nil, nil
}
func (b *revertTestBackend) MarkTerminal(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (b *revertTestBackend) Reopen(id string, reason string, repoPath string) error { return nil }
func (b *revertTestBackend) Rewind(id string, targetState string, reason string, repoPath string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rewindCalls = append(b.rewindCalls, rewindCall{id, targetState, reason})
	if b.failNextRewind != nil {
		err := b.failNextRewind
		b.failNextRewind = nil
		return err
	}
	return nil
}
func (b *revertTestBackend) Search(query string, filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *revertTestBackend) Query(expression string, options *backend.BeadQueryOptions, repoPath string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *revertTestBackend) AddDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (b *revertTestBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (b *revertTestBackend) ListDependencies(id string, repoPath string, options *backend.DependencyListOptions) ([]backend.BeadDependency, error) {
	return nil, nil
}
func (b *revertTestBackend) BuildTakePrompt(beadID string, options *backend.TakePromptOptions, repoPath string) (*backend.TakePromptResult, error) {
	return nil, nil
}
func (b *revertTestBackend) BuildPollPrompt(options *backend.PollPromptOptions, repoPath string) (*backend.PollPromptResult, error) {
	return nil, nil
}
func (b *revertTestBackend) Comment(id string, body string, repoPath string) error { return nil }
func (b *revertTestBackend) Capabilities() backend.BackendCapabilities {
	return backend.BackendCapabilities{}
}

// seedActiveDecision writes a real Decision node linked to bead via
// has_decision, using the same production path (WriteDecisionRecordNode) a
// real run takes - not a hand-rolled fixture - so these tests exercise the
// exact shape resolveActiveDecision will actually read.
func seedActiveDecision(t *testing.T, g *graph.Graph, beadID string) string {
	t.Helper()
	ref := BeadRef{ID: beadID, Title: "bead " + beadID, TrackerKind: "br", RepoPath: "/repo"}
	runID := seedRunWithBeads(t, g, "bead "+beadID, []BeadRef{ref})
	sections := backend.DecisionRecordSectionBodies(wellFormedDecisionRecord)
	id, err := WriteDecisionRecordNode(context.Background(), g, sections, ref, ref, runID)
	if err != nil {
		t.Fatalf("seedActiveDecision: WriteDecisionRecordNode: %v", err)
	}
	return id
}

func getDecision(t *testing.T, g *graph.Graph, id string) *nodes.Decision {
	t.Helper()
	var d *nodes.Decision
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var err error
		d, err = nodes.GetDecision(context.Background(), tx, id)
		return err
	}); err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	return d
}

func TestRevertDecisionAndReopenBead_HappyPath(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	decisionID := seedActiveDecision(t, g, "kb-1")
	be := newRevertTestBackend(backend.Bead{ID: "kb-1", Description: "Original description."})

	result, err := RevertDecisionAndReopenBead(context.Background(), g, be, "/repo", RevertDecisionInput{
		BeadID:      "kb-1",
		TargetState: "ready_for_implementation",
		Reason:      "wrong dependency direction",
	})
	if err != nil {
		t.Fatalf("RevertDecisionAndReopenBead: %v", err)
	}
	if result.DecisionID != decisionID {
		t.Errorf("DecisionID = %q, want %q", result.DecisionID, decisionID)
	}
	if result.TargetState != "ready_for_implementation" {
		t.Errorf("TargetState = %q, want ready_for_implementation", result.TargetState)
	}

	d := getDecision(t, g, decisionID)
	if d.RevertedAt == nil {
		t.Error("RevertedAt is nil, want set")
	}
	if d.RevertReason == nil || *d.RevertReason != "wrong dependency direction" {
		t.Errorf("RevertReason = %v, want \"wrong dependency direction\"", d.RevertReason)
	}

	bead, _ := be.Get("kb-1", "/repo")
	if !strings.Contains(bead.Description, "Original description.") {
		t.Errorf("original description lost: %q", bead.Description)
	}
	if !strings.Contains(bead.Description, "Do not choose this option again") {
		t.Errorf("constraint directive missing: %q", bead.Description)
	}
	if !strings.Contains(bead.Description, "wrong dependency direction") {
		t.Errorf("revert reason missing from description: %q", bead.Description)
	}

	if len(be.rewindCalls) != 1 || be.rewindCalls[0] != (rewindCall{"kb-1", "ready_for_implementation", "wrong dependency direction"}) {
		t.Errorf("rewindCalls = %v, want exactly one matching call", be.rewindCalls)
	}
}

// The acceptance criterion is that the RENDERED PROMPT carries the
// constraint, not merely that it was stored - a test asserting storage alone
// does not prove the agent will ever read it. This builds the actual prompt
// BuildBeadStagePrompt sends to an implementer, off the bead state
// RevertDecisionAndReopenBead left behind, and checks the constraint text is
// in it.
func TestRevertDecisionAndReopenBead_ConstraintReachesRenderedPrompt(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	seedActiveDecision(t, g, "kb-2")
	be := newRevertTestBackend(backend.Bead{ID: "kb-2", Description: "Wire the SSE reconnect."})

	if _, err := RevertDecisionAndReopenBead(context.Background(), g, be, "/repo", RevertDecisionInput{
		BeadID:      "kb-2",
		TargetState: "ready_for_implementation",
		Reason:      "picked the wrong retry backoff",
	}); err != nil {
		t.Fatalf("RevertDecisionAndReopenBead: %v", err)
	}

	bead, _ := be.Get("kb-2", "/repo")
	prompt := BuildBeadStagePrompt(StagePromptInput{Bead: bead, State: "implementation"})

	if !strings.Contains(prompt, "Do not choose this option again") {
		t.Errorf("rendered prompt does not carry the constraint directive:\n%s", prompt)
	}
	if !strings.Contains(prompt, "picked the wrong retry backoff") {
		t.Errorf("rendered prompt does not carry the revert reason:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Wire the SSE reconnect.") {
		t.Errorf("rendered prompt lost the original description:\n%s", prompt)
	}
}

// Given the bead, the graph must answer "what decision was reverted here and
// why" - this is the traceability acceptance criterion, asked directly of
// the graph rather than of RevertDecisionAndReopenBead's return value.
func TestRevertDecisionAndReopenBead_GraphAnswersWhatWasRevertedAndWhy(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	seedActiveDecision(t, g, "kb-3")
	be := newRevertTestBackend(backend.Bead{ID: "kb-3", Description: "original"})

	if _, err := RevertDecisionAndReopenBead(context.Background(), g, be, "/repo", RevertDecisionInput{
		BeadID:      "kb-3",
		TargetState: "ready_for_implementation",
		Reason:      "the export manifest should not have been TOML",
	}); err != nil {
		t.Fatalf("RevertDecisionAndReopenBead: %v", err)
	}

	// A reader who only has the bead id - not RevertDecisionAndReopenBead's
	// return value - must still be able to answer the question by walking
	// has_decision edges out of the bead.
	var reverted *nodes.Decision
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		out, err := edges.Outgoing(context.Background(), tx, "kb-3", edges.WithType(edges.EdgeTypeHasDecision))
		if err != nil {
			return err
		}
		for _, e := range out {
			d, err := nodes.GetDecision(context.Background(), tx, e.Dst)
			if err != nil {
				return err
			}
			if d.RevertedAt != nil {
				reverted = d
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("querying the graph for kb-3's reverted decision: %v", err)
	}

	if reverted == nil {
		t.Fatal("the graph has no answer for what decision was reverted on kb-3")
	}
	if reverted.RevertReason == nil || *reverted.RevertReason != "the export manifest should not have been TOML" {
		t.Errorf("RevertReason = %v, want the recorded reason", reverted.RevertReason)
	}
}

func TestRevertDecisionAndReopenBead_MissingFieldsFailLoud(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	be := newRevertTestBackend()

	cases := []struct {
		name string
		in   RevertDecisionInput
	}{
		{"no bead id", RevertDecisionInput{TargetState: "ready_for_implementation", Reason: "x"}},
		{"no target state", RevertDecisionInput{BeadID: "kb-1", Reason: "x"}},
		{"no reason", RevertDecisionInput{BeadID: "kb-1", TargetState: "ready_for_implementation"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := RevertDecisionAndReopenBead(context.Background(), g, be, "/repo", tc.in)
			if err == nil {
				t.Fatal("expected a fail-loud error")
			}
			if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
				t.Errorf("error must carry the fail-loud marker, got: %v", err)
			}
		})
	}
}

// Two decision-bearing stages on the same bead means resolveActiveDecision
// cannot guess which one the operator means; it must ask for --decision
// rather than pick one silently.
func TestRevertDecisionAndReopenBead_AmbiguousWithoutExplicitDecisionFailsLoud(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ref := BeadRef{ID: "kb-4", Title: "bead kb-4", TrackerKind: "br", RepoPath: "/repo"}

	runID1 := seedRunWithBeads(t, g, "bead kb-4", []BeadRef{ref})
	sections1 := backend.DecisionRecordSectionBodies(wellFormedDecisionRecord)
	if _, err := WriteDecisionRecordNode(context.Background(), g, sections1, ref, ref, runID1); err != nil {
		t.Fatalf("seed decision 1: %v", err)
	}

	runID2 := seedRunWithBeads(t, g, "bead kb-4 retry", []BeadRef{ref})
	secondRecord := strings.ReplaceAll(wellFormedDecisionRecord, "typed constant", "second, distinct decision")
	sections2 := backend.DecisionRecordSectionBodies(secondRecord)
	if _, err := WriteDecisionRecordNode(context.Background(), g, sections2, ref, ref, runID2); err != nil {
		t.Fatalf("seed decision 2: %v", err)
	}

	be := newRevertTestBackend(backend.Bead{ID: "kb-4", Description: "d"})
	_, err := RevertDecisionAndReopenBead(context.Background(), g, be, "/repo", RevertDecisionInput{
		BeadID: "kb-4", TargetState: "ready_for_implementation", Reason: "x",
	})
	if err == nil {
		t.Fatal("expected an ambiguity error")
	}
	if !strings.Contains(err.Error(), "--decision") {
		t.Errorf("error must point at --decision to disambiguate, got: %v", err)
	}
}

// A decision id that belongs to a different bead must be rejected, not
// silently accepted - a copy-paste from the wrong bead must not revert an
// unrelated decision.
func TestRevertDecisionAndReopenBead_ExplicitDecisionNotLinkedFailsLoud(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	otherDecisionID := seedActiveDecision(t, g, "kb-other")
	seedActiveDecision(t, g, "kb-5")

	be := newRevertTestBackend(backend.Bead{ID: "kb-5", Description: "d"})
	_, err := RevertDecisionAndReopenBead(context.Background(), g, be, "/repo", RevertDecisionInput{
		BeadID: "kb-5", DecisionID: otherDecisionID, TargetState: "ready_for_implementation", Reason: "x",
	})
	if err == nil {
		t.Fatal("expected a not-linked error")
	}
	if !strings.Contains(err.Error(), "not linked") {
		t.Errorf("error must say the decision is not linked to this bead, got: %v", err)
	}
}

// A rewind failure must leave a state that is safely retryable: the
// decision stays marked reverted (not re-marked, not double-reasoned), the
// description is not double-prepended, and the retry succeeds once the
// backend recovers.
func TestRevertDecisionAndReopenBead_RetryAfterRewindFailureConverges(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	decisionID := seedActiveDecision(t, g, "kb-6")
	be := newRevertTestBackend(backend.Bead{ID: "kb-6", Description: "original"})
	be.failNextRewind = fmt.Errorf("br: connection refused")

	in := RevertDecisionInput{BeadID: "kb-6", TargetState: "ready_for_implementation", Reason: "bad default"}

	_, err := RevertDecisionAndReopenBead(context.Background(), g, be, "/repo", in)
	if err == nil {
		t.Fatal("expected the first call to fail on rewind")
	}
	if !strings.Contains(err.Error(), "retry this exact command") {
		t.Errorf("error must tell the operator the state is safe to retry, got: %v", err)
	}

	bead, _ := be.Get("kb-6", "/repo")
	firstDescription := bead.Description
	if strings.Count(firstDescription, "kernl:reverted-decision:") != 1 {
		t.Fatalf("expected exactly one constraint marker after the first (partial) call, description: %q", firstDescription)
	}

	// A blind retry with no --decision must not silently pick the
	// already-reverted decision back up (see resolveActiveDecision's doc
	// comment on why) - it has to name the id and point at --decision so
	// the retry can be explicit about which revert it is completing.
	_, err = RevertDecisionAndReopenBead(context.Background(), g, be, "/repo", in)
	if err == nil {
		t.Fatal("a blind retry with no --decision must fail loud, not silently no-op")
	}
	if !strings.Contains(err.Error(), "--decision") {
		t.Errorf("error must point at --decision to complete the retry, got: %v", err)
	}

	retryIn := in
	retryIn.DecisionID = decisionID
	result, err := RevertDecisionAndReopenBead(context.Background(), g, be, "/repo", retryIn)
	if err != nil {
		t.Fatalf("retry with --decision must succeed once the backend recovers: %v", err)
	}
	if result.DecisionID != decisionID {
		t.Errorf("DecisionID = %q, want %q", result.DecisionID, decisionID)
	}

	bead, _ = be.Get("kb-6", "/repo")
	if strings.Count(bead.Description, "kernl:reverted-decision:") != 1 {
		t.Errorf("retry must not double-prepend the constraint, description: %q", bead.Description)
	}
	if len(be.rewindCalls) != 2 {
		t.Fatalf("expected two rewind attempts (one failed, one succeeded), got %d", len(be.rewindCalls))
	}

	d := getDecision(t, g, decisionID)
	if d.RevertReason == nil || *d.RevertReason != "bad default" {
		t.Errorf("RevertReason = %v, want unchanged across the retry", d.RevertReason)
	}
}

// A second, DIFFERENT revert of an already-reverted decision must not
// silently overwrite the first revert's record - that would erase exactly
// the information this mechanism exists to keep.
func TestRevertDecisionAndReopenBead_DifferentReasonOnAlreadyRevertedFailsLoud(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	decisionID := seedActiveDecision(t, g, "kb-7")
	be := newRevertTestBackend(backend.Bead{ID: "kb-7", Description: "original"})

	if _, err := RevertDecisionAndReopenBead(context.Background(), g, be, "/repo", RevertDecisionInput{
		BeadID: "kb-7", TargetState: "ready_for_implementation", Reason: "first reason",
	}); err != nil {
		t.Fatalf("first revert: %v", err)
	}

	// Named explicitly (as an operator would, having already seen the id in
	// the first call's own success) rather than left to auto-resolution:
	// resolveActiveDecision deliberately will not auto-pick an
	// already-reverted decision back up (see its own doc comment), so this
	// is exercising markDecisionReverted's own guard, not that one.
	_, err := RevertDecisionAndReopenBead(context.Background(), g, be, "/repo", RevertDecisionInput{
		BeadID: "kb-7", DecisionID: decisionID, TargetState: "ready_for_implementation", Reason: "a different reason",
	})
	if err == nil {
		t.Fatal("expected the second, different revert to fail loud")
	}
	if !strings.Contains(err.Error(), "already reverted") {
		t.Errorf("error must say the decision was already reverted, got: %v", err)
	}
}
