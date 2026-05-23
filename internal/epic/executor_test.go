package epic

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

func diamondEpic(t *testing.T) *Epic {
	t.Helper()
	nodes := []Node{
		{ID: "a", DependsOn: []string{}},
		{ID: "b", DependsOn: []string{"a"}},
		{ID: "c", DependsOn: []string{"a"}},
		{ID: "d", DependsOn: []string{"b", "c"}},
	}
	dag, err := NewDAG(nodes)
	if err != nil {
		t.Fatalf("NewDAG: %v", err)
	}
	children := make([]backend.Bead, 4)
	for i, id := range []string{"a", "b", "c", "d"} {
		children[i] = backend.Bead{ID: id}
	}
	return &Epic{ID: "epic-1", DAG: dag, Children: children}
}

type fakeWorktree struct {
	added map[string]string
}

func newFakeWorktree() *fakeWorktree {
	return &fakeWorktree{added: make(map[string]string)}
}

func (f *fakeWorktree) Add(epicID, beadID string) (string, error) {
	path := "/tmp/" + epicID + "/" + beadID
	f.added[epicID+"/"+beadID] = path
	return path, nil
}

func fakeWT() *fakeWorktree {
	return newFakeWorktree()
}

func wideEpic(t *testing.T, n int) *Epic {
	t.Helper()
	nodes := make([]Node, n)
	children := make([]backend.Bead, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("w%d", i)
		nodes[i] = Node{ID: id}
		children[i] = backend.Bead{ID: id}
	}
	dag, err := NewDAG(nodes)
	if err != nil {
		t.Fatalf("NewDAG: %v", err)
	}
	return &Epic{ID: "wide-epic", DAG: dag, Children: children}
}

func TestExecutorFailFastOnTerminalChildFailure(t *testing.T) {
	ep := diamondEpic(t)
	runBead := func(ctx context.Context, in RunInput) (RunResult, error) {
		if in.BeadID == "b" {
			return RunResult{FinalState: "blocked", Success: false}, nil
		}
		return RunResult{FinalState: "done", Success: true}, nil
	}
	ex := NewExecutor(ExecutorDeps{Epic: ep, RunBead: runBead, Worktree: fakeWT(), MaxConcurrent: 5})
	err := ex.Run(context.Background())
	if err == nil {
		t.Fatal("expected fail-fast error when child b fails terminally")
	}
	if ex.State() != EpicBlocked {
		t.Errorf("epic state = %v, want blocked", ex.State())
	}
	if ex.Dispatched("d") {
		t.Error("d must not be dispatched after b failed")
	}
}

func TestExecutorSemaphoreCapsConcurrency(t *testing.T) {
	ep := wideEpic(t, 10)
	var mu sync.Mutex
	var concurrent, peak int
	runBead := func(ctx context.Context, in RunInput) (RunResult, error) {
		mu.Lock()
		concurrent++
		if concurrent > peak {
			peak = concurrent
		}
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		concurrent--
		mu.Unlock()
		return RunResult{FinalState: "done", Success: true}, nil
	}
	ex := NewExecutor(ExecutorDeps{Epic: ep, RunBead: runBead, Worktree: fakeWT(), MaxConcurrent: 3})
	ex.Run(context.Background())
	if peak > 3 {
		t.Errorf("peak %d exceeded MaxConcurrent 3", peak)
	}
}

func TestExecutorRunsIndependentChildrenConcurrently(t *testing.T) {
	ep := diamondEpic(t)
	var mu sync.Mutex
	var concurrent, peak int
	runBead := func(ctx context.Context, in RunInput) (RunResult, error) {
		mu.Lock()
		concurrent++
		if concurrent > peak {
			peak = concurrent
		}
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		concurrent--
		mu.Unlock()
		return RunResult{FinalState: "done", Success: true}, nil
	}
	ex := NewExecutor(ExecutorDeps{Epic: ep, RunBead: runBead, Worktree: newFakeWorktree(), MaxConcurrent: 5})
	if err := ex.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if peak < 2 {
		t.Errorf("b and c should run concurrently, peak = %d", peak)
	}
}

// TestExecutorResumesSession passes the session ID through RunInput to the
// bead worker when SessionResumes is wired.
func TestExecutorResumesSession(t *testing.T) {
	ep := wideEpic(t, 1)
	receivedSID := ""
	runBead := func(ctx context.Context, in RunInput) (RunResult, error) {
		receivedSID = in.SessionID
		return RunResult{FinalState: "done", Success: true}, nil
	}
	ex := NewExecutor(ExecutorDeps{
		Epic:           ep,
		RunBead:        runBead,
		Worktree:       fakeWT(),
		MaxConcurrent:  1,
		SessionResumes: map[string]string{"w0": "ses-abc123"},
	})
	if err := ex.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if receivedSID != "ses-abc123" {
		t.Errorf("expected session ID ses-abc123, got %q", receivedSID)
	}
}

// TestExecutorReusesWorktreePath verifies that GetWorktree is consulted before
// creating a fresh worktree.
func TestExecutorReusesWorktreePath(t *testing.T) {
	ep := wideEpic(t, 1)
	wt := fakeWT()
	reused := false
	getWT := func(epicID, beadID string) (string, bool) {
		if beadID == "w0" {
			reused = true
			return "/already/exists/w0", true
		}
		return "", false
	}
	runBead := func(ctx context.Context, in RunInput) (RunResult, error) {
		if in.Worktree != "/already/exists/w0" {
			t.Errorf("expected reused worktree, got %s", in.Worktree)
		}
		return RunResult{FinalState: "done", Success: true}, nil
	}
	ex := NewExecutor(ExecutorDeps{
		Epic:          ep,
		RunBead:       runBead,
		Worktree:      wt,
		GetWorktree:   getWT,
		MaxConcurrent: 1,
	})
	if err := ex.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !reused {
		t.Error("expected GetWorktree to be called")
	}
	if _, called := wt.added["wide-epic/w0"]; called {
		t.Error("worktree Add should NOT have been called when GetWorktree returned a path")
	}
}
