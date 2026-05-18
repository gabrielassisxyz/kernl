package app

import "fmt"

type NudgeCause string

const (
	NudgeTurnEnded                NudgeCause = "turn_ended"
	NudgeResumedAfterInterruption NudgeCause = "resumed_after_interruption"
)

type NudgeInput struct {
	BeadID string
	State  string
	Cause  NudgeCause
}

func BuildNudgePrompt(input NudgeInput) string {
	core := fmt.Sprintf(
		"Either complete the action to advance the knot, or run `kno rollback` if you cannot proceed. Do not exit without advancing or rolling back.",
	)

	switch input.Cause {
	case NudgeResumedAfterInterruption:
		return fmt.Sprintf(
			"You were resumed after an interruption. Bead `%s` is in state `%s`. %s",
			input.BeadID, input.State, core,
		)
	default:
		return fmt.Sprintf(
			"Your turn ended but bead `%s` is still in state `%s`. %s",
			input.BeadID, input.State, core,
		)
	}
}
