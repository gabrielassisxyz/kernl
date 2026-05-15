package orchestration

import "fmt"

func MarkTerminal(beatID, state string) error {
	if !isTerminalTarget(state) {
		return fmt.Errorf("FOOLERY WORKFLOW CORRECTION FAILURE: state %q is not a valid terminal target for beat %s", state, beatID)
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