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

type conflictTestBackend struct {
	*fakeBeadBackend
}

func newConflictTestBackend() *conflictTestBackend {
	return &conflictTestBackend{fakeBeadBackend: newFakeBeadBackend()}
}

func (b *conflictTestBackend) Update(id string, input backend.UpdateBeadInput, _ string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	bead, ok := b.beads[id]
	if !ok {
		return nil
	}
	if input.State != "" {
		bead.State = input.State
	}
	if input.Description != "" {
		bead.Description = input.Description
	}
	b.beads[id] = bead
	return nil
}

func (b *conflictTestBackend) Close(id, reason string, _ string) (*backend.TerminalState, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	bead, ok := b.beads[id]
	if !ok {
		return nil, nil
	}
	bead.State = "closed"
	b.beads[id] = bead
	return &backend.TerminalState{State: "closed", Reason: reason}, nil
}

type conflictMergeManager struct {
	mu          sync.Mutex
	routeCalls  int
	tryCalls    int
	lastOutcome merge.Outcome
}

func (m *conflictMergeManager) TryTrigger(epicID string) error {
	m.mu.Lock()
	m.tryCalls++
	m.mu.Unlock()
	return nil
}

func (m *conflictMergeManager) RouteOutcome(epicID string) error {
	m.mu.Lock()
	m.routeCalls++
	m.lastOutcome = merge.OutcomeSuccess
	m.mu.Unlock()
	return nil
}

func (m *conflictMergeManager) TryTriggerCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tryCalls
}

func (m *conflictMergeManager) DispatchMerger(epicID string) error {
	return nil
}

func (m *conflictMergeManager) RouteOutcomeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.routeCalls
}

func TestEpicRunConflictAndMergeRecovery(t *testing.T) {
	epicID := "epic-1"

	be := newConflictTestBackend()
	be.add(backend.Bead{ID: epicID, Type: "epic", State: "open", Description: "merge_conflict_at: some/file.go"})
	be.add(backend.Bead{ID: "a", Type: "task", State: "awaiting_integration", ParentID: epicID})
	be.add(backend.Bead{ID: "b", Type: "task", State: "awaiting_integration", ParentID: epicID})
	be.add(backend.Bead{ID: "c", Type: "task", State: "awaiting_integration", ParentID: epicID})
	be.add(backend.Bead{ID: "d", Type: "task", State: "awaiting_integration", ParentID: epicID})

	mm := &conflictMergeManager{}
	scm := session.NewSessionConnectionManager(&conflictSessionProvider{}, nil)
	driver := app.NewSessionDriver(app.DriverDeps{
		Backend: be,
		Spawn:   fakeConflictSpawn,
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

	be.Update(epicID, backend.UpdateBeadInput{
		State:       "blocked",
		Description: "merge_conflict_at: some/file.go",
	}, "")

	epicBead, ok := be.get(epicID)
	if !ok {
		t.Fatal("epic bead not found")
	}
	if epicBead.State != "blocked" {
		t.Fatalf("epic state = %q, want blocked", epicBead.State)
	}

	for _, childID := range []string{"a", "b", "c", "d"} {
		b, ok := be.get(childID)
		if !ok {
			t.Fatalf("child %s not found", childID)
		}
		if b.State != "awaiting_integration" {
			t.Errorf("child %s state = %q, want awaiting_integration", childID, b.State)
		}
	}

	errMerge := testApp.EpicMerge(epicID)
	if errMerge != nil {
		t.Fatalf("epic merge: %v", errMerge)
	}

	epicBeadAfter, ok := be.get(epicID)
	if !ok {
		t.Fatal("epic bead not found after merge")
	}
	if epicBeadAfter.State != "in_progress" {
		t.Errorf("epic state after merge = %q, want in_progress (description cleared, awaiting dispatch)", epicBeadAfter.State)
	}
}
