package epic

import (
	"context"
	"fmt"
	"sync"
	"time"
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
	MaxConcurrent int
	Emit          func(EpicEvent)
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
				return nil
			}
			if ex.failFast {
				ex.mu.Unlock()
				return fmt.Errorf("KERNL DISPATCH FAILURE: epic %s blocked after bead failure — Fix: review failed beads and re-run", ex.deps.Epic.ID)
			}
			ex.state = EpicFailed
			ex.mu.Unlock()
			return fmt.Errorf("KERNL DISPATCH FAILURE: deadlock in epic %s — ReadySet returned no beads but %d/%d children are done — Fix: check the DAG for missing dependencies or cycles", ex.deps.Epic.ID, len(ex.done), len(ex.deps.Epic.Children))
		}
		ex.mu.Unlock()

		if err := ex.processWave(ctx, ready); err != nil {
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
