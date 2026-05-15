package orchestration

import "fmt"

func MarkTerminal(beadID, state string) error {
	if !isTerminalTarget(state) {
		return fmt.Errorf("KERNL WORKFLOW CORRECTION FAILURE: state %q is not a valid terminal target for bead %s", state, beadID)
	}
	return nil
}

func isTerminalTarget(state string) bool {
	switch state {
	case "shipped", "closed", "done", "abandoned":
		return true
	default:
		return false
	}
}