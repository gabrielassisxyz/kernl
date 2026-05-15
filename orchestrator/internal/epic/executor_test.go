package epic

import (
	"context"
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
