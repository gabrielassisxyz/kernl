package epic

import (
	"fmt"
	"os"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/runstate"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

type ResumeAction string

const (
	ResumeSkip          ResumeAction = "skip"
	ResumeSession       ResumeAction = "resume_session"
	ResumeFreshDispatch ResumeAction = "fresh_dispatch"
	ResumeError         ResumeAction = "error"
)

type ResumePlan struct {
	actions    map[string]ResumeAction
	sessionIDs map[string]string
	details    map[string]string
}

func (rp *ResumePlan) Action(beadID string) ResumeAction {
	if action, ok := rp.actions[beadID]; ok {
		return action
	}
	return ResumeFreshDispatch
}

func (rp *ResumePlan) SessionID(beadID string) string {
	return rp.sessionIDs[beadID]
}

func (rp *ResumePlan) Detail(beadID string) string {
	return rp.details[beadID]
}

// ResumeFilterProvider abstracts the workflow-aware state check so
// resume.go does not hardcode terminal or human-gate states.
// The caller (cmd/kernl/epic.go) supplies the backend resolver.
func (rp *ResumePlan) DoneSet() map[string]bool {
	result := make(map[string]bool)
	for id, action := range rp.actions {
		if action == ResumeSkip {
			result[id] = true
		}
	}
	return result
}

type ResumeFilterProvider interface {
	// IsTerminal returns true when the bead has reached a terminal
	// workflow state (shipped, abandoned, deferred, closed, etc.).
	IsTerminal(bead *backend.Bead) bool
	// IsHumanGate returns true when the bead is at a state that
	// requires human action (implementation_review as human gate,
	// awaiting_integration, etc.). Such beads are treated as "done"
	// from the executor's perspective — the human must unblock them.
	IsHumanGate(bead *backend.Bead) bool
}

// workflowResumeAdapter implements ResumeFilterProvider using the
// standard backend workflow resolver.
type workflowResumeAdapter struct{}

func (a *workflowResumeAdapter) IsTerminal(bead *backend.Bead) bool {
	wf := backend.ResolveWorkflow(bead)
	for _, ts := range wf.TerminalStates {
		if ts == bead.State {
			return true
		}
	}
	if bead.State == string(workflow.StatusClosed) || bead.State == "deferred" || bead.State == "abandoned" {
		return true
	}
	return false
}

func (a *workflowResumeAdapter) IsHumanGate(bead *backend.Bead) bool {
	if bead.State == string(workflow.StatusAwaitingIntegration) || bead.State == string(workflow.StatusAwaitingPRReview) {
		return true
	}
	rt := backend.DeriveWorkflowRuntimeState(backend.ResolveWorkflow(bead), bead.State)
	return rt.RequiresHumanAction
}

type RunStatePort interface {
	AgentRecord(beadID, state string) (runstate.AgentRecord, bool)
	Worktree(epicID, beadID string) (string, bool)
}

func PlanResume(be backend.BackendPort, rs RunStatePort, ep *Epic, repoPath string) *ResumePlan {
	return PlanResumeWithFilter(be, rs, ep, repoPath, &workflowResumeAdapter{})
}

func PlanResumeWithFilter(be backend.BackendPort, rs RunStatePort, ep *Epic, repoPath string, filter ResumeFilterProvider) *ResumePlan {
	rp := &ResumePlan{
		actions:    make(map[string]ResumeAction),
		sessionIDs: make(map[string]string),
		details:    make(map[string]string),
	}

	for _, child := range ep.Children {
		bead, err := be.Get(child.ID, repoPath)
		if err != nil || bead == nil {
			rp.actions[child.ID] = ResumeError
			rp.details[child.ID] = fmt.Sprintf("KERNL DISPATCH FAILURE: bead %s not found -- cause: backend.Get returned error -- Fix: verify bead exists in bead tracker", child.ID)
			continue
		}

		// Terminal or human-gate = skip (no further agent action needed).
		if filter.IsTerminal(bead) {
			rp.actions[child.ID] = ResumeSkip
			continue
		}
		if filter.IsHumanGate(bead) {
			rp.actions[child.ID] = ResumeSkip
			continue
		}

		// If there is a recorded agent session for this bead/state,
		// resume the session (e.g. after an orchestrator restart).
		rec, ok := rs.AgentRecord(child.ID, bead.State)
		if ok && rec.SessionID != "" {
			rp.actions[child.ID] = ResumeSession
			rp.sessionIDs[child.ID] = rec.SessionID
			continue
		}

		// If there is a recorded worktree that no longer exists on disk,
		// surface an error so the user can investigate.
		path, hasWorktree := rs.Worktree(ep.ID, child.ID)
		if hasWorktree {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				rp.actions[child.ID] = ResumeError
				rp.details[child.ID] = fmt.Sprintf(
					"KERNL DISPATCH FAILURE: worktree for bead %s missing -- %s. Fix: re-create or rm the bead's branch; Next: kernl epic run %s",
					child.ID, path, ep.ID,
				)
				continue
			}
		}

		rp.actions[child.ID] = ResumeFreshDispatch
	}

	return rp
}
