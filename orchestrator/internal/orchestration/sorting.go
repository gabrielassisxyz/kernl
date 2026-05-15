package orchestration

import "sort"

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
	case "ready_for_implementation":
		return 0
	case "implementation":
		return 1
	case "plan_review":
		return 2
	case "shipment_review":
		return 3
	case "shipped":
		return 4
	default:
		return 99
	}
}