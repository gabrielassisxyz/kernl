package epic

import (
	"fmt"
	"os"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/runstate"
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

type RunStatePort interface {
	AgentRecord(beadID, state string) (runstate.AgentRecord, bool)
	Worktree(epicID, beadID string) (string, bool)
}

var terminalStates = map[string]bool{
	"done":    true,
	"closed":  true,
	"blocked": true,
	"skipped": true,
}

var activeStates = map[string]bool{
	"implementing":               true,
	"in_progress":                true,
	"in-review":                  true,
	"reviewing":                  true,
	"ready_for_review":           true,
	"running":                    true,
}

func PlanResume(be backend.BackendPort, rs RunStatePort, ep *Epic, repoPath string) *ResumePlan {
	rp := &ResumePlan{
		actions:    make(map[string]ResumeAction),
		sessionIDs: make(map[string]string),
		details:    make(map[string]string),
	}

	for _, child := range ep.Children {
		bead, err := be.Get(child.ID, repoPath)
		if err != nil || bead == nil {
			rp.actions[child.ID] = ResumeError
			rp.details[child.ID] = fmt.Sprintf("KERNL DISPATCH FAILURE: bead %s not found — cause: backend.Get returned error — Fix: verify bead exists in bead tracker", child.ID)
			continue
		}

		if terminalStates[bead.State] {
			rp.actions[child.ID] = ResumeSkip
			continue
		}

		path, hasWorktree := rs.Worktree(ep.ID, child.ID)
		if hasWorktree {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				rp.actions[child.ID] = ResumeError
				rp.details[child.ID] = fmt.Sprintf(
					"KERNL DISPATCH FAILURE: worktree for bead %s missing — %s. Fix: re-create or rm the bead's branch; Next: kernl epic run %s",
					child.ID, path, ep.ID,
				)
				continue
			}
		}

		if activeStates[bead.State] {
			rec, ok := rs.AgentRecord(child.ID, bead.State)
			if ok {
				rp.actions[child.ID] = ResumeSession
				rp.sessionIDs[child.ID] = rec.SessionID
				continue
			}
		}

		rp.actions[child.ID] = ResumeFreshDispatch
	}

	return rp
}
