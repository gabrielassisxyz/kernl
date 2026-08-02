package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

// IntegrationFixupLabel marks a bead this package created via the fix-up
// mechanism: both the label the run report checks (via
// BeadReference.IsFixup, set from this label in recordDecisionIfGateType)
// and the durable fact surveyFixupChildren reads back from the tracker
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
// closed/shipped by hand. See surveyFixupChildren's own doc comment for why
// this - and not merely "does a fix-up child exist at all" - is what counts a
// round as spent.
var fixupWorkerCycleCompleteStates = map[string]bool{
	"awaiting_integration": true,
	"abandoned":            true,
	"closed":               true,
	"shipped":              true,
}

// fixupHistory is what an epic's own children say about the fix-up rounds it
// has already spent, and the one it may still be in the middle of.
type fixupHistory struct {
	// Pending is a fix-up child that exists but has NOT yet completed its own
	// worker cycle - evidence a prior attempt created and linked the bead and
	// then failed before the epic was rewound (see finalizeFixupBead), not
	// evidence of a round already spent.
	Pending *backend.Bead
	// Spent counts the fix-up children that did complete their cycle. This is
	// the number the budget is measured against.
	Spent int
}

// surveyFixupChildren lists epicID's own children and sorts the fix-up ones
// into that history.
//
// The budget this package enforces is counted from the tracker itself rather
// than from a marker in the epic's own description, for two reasons measured
// against how the tracker actually gets used, not assumed:
//
//  1. A description is mutable prose. A crash between creating the fix-up
//     bead and writing the marker leaves the marker never written at all; a
//     later `POST /api/beads/{id}` or `bead refine-scope --description`
//     replaces the field wholesale (the same "read-modify-write with no
//     compare-and-swap" gap RevertDecisionAndReopenBead's own doc comment
//     already accepts for a different write) and erases a marker that WAS
//     written. Either way the budget silently stops applying.
//  2. A bead is durable and nothing but this package's own writes touches
//     its labels. The fix-up bead already carries ParentID=epicID (this is
//     how the epic's own next `epic run` discovers it as a real child), so
//     the same List call every other reader of an epic's children already
//     uses is enough - no second storage mechanism next to the tracker
//     record itself.
func surveyFixupChildren(be backend.BackendPort, repoPath, epicID string) (fixupHistory, error) {
	children, err := be.List(&backend.BeadListFilters{Parent: epicID}, repoPath)
	if err != nil {
		return fixupHistory{}, fmt.Errorf("KERNL DISPATCH FAILURE: listing epic %s's children to check its fix-up history failed: %w", epicID, err)
	}
	var history fixupHistory
	for i := range children {
		if !HasLabel(children[i].Labels, IntegrationFixupLabel) {
			continue
		}
		if fixupWorkerCycleCompleteStates[children[i].State] {
			history.Spent++
			continue
		}
		if history.Pending == nil {
			history.Pending = &children[i]
		}
	}
	return history, nil
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
	// operator: something about it is expensive to undo, or it is not a
	// reversibility question at all (the reviewer asked for a decision, or
	// declared nothing this package could parse).
	Escalated bool
	// EscalationReason is set whenever Escalated is true - the text the CLI
	// surfaces, naming why, rather than the same generic "blocked" message
	// a hard stage failure would also produce.
	EscalationReason string
	// ReversibilityCause names which gate fired, on both paths: the cause of
	// an escalation, or GateCheapToReverse when the run continued. Carried out of
	// here so the CLI can print it verbatim - an escalation nobody can grep
	// for later is the failure mode this gate replaced, and a silent
	// "continue" would be the same failure in the other direction.
	ReversibilityCause GateCause
	// ReversibilityReason is the judgment behind that cause, including the
	// oracle's own words when it was asked.
	ReversibilityReason string
}

// DriveEpicIntegrationTailDeps is DriveBeadDeps plus what only the fix-up
// path needs beyond driving a single bead.
type DriveEpicIntegrationTailDeps struct {
	DriveBeadDeps
	// EpicID is the epic bead's own id, equal to DriveBeadDeps.BeadID for
	// this call - named separately because a fix-up bead's ParentID must be
	// exactly this, not re-derived from the (already-blocked) epic bead.
	EpicID string
	// EpicBranch and BaseBranch bound the range the reversibility facts are
	// measured over: what this epic changed, and against what.
	EpicBranch string
	BaseBranch string
	// IrreversibleSurfaces is registry.repos[].irreversibleSurfaces for the
	// repository being worked on. Empty means the repository declares none.
	IrreversibleSurfaces []string
	// Inspector is the git seam GatherReversibilityFacts measures through.
	// Nil defaults to the real one; only tests inject a fake.
	Inspector BranchInspector
	// Judge answers whether a rejection would be expensive to undo. Nil means
	// nothing is configured to answer it, and an unanswered gate escalates -
	// it never continues by default.
	Judge ReversibilityJudge
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

	history, listErr := surveyFixupChildren(deps.Backend, deps.RepoPath, deps.EpicID)
	if listErr != nil {
		return EpicIntegrationTailResult{RunBeadResult: res}, listErr
	}

	rejection := ParseIntegrationRejection(trimmed)
	facts, factsErr := GatherReversibilityFacts(GatherReversibilityFactsInput{
		EpicID:               deps.EpicID,
		RepoPath:             deps.RepoPath,
		BaseBranch:           deps.BaseBranch,
		EpicBranch:           deps.EpicBranch,
		IrreversibleSurfaces: deps.IrreversibleSurfaces,
		FixupsSpent:          history.Spent,
		Budget:               fixupBudget(deps.Config),
		Inspector:            deps.Inspector,
	})
	if factsErr != nil {
		// Continuing on a facts struct that could not be measured would read
		// "not published, nothing irreversible touched" - the two answers that
		// let a run proceed without a human - from a failure to look.
		return escalate(res, GateReversibilityUnknown, factsErr.Error(), deps.EpicID, reviewPath)
	}
	decision := DecideFixupAction(ctx, rejection, facts, deps.Judge)

	if decision.Action == FixupActionEscalate {
		return escalate(res, decision.Cause, decision.Reason, deps.EpicID, reviewPath)
	}
	if history.Pending != nil {
		// Resume the still-pending fix-up instead of creating a second bead
		// for the same rejection.
		return finalizeFixupBead(deps, history.Pending, rejection, decision)
	}
	return createFixupBead(deps, rejection, decision)
}

// fixupBudget reads orchestrator.fixupBudget, tolerating a nil config so a
// caller that never loaded one still gets the hard stop rather than an
// unbounded loop.
func fixupBudget(cfg *config.Config) int {
	if cfg == nil {
		return config.DefaultFixupBudget
	}
	return cfg.Orchestrator.FixupBudget
}

// escalate renders one escalation the same way whichever gate produced it: the
// cause, the reason, and the artifact to go read. The cause is spelled out
// verbatim so a run's output can be grepped for the gate that fired, rather
// than for the prose that happened to describe it.
func escalate(res RunBeadResult, cause GateCause, reason, epicID, reviewPath string) (EpicIntegrationTailResult, error) {
	text := fmt.Sprintf("integration review rejected epic %s and it escalates [%s]: %s (see %s)", epicID, cause, reason, reviewPath)
	return EpicIntegrationTailResult{
			RunBeadResult:       res,
			Escalated:           true,
			EscalationReason:    text,
			ReversibilityCause:  cause,
			ReversibilityReason: reason,
		},
		fmt.Errorf("KERNL DISPATCH FAILURE: %s", text)
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
func createFixupBead(deps DriveEpicIntegrationTailDeps, rejection *IntegrationRejection, decision FixupDecision) (EpicIntegrationTailResult, error) {
	title := fallbackDecisionTitle(rejection.WhatIsWrong)
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
	return finalizeFixupBead(deps, fixup, rejection, decision)
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
func finalizeFixupBead(deps DriveEpicIntegrationTailDeps, fixup *backend.Bead, rejection *IntegrationRejection, decision FixupDecision) (EpicIntegrationTailResult, error) {
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
		RunBeadResult:       RunBeadResult{FinalState: "ready_for_integration", Success: true},
		FixupBeadID:         fixup.ID,
		ReversibilityCause:  decision.Cause,
		ReversibilityReason: decision.Reason,
	}, nil
}
