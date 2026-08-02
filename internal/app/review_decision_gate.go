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
// pass) when this workflow declares no retake state (or one that points
// back at the reviewer itself) to send an answer back to - decided BEFORE
// the DA is ever consulted, never by DecideForkAction itself.
//
// This used to also fire when the ordinary rewind budget was merely spent,
// on the theory that an answer with nowhere to go should never be asked
// for. That conflated two different facts under one cause (see this
// constant's own history): a workflow with no retake state truly has
// nowhere an answer could ever go, no matter how many rewinds exist, while a
// spent BUDGET is exactly the signal that this disagreement is worth an
// outside judgment - see ForkCauseRewindBudgetSpentAfterGrant, and
// handleReviewRaisedDecision's own doc comment, for what a spent budget does
// instead now.
const ForkCauseRewindNotPossible ForkGateCause = "review_decision_rewind_not_possible"

// ForkCauseRewindBudgetSpentAfterGrant names the escalation
// handleReviewRaisedDecision's own pre-check produces when the ordinary
// rewind budget (implementationReviewRewindLimit) is spent AND this bead has
// already spent the one extra rewind the DA may grant (see
// reviewRewindExtraAlreadyGranted). The DA gets exactly one chance to turn a
// spent budget into one more rewind per bead; a second exhaustion after that
// grant is consumed is the same disagreement continuing, and asking the DA
// again could not resolve it any differently - only a human can now.
const ForkCauseRewindBudgetSpentAfterGrant ForkGateCause = "review_decision_rewind_budget_spent_after_grant"

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

// reviewRewindExtraGrantArtifactPath is the well-known artifact recording
// that this bead's ordinary rewind budget (implementationReviewRewindLimit)
// has already been topped up once by the DA. A SEPARATE file from
// reviewDecisionAlreadyRouted's own marker (reviewDecisionRoutedMarkerSuffix,
// above): that one is keyed by the exact rejection TEXT it answered, and
// exists to stop the identical stale review being routed twice. This one is
// a fact about the BEAD as a whole - "has the DA already spent its one
// top-up here" - that must hold regardless of which review happens to be on
// disk, or whether it changed, the next time the budget runs out.
//
// The path this resolves to is scoped by the bead's CURRENT artifact
// directory, which is keyed in part by its current parent epic (see
// ArtifactDirPath). Re-parenting or detaching the bead between rejections
// resolves a different directory with no marker in it, and the bead could
// earn one more grant than intended. Accepted for a solo-dev tool: bounded to
// one extra grant, and re-parenting a bead mid-rejection-cycle is rare.
const reviewRewindExtraGrantArtifactPath = "<artifact_dir>/review-rewind-extra-grant.md"

func resolvedReviewRewindExtraGrantPath(beadID, artifactDir string) string {
	return backend.ResolveArtifactPath(reviewRewindExtraGrantArtifactPath, beadID, artifactDir)
}

// reviewRewindExtraAlreadyGranted reports whether the DA has already topped
// up this bead's rewind budget once - checked before consulting the DA a
// second time the budget is found spent, so the top-up is granted AT MOST
// ONCE per bead. This has to be a file, not the in-memory ReviewRewinds
// counter threaded through gateFailureContext/gateFailureHandled: that
// counter is scoped to one DriveBeadToTerminal call and starts back at zero
// the next time this bead is resumed by a separate `kernl epic run`
// invocation, which is exactly the shape the real deadlock this backstop
// exists for took (three implementation attempts spanning two runs, per the
// case that motivated this gate). A file in the bead's own artifact
// directory - the same durability reviewDecisionAlreadyRouted's own marker
// already relies on - survives both: the re-entry loop inside one call, and
// a bead picked back up later. What "survives" actually buys a resumed run
// is narrower than it sounds, though: the reset counter means
// reviewRewindBudgetSpent is false again on the first rejection after
// resume, so this marker is not even consulted then - the ordinary budget
// has room and the rejection rewinds normally. The marker only starts
// mattering on the SECOND exhaustion within the resumed run, the same as it
// would within one continuous run.
//
// The check below is not atomic with markReviewRewindExtraGranted's own
// write - two concurrent `kernl epic run` processes hitting the same
// spent-budget rejection could both Stat before either has written, and both
// grant. Left unguarded: the blast radius is bounded (one bead earns at most
// two extra rewinds instead of one, not an unbounded loop), a real collision
// needs two processes racing the same bead's rejection at the same instant,
// and the runstate layer upstream already contends for a bead before this
// gate is reached. Not worth a lock file for a solo-dev tool.
func reviewRewindExtraAlreadyGranted(beadID, artifactDir string) (bool, error) {
	path := resolvedReviewRewindExtraGrantPath(beadID, artifactDir)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("KERNL DISPATCH FAILURE: checking whether bead %s already spent its one DA-granted rewind top-up at %s: %w - Fix: kernl needs read access to this bead's own artifact directory", beadID, path, err)
}

// markReviewRewindExtraGranted records that fact, durably, the moment the DA
// decides - never on an escalation (see handleReviewRaisedDecision), so an
// escalating DA answer never spends the one grant a future, genuinely
// decisive consultation could still use.
func markReviewRewindExtraGranted(beadID, artifactDir, reason string) error {
	path := resolvedReviewRewindExtraGrantPath(beadID, artifactDir)
	if err := os.WriteFile(path, []byte(reason), 0o644); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: recording bead %s's one DA-granted rewind top-up at %s: %w - Fix: kernl needs write access to this bead's own artifact directory", beadID, path, err)
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
	// unchanged. No DA-granted top-up applies on this path - only a
	// decision-classified rejection can ever earn one.
	rewound, err := rewindAfterReviewRejection(in.Deps, in.WF, in.GateReason, in.ReviewRewinds, false)
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
//
// Two pre-checks run before the DA is ever consulted, and they are no longer
// one combined check (contrast this with the original fork/decision-gate
// hardening pass, whose single combined predicate folded both together):
//
//  1. No retake state to send an answer to (reviewRewindHasNoRetakeState) -
//     unconditional, because no amount of rewind budget fixes a workflow
//     that has nowhere to route the answer. The DA is never asked.
//  2. The ordinary rewind budget spent AND this bead's one DA-granted
//     top-up already used (reviewRewindExtraAlreadyGranted) - also skips
//     the DA, because a second exhaustion after the grant is the same
//     disagreement continuing.
//
// A spent budget alone (case 2, before the grant has ever been used) no
// longer skips the DA - it is exactly the moment three rejections deep that
// an outside judgment is worth the most, which is the defect this rewrite
// closes. Instead the DA is consulted, and if - and only if - it returns
// ForkActionDecided, this bead earns ONE extra rewind for the rest of its
// life (see markReviewRewindExtraGranted): the DA's answer needs somewhere
// to land, and the budget is what carries it there.
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

	// A workflow with nowhere to route an answer is unconditional: no
	// rewind budget, granted or not, changes that. Checked first, and
	// independent of the budget checks below.
	if reviewRewindHasNoRetakeState(in.WF) {
		slog.Info("DRIVE_TRACE review-raised decision escalating before consulting the DA: no retake state to send an answer to",
			"bead", in.Deps.BeadID, "workflow", in.WF.ID, "retake", in.WF.RetakeState)
		decision := ForkDecision{
			Action: ForkActionEscalate,
			Cause:  ForkCauseRewindNotPossible,
			Reason: "this workflow declares no retake state (or one that points back at the reviewer itself) to send an answer back to - consulting the DA for an answer that could not reach anyone was skipped rather than wasted",
		}
		return gateFailureHandled{Result: blockBeadForDecision(in.Deps, in.ActiveState, "review_decision_escalated", decision)}, nil
	}

	// The ordinary budget being spent no longer escalates by itself - it
	// only does once this bead has already spent the one top-up the DA may
	// grant for it (see reviewRewindExtraAlreadyGranted's own doc comment
	// for why that fact has to be a durable file rather than the in-memory
	// ReviewRewinds counter). Anything short of that still falls through to
	// consult the DA below, budget spent or not.
	budgetSpent := reviewRewindBudgetSpent(in.ReviewRewinds)
	if budgetSpent {
		alreadyGranted, err := reviewRewindExtraAlreadyGranted(in.Deps.BeadID, in.ArtifactDir)
		if err != nil {
			return gateFailureHandled{}, err
		}
		if alreadyGranted {
			slog.Info("DRIVE_TRACE review-raised decision escalating: rewind budget spent and its one DA-granted top-up already used",
				"bead", in.Deps.BeadID, "reviewRewinds", in.ReviewRewinds)
			decision := ForkDecision{
				Action: ForkActionEscalate,
				Cause:  ForkCauseRewindBudgetSpentAfterGrant,
				Reason: "the rewind budget is spent again after the DA already granted this bead its one extra rewind - Fix: resolve the pending decision by hand",
			}
			return gateFailureHandled{Result: blockBeadForDecision(in.Deps, in.ActiveState, "review_decision_escalated", decision)}, nil
		}
	}

	facts, err := gatherForkScopeFactsForBead(ctx, in.Deps, in.Bead, in.EpicID)
	if err != nil {
		return gateFailureHandled{}, err
	}

	handover := forkHandoverFromImplementationRejection(rejection, reviewText)
	decision := DecideForkAction(ctx, handover, facts, in.Deps.DA)

	if decision.Action != ForkActionDecided {
		// A non-decided answer (escalate, or the DA unreachable/unparseable)
		// must never spend the one grant: nothing here has a rewind to
		// carry it to, and a future consultation that genuinely decides
		// should not find the top-up already gone because an earlier one
		// declined to use it.
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

	// When the ordinary budget was spent, the DA having just decided means
	// this bead has earned its one grant: recorded durably FIRST (same
	// reasoning as markReviewDecisionRouted above - even a rewind that then
	// fails for an unrelated tracker reason must leave the grant spent,
	// never earning a second one on retry), then threaded into
	// rewindAfterReviewRejection as an explicit "this attempt is running on
	// the granted top-up" signal, rather than faked by presenting the
	// budget check one fewer rewind than actually used. A decrement only
	// ever creates room when the counter sits exactly at the limit, and does
	// nothing - no error, no rewind, just a burned grant - the moment it
	// sits any higher, which a caller that constructs gateFailureContext
	// directly (as tests do, and as a bead resumed by a separate run
	// legitimately can) is free to produce. Passing budgetSpent itself as
	// the override makes rewindAfterReviewRejection agree with THIS call's
	// own budget check no matter what in.ReviewRewinds actually is.
	// implementationReviewRewindLimit itself is never touched - every OTHER
	// rejection this run (or any other bead) is still measured against the
	// same constant.
	if budgetSpent {
		reason := fmt.Sprintf("granted after the DA decided %q [%s]: %s", decision.ChosenOption, decision.Cause, decision.Reason)
		if err := markReviewRewindExtraGranted(in.Deps.BeadID, in.ArtifactDir, reason); err != nil {
			return gateFailureHandled{}, err
		}
	}

	rewound, err := rewindAfterReviewRejection(in.Deps, in.WF, in.GateReason, in.ReviewRewinds, budgetSpent)
	if err != nil {
		return gateFailureHandled{}, err
	}
	if rewound {
		return gateFailureHandled{Reenter: true, ReviewRewinds: in.ReviewRewinds + 1, ForkGateCalls: in.ForkGateCalls + 1}, nil
	}
	// Both of rewindAfterReviewRejection's own reasons to hand back a quiet
	// false are already excluded by this point: reviewRewindHasNoRetakeState
	// was checked false above before the DA was ever consulted, and passing
	// budgetSpent itself as the override means its own budget check can
	// never disagree with the budgetSpent computed here. So a plain false
	// (no error) is not a state this call can reach - if it somehow does,
	// that is a defect in the reasoning above, not an ordinary "nowhere to
	// send it" rejection, and must not be blocked silently under the same
	// gate_failed reason as one.
	return gateFailureHandled{}, fmt.Errorf("KERNL DISPATCH FAILURE: bead %s's DA-granted rewind was not applied even though the pre-checks above found it possible - Fix: this is a defect in handleReviewRaisedDecision's own reasoning, not a tracker or config problem - report it rather than retrying", in.Deps.BeadID)
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
