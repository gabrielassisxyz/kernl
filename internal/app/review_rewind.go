package app

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// implementationReviewRewindLimit is how many times one call to
// DriveBeadToTerminal will hand a rejected implementation back to an
// implementer before it gives up and blocks the bead.
//
// One, matching the integration fix-up's own cap and for the same reason: a
// reviewer that rejects the same work twice is describing something the loop
// is not going to fix by running again. The second rejection is a signal for
// a human, not a third attempt.
//
// The budget is per call, deliberately. An operator who re-runs the bead is
// making a decision, and a decision is exactly what an exhausted automatic
// budget is asking for.
const implementationReviewRewindLimit = 1

// verdictRejectReasonPrefix is the gate reason backend.evaluateSingleExitGate
// writes for a review that ended in "VERDICT: REJECT" - a deliberate
// rejection, as opposed to verdict_not_pass, which is every other way a
// verdict gate fails (missing artifact, truncated output, a reviewer that ran
// out of budget). Only the first sends work back.
const verdictRejectReasonPrefix = "verdict_reject:"

// isDeliberateRejection reports whether a gate failure at this state is a
// reviewer rejecting the work, rather than the stage failing some other way.
func isDeliberateRejection(state, gateReason string) bool {
	return state == "implementation_review" && strings.HasPrefix(gateReason, verdictRejectReasonPrefix)
}

// reviewRewindBudgetSpent reports whether this call has already used up
// implementationReviewRewindLimit - shared by rewindAfterReviewRejection and
// handleReviewRaisedDecision's own pre-check (finding 4 of the
// fork/decision-gate hardening pass), so the two can never disagree about
// whether a rewind is even worth attempting.
func reviewRewindBudgetSpent(rewindsUsed int) bool {
	return rewindsUsed >= implementationReviewRewindLimit
}

// reviewRewindHasNoRetakeState reports whether wf leaves nowhere for a
// rejected implementation_review to be sent back to - either no retake state
// at all, or one that points right back at the reviewing stage itself, which
// would re-review the same unchanged work forever. Shared with
// handleReviewRaisedDecision's own pre-check for the same reason
// reviewRewindBudgetSpent is.
func reviewRewindHasNoRetakeState(wf backend.WorkflowDescriptor) bool {
	retake := strings.TrimSpace(wf.RetakeState)
	return retake == "" || retake == "implementation_review"
}

// reviewRejectionCanBeRewound reports whether rewindAfterReviewRejection has
// anywhere at all to send a rejected bead, without attempting the tracker
// call itself - built from the exact same two checks
// (reviewRewindBudgetSpent, reviewRewindHasNoRetakeState) that function
// itself makes, so the two can never drift apart into disagreeing about
// whether a rewind is possible.
//
// handleReviewRaisedDecision (review_decision_gate.go) no longer calls this
// as one combined "may the DA even be asked" gate the way it originally did
// (finding 4 of the fork/decision-gate hardening pass): a spent budget and a
// missing retake state stopped being the same reason to skip the DA the
// moment the DA was given one rewind to grant back (see that function's own
// doc comment). It now calls reviewRewindHasNoRetakeState and
// reviewRewindBudgetSpent separately - the same two building blocks this
// predicate is made of - so the invariant this comment describes still
// holds, just no longer through one shared call site.
func reviewRejectionCanBeRewound(wf backend.WorkflowDescriptor, rewindsUsed int) bool {
	return !reviewRewindBudgetSpent(rewindsUsed) && !reviewRewindHasNoRetakeState(wf)
}

// rewindAfterReviewRejection sends a rejected bead back to the stage that can
// answer the rejection, and reports whether it did.
//
// It answers false - leaving the caller to block the bead exactly as before -
// in three cases, each of which is a rejection with nowhere to go:
//
//   - the budget is spent (see implementationReviewRewindLimit)
//   - the workflow declares no retake state, so there is no stage to return to
//   - the retake state is the reviewing stage itself, which would re-review
//     the same unchanged work forever
//
// A tracker that refuses the rewind is an error, not a quiet block: the bead
// would otherwise be left reading "blocked" while this function reported it
// had been sent back.
func rewindAfterReviewRejection(deps DriveBeadDeps, wf backend.WorkflowDescriptor, gateReason string, rewindsUsed int) (bool, error) {
	if reviewRewindBudgetSpent(rewindsUsed) {
		slog.Info("DRIVE_TRACE review rejection not rewound: budget spent",
			"bead", deps.BeadID, "limit", implementationReviewRewindLimit)
		return false, nil
	}
	if reviewRewindHasNoRetakeState(wf) {
		slog.Info("DRIVE_TRACE review rejection not rewound: no retake state",
			"bead", deps.BeadID, "workflow", wf.ID, "retake", strings.TrimSpace(wf.RetakeState))
		return false, nil
	}

	retake := strings.TrimSpace(wf.RetakeState)
	reason := fmt.Sprintf("implementation_review rejected this work (%s); returning it to %s to be answered", gateReason, retake)
	if err := deps.Backend.Rewind(deps.BeadID, retake, reason, deps.RepoPath); err != nil {
		return false, fmt.Errorf("KERNL DISPATCH FAILURE: bead %s was rejected at implementation_review and could not be returned to %s: %w - Fix: the bead is left at implementation_review; retry once the tracker accepts the update", deps.BeadID, retake, err)
	}
	slog.Info("DRIVE_TRACE review rejection rewound", "bead", deps.BeadID, "to", retake)
	return true, nil
}

// readRejectedReview returns the text of a review artifact that ends in
// "VERDICT: REJECT", or "" for anything else.
//
// The artifact is its own record of whether it rejected, so the next
// implementer's prompt needs no separate flag to decide whether to carry it:
// a review that passed, a review that was never written, and a review that
// failed some other way all read as "nothing to answer" here. A REJECT left
// on disk from an earlier attempt is not stale for this purpose either - it
// means the last review of this bead did reject, which is exactly what the
// implementer needs to know.
func readRejectedReview(path string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	trimmed := strings.TrimSpace(string(data))
	lastLine := trimmed
	if idx := strings.LastIndexByte(trimmed, '\n'); idx != -1 {
		lastLine = strings.TrimSpace(trimmed[idx+1:])
	}
	if lastLine != "VERDICT: REJECT" {
		return ""
	}
	return trimmed
}
