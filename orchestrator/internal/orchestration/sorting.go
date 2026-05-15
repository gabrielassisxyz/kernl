package orchestration

import "sort"

func SortBeatsPriorityThenState(beats []BeatEntry) {
	sort.SliceStable(beats, func(i, j int) bool {
		if beats[i].Priority != beats[j].Priority {
			return beats[i].Priority > beats[j].Priority
		}
		return stateRank(beats[i].State) < stateRank(beats[j].State)
	})
}

type BeatEntry struct {
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