package epic

import (
	"context"
	"fmt"
	"time"
)

func (ex *Executor) processWave(ctx context.Context, ready []string) error {
	ex.emit(EpicEvent{
		Type:   WaveAdvanced,
		EpicID: ex.deps.Epic.ID,
		Time:   time.Now().Unix(),
	})

	ch := make(chan beadResult, len(ready))
	launched := 0

	for _, beadID := range ready {
		ex.mu.Lock()
		ex.dispatched[beadID] = true
		ex.tracker.Started(beadID)
		ex.mu.Unlock()

		var wtPath string
		var err error
		if ex.deps.GetWorktree != nil {
			if p, ok := ex.deps.GetWorktree(ex.deps.Epic.ID, beadID); ok {
				wtPath = p
			}
		}
		if wtPath == "" {
			wtPath, err = ex.deps.Worktree.Add(ex.deps.Epic.ID, beadID)
			if err != nil {
				return fmt.Errorf("KERNL DISPATCH FAILURE: cannot create worktree for bead %s in epic %s -- %w -- Fix: verify the worktree root is writable", beadID, ex.deps.Epic.ID, err)
			}
		}

		select {
		case ex.sem <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}

		var sid string
		if ex.deps.SessionResumes != nil {
			sid = ex.deps.SessionResumes[beadID]
		}
		launched++
		go func(id string, path string, sessionID string) {
			defer func() { <-ex.sem }()
			result, err := ex.deps.RunBead(ctx, RunInput{BeadID: id, Worktree: path, SessionID: sessionID})
			ch <- beadResult{beadID: id, result: result, err: err}
		}(beadID, wtPath, sid)
	}

	collected := 0
	for collected < launched {
		r := <-ch
		collected++

		ex.mu.Lock()
		ex.tracker.Finished(r.beadID)
		ex.mu.Unlock()

		if r.err != nil {
			ex.handleBeadFailure(r)
			ex.drainRemaining(ch, launched, collected)
			return fmt.Errorf("KERNL DISPATCH FAILURE: bead %s in epic %s returned error — %w", r.beadID, ex.deps.Epic.ID, r.err)
		}
		if !r.result.Success {
			ex.handleBeadFailure(r)
			ex.drainRemaining(ch, launched, collected)
			return fmt.Errorf("KERNL DISPATCH FAILURE: bead %s in epic %s failed — final state %q", r.beadID, ex.deps.Epic.ID, r.result.FinalState)
		}

		ex.mu.Lock()
		ex.done[r.beadID] = true
		epicBlocked := ex.state == EpicBlocked
		ex.mu.Unlock()

		ex.emit(EpicEvent{
			Type:   BeadStateChanged,
			EpicID: ex.deps.Epic.ID,
			BeadID: r.beadID,
			Detail: r.result.FinalState,
			Time:   time.Now().Unix(),
		})

		if !epicBlocked && r.result.FinalState == "awaiting_integration" && ex.deps.MergeManager != nil {
			_ = ex.deps.MergeManager.TryTrigger(ex.deps.Epic.ID)
		}
	}

	return nil
}

func (ex *Executor) handleBeadFailure(r beadResult) {
	ex.mu.Lock()
	ex.failFast = true
	ex.state = EpicBlocked
	ex.mu.Unlock()

	ex.emit(EpicEvent{
		Type:   BeadStateChanged,
		EpicID: ex.deps.Epic.ID,
		BeadID: r.beadID,
		Detail: fmt.Sprintf("blocked: %s", r.result.FinalState),
		Time:   time.Now().Unix(),
	})
}

func (ex *Executor) drainRemaining(ch chan beadResult, launched, collected int) {
	for collected < launched {
		rr := <-ch
		collected++
		ex.mu.Lock()
		ex.tracker.Finished(rr.beadID)
		ex.mu.Unlock()
	}
}
