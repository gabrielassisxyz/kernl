package app

import (
	"strings"
	"testing"
)

func TestBuildNudgePromptByCause(t *testing.T) {
	turnEnded := BuildNudgePrompt(NudgeInput{BeadID: "kb-1", State: "implementing", Cause: NudgeTurnEnded})
	if !strings.Contains(turnEnded, "kb-1") || !strings.Contains(turnEnded, "implementing") {
		t.Errorf("turn-ended nudge must name bead and state: %q", turnEnded)
	}
	resumed := BuildNudgePrompt(NudgeInput{BeadID: "kb-1", State: "implementing", Cause: NudgeResumedAfterInterruption})
	if resumed == turnEnded {
		t.Error("resumed-after-interruption nudge must differ from turn-ended")
	}
}
