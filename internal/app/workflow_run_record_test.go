package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func startedAtFixture() time.Time {
	return time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
}

func TestStartWorkflowRun_CreatesRunningNodeWithRoundTrippingRunData(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	runID, err := StartWorkflowRun(ctx, g, StartWorkflowRunInput{
		EntryPoint:     "epic run",
		Title:          "test epic",
		WorkflowName:   "worker",
		Beads:          []BeadRef{{ID: "kb-epic-1", Title: "test epic"}, {ID: "kb-child-1", Title: "child"}},
		RepoPath:       "/repo",
		BaseBranch:     "master",
		VerifyCommand:  "bin/ci",
		TrackerCommand: "br --db '/repo/.beads/beads.db'",
		DryRun:         false,
		StartedAt:      startedAtFixture(),
	})
	if err != nil {
		t.Fatalf("StartWorkflowRun: %v", err)
	}
	if runID == "" {
		t.Fatal("expected a non-empty run id")
	}

	var wr *nodes.WorkflowRun
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		wr, err = nodes.GetWorkflowRun(ctx, tx, runID)
		return err
	}); err != nil {
		t.Fatalf("GetWorkflowRun: %v", err)
	}

	if wr.WorkflowName != "worker" {
		t.Errorf("WorkflowName = %q, want %q", wr.WorkflowName, "worker")
	}
	if wr.Status != "running" {
		t.Errorf("Status = %q, want %q", wr.Status, "running")
	}

	var data runData
	if err := json.Unmarshal([]byte(wr.RunData), &data); err != nil {
		t.Fatalf("RunData does not round-trip as JSON: %v", err)
	}
	if data.EntryPoint != "epic run" {
		t.Errorf("RunData.EntryPoint = %q, want %q", data.EntryPoint, "epic run")
	}
	if data.RepoPath != "/repo" || data.BaseBranch != "master" || data.VerifyCommand != "bin/ci" {
		t.Errorf("RunData did not round-trip repo facts: %+v", data)
	}
	if !data.StartedAt.Equal(startedAtFixture()) {
		t.Errorf("RunData.StartedAt = %v, want %v", data.StartedAt, startedAtFixture())
	}
	if len(data.BeadIDs) != 2 || data.BeadIDs[0] != "kb-epic-1" || data.BeadIDs[1] != "kb-child-1" {
		t.Errorf("RunData.BeadIDs = %v, want [kb-epic-1 kb-child-1]", data.BeadIDs)
	}
	if data.FinishedAt != nil {
		t.Errorf("RunData.FinishedAt = %v, want nil before the run is closed", data.FinishedAt)
	}

	// camelCase on the wire, per AGENTS.md's JSON contract - GET /api/runs
	// forwards this blob straight through.
	if !strings.Contains(wr.RunData, `"entryPoint"`) || !strings.Contains(wr.RunData, `"beadIds"`) {
		t.Errorf("RunData is not camelCase: %s", wr.RunData)
	}
}

func TestStartWorkflowRun_CreatesOneRanBeadEdgePerBeadAndTheirReferenceNodes(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	beads := []BeadRef{{ID: "kb-epic-2", Title: "epic"}, {ID: "kb-a", Title: "a"}, {ID: "kb-b", Title: "b"}}
	runID, err := StartWorkflowRun(ctx, g, StartWorkflowRunInput{
		EntryPoint:     "epic run",
		Title:          "epic",
		WorkflowName:   "worker",
		Beads:          beads,
		RepoPath:       "/repo",
		TrackerCommand: "bd -C '/repo'",
		StartedAt:      startedAtFixture(),
	})
	if err != nil {
		t.Fatalf("StartWorkflowRun: %v", err)
	}

	var out []edges.Edge
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		out, err = edges.Outgoing(ctx, tx, runID, edges.WithType(edges.EdgeTypeRanBead))
		return err
	}); err != nil {
		t.Fatalf("edges.Outgoing: %v", err)
	}
	if len(out) != len(beads) {
		t.Fatalf("expected %d ran_bead edges from the run, got %d: %+v", len(beads), len(out), out)
	}
	dsts := map[string]bool{}
	for _, e := range out {
		dsts[e.Dst] = true
	}
	for _, b := range beads {
		if !dsts[b.ID] {
			t.Errorf("no ran_bead edge from run %s to bead %s", runID, b.ID)
		}

		var ref *nodes.BeadReference
		if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
			var err error
			ref, err = nodes.GetBeadReference(ctx, tx, b.ID)
			return err
		}); err != nil {
			t.Errorf("bead reference node for %s was not created: %v", b.ID, err)
			continue
		}
		if ref.Title != b.Title {
			t.Errorf("bead reference %s Title = %q, want %q", b.ID, ref.Title, b.Title)
		}
		if ref.TrackerKind != "bd" {
			t.Errorf("bead reference %s TrackerKind = %q, want %q (derived from TrackerCommand)", b.ID, ref.TrackerKind, "bd")
		}
	}
}

func TestStartWorkflowRun_ZeroBeadsFailsLoud(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	_, err := StartWorkflowRun(context.Background(), g, StartWorkflowRunInput{
		EntryPoint:     "bead run",
		Title:          "orphan",
		TrackerCommand: "bd -C '/repo'",
		StartedAt:      startedAtFixture(),
	})
	if err == nil {
		t.Fatal("expected an error: a run with zero beads has nothing a ran_bead edge can point at")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error %q does not carry the KERNL DISPATCH FAILURE marker", err.Error())
	}
}

func TestStartWorkflowRun_NilGraphFailsLoud(t *testing.T) {
	_, err := StartWorkflowRun(context.Background(), nil, StartWorkflowRunInput{
		EntryPoint:     "bead run",
		Title:          "x",
		TrackerCommand: "bd -C '/repo'",
		Beads:          []BeadRef{{ID: "kb-1", Title: "x"}},
		StartedAt:      startedAtFixture(),
	})
	if err == nil {
		t.Fatal("expected an error: no graph is open")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error %q does not carry the KERNL DISPATCH FAILURE marker", err.Error())
	}
}

// TestCloseWorkflowRun_PreservesStartFields is the regression test for the
// bug a naive UpdateWorkflowRun would reintroduce: replacing the whole node
// on close, rather than merging into what StartWorkflowRun already wrote,
// silently drops entryPoint/repoPath/beadIds/startedAt and every other
// start-time fact the moment a run is closed.
func TestCloseWorkflowRun_PreservesStartFields(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	runID, err := StartWorkflowRun(ctx, g, StartWorkflowRunInput{
		EntryPoint:     "bead run",
		Title:          "standalone",
		WorkflowName:   "autopilot_no_planning",
		Beads:          []BeadRef{{ID: "kb-solo-1", Title: "solo"}},
		RepoPath:       "/repo",
		BaseBranch:     "master",
		VerifyCommand:  "bin/ci",
		TrackerCommand: "br --db '/repo/.beads/beads.db'",
		StartedAt:      startedAtFixture(),
	})
	if err != nil {
		t.Fatalf("StartWorkflowRun: %v", err)
	}

	finishedAt := startedAtFixture().Add(5 * time.Minute)
	if err := CloseWorkflowRun(ctx, g, runID, CloseWorkflowRunInput{Status: "completed", FinishedAt: finishedAt}); err != nil {
		t.Fatalf("CloseWorkflowRun: %v", err)
	}

	var wr *nodes.WorkflowRun
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		wr, err = nodes.GetWorkflowRun(ctx, tx, runID)
		return err
	}); err != nil {
		t.Fatalf("GetWorkflowRun: %v", err)
	}

	if wr.Status != "completed" {
		t.Errorf("Status = %q, want %q", wr.Status, "completed")
	}
	if wr.WorkflowName != "autopilot_no_planning" {
		t.Errorf("WorkflowName = %q, want the value StartWorkflowRun wrote, got clobbered", wr.WorkflowName)
	}

	var data runData
	if err := json.Unmarshal([]byte(wr.RunData), &data); err != nil {
		t.Fatalf("RunData does not parse: %v", err)
	}
	if data.EntryPoint != "bead run" || data.RepoPath != "/repo" || data.BaseBranch != "master" || data.VerifyCommand != "bin/ci" {
		t.Errorf("close clobbered fields StartWorkflowRun wrote: %+v", data)
	}
	if !data.StartedAt.Equal(startedAtFixture()) {
		t.Errorf("StartedAt = %v, want the value StartWorkflowRun wrote (%v), got clobbered", data.StartedAt, startedAtFixture())
	}
	if len(data.BeadIDs) != 1 || data.BeadIDs[0] != "kb-solo-1" {
		t.Errorf("BeadIDs = %v, want [kb-solo-1], got clobbered", data.BeadIDs)
	}
	if data.FinishedAt == nil || !data.FinishedAt.Equal(finishedAt) {
		t.Errorf("FinishedAt = %v, want %v", data.FinishedAt, finishedAt)
	}
}

func TestCloseWorkflowRun_FailedStatusCarriesFailureText(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	runID, err := StartWorkflowRun(ctx, g, StartWorkflowRunInput{
		EntryPoint:     "bead run",
		Title:          "will fail",
		RepoPath:       "/repo",
		TrackerCommand: "bd -C '/repo'",
		Beads:          []BeadRef{{ID: "kb-fails-1", Title: "will fail"}},
		StartedAt:      startedAtFixture(),
	})
	if err != nil {
		t.Fatalf("StartWorkflowRun: %v", err)
	}

	closeErr := CloseWorkflowRun(ctx, g, runID, CloseWorkflowRunInput{
		Status:     "failed",
		FinishedAt: startedAtFixture().Add(time.Minute),
		Failure:    "KERNL DISPATCH FAILURE: agent exited non-zero",
	})
	if closeErr != nil {
		t.Fatalf("CloseWorkflowRun: %v", closeErr)
	}

	var wr *nodes.WorkflowRun
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		wr, err = nodes.GetWorkflowRun(ctx, tx, runID)
		return err
	}); err != nil {
		t.Fatalf("GetWorkflowRun: %v", err)
	}
	if wr.Status != "failed" {
		t.Errorf("Status = %q, want %q", wr.Status, "failed")
	}

	var data runData
	if err := json.Unmarshal([]byte(wr.RunData), &data); err != nil {
		t.Fatalf("RunData does not parse: %v", err)
	}
	if data.Failure != "KERNL DISPATCH FAILURE: agent exited non-zero" {
		t.Errorf("Failure = %q, want the text passed to CloseWorkflowRun", data.Failure)
	}
}

func TestCloseWorkflowRun_EmptyStatusFailsLoud(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	err := CloseWorkflowRun(context.Background(), g, "does-not-matter", CloseWorkflowRunInput{FinishedAt: startedAtFixture()})
	if err == nil {
		t.Fatal("expected an error: Status is empty")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error %q does not carry the KERNL DISPATCH FAILURE marker", err.Error())
	}
}

func TestCloseWorkflowRun_NilGraphFailsLoud(t *testing.T) {
	err := CloseWorkflowRun(context.Background(), nil, "kb-run-1", CloseWorkflowRunInput{Status: "completed", FinishedAt: startedAtFixture()})
	if err == nil {
		t.Fatal("expected an error: no graph is open")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error %q does not carry the KERNL DISPATCH FAILURE marker", err.Error())
	}
}

func TestCloseWorkflowRun_UnknownRunIDFailsLoud(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	err := CloseWorkflowRun(context.Background(), g, "never-started", CloseWorkflowRunInput{Status: "completed", FinishedAt: startedAtFixture()})
	if err == nil {
		t.Fatal("expected an error: no such run was ever started")
	}
}
