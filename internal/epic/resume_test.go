package epic

import (
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/runstate"
)

type memRunStateStore struct {
	agents    map[string]map[string]runstate.AgentRecord
	worktrees map[string]string
}

func memStore(t *testing.T) *memRunStateStore {
	t.Helper()
	return &memRunStateStore{
		agents:    make(map[string]map[string]runstate.AgentRecord),
		worktrees: make(map[string]string),
	}
}

func (m *memRunStateStore) RecordAgent(beadID, state string, rec runstate.AgentRecord) {
	if m.agents[beadID] == nil {
		m.agents[beadID] = make(map[string]runstate.AgentRecord)
	}
	m.agents[beadID][state] = rec
}

func (m *memRunStateStore) AgentRecord(beadID, state string) (runstate.AgentRecord, bool) {
	if m.agents[beadID] == nil {
		return runstate.AgentRecord{}, false
	}
	rec, ok := m.agents[beadID][state]
	return rec, ok
}

func (m *memRunStateStore) SetWorktree(epicID, beadID, path string) {
	m.worktrees[epicID+"/"+beadID] = path
}

func (m *memRunStateStore) Worktree(epicID, beadID string) (string, bool) {
	path, ok := m.worktrees[epicID+"/"+beadID]
	return path, ok
}

func beActive(beadID string) *fakeBackend {
	return &fakeBackend{
		state: map[string]string{
			beadID: "implementing",
		},
	}
}

func storeWithMissingWorktree(t *testing.T, beadID string) *memRunStateStore {
	t.Helper()
	s := memStore(t)
	s.SetWorktree("epic-1", beadID, "/nonexistent/path/worktree")
	return s
}

func TestPlanResumeSkipsDoneResumesInterruptedRedispatchesGap(t *testing.T) {
	be := &fakeBackend{state: map[string]string{
		"a": "done",
		"b": "implementing",
		"c": "ready_for_implementation",
	}}
	rs := memStore(t)
	rs.RecordAgent("b", "implementing", runstate.AgentRecord{AgentID: "opencode", SessionID: "term-9", Status: "running"})

	plan := PlanResume(be, rs, diamondEpic(t), "/repo")

	if plan.Action("a") != ResumeSkip {
		t.Errorf("a should be skipped, got %s", plan.Action("a"))
	}
	if plan.Action("b") != ResumeSession || plan.SessionID("b") != "term-9" {
		t.Errorf("b should resume session term-9, got action=%s sessionID=%s", plan.Action("b"), plan.SessionID("b"))
	}
	if plan.Action("c") != ResumeFreshDispatch {
		t.Errorf("c should be a fresh dispatch, got %s", plan.Action("c"))
	}
}

func TestResumeMissingWorktreeFailsLoud(t *testing.T) {
	plan := PlanResume(beActive("b"), storeWithMissingWorktree(t, "b"), diamondEpic(t), "/repo")
	if plan.Action("b") != ResumeError || plan.Detail("b") == "" {
		t.Errorf("missing worktree must surface as ResumeError with a detail, got action=%s detail=%q", plan.Action("b"), plan.Detail("b"))
	}
}
