package orchestration

import (
	"sort"

	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

func SortBeadsPriorityThenState(beads []BeadEntry) {
	sort.SliceStable(beads, func(i, j int) bool {
		if beads[i].Priority != beads[j].Priority {
			return beads[i].Priority > beads[j].Priority
		}
		return stateRank(beads[i].State) < stateRank(beads[j].State)
	})
}

type BeadEntry struct {
	ID        string
	State     string
	Priority  int
	UpdatedAt string
}

func stateRank(state string) int {
	switch state {
	case string(workflow.StatusOpen):
		return 0
	case string(workflow.StatusInProgress):
		return 1
	case "plan_review":
		return 2
	case string(workflow.StatusClosed):
		return 3
	default:
		return 99
	}
}
