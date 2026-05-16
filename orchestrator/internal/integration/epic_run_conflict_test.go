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
)

type conflictTestProcess struct{}

func (p *conflictTestProcess) Wait() error { return nil }
func (p *conflictTestProcess) Kill() error { return nil }

type conflictSessionProvider struct{}

func (p *conflictSessionProvider) GetSessionEntry(id string) (session.SessionInfo, bool) {
	return session.SessionInfo{}, false
}
func (p *conflictSessionProvider) ListSessionIDs() []session.SessionInfo { return nil }
func (p *conflictSessionProvider) PushEvent(id string, evt session.TerminalEvent) {}

func fakeConflictSpawn(ctx context.Context, cmd string, args []string, cwd string, env []string) (app.Process, io.Reader, io.Reader, error) {
	return &conflictTestProcess{}, strings.NewReader(""), strings.NewReader(""), nil
}

type conflictMergeManager struct {
	mu          sync.Mutex
	routeCalls  int
	tryCalls    int
	lastOutcome merge.Outcome
}

func (m *conflictMergeManager) TryTrigger(epicID string) {
	m.mu.Lock()
	m.tryCalls++
	m.mu.Unlock()
}

func (m *conflictMergeManager) RouteOutcome(epicID string) {
	m.mu.Lock()
	m.routeCalls++
	m.lastOutcome = merge.OutcomeSuccess
	m.mu.Unlock()
}

func (m *conflictMergeManager) TryTriggerCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tryCalls
}

func (m *conflictMergeManager) RouteOutcomeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.routeCalls
}

func TestEpicRunConflictAndMergeRecovery(t *testing.T) {
	h := NewEpicHarness(t)
	defer h.Cleanup()

	epicID := "epic-1"
	repoPath := h.RepoPath

	be := backend.NewBdCliBackend(repoPath)

	be.Update("a", backend.UpdateBeadInput{State: "awaiting_integration"}, repoPath)
	be.Update("b", backend.UpdateBeadInput{State: "awaiting_integration"}, repoPath)
	be.Update("c", backend.UpdateBeadInput{State: "awaiting_integration"}, repoPath)
	be.Update("d", backend.UpdateBeadInput{State: "awaiting_integration"}, repoPath)

	mm := &conflictMergeManager{}
	scm := session.NewSessionConnectionManager(&conflictSessionProvider{}, nil)
	driver := app.NewSessionDriver(app.DriverDeps{
		Backend: be,
		Spawn:   fakeConflictSpawn,
		SCM:     scm,
	})

	cfg := *h.Config
	cfg.Registry.Repos = []config.RepoEntry{{Path: repoPath}}
	cfg.Orchestrator.WorktreeRoot = t.TempDir()

	testApp := &app.App{
		Backend:      be,
		Driver:       driver,
		MergeManager: mm,
		Config:       &cfg,
		EpicEvents:   epic.NewEpicEventHub(),
	}

	ep, err := epic.LoadEpic(be, epicID, repoPath)
	if err != nil {
		t.Fatalf("load epic: %v", err)
	}

	wm := epic.NewWorktreeManager(t.TempDir(), repoPath, nil, nil)
	ex := epic.NewExecutor(epic.ExecutorDeps{
		Epic: ep,
		RunBead: func(ctx context.Context, in epic.RunInput) (epic.RunResult, error) {
			bead, gErr := be.Get(in.BeadID, repoPath)
			if gErr != nil || bead == nil {
				return epic.RunResult{FinalState: "error", Success: false}, gErr
			}
			return epic.RunResult{FinalState: bead.State, Success: true}, nil
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

	if mm.TryTriggerCount() < 2 {
		t.Errorf("TryTrigger called %d times, want >= 2", mm.TryTriggerCount())
	}
	if mm.RouteOutcomeCount() != 1 {
		t.Errorf("RouteOutcome called %d times, want 1", mm.RouteOutcomeCount())
	}

	if err := be.Update(epicID, backend.UpdateBeadInput{State: "blocked"}, repoPath); err != nil {
		t.Fatalf("set epic blocked: %v", err)
	}

	epicBead, err := be.Get(epicID, repoPath)
	if err != nil || epicBead == nil {
		t.Fatalf("get epic: %v", err)
	}
	if epicBead.State != "blocked" {
		t.Fatalf("epic state = %q, want blocked", epicBead.State)
	}

	for _, childID := range []string{"a", "b", "c", "d"} {
		state := h.BeadState(t, childID)
		if state != "awaiting_integration" {
			t.Errorf("child %s state = %q, want awaiting_integration", childID, state)
		}
	}

	errMerge := testApp.EpicMerge(epicID)
	if errMerge != nil {
		t.Fatalf("epic merge: %v", errMerge)
	}

	epicBeadAfter, err := be.Get(epicID, repoPath)
	if err != nil || epicBeadAfter == nil {
		t.Fatalf("get epic after merge: %v", err)
	}
	if epicBeadAfter.State != "awaiting_pr_review" {
		t.Errorf("epic state after merge = %q, want awaiting_pr_review", epicBeadAfter.State)
	}

	for _, childID := range []string{"a", "b", "c", "d"} {
		state := h.BeadState(t, childID)
		if state != "closed" {
			t.Errorf("child %s state after merge = %q, want closed", childID, state)
		}
	}
}
