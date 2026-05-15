//go:build integration

package integration

import (
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/epic"
	"github.com/gabrielassisxyz/kernl/internal/runstate"
)

func TestPlanResumeSkipsDoneResumesInterruptedRedispatchesFresh(t *testing.T) {
	h := NewEpicHarness(t)
	defer h.Cleanup()
	epicID := h.SeedEpic(t, "beads-epic-diamond")

	be := backend.NewBdCliBackend(h.RepoPath)

	if err := be.MarkTerminal("a", "done", "completed in prior run", h.RepoPath); err != nil {
		t.Fatalf("mark a done: %v", err)
	}
	if err := be.Update("b", backend.UpdateBeadInput{State: "implementing"}, h.RepoPath); err != nil {
		t.Fatalf("set b implementing: %v", err)
	}

	rsPath := t.TempDir() + "/runstate.db"
	rs, err := runstate.Open(rsPath)
	if err != nil {
		t.Fatalf("open runstate: %v", err)
	}
	defer rs.Close()

	rs.SetWorktree(epicID, "b", t.TempDir()+"/worktree/epic-1/b")
	rs.RecordAgent("b", "implementing", runstate.AgentRecord{
		AgentID:   "opencode",
		SessionID: "term-interrupted",
		Status:    "running",
	})

	if err := rs.SetWorktree(epicID, "a", t.TempDir()+"/worktree/epic-1/a"); err != nil {
		t.Fatalf("set worktree a: %v", err)
	}

	ep, err := epic.LoadEpic(be, epicID, h.RepoPath)
	if err != nil {
		t.Fatalf("load epic: %v", err)
	}

	plan := epic.PlanResume(be, rs, ep, h.RepoPath)

	if plan.Action("a") != epic.ResumeSkip {
		t.Errorf("a should be skipped (done), got %s", plan.Action("a"))
	}
	if plan.Action("b") != epic.ResumeSession {
		t.Errorf("b should resume session, got %s", plan.Action("b"))
	}
	if plan.SessionID("b") != "term-interrupted" {
		t.Errorf("b session = %q, want term-interrupted", plan.SessionID("b"))
	}
	if plan.Action("c") != epic.ResumeFreshDispatch {
		t.Errorf("c should be fresh dispatch, got %s", plan.Action("c"))
	}
	if plan.Action("d") != epic.ResumeFreshDispatch {
		t.Errorf("d should be fresh dispatch, got %s", plan.Action("d"))
	}
}

func TestPlanResumeMissingWorktreeFailsLoud(t *testing.T) {
	h := NewEpicHarness(t)
	defer h.Cleanup()

	be := backend.NewBdCliBackend(h.RepoPath)
	if err := be.Update("a", backend.UpdateBeadInput{State: "implementing"}, h.RepoPath); err != nil {
		t.Fatalf("set a implementing: %v", err)
	}

	rsPath := t.TempDir() + "/runstate.db"
	rs, err := runstate.Open(rsPath)
	if err != nil {
		t.Fatalf("open runstate: %v", err)
	}
	defer rs.Close()

	rs.SetWorktree(h.SeedEpic(t, "beads-epic-diamond"), "a", "/nonexistent/path/worktree")

	ep, err := epic.LoadEpic(be, "epic-1", h.RepoPath)
	if err != nil {
		t.Fatalf("load epic: %v", err)
	}

	plan := epic.PlanResume(be, rs, ep, h.RepoPath)

	if plan.Action("a") != epic.ResumeError {
		t.Errorf("missing worktree must surface as ResumeError, got %s", plan.Action("a"))
	}
	if plan.Detail("a") == "" {
		t.Error("missing worktree must include a detail message")
	}
}

func TestPlanResumeCrossAgentRedispatchesReview(t *testing.T) {
	h := NewEpicHarness(t)
	defer h.Cleanup()

	be := backend.NewBdCliBackend(h.RepoPath)

	if err := be.MarkTerminal("a", "done", "implemented by agent-1", h.RepoPath); err != nil {
		t.Fatalf("mark a done: %v", err)
	}
	if err := be.Update("b", backend.UpdateBeadInput{State: "ready_for_review"}, h.RepoPath); err != nil {
		t.Fatalf("set b ready_for_review: %v", err)
	}

	rsPath := t.TempDir() + "/runstate.db"
	rs, err := runstate.Open(rsPath)
	if err != nil {
		t.Fatalf("open runstate: %v", err)
	}
	defer rs.Close()

	rs.RecordAgent("b", "implementing", runstate.AgentRecord{
		AgentID:   "opencode",
		SessionID: "term-review",
		Status:    "running",
	})

	ep, err := epic.LoadEpic(be, "epic-1", h.RepoPath)
	if err != nil {
		t.Fatalf("load epic: %v", err)
	}

	plan := epic.PlanResume(be, rs, ep, h.RepoPath)

	if plan.Action("b") != epic.ResumeFreshDispatch {
		t.Errorf("b in ready_for_review without active agent record should re-dispatch fresh (cross-agent review), got %s", plan.Action("b"))
	}
}
