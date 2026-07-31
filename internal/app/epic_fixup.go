package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// IntegrationFixupLabel marks a bead this package created via the fix-up
// mechanism: both the label the run report checks (via
// BeadReference.IsFixup, set from this label in recordDecisionIfGateType)
// and the durable fact findExistingFixupChild reads back from the tracker
// itself, never from mutable prose - see that function's own doc comment
// for why.
const IntegrationFixupLabel = "kernl:fixup"

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

// fixupWorkerCycleCompleteStates are the states a fix-up bead (driven under
// the "worker" profile, like any other epic child) reaches once its own
// implementation cycle is over - reached the profile's own terminal state
// (awaiting_integration, handed to the epic; or abandoned, gave up), or
// closed/shipped by hand. See findExistingFixupChild's own doc comment for
// why this - and not merely "does a fix-up child exist at all" - is the cap
// this package checks.
var fixupWorkerCycleCompleteStates = map[string]bool{
	"awaiting_integration": true,
	"abandoned":            true,
	"closed":               true,
	"shipped":              true,
}

// findExistingFixupChild lists epicID's own children and returns the one
// carrying IntegrationFixupLabel, or nil if none exists.
//
// The cap this package enforces - "a fix-up cannot spawn a fix-up" - reads
// this instead of a marker in the epic's own description, for two reasons
// measured against how the tracker actually gets used, not assumed:
//
//  1. A description is mutable prose. A crash between creating the fix-up
//     bead and writing the marker leaves the marker never written at all; a
//     later `POST /api/beads/{id}` or `bead refine-scope --description`
//     replaces the field wholesale (the same "read-modify-write with no
//     compare-and-swap" gap RevertDecisionAndReopenBead's own doc comment
//     already accepts for a different write) and erases a marker that WAS
//     written. Either way the cap silently stops applying.
//  2. A bead is durable and nothing but this package's own writes touches
//     its labels. The fix-up bead already carries ParentID=epicID (this is
//     how the epic's own next `epic run` discovers it as a real child), so
//     the same List call every other reader of an epic's children already
//     uses is enough - no second storage mechanism next to the tracker
//     record itself.
func findExistingFixupChild(be backend.BackendPort, repoPath, epicID string) (*backend.Bead, error) {
	children, err := be.List(&backend.BeadListFilters{Parent: epicID}, repoPath)
	if err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: listing epic %s's children to check its fix-up history failed: %w", epicID, err)
	}
	for i := range children {
		if HasLabel(children[i].Labels, IntegrationFixupLabel) {
			return &children[i], nil
		}
	}
	return nil, nil
}

// EpicIntegrationTailResult reports what happened driving the epic bead's
// own integration -> integration_review -> shipment tail, distinguishing
// three outcomes a plain RunBeadResult cannot express: normal completion (or
// a normal blocked failure unrelated to any rejection), a fix-up bead
// created and the epic paused to be resumed, or an escalation only the
// operator can resolve.
type EpicIntegrationTailResult struct {
	RunBeadResult
	// FixupBeadID is non-empty when this call created (or resumed) a
	// fix-up bead and rewound the epic to ready_for_integration - a
	// graceful, expected pause, not a failure: re-running `kernl epic run
	// <epicID>` discovers the bead as a real child (it is linked via
	// CreateBeadInput.ParentID) and, once it reaches awaiting_integration,
	// re-attempts integration.
	FixupBeadID string
	// Escalated is true when this integration_review rejection needs the
	// operator: the reviewer declared a decision, declared nothing this
	// package could parse, or this epic already has a fix-up bead that
	// completed its own cycle and a fix-up cannot spawn a fix-up.
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
	// The gate that blocked this bead must have been integration_review's
	// own - not a different stage's failure while an integration-review.md
	// from an EARLIER attempt happens to still sit on disk at the same
	// well-known path. Without this check, re-reading that stale file would
	// find its old REJECT verdict and dispatch a fix-up for a rejection
	// that already has nothing to do with the CURRENT failure. res.BlockedAtState
	// answers this whether the block just happened (the gate-failure branch
	// set it directly) or is being revisited on a retry (the bead was
	// already "blocked" on entry, and DriveBeadToTerminal recovers it from
	// the still-stale wf:state:* label - see stateFromStaleLabel).
	if res.BlockedAtState != "integration_review" {
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
	if !hasExactLastLine(trimmed, "VERDICT: REJECT") {
		return EpicIntegrationTailResult{RunBeadResult: res}, err
	}

	existingFixup, listErr := findExistingFixupChild(deps.Backend, deps.RepoPath, deps.EpicID)
	if listErr != nil {
		return EpicIntegrationTailResult{RunBeadResult: res}, listErr
	}
	// A fix-up child that exists but has NOT yet completed its own worker
	// cycle is not evidence the cap was already used - it is evidence a
	// PRIOR attempt got as far as creating (and linking) the bead and then
	// failed before the epic was rewound (see finalizeFixupBead). The
	// executor always drives every child, including this one, to a
	// terminal state before this tail ever runs again, so "exists but not
	// yet terminal" can only mean that earlier rewind never completed.
	alreadyFixedUp := existingFixup != nil && fixupWorkerCycleCompleteStates[existingFixup.State]

	rejection := ParseIntegrationRejection(trimmed)
	action := DecideFixupAction(rejection, alreadyFixedUp)

	switch action {
	case FixupActionCreateBead:
		if existingFixup != nil {
			// Resume the still-pending fix-up instead of creating a second
			// bead for the same rejection.
			return finalizeFixupBead(deps, existingFixup, rejection)
		}
		return createFixupBead(deps, rejection)
	case FixupActionEscalateAlreadyFixedUp:
		reason := fmt.Sprintf("integration review rejected epic %s again, but fix-up bead %s already completed its own cycle - a fix-up cannot spawn a fix-up; resolve it by hand (see %s)", deps.EpicID, existingFixup.ID, reviewPath)
		return EpicIntegrationTailResult{RunBeadResult: res, Escalated: true, EscalationReason: reason},
			fmt.Errorf("KERNL DISPATCH FAILURE: %s", reason)
	default: // FixupActionEscalateDecisionOrAmbiguous
		reason := escalationReasonForAmbiguousOrDecision(rejection, reviewPath)
		return EpicIntegrationTailResult{RunBeadResult: res, Escalated: true, EscalationReason: reason},
			fmt.Errorf("KERNL DISPATCH FAILURE: %s", reason)
	}
}

// hasExactLastLine reports whether the trimmed last line of content equals
// want exactly - the same "end with the literal line" test
// evaluateSingleExitGate's artifact_verdict case uses, so this wrapper and
// the gate it is reacting to can never disagree about what counts as a
// rejection (a plain HasSuffix over the whole document would also match a
// fabricated line like "NOT A VALID VERDICT: REJECT").
func hasExactLastLine(content, want string) bool {
	lastLine := content
	if idx := strings.LastIndexByte(content, '\n'); idx != -1 {
		lastLine = strings.TrimSpace(content[idx+1:])
	}
	return lastLine == want
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

// createFixupBead is the first-time half of FixupActionCreateBead: create
// the fix-up bead as a real child of the epic, then hand off to
// finalizeFixupBead for the step every path (fresh create or resume) shares.
//
// A Create that fails is NOT always "nothing happened": br create's own
// issue creation can succeed while the acceptance/notes follow-up it
// performs internally fails (see BrCliBackend.Create). That case comes back
// as a *backend.CreatePartialError carrying the bead that DOES exist, and
// is resumed from here rather than treated as a clean failure that makes
// retrying safe - retrying blind on a false "nothing was created" is
// exactly what would create a duplicate.
func createFixupBead(deps DriveEpicIntegrationTailDeps, rejection *IntegrationRejection) (EpicIntegrationTailResult, error) {
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
		var partial *backend.CreatePartialError
		if !errors.As(err, &partial) || partial.Bead == nil {
			return EpicIntegrationTailResult{}, fmt.Errorf("KERNL DISPATCH FAILURE: integration review rejected epic %s with a fix-up declared, but creating the fix-up bead failed: %w - Fix: this repository's tracker needs a working Create (BackendPort.Create); nothing was created, so retrying is safe", deps.EpicID, err)
		}
		// The issue itself (and its ParentID link, and its label) was
		// already created - only its acceptance/notes follow-up failed.
		// finalizeFixupBead below fills in what is missing.
		fixup = partial.Bead
	}
	return finalizeFixupBead(deps, fixup, rejection)
}

// finalizeFixupBead is the one remaining step whether fixup was just
// created or is being resumed from a prior, incomplete attempt: make sure
// it carries the rejection's own acceptance criteria (a partial Create may
// have skipped this), then rewind the epic to ready_for_integration so a
// later `epic run` re-attempts integration once the bead reaches
// awaiting_integration.
//
// rejection is never nil here: DecideFixupAction only reaches this path
// when rejection.Kind == review.KindFixup, which requires a non-nil
// rejection by construction (see ParseIntegrationRejection).
func finalizeFixupBead(deps DriveEpicIntegrationTailDeps, fixup *backend.Bead, rejection *IntegrationRejection) (EpicIntegrationTailResult, error) {
	if fixup.Acceptance == "" && rejection.Acceptance != "" {
		if err := deps.Backend.Update(fixup.ID, backend.UpdateBeadInput{Acceptance: rejection.Acceptance}, deps.RepoPath); err != nil {
			return EpicIntegrationTailResult{}, fmt.Errorf("KERNL DISPATCH FAILURE: fix-up bead %s exists for epic %s but still lacks its acceptance criteria: %w - Fix: retry once the tracker issue is resolved; the bead will not be duplicated", fixup.ID, deps.EpicID, err)
		}
	}

	reason := "integration review rejected: " + rejection.WhatIsWrong
	if err := deps.Backend.Rewind(deps.EpicID, "ready_for_integration", reason, deps.RepoPath); err != nil {
		return EpicIntegrationTailResult{}, fmt.Errorf("KERNL DISPATCH FAILURE: fix-up bead %s exists for epic %s, but rewinding the epic to ready_for_integration failed: %w - Fix: the epic is still blocked; retry once the tracker issue is resolved - the fix-up bead will not be duplicated", fixup.ID, deps.EpicID, err)
	}

	return EpicIntegrationTailResult{
		RunBeadResult: RunBeadResult{FinalState: "ready_for_integration", Success: true},
		FixupBeadID:   fixup.ID,
	}, nil
}
