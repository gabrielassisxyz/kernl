package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// IntegrationFixupLabel marks a bead CreateFixupBead-equivalent code created:
// both the label the run report checks (via BeadReference.IsFixup, set from
// this label in recordDecisionIfGateType) and the fact a fix-up bead's own
// decisions get sorted first in the report because of it.
const IntegrationFixupLabel = "kernl:fixup"

// fixupCreatedMarkerPrefix is prepended to the epic bead's description the
// same way revert_decision.go's revertedDecisionMarkerPrefix is: a durable,
// greppable fact placed where DriveEpicIntegrationTail's cap check
// (epicAlreadyHasFixup) can read it back on a later run of the same epic,
// without a second storage mechanism next to the tracker record itself.
const fixupCreatedMarkerPrefix = "<!-- kernl:fixup-created:"

// HasLabel reports whether label is present in labels verbatim. Exported so
// cmd/kernl can check a bead's own labels (e.g. IntegrationFixupLabel) when
// building the BeadRef list StartWorkflowRun seeds its bead_reference nodes
// from - that seeding is the first write for those ids, so it is the only
// place IsFixup can still be set (see StartWorkflowRun's own doc comment on
// why a later ensureBeadReferenceNode call is a no-op).
func HasLabel(labels []string, label string) bool {
	for _, l := range labels {
		if l == label {
			return true
		}
	}
	return false
}

// epicAlreadyHasFixup reports whether description already carries the
// fixupCreatedMarkerPrefix - this epic already spawned one fix-up bead, so
// per §7 a second integration_review rejection must escalate rather than
// create another one.
func epicAlreadyHasFixup(description string) bool {
	return strings.Contains(description, fixupCreatedMarkerPrefix)
}

// EpicIntegrationTailResult reports what happened driving the epic bead's
// own integration -> integration_review -> shipment tail, distinguishing
// three outcomes a plain RunBeadResult cannot express: normal completion (or
// a normal blocked failure unrelated to any rejection), a fix-up bead
// created and the epic paused to be resumed, or an escalation only the
// operator can resolve.
type EpicIntegrationTailResult struct {
	RunBeadResult
	// FixupBeadID is non-empty when this call created a fix-up bead and
	// rewound the epic to ready_for_integration - a graceful, expected
	// pause, not a failure: re-running `kernl epic run <epicID>` discovers
	// the new bead as a real child (it is linked via CreateBeadInput.ParentID)
	// and, once it reaches awaiting_integration, re-attempts integration.
	FixupBeadID string
	// Escalated is true when this integration_review rejection needs the
	// operator: the reviewer declared a decision, declared nothing this
	// package could parse, or this epic already spawned one fix-up bead and
	// a fix-up cannot spawn a fix-up.
	Escalated bool
	// EscalationReason is set whenever Escalated is true - the text the CLI
	// surfaces, naming why, rather than the same generic "blocked" message
	// a hard stage failure would also produce.
	EscalationReason string
}

// DriveEpicIntegrationTailDeps is DriveBeadDeps plus what only the fix-up
// path needs beyond driving a single bead.
type DriveEpicIntegrationTailDeps struct {
	DriveBeadDeps
	// EpicID is the epic bead's own id, equal to DriveBeadDeps.BeadID for
	// this call - named separately because a fix-up bead's ParentID must be
	// exactly this, not re-derived from the (already-blocked) epic bead.
	EpicID string
}

// DriveEpicIntegrationTail drives the epic bead exactly as
// DriveBeadToTerminal always has, then - only when the result is a plain
// "blocked" - checks whether the stage that blocked it was
// integration_review deliberately rejecting (backend's "verdict_reject" gate
// reason, surfaced here by re-reading the same review artifact the gate
// itself read) rather than genuinely failing. Every other outcome (success,
// a human gate, a hard failure at any other stage, a missing or non-rejecting
// review artifact) passes through unchanged: this function adds behavior on
// top of DriveBeadToTerminal, it does not replace any of its existing paths.
func DriveEpicIntegrationTail(ctx context.Context, deps DriveEpicIntegrationTailDeps) (EpicIntegrationTailResult, error) {
	res, err := DriveBeadToTerminal(ctx, deps.DriveBeadDeps)
	if err != nil || res.Success || res.FinalState != "blocked" {
		return EpicIntegrationTailResult{RunBeadResult: res}, err
	}

	artifactDir, dirErr := resolveArtifactDir(deps.StateDir, deps.EpicID, deps.EpicID)
	if dirErr != nil {
		// The epic really is blocked and this cannot even check why -
		// report the original failure, not a new one about a directory.
		return EpicIntegrationTailResult{RunBeadResult: res}, err
	}
	reviewPath := backend.ResolveArtifactPath("<artifact_dir>/integration-review.md", deps.EpicID, artifactDir)
	data, readErr := os.ReadFile(reviewPath)
	if readErr != nil {
		// No review artifact (or one from a different, non-rejecting
		// failure) - the existing "epic blocked" path handles it unchanged.
		return EpicIntegrationTailResult{RunBeadResult: res}, err
	}
	trimmed := strings.TrimSpace(string(data))
	if !strings.HasSuffix(trimmed, "VERDICT: REJECT") {
		return EpicIntegrationTailResult{RunBeadResult: res}, err
	}

	epicBead, getErr := deps.Backend.Get(deps.EpicID, deps.RepoPath)
	if getErr != nil || epicBead == nil {
		return EpicIntegrationTailResult{RunBeadResult: res},
			fmt.Errorf("KERNL DISPATCH FAILURE: integration_review rejected epic %s but the epic could not be re-read to check its fix-up history: %v", deps.EpicID, getErr)
	}

	rejection := ParseIntegrationRejection(trimmed)
	action := DecideFixupAction(rejection, epicAlreadyHasFixup(epicBead.Description))

	switch action {
	case FixupActionCreateBead:
		return createFixupBeadAndRewind(deps, epicBead, rejection)
	case FixupActionEscalateAlreadyFixedUp:
		reason := fmt.Sprintf("integration review rejected epic %s again, but it already created one fix-up bead - a fix-up cannot spawn a fix-up; resolve it by hand (see %s)", deps.EpicID, reviewPath)
		return EpicIntegrationTailResult{RunBeadResult: res, Escalated: true, EscalationReason: reason},
			fmt.Errorf("KERNL DISPATCH FAILURE: %s", reason)
	default: // FixupActionEscalateDecisionOrAmbiguous
		reason := escalationReasonForAmbiguousOrDecision(rejection, reviewPath)
		return EpicIntegrationTailResult{RunBeadResult: res, Escalated: true, EscalationReason: reason},
			fmt.Errorf("KERNL DISPATCH FAILURE: %s", reason)
	}
}

// escalationReasonForAmbiguousOrDecision renders the operator-facing message
// for the two cases DecideFixupAction folds together: a well-formed decision
// declaration, or a rejection this package could not parse at all. Naming
// which one happened is worth the branch even though the action taken is
// identical, because a message that always said "ambiguous" would leave the
// operator re-deriving a real question the reviewer already wrote down.
func escalationReasonForAmbiguousOrDecision(rejection *IntegrationRejection, reviewPath string) string {
	if rejection == nil {
		return fmt.Sprintf("integration review rejected, but its declaration in %s could not be parsed as a fixup or a decision - an unparseable declaration is treated as ambiguous, and per the orchestrator-autonomy decision model's §7 an ambiguous rejection must escalate rather than be guessed into a fix-up", reviewPath)
	}
	return fmt.Sprintf("integration review rejected with a decision only the operator can make: %s (see %s for the full context)", rejection.Question, reviewPath)
}

// createFixupBeadAndRewind is the FixupActionCreateBead branch: create the
// fix-up bead as a real child of the epic, mark the epic so a second
// rejection escalates instead of creating a second one, then rewind the
// epic to ready_for_integration so a later `epic run` re-attempts
// integration once the fix-up bead reaches awaiting_integration.
//
// The three steps are NOT safe to retry blindly if a later one fails: unlike
// RevertDecisionAndReopenBead (which validates everything before its first
// durable write), there is no id to check "already created?" against before
// Create itself runs, so a Create that succeeds followed by a failed Update
// or Rewind leaves a fix-up bead that exists but is not yet marked or not
// yet resumable. Each failure below names the exact manual recovery instead
// of promising an automatic one - the same trade-off bead D's own retry
// story already accepted for a different two-step write, recorded there as
// deliberate rather than an oversight.
func createFixupBeadAndRewind(deps DriveEpicIntegrationTailDeps, epicBead *backend.Bead, rejection *IntegrationRejection) (EpicIntegrationTailResult, error) {
	title, _ := decisionTitleAndContext(rejection.WhatIsWrong)
	input := backend.CreateBeadInput{
		Title:       "fix-up: " + title,
		Description: rejection.WhatIsWrong,
		Type:        "bug",
		Acceptance:  rejection.Acceptance,
		Labels:      []string{IntegrationFixupLabel},
		ParentID:    deps.EpicID,
	}
	fixup, err := deps.Backend.Create(input, deps.RepoPath)
	if err != nil {
		return EpicIntegrationTailResult{}, fmt.Errorf("KERNL DISPATCH FAILURE: integration review rejected epic %s with a fix-up declared, but creating the fix-up bead failed: %w - Fix: this repository's tracker needs a working Create (BackendPort.Create); the epic is still blocked and nothing was linked to it, so retrying this call is safe", deps.EpicID, err)
	}

	marker := fixupCreatedMarkerPrefix + fixup.ID + " -->"
	newDescription := marker + "\n\n" + epicBead.Description
	if err := deps.Backend.Update(deps.EpicID, backend.UpdateBeadInput{Description: newDescription}, deps.RepoPath); err != nil {
		return EpicIntegrationTailResult{}, fmt.Errorf("KERNL DISPATCH FAILURE: fix-up bead %s was created for epic %s but marking the epic as fixed-up failed: %w - Fix: without this marker a second rejection could create a second fix-up bead; write %q into the epic's description by hand, or fix the tracker error, before re-running - do not re-run blindly, it would create a duplicate fix-up bead", fixup.ID, deps.EpicID, err, marker)
	}

	reason := "integration review rejected: " + rejection.WhatIsWrong
	if err := deps.Backend.Rewind(deps.EpicID, "ready_for_integration", reason, deps.RepoPath); err != nil {
		return EpicIntegrationTailResult{}, fmt.Errorf("KERNL DISPATCH FAILURE: fix-up bead %s created and linked to epic %s, but rewinding the epic to ready_for_integration failed: %w - Fix: the epic is still blocked; retry once the tracker issue is resolved - the fix-up bead and its marker are already in place and will not be duplicated", fixup.ID, deps.EpicID, err)
	}

	return EpicIntegrationTailResult{
		RunBeadResult: RunBeadResult{FinalState: "ready_for_integration", Success: true},
		FixupBeadID:   fixup.ID,
	}, nil
}
