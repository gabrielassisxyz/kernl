package epic

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/merge"
)

type EpicState string

const (
	EpicRunning   EpicState = "running"
	EpicCompleted EpicState = "completed"
	EpicFailed    EpicState = "failed"
	EpicBlocked   EpicState = "blocked"
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
	// GetWorktree is an optional hook to reuse an existing worktree
	// path from a prior run (e.g. for session resume). Returns
	// (path, true) when the path is known and still exists.
	GetWorktree   func(epicID, beadID string) (string, bool)
	MaxConcurrent int
	Emit          func(EpicEvent)
	MergeManager  merge.TriggerRouter
}

type Executor struct {
	deps       ExecutorDeps
	done       map[string]bool
	dispatched map[string]bool
	state      EpicState
	tracker    *ParallelismTracker
	sem        chan struct{}
	failFast   bool
	mu         sync.Mutex
}

func NewExecutor(deps ExecutorDeps) *Executor {
	mc := deps.MaxConcurrent
	if mc <= 0 {
		mc = 1
	}
	return &Executor{
		deps:       deps,
		done:       make(map[string]bool),
		dispatched: make(map[string]bool),
		state:      EpicRunning,
		tracker:    NewParallelismTracker(len(deps.Epic.Children)),
		sem:        make(chan struct{}, mc),
	}
}

// NewExecutorWithDoneSet creates an executor pre-populated with a "done"
// set, e.g. from a ResumePlan that marks terminal or human-gated beads as
// already completed.
func NewExecutorWithDoneSet(deps ExecutorDeps, doneSet map[string]bool) *Executor {
	ex := NewExecutor(deps)
	for k, v := range doneSet {
		if v {
			ex.done[k] = true
		}
	}
	return ex
}

type beadResult struct {
	beadID string
	result RunResult
	err    error
}

func (ex *Executor) Run(ctx context.Context) error {
	ex.emit(EpicEvent{
		Type:   SessionStarted,
		EpicID: ex.deps.Epic.ID,
		Time:   time.Now().Unix(),
	})

	for {
		ex.mu.Lock()
		ready := ex.deps.Epic.DAG.ReadySet(ex.done)
		if len(ready) == 0 {
			if len(ex.done) == len(ex.deps.Epic.Children) {
				ex.state = EpicCompleted
				ex.mu.Unlock()
			if ex.deps.MergeManager != nil {
				_ = ex.deps.MergeManager.RouteOutcome(ex.deps.Epic.ID)
			}
				return nil
			}
			var msg string
			if ex.failFast {
				msg = fmt.Sprintf("epic %s blocked after bead failure", ex.deps.Epic.ID)
			} else {
				ex.state = EpicFailed
				msg = fmt.Sprintf("deadlock in epic %s: %d/%d children done", ex.deps.Epic.ID, len(ex.done), len(ex.deps.Epic.Children))
			}
			ex.mu.Unlock()
			ex.emit(EpicEvent{
				Type:   SessionError,
				EpicID: ex.deps.Epic.ID,
				Detail: msg,
				Time:   time.Now().Unix(),
			})
			if ex.failFast {
				return fmt.Errorf("KERNL DISPATCH FAILURE: %s — Fix: review failed beads and re-run", msg)
			}
			return fmt.Errorf("KERNL DISPATCH FAILURE: %s — Fix: check the DAG for missing dependencies or cycles", msg)
		}
		ex.mu.Unlock()

		if err := ex.processWave(ctx, ready); err != nil {
			ex.emit(EpicEvent{
				Type:   SessionError,
				EpicID: ex.deps.Epic.ID,
				Detail: err.Error(),
				Time:   time.Now().Unix(),
			})
			return err
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

func (ex *Executor) Dispatched(id string) bool {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	return ex.dispatched[id]
}

func (ex *Executor) Parallelism() ParallelismMetric {
	return ex.tracker.Metric()
}

func (ex *Executor) emit(event EpicEvent) {
	if ex.deps.Emit != nil {
		ex.deps.Emit(event)
	}
}
