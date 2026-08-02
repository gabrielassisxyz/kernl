package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/review"
)

// ForkCauseReviewDecisionAlreadyRouted names an escalation this package's own
// loop produces (see reviewDecisionAlreadyRouted), not DecideForkAction's -
// declared here rather than in fork_handover.go for the same reason
// ForkCauseBudgetSpent lives in fork_gate.go (see that constant's own doc
// comment): this cause is produced by this gate's own bookkeeping, never by
// the pure policy function.
const ForkCauseReviewDecisionAlreadyRouted ForkGateCause = "review_decision_already_routed"

// ForkCauseRewindNotPossible names an escalation handleReviewRaisedDecision's
// own pre-check produces (finding 4 of the fork/decision-gate hardening
// pass) when reviewRejectionCanBeRewound already reports there is nowhere to
// send an answer - decided BEFORE the DA is ever consulted, never by
// DecideForkAction itself.
const ForkCauseRewindNotPossible ForkGateCause = "review_decision_rewind_not_possible"

// reviewDecisionRoutedMarkerSuffix names the sibling file
// handleReviewRaisedDecision writes once it routes a review-raised decision
// to the DA - never the review artifact itself. See
// reviewDecisionAlreadyRouted's own doc comment for why this has to be a
// SEPARATE file rather than consuming or archiving implementation-review.md
// the way consumeForkHandover consumes fork.md.
const reviewDecisionRoutedMarkerSuffix = ".decision-routed"

func resolvedReviewDecisionRoutedMarkerPath(beadID, artifactDir string) string {
	return backend.ResolveArtifactPath("<artifact_dir>/implementation-review.md", beadID, artifactDir) + reviewDecisionRoutedMarkerSuffix
}

// reviewDecisionAlreadyRouted reports whether THIS EXACT rejection text -
// the same content readRejectedReview would return right now - already had a
// decision routed to the DA once.
//
// This cannot reuse consumeForkHandover's own trick (rename the artifact so
// it can never be read as fresh again): readRejectedReview's own contract
// deliberately keeps a stale REJECT readable forever, because the text is
// what still has to reach the implementer's prompt as the objection to fix
// (see that function's own doc comment) - consuming or archiving
// implementation-review.md here would silence that prompt section the
// moment a decision was routed, which finding 1's own fix must not do. So
// "has this been routed" is tracked in a SEPARATE marker file, holding the
// exact text that was routed, rather than folded into the review file
// itself: a later, genuinely NEW rejection (different text, even if also
// classified `decision`) compares unequal and is routed exactly like a first
// one, while the identical, unrewritten stale file compares equal and is
// refused.
func reviewDecisionAlreadyRouted(beadID, artifactDir, reviewText string) bool {
	data, err := os.ReadFile(resolvedReviewDecisionRoutedMarkerPath(beadID, artifactDir))
	if err != nil {
		return false
	}
	return string(data) == reviewText
}

// markReviewDecisionRouted records reviewText as the rejection this bead's
// review-raised decision gate has now routed to the DA, so a later gate
// failure re-reading the identical stale implementation-review.md cannot
// route it a second time (see reviewDecisionAlreadyRouted).
func markReviewDecisionRouted(beadID, artifactDir, reviewText string) error {
	path := resolvedReviewDecisionRoutedMarkerPath(beadID, artifactDir)
	if err := os.WriteFile(path, []byte(reviewText), 0o644); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: recording that bead %s's review-raised decision was routed to the DA, at %s: %w - Fix: kernl needs write access to this bead's own artifact directory", beadID, path, err)
	}
	return nil
}

// gateFailureContext is everything one dispatch path in drive_bead.go
// already knows at the point backend.EvaluateExitGate has failed a stage -
// the subprocess flow and the native (agent-spawn) flow both reach here with
// the same shape, and that is the point: this used to be two separate
// inlined copies of "isDeliberateRejection -> rewindAfterReviewRejection",
// one in each flow. The plan's §0 records exactly this failure mode once
// already (a prior revision fixed one of two identical code paths and left
// the other standing), so here it is written once and both flows call it.
type gateFailureContext struct {
	Deps        DriveBeadDeps
	WF          backend.WorkflowDescriptor
	Bead        *backend.Bead
	ActiveState string
	ArtifactDir string
	EpicID      string
	GateReason  string
	// ReviewRewinds and ForkGateCalls are the caller's own per-call budgets
	// (implementationReviewRewindLimit, forkHandoverLimit) as spent so far
	// THIS call to DriveBeadToTerminal - passed in rather than read from a
	// package-level counter, for the same reason forkGateAttemptContext.CallsUsed
	// is: a unit test constructs this struct directly, with no dependency on
	// DriveBeadToTerminal's own loop ever running.
	ReviewRewinds int
	ForkGateCalls int
}

// gateFailureHandled is what handleGateFailure decided.
type gateFailureHandled struct {
	// Reenter is true when the bead must be re-dispatched: an ordinary
	// rewind to the implementer, or a reviewer-raised decision the DA just
	// answered and that was then rewound the same way. ReviewRewinds and
	// ForkGateCalls carry the caller's own counters forward, incremented
	// exactly where a budget was spent - mirroring forkGateHandled's own
	// Reenter contract in fork_gate.go.
	Reenter       bool
	ReviewRewinds int
	ForkGateCalls int
	// Result is set whenever Reenter is false: the bead blocks, for any of
	// three reasons - an ordinary (non-rejection) gate failure, an
	// exhausted rewind budget with nowhere to send the work, or an
	// escalated reviewer-raised decision.
	Result RunBeadResult
}

// handleGateFailure is the one place both dispatch paths in drive_bead.go
// call once an ordinary exit-gate evaluation has failed a stage.
//
// Beyond the rewind this already did (today's behavior, unchanged for an
// unclassified or KindFixup rejection - see ParseImplementationRejection's
// own doc comment for why that default must never change), it now also
// reads whether the reviewer classified the rejection review.KindDecision.
// If so, the rejection is routed through the SAME DecideForkAction the
// proactive fork gate uses (fork_handover.go), sharing its per-call budget
// (forkHandoverLimit / ForkGateCalls): an implementer and a reviewer
// volleying the same disagreement back and forth is exactly what a shared
// budget bounds, and two separate budgets would not.
func handleGateFailure(ctx context.Context, in gateFailureContext) (gateFailureHandled, error) {
	if !isDeliberateRejection(in.ActiveState, in.GateReason) {
		return gateFailureHandled{Result: blockBeadForGateFailure(in.Deps, in.ActiveState, in.GateReason)}, nil
	}

	reviewPath := backend.ResolveArtifactPath("<artifact_dir>/implementation-review.md", in.Deps.BeadID, in.ArtifactDir)
	reviewText := readRejectedReview(reviewPath)
	rejection := ParseImplementationRejection(reviewText)

	if rejection != nil && rejection.Kind == review.KindDecision {
		// readRejectedReview deliberately keeps a stale REJECT readable
		// forever (see that function's own doc comment), so a reviewer stage
		// that exits zero WITHOUT rewriting implementation-review.md leaves
		// the IDENTICAL rejection on disk for the next gate failure to read.
		// Without this check that re-read looks exactly like a fresh
		// decision and gets routed to the DA a second time - the failure
		// consumeForkHandover's own doc comment already names once for
		// fork.md, standing here for implementation-review.md's own
		// decision-routing fact (never the review text itself, which must
		// keep reaching the implementer's prompt unchanged - see
		// reviewDecisionAlreadyRouted's own doc comment for why the two
		// cannot be conflated into one file).
		if reviewDecisionAlreadyRouted(in.Deps.BeadID, in.ArtifactDir, reviewText) {
			slog.Info("DRIVE_TRACE review-raised decision not routed again: this exact rejection was already answered once",
				"bead", in.Deps.BeadID)
			decision := ForkDecision{
				Action: ForkActionEscalate,
				Cause:  ForkCauseReviewDecisionAlreadyRouted,
				Reason: "this exact implementation-review.md rejection already routed a decision to the DA once - a reviewer that re-rejects with the identical, unrewritten review must not have the same question asked a second time - Fix: have the reviewer stage write a fresh review, or resolve this bead by hand",
			}
			return gateFailureHandled{Result: blockBeadForDecision(in.Deps, in.ActiveState, "review_decision_escalated", decision)}, nil
		}
		return handleReviewRaisedDecision(ctx, in, rejection, reviewText)
	}

	// Unclassified, or explicitly review.KindFixup: today's behavior,
	// unchanged.
	rewound, err := rewindAfterReviewRejection(in.Deps, in.WF, in.GateReason, in.ReviewRewinds)
	if err != nil {
		return gateFailureHandled{}, err
	}
	if rewound {
		_ = in.Deps.Backend.Comment(in.Deps.BeadID, "review_rejected: "+in.GateReason, in.Deps.RepoPath)
		return gateFailureHandled{Reenter: true, ReviewRewinds: in.ReviewRewinds + 1, ForkGateCalls: in.ForkGateCalls}, nil
	}
	return gateFailureHandled{Result: blockBeadForGateFailure(in.Deps, in.ActiveState, in.GateReason)}, nil
}

// handleReviewRaisedDecision routes one implementation_review rejection the
// reviewer classified review.KindDecision through DecideForkAction, adapting
// it into the ForkHandover shape at the boundary (see
// forkHandoverFromImplementationRejection) rather than forking the policy
// function pass 1 already defined.
func handleReviewRaisedDecision(ctx context.Context, in gateFailureContext, rejection *ImplementationRejection, reviewText string) (gateFailureHandled, error) {
	if in.ForkGateCalls >= forkHandoverLimit {
		slog.Info("DRIVE_TRACE review-raised decision escalating: shared fork/decision budget spent",
			"bead", in.Deps.BeadID, "limit", forkHandoverLimit)
		decision := ForkDecision{
			Action: ForkActionEscalate,
			Cause:  ForkCauseBudgetSpent,
			Reason: fmt.Sprintf("this run already spent its shared budget of %d fork/decision handover(s) - Fix: resolve the pending decision by hand, or raise it deliberately if this bead genuinely needs more", forkHandoverLimit),
		}
		return gateFailureHandled{Result: blockBeadForDecision(in.Deps, in.ActiveState, "review_decision_escalated", decision)}, nil
	}

	// Checked BEFORE the DA is ever asked anything (finding 4 of the
	// fork/decision-gate hardening pass): consulting the DA for an answer
	// that cannot be rewound to any stage anyway - the rewind budget already
	// spent, or this workflow declaring no retake state - would spend a real
	// consultation and record a real decision that nothing downstream could
	// ever carry to another attempt. reviewRejectionCanBeRewound is the exact
	// predicate rewindAfterReviewRejection itself uses below, so the two can
	// never disagree about whether a rewind is possible.
	if !reviewRejectionCanBeRewound(in.WF, in.ReviewRewinds) {
		slog.Info("DRIVE_TRACE review-raised decision escalating before consulting the DA: no rewind is possible",
			"bead", in.Deps.BeadID, "reviewRewinds", in.ReviewRewinds, "retake", in.WF.RetakeState)
		decision := ForkDecision{
			Action: ForkActionEscalate,
			Cause:  ForkCauseRewindNotPossible,
			Reason: "the rewind budget is already spent, or this workflow declares no retake state to send an answer back to - consulting the DA for an answer that could not reach anyone was skipped rather than wasted",
		}
		return gateFailureHandled{Result: blockBeadForDecision(in.Deps, in.ActiveState, "review_decision_escalated", decision)}, nil
	}

	facts, err := gatherForkScopeFactsForBead(ctx, in.Deps, in.Bead, in.EpicID)
	if err != nil {
		return gateFailureHandled{}, err
	}

	handover := forkHandoverFromImplementationRejection(rejection, reviewText)
	decision := DecideForkAction(ctx, handover, facts, in.Deps.DA)

	if decision.Action != ForkActionDecided {
		return gateFailureHandled{Result: blockBeadForDecision(in.Deps, in.ActiveState, "review_decision_escalated", decision)}, nil
	}

	if err := writeForkAnswerArtifact(in.Deps.BeadID, in.ArtifactDir, decision); err != nil {
		return gateFailureHandled{}, err
	}
	// Recorded the moment the decision becomes durable - not after the
	// rewind attempt below, so even a rewind that then fails (a tracker
	// error, not "no rewind possible" - that path already returned above)
	// still leaves the fact that this exact rejection was answered, so a
	// human retrying the bead by hand cannot cause it to be asked twice
	// either. See reviewDecisionAlreadyRouted's own doc comment.
	if err := markReviewDecisionRouted(in.Deps.BeadID, in.ArtifactDir, reviewText); err != nil {
		return gateFailureHandled{}, err
	}
	_ = in.Deps.Backend.Comment(in.Deps.BeadID,
		fmt.Sprintf("review_decision_decided: chose %q [%s]: %s", decision.ChosenOption, decision.Cause, decision.Reason),
		in.Deps.RepoPath)

	rewound, err := rewindAfterReviewRejection(in.Deps, in.WF, in.GateReason, in.ReviewRewinds)
	if err != nil {
		return gateFailureHandled{}, err
	}
	if rewound {
		return gateFailureHandled{Reenter: true, ReviewRewinds: in.ReviewRewinds + 1, ForkGateCalls: in.ForkGateCalls + 1}, nil
	}
	// reviewRejectionCanBeRewound was already checked true above, using the
	// exact same predicate rewindAfterReviewRejection itself consults, so
	// rewound=false here means only a genuine tracker error occurred -
	// nothing this function's own state (WF, ReviewRewinds) could have
	// predicted and pre-empted. Blocking rather than treating it as a hard
	// failure keeps this branch's contract identical to every other
	// unrewound rejection's "nowhere to send it" fallback.
	return gateFailureHandled{Result: blockBeadForGateFailure(in.Deps, in.ActiveState, in.GateReason)}, nil
}

// forkHandoverFromImplementationRejection adapts a reviewer's KindDecision
// classification into the ForkHandover shape DecideForkAction already
// consumes (fork_handover.go), rather than forking the policy function for a
// second caller.
//
// The three ForkHandover sections do not map onto what a reviewer writes
// one-for-one: an implementer's own handover states a fork, the options it
// weighed, and what would have to agree with the choice, as three separate,
// deliberate sections. A reviewer raising a decision post hoc writes only
// ONE thing - the "## Question for the operator" this package requires - so
// the mapping is:
//
//   - Fork: the reviewer's own question - what has to be decided.
//   - OptionsConsidered: the full rejected review, verbatim. The reviewer's
//     own reasoning about the option space lives in that prose, not in a
//     second heading this parser would have to invent and then require the
//     reviewer to duplicate.
//   - WhatWouldHaveToAgree: synthesized, not authored. The very fact the
//     reviewer classified this review.KindDecision already IS the answer -
//     "this needs the operator's explicit agreement, not silent
//     resolution" is what the classification itself asserts - so there is
//     nothing further for the reviewer to declare here.
func forkHandoverFromImplementationRejection(rejection *ImplementationRejection, reviewText string) *ForkHandover {
	return &ForkHandover{
		Fork:                 rejection.Question,
		OptionsConsidered:    strings.TrimSpace(reviewText),
		WhatWouldHaveToAgree: "a reviewer classified this rejection as needing an explicit approved decision rather than a fix-up - that classification is itself the claim that something outside silent resolution has to agree",
	}
}

// blockBeadForGateFailure is the ordinary "nowhere to send this" block every
// non-rejection gate failure (and every rejection that cannot be rewound)
// falls back to - factored out so handleGateFailure's own return points read
// identically to what drive_bead.go inlined twice before this pass.
func blockBeadForGateFailure(deps DriveBeadDeps, activeState, gateReason string) RunBeadResult {
	blockBeadWithCause(deps.Backend, deps.BeadID, deps.RepoPath, BlockedCauseGate)
	_ = deps.Backend.Comment(deps.BeadID, "gate_failed: "+gateReason, deps.RepoPath)
	return RunBeadResult{FinalState: "blocked", Success: false, BlockedAtState: activeState, GateFailureReason: gateReason}
}

// blockBeadForDecision blocks a bead for the operator with a ForkDecision's
// own cause and reason rendered under gateName - the one place BOTH the
// proactive fork gate (fork_gate.go's handleForkGate) and the
// reviewer-raised decision gate (handleReviewRaisedDecision, above) turn an
// escalation into the same shape of blocked bead, so their vocabularies
// never drift apart - the same reuse epic_fixup.go's own escalate() already
// established for the fix-up gate. One vocabulary across all three gates.
func blockBeadForDecision(deps DriveBeadDeps, activeState, gateName string, decision ForkDecision) RunBeadResult {
	reason := fmt.Sprintf("%s [%s]: %s", gateName, decision.Cause, decision.Reason)
	blockBeadWithCause(deps.Backend, deps.BeadID, deps.RepoPath, BlockedCauseJudgment)
	_ = deps.Backend.Comment(deps.BeadID, reason, deps.RepoPath)
	return RunBeadResult{
		FinalState:        "blocked",
		Success:           false,
		BlockedAtState:    activeState,
		GateFailureReason: reason,
	}
}
