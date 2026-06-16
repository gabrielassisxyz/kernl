package orchestration

import (
	"fmt"

	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

func MarkTerminal(beadID, state string) error {
	if !isTerminalTarget(state) {
		return fmt.Errorf("KERNL WORKFLOW CORRECTION FAILURE: state %q is not a valid terminal target for bead %s", state, beadID)
	}
	return nil
}

func isTerminalTarget(state string) bool {
	switch state {
	case string(workflow.StatusClosed):
		return true
	default:
		return false
	}
}
