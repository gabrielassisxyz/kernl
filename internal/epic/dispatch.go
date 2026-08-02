package epic

import (
	"context"
	"fmt"
	"time"
)

// dispatchLoop drives continuous bead dispatch: whenever a bead finishes, the
// ready set is recomputed and anything it unblocked launches immediately,
// instead of waiting for the rest of an earlier round to finish first. Wall
// clock then tracks the slowest dependency chain, not the slowest bead per
// round. failErr, once set by a bead failure, stops new dispatch but the
// loop keeps collecting already in-flight beads until every one has
// reported back - a coding agent mid-work is never killed for committed but
// unpushed work.
func (ex *Executor) dispatchLoop(ctx context.Context) error {
	ch := make(chan beadResult, len(ex.deps.Epic.Children))
	launched := 0
	collected := 0
	var failErr error

	for {
		if failErr == nil {
			n, err := ex.launchReady(ctx, ch)
			if err != nil {
				return err
			}
			launched += n
			if n > 0 {
				ex.emit(EpicEvent{Type: WaveAdvanced, EpicID: ex.deps.Epic.ID, Time: time.Now().Unix()})
			}
		}

		if collected == launched {
			return ex.terminalResult(failErr)
		}

		r := <-ch
		collected++
		ex.mu.Lock()
		ex.tracker.Finished(r.beadID)
		ex.mu.Unlock()

		if failErr != nil {
			continue // draining: an earlier failure already stopped new dispatch
		}
		if err := ex.recordResult(r); err != nil {
			failErr = err
		}
	}
}

// launchReady dispatches every currently-ready bead that has not already
// been dispatched. It runs once up front and again after every completion,
// which is what turns per-wave dispatch into continuous dispatch. The
// dispatched check is required because DAG.ReadySet reports a bead as ready
// for as long as it is not yet in "done" - including one whose goroutine is
// already running - so without it a still-running bead would be launched a
// second time.
func (ex *Executor) launchReady(ctx context.Context, ch chan beadResult) (int, error) {
	ex.mu.Lock()
	ready := ex.deps.Epic.DAG.ReadySet(ex.done)
	toLaunch := make([]string, 0, len(ready))
	for _, id := range ready {
		if !ex.dispatched[id] {
			toLaunch = append(toLaunch, id)
		}
	}
	for _, id := range toLaunch {
		ex.dispatched[id] = true
	}
	ex.mu.Unlock()

	for _, beadID := range toLaunch {
		if err := ex.launchBead(ctx, beadID, ch); err != nil {
			return 0, err
		}
	}
	return len(toLaunch), nil
}

// launchBead resolves the bead's worktree, acquires a semaphore slot, and
// spawns the goroutine that runs it.
func (ex *Executor) launchBead(ctx context.Context, beadID string, ch chan beadResult) error {
	var sid string
	if ex.deps.SessionResumes != nil {
		sid = ex.deps.SessionResumes[beadID]
	}

	var wtPath string
	var err error
	// Reuse a cached worktree ONLY for a genuine session resume. A fresh
	// dispatch must always go through Add, which cleans any leftover
	// worktree and merges in dependency branches - reusing a stale worktree
	// here would silently skip that merge and leave a dependent child
	// without its dependency's committed code.
	if sid != "" && ex.deps.GetWorktree != nil {
		if p, ok := ex.deps.GetWorktree(ex.deps.Epic.ID, beadID); ok {
			wtPath = p
		}
	}
	if wtPath == "" {
		wtPath, err = ex.deps.Worktree.Add(ex.deps.Epic.ID, beadID, ex.deps.Epic.DAG.DependenciesOf(beadID))
		if err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: cannot create worktree for bead %s in epic %s -- %w -- Fix: verify the worktree root is writable", beadID, ex.deps.Epic.ID, err)
		}
	}

	select {
	case ex.sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}

	// Counted here rather than where the bead is marked dispatched, because a
	// bead queued behind the semaphore is not running and must not be reported
	// as if it were. Continuous dispatch hands launchReady the whole ready set
	// at once, so counting on dispatch would report a peak above
	// maxConcurrentBeads - a figure the run prints and that cannot happen.
	ex.mu.Lock()
	ex.tracker.Started(beadID)
	ex.mu.Unlock()

	go func(id string, path string, sessionID string) {
		defer func() { <-ex.sem }()
		result, runErr := ex.deps.RunBead(ctx, RunInput{BeadID: id, Worktree: path, SessionID: sessionID})
		ch <- beadResult{beadID: id, result: result, err: runErr}
	}(beadID, wtPath, sid)

	return nil
}

// recordResult marks a successful bead done and emits its state change, or
// triggers fail-fast handling for an error/unsuccessful result. It returns
// the fail-fast error, if any, for dispatchLoop to latch.
func (ex *Executor) recordResult(r beadResult) error {
	if r.err != nil {
		ex.handleBeadFailure(r)
		return fmt.Errorf("KERNL DISPATCH FAILURE: bead %s in epic %s returned error - %w", r.beadID, ex.deps.Epic.ID, r.err)
	}
	if !r.result.Success {
		ex.handleBeadFailure(r)
		return fmt.Errorf("KERNL DISPATCH FAILURE: bead %s in epic %s failed - final state %q", r.beadID, ex.deps.Epic.ID, r.result.FinalState)
	}

	ex.mu.Lock()
	ex.done[r.beadID] = true
	ex.mu.Unlock()

	ex.emit(EpicEvent{
		Type:      BeadStateChanged,
		EpicID:    ex.deps.Epic.ID,
		BeadID:    r.beadID,
		SessionID: r.result.SessionID,
		Detail:    r.result.FinalState,
		Time:      time.Now().Unix(),
	})
	return nil
}

// handleBeadFailure marks the epic blocked. The "stop dispatching" half lives
// in dispatchLoop's own failErr, so there is no second copy of that fact on the
// Executor to fall out of step with it.
func (ex *Executor) handleBeadFailure(r beadResult) {
	ex.mu.Lock()
	ex.state = EpicBlocked
	ex.mu.Unlock()

	ex.emit(EpicEvent{
		Type:      BeadStateChanged,
		EpicID:    ex.deps.Epic.ID,
		BeadID:    r.beadID,
		SessionID: r.result.SessionID,
		Detail:    fmt.Sprintf("blocked: %s", r.result.FinalState),
		Time:      time.Now().Unix(),
	})
}

// terminalResult is reached once nothing is in flight and dispatch produced
// no new beads: either a prior failure is now fully drained, every child is
// done, or the graph is stuck with no failure to explain it (deadlock).
func (ex *Executor) terminalResult(failErr error) error {
	if failErr != nil {
		return failErr
	}

	ex.mu.Lock()
	doneCount := len(ex.done)
	total := len(ex.deps.Epic.Children)
	if doneCount == total {
		ex.state = EpicCompleted
		ex.mu.Unlock()
		return nil
	}
	ex.state = EpicFailed
	ex.mu.Unlock()

	msg := fmt.Sprintf("deadlock in epic %s: %d/%d children done", ex.deps.Epic.ID, doneCount, total)
	return fmt.Errorf("KERNL DISPATCH FAILURE: %s - Fix: check the DAG for missing dependencies or cycles", msg)
}
