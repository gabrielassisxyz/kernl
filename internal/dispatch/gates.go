package dispatch

import (
	"strings"
)

var hardGates = []string{
	"git push",
	"gh pr merge",
	"drop database",
	"drop table",
	"rm -rf",
}

// IsHardGate checks if the requested action string matches any hard gate definitions.
// Hard gates include actions like git push, PR merges, irreversible deletes,
// or external mutations.
func IsHardGate(action string) bool {
	lowerAction := strings.ToLower(action)
	for _, gate := range hardGates {
		if strings.Contains(lowerAction, gate) {
			return true
		}
	}
	return false
}

// CanAutoApprove returns true if the action can be auto-approved under autonomous mode.
// Hard gates are NEVER silenced.
func CanAutoApprove(isAutonomous bool, action string) bool {
	if !isAutonomous {
		return false
	}
	if IsHardGate(action) {
		return false
	}
	return true
}
