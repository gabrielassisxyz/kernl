package app

import (
	"strings"
	"testing"
)

// A follow-up tells the agent to inspect and advance the bead, so it has to
// name a tracker that exists. Both presets hardcoded "bd -C <repo>", which in a
// repository tracked with br instructed the agent to run a program that is not
// installed - and the follow-up is the path taken precisely when a stage is
// already going wrong.
func TestDefaultNudgePromptNamesTheRepositorysTracker(t *testing.T) {
	tracker := "br --db '/repo/.beads/beads.db'"
	for _, preset := range []NudgePreset{NudgePresetGeneric, NudgePresetAdvanceStatus} {
		prompt := DefaultNudgePrompt(preset, "kb-1", tracker)
		if !strings.Contains(prompt, tracker+" show kb-1") {
			t.Errorf("preset %q must inspect the bead through the repository's tracker:\n%s", preset, prompt)
		}
		if !strings.Contains(prompt, tracker+" update kb-1 --status") {
			t.Errorf("preset %q must advance through the repository's tracker:\n%s", preset, prompt)
		}
		if strings.Contains(prompt, "bd -C") {
			t.Errorf("preset %q must not name kernl's own tracker:\n%s", preset, prompt)
		}
	}
}

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
