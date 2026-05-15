package epic

import (
	"context"
	"fmt"
	"sync"
)

type EpicState string

const (
	EpicRunning   EpicState = "running"
	EpicCompleted EpicState = "completed"
	EpicFailed    EpicState = "failed"
)

type RunInput struct {
	BeadID   string
	Worktree string
}

type RunResult struct {
	FinalState string
	Success    bool
}

type worktreeAdder interface {
	Add(epicID, beadID string) (string, error)
}

type ExecutorDeps struct {
	Epic          *Epic
	RunBead       func(ctx context.Context, in RunInput) (RunResult, error)
	Worktree      worktreeAdder
	MaxConcurrent int
}

type Executor struct {
	deps  ExecutorDeps
	done  map[string]bool
	state EpicState
	mu    sync.Mutex
}

func NewExecutor(deps ExecutorDeps) *Executor {
	return &Executor{
		deps:  deps,
		done:  make(map[string]bool),
		state: EpicRunning,
	}
}

type beadResult struct {
	beadID string
	result RunResult
	err    error
}

func (ex *Executor) Run(ctx context.Context) error {
	for {
		ex.mu.Lock()
		ready := ex.deps.Epic.DAG.ReadySet(ex.done)
		if len(ready) == 0 {
			if len(ex.done) == len(ex.deps.Epic.Children) {
				ex.state = EpicCompleted
				ex.mu.Unlock()
				return nil
			}
			ex.state = EpicFailed
			ex.mu.Unlock()
			return fmt.Errorf("KERNL DISPATCH FAILURE: deadlock in epic %s — ReadySet returned no beads but %d/%d children are done — Fix: check the DAG for missing dependencies or cycles", ex.deps.Epic.ID, len(ex.done), len(ex.deps.Epic.Children))
		}
		ex.mu.Unlock()

		ch := make(chan beadResult, len(ready))
		for _, beadID := range ready {
			wtPath, err := ex.deps.Worktree.Add(ex.deps.Epic.ID, beadID)
			if err != nil {
				return fmt.Errorf("KERNL DISPATCH FAILURE: cannot create worktree for bead %s in epic %s — %w — Fix: verify the worktree root is writable", beadID, ex.deps.Epic.ID, err)
			}
			go func(id string, path string) {
				result, err := ex.deps.RunBead(ctx, RunInput{BeadID: id, Worktree: path})
				ch <- beadResult{beadID: id, result: result, err: err}
			}(beadID, wtPath)
		}

		for range ready {
			r := <-ch
			if r.err != nil {
				ex.mu.Lock()
				ex.state = EpicFailed
				ex.mu.Unlock()
				return fmt.Errorf("KERNL DISPATCH FAILURE: bead %s in epic %s returned error — %w", r.beadID, ex.deps.Epic.ID, r.err)
			}
			if !r.result.Success {
				ex.mu.Lock()
				ex.state = EpicFailed
				ex.mu.Unlock()
				return fmt.Errorf("KERNL DISPATCH FAILURE: bead %s in epic %s failed — final state %q", r.beadID, ex.deps.Epic.ID, r.result.FinalState)
			}
			ex.mu.Lock()
			ex.done[r.beadID] = true
			ex.mu.Unlock()
		}
	}
}

func (ex *Executor) DoneSet() map[string]bool {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	result := make(map[string]bool, len(ex.done))
	for k, v := range ex.done {
		result[k] = v
	}
	return result
}

func (ex *Executor) State() EpicState {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	return ex.state
}

func (ex *Executor) Emit(event EpicEvent) {}
