//go:build integration

package integration

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/epic"
	"github.com/gabrielassisxyz/kernl/internal/merge"
	"github.com/gabrielassisxyz/kernl/internal/session"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

type pushFailBackend struct {
	*fakeBeadBackend
}

func newPushFailBackend() *pushFailBackend {
	return &pushFailBackend{fakeBeadBackend: newFakeBeadBackend()}
}

func (p *pushFailBackend) Update(id string, input backend.UpdateBeadInput, _ string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	b, ok := p.beads[id]
	if !ok {
		return nil
	}
	if input.State != "" {
		b.State = input.State
	}
	if input.Description != "" {
		b.Description = input.Description
	}
	p.beads[id] = b
	return nil
}

func (p *pushFailBackend) Close(id, reason string, _ string) (*backend.TerminalState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	b, ok := p.beads[id]
	if !ok {
		return nil, nil
	}
	b.State = "closed"
	p.beads[id] = b
	return &backend.TerminalState{State: "closed", Reason: reason}, nil
}

type pushFailMergeRouter struct {
	mu        sync.Mutex
	be        *pushFailBackend
	routeCall int
	mergeCall int
}

func (m *pushFailMergeRouter) TryTrigger(epicID string) error {
	return nil
}

func (m *pushFailMergeRouter) DispatchMerger(epicID string) error {
	m.mu.Lock()
	m.mergeCall++
	m.mu.Unlock()
	return nil
}

func (m *pushFailMergeRouter) RouteOutcome(epicID string) error {
	m.mu.Lock()
	m.routeCall++
	call := m.routeCall
	m.mu.Unlock()

	recovered := false
	if call == 1 {
		return m.be.Update(epicID, backend.UpdateBeadInput{
			State:       "blocked",
			Description: "merge_outcome: push_failed",
		}, "")
	}

	bead, err := m.be.Get(epicID, "")
	if err != nil || bead == nil {
		return err
	}
	outcomeStr := workflow.GetMergeOutcome(bead.Description)
	if outcomeStr == string(merge.OutcomeSuccess) {
		recovered = true
		children, _ := m.be.List(&backend.BeadListFilters{Parent: epicID}, "")
		for _, c := range children {
			m.be.Close(c.ID, "merged", "")
		}
		m.be.Update(epicID, backend.UpdateBeadInput{
			State: "awaiting_pr_review",
		}, "")
	}

	if !recovered {
		return m.be.Update(epicID, backend.UpdateBeadInput{
			State: "blocked",
		}, "")
	}
	return nil
}

type pushFailSpawnProcess struct{}

func (p *pushFailSpawnProcess) Wait() error { return nil }
func (p *pushFailSpawnProcess) Kill() error { return nil }

type pushFailSP struct{}

func (p *pushFailSP) GetSessionEntry(id string) (session.SessionInfo, bool) {
	return session.SessionInfo{}, false
}
func (p *pushFailSP) ListSessionIDs() []session.SessionInfo { return nil }
func (p *pushFailSP) PushEvent(id string, evt session.TerminalEvent) {}

func pushFailSpawn(ctx context.Context, c string, a []string, wd string, env []string) (app.Process, io.Reader, io.Reader, error) {
	return &pushFailSpawnProcess{}, strings.NewReader(""), strings.NewReader(""), nil
}

func TestEpicRunPushFailAndMergeRecovery(t *testing.T) {
	epicID := "epic-1"

	be := newPushFailBackend()
	be.add(backend.Bead{ID: epicID, Type: "epic", State: "open"})
	be.add(backend.Bead{ID: "a", Type: "task", State: "awaiting_integration", ParentID: epicID})
	be.add(backend.Bead{ID: "b", Type: "task", State: "awaiting_integration", ParentID: epicID})
	be.add(backend.Bead{ID: "c", Type: "task", State: "awaiting_integration", ParentID: epicID})
	be.add(backend.Bead{ID: "d", Type: "task", State: "awaiting_integration", ParentID: epicID})

	mm := &pushFailMergeRouter{be: be}
	scm := session.NewSessionConnectionManager(&pushFailSP{}, nil)
	driver := app.NewSessionDriver(app.DriverDeps{
		Backend: be,
		Spawn:   pushFailSpawn,
		SCM:     scm,
	})

	cfg := &config.Config{
		Registry: config.RegistryConfig{
			Repos: []config.RepoEntry{{Path: "test-repo"}},
		},
		Orchestrator: config.OrchestratorConfig{
			WorktreeRoot:      t.TempDir(),
			MaxConcurrentBeads: 2,
		},
	}

	testApp := &app.App{
		Backend:      be,
		Driver:       driver,
		MergeManager: mm,
		Config:       cfg,
		EpicEvents:   epic.NewEpicEventHub(),
	}

	ep, err := epic.LoadEpic(be, epicID, "")
	if err != nil {
		t.Fatalf("load epic: %v", err)
	}

	wm := epic.NewWorktreeManager(t.TempDir(), "", nil, nil)
	ex := epic.NewExecutor(epic.ExecutorDeps{
		Epic: ep,
		RunBead: func(ctx context.Context, in epic.RunInput) (epic.RunResult, error) {
			bead, gErr := be.Get(in.BeadID, "")
			if gErr != nil || bead == nil {
				return epic.RunResult{FinalState: "error", Success: false}, gErr
			}
			return epic.RunResult{FinalState: "done", Success: true}, nil
		},
		Worktree:      wm,
		MaxConcurrent: 2,
		MergeManager:  mm,
		Emit:          func(ev epic.EpicEvent) {},
	})

	if err := ex.Run(context.Background()); err != nil {
		t.Fatalf("executor run: %v", err)
	}

	if ex.State() != epic.EpicCompleted {
		t.Fatalf("epic state = %v, want completed", ex.State())
	}

	mm.mu.Lock()
	routeCalls := mm.routeCall
	mm.mu.Unlock()
	if routeCalls != 1 {
		t.Fatalf("RouteOutcome called %d times, want 1", routeCalls)
	}

	epicBead, ok := be.get(epicID)
	if !ok {
		t.Fatal("epic bead not found after run")
	}
	if epicBead.State != "blocked" {
		t.Fatalf("epic state = %q, want blocked", epicBead.State)
	}
	if workflow.GetMergeOutcome(epicBead.Description) != "push_failed" {
		t.Fatalf("merge_outcome = %q, want push_failed", workflow.GetMergeOutcome(epicBead.Description))
	}

	for _, cid := range []string{"a", "b", "c", "d"} {
		b, _ := be.get(cid)
		if b.State != "awaiting_integration" {
			t.Fatalf("child %s state = %q, want awaiting_integration", cid, b.State)
		}
	}

	if err := testApp.EpicMerge(epicID); err != nil {
		t.Fatalf("epic merge: %v", err)
	}

	epicInProg, _ := be.get(epicID)
	if epicInProg.State != "in_progress" {
		t.Fatalf("epic state after merge = %q, want in_progress", epicInProg.State)
	}

	mm.mu.Lock()
	mergeCalls := mm.mergeCall
	mm.mu.Unlock()
	if mergeCalls != 1 {
		t.Fatalf("DispatchMerger called %d times, want 1", mergeCalls)
	}

	be.Update(epicID, backend.UpdateBeadInput{
		Description: "merge_outcome: success\npr_url: https://x/pr/1",
	}, "")

	if err := mm.RouteOutcome(epicID); err != nil {
		t.Fatalf("route outcome after merge: %v", err)
	}

	epicBeadAfter, ok2 := be.get(epicID)
	if !ok2 {
		t.Fatal("epic bead not found after merge recovery")
	}
	if epicBeadAfter.State != "awaiting_pr_review" {
		t.Fatalf("epic state after merge = %q, want awaiting_pr_review", epicBeadAfter.State)
	}

	for _, cid := range []string{"a", "b", "c", "d"} {
		b, _ := be.get(cid)
		if b.State != "closed" {
			t.Fatalf("child %s state = %q, want closed", cid, b.State)
		}
	}
}
