package app

import (
	"strconv"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// BlockedCause is the durable, tracker-side answer to "why is this bead
// blocked". "blocked" itself stays the single tracker status kernl uses -
// adding a second terminal status ("failed") would touch the profile
// descriptors, the state machine, the resume filter, the GUI, the run report
// and both trackers, for a distinction that a label already carries just as
// durably. Three call sites write one of these, and only ever one at a time
// (see blockBeadWithCause):
//
//   - BlockedCauseSubprocess: drive_bead.go's subprocess flow - the agent CLI
//     itself died (crash, rate limit, timeout, output cap, non-zero exit).
//     Nobody decided anything.
//   - BlockedCauseGate: review_decision_gate.go's blockBeadForGateFailure -
//     an exit gate failed for a mechanical reason (missing artifact, no
//     commit, truncated output). A stage that did not complete.
//   - BlockedCauseJudgment: review_decision_gate.go's blockBeadForDecision -
//     a deliberate escalation (a proactive fork handover, or a
//     reviewer-raised decision) whose rewind was refused. The only one of
//     the three that actually means a human is needed.
type BlockedCause string

const (
	BlockedCauseSubprocess BlockedCause = "subprocess"
	BlockedCauseGate       BlockedCause = "gate"
	BlockedCauseJudgment   BlockedCause = "judgment"
)

// IsMechanical reports whether cause names an outcome a re-run may retry on
// its own, as opposed to BlockedCauseJudgment (or no recorded cause at all),
// which asks for a decision only a human can make and must never be resumed
// silently.
func (c BlockedCause) IsMechanical() bool {
	return c == BlockedCauseSubprocess || c == BlockedCauseGate
}

// blockedCauseLabelPrefix and blockedRetryLabelPrefix are the label families
// DriveBeadToTerminal's entry branch reads back once a bead arrives already
// "blocked" - mirroring wf:state:*'s own durable-fact pattern (see
// stateFromStaleLabel in drive_bead.go) rather than only ever describing the
// cause in a mutable comment. blockedRetryLabelPrefix counts how many
// automatic mechanical resumes have already been spent on the CURRENT block,
// so a subprocess or gate that keeps failing cannot be retried forever
// across many separate `kernl epic run` invocations (see
// mechanicalBlockRetryLimit).
const (
	blockedCauseLabelPrefix = "wf:blocked:"
	blockedRetryLabelPrefix = "wf:blocked-retries:"
)

// mechanicalBlockRetryLimit bounds how many times DriveBeadToTerminal's entry
// branch will clear a mechanical block (BlockedCauseSubprocess or
// BlockedCauseGate) and re-attempt its stage before leaving the bead blocked
// for a human instead.
//
// Two. A single mechanical failure is very often a blip - a rate limit, a
// dropped connection - and the first automatic retry exists to absorb
// exactly that without spending a human's attention. A second consecutive
// failure at the very same stage stops looking like a blip and starts
// looking like a real, repeating problem that a third blind attempt is not
// going to fix - the same reasoning implementationReviewRewindLimit already
// applies to a rejected review, loosened by one step because a mechanical
// failure, unlike a reviewer's deliberate rejection, carries no information
// about what to change before trying again.
const mechanicalBlockRetryLimit = 2

// blockBeadWithCause sets beadID to "blocked" and records cause as a durable
// wf:blocked:<cause> label - the one write path all three of
// DriveBeadToTerminal's own block sites go through (drive_bead.go's
// subprocess failure branch, and review_decision_gate.go's
// blockBeadForGateFailure / blockBeadForDecision), so the label vocabulary
// can never drift between them.
//
// Labels are read fresh from the tracker rather than from a caller-held bead
// snapshot: DriveBeadToTerminal's own claim step can already have advanced
// the bead's wf:state label earlier in the SAME iteration a block is
// written in, and a stale snapshot taken before that write would roll
// wf:state back to its pre-claim value the moment this call replaced the
// label set wholesale via SetLabels. Any existing wf:blocked:* is replaced,
// never accumulated - a bead has exactly one live cause at a time - while
// every other label, in particular wf:state:* and wf:blocked-retries:*, is
// left untouched.
//
// Best-effort like every other write on this path (the Update error is
// discarded exactly as it already was before this label existed): a tracker
// that refuses the write leaves the bead's State stuck at whatever it was,
// which is a worse, more visible failure than a missing label on top of it.
func blockBeadWithCause(be backend.BackendPort, beadID, repoPath string, cause BlockedCause) {
	_ = BlockBeadWithCause(be, beadID, repoPath, cause)
}

// BlockBeadWithCause is the same write for a caller that has somewhere to
// report a failed one. cmd/kernl's post-shipment verification is the only
// such caller: it blocks an epic that published to a repository outside the
// configured allowlist, and a block that silently failed there would leave
// the tracker reading as a successful run, which is the one outcome that
// check exists to prevent.
//
// It records BlockedCauseJudgment, and doing so explicitly matters even
// though an absent label already resumes the same way. Behaviour that is
// correct only because nothing was written is behaviour one stale label
// away from being wrong: a wf:blocked:subprocess left over from an earlier
// mechanical block would otherwise survive this write and invite a re-run to
// resume, automatically, an epic held back precisely because a human has to
// look at where it published.
func BlockBeadWithCause(be backend.BackendPort, beadID, repoPath string, cause BlockedCause) error {
	var labels []string
	if bead, err := be.Get(beadID, repoPath); err == nil && bead != nil {
		labels = bead.Labels
	}
	labels = filterOutLabelPrefix(labels, blockedCauseLabelPrefix)
	labels = append(labels, blockedCauseLabelPrefix+string(cause))
	return be.Update(beadID, backend.UpdateBeadInput{State: "blocked", SetLabels: labels}, repoPath)
}

// BlockedCauseFromLabels recovers which of the three causes blocked a bead,
// or "" when no wf:blocked:* label is present - a bead blocked some other
// way, e.g. by hand. DriveBeadToTerminal's entry branch and the CLI's own
// blocked-run message both treat that the same as BlockedCauseJudgment: stay
// put, a human is needed.
func BlockedCauseFromLabels(labels []string) BlockedCause {
	for _, l := range labels {
		if strings.HasPrefix(l, blockedCauseLabelPrefix) {
			return BlockedCause(strings.TrimPrefix(l, blockedCauseLabelPrefix))
		}
	}
	return ""
}

// blockedRetryCountFromLabels recovers how many automatic mechanical resumes
// the current block has already spent - 0 when no wf:blocked-retries: label
// is present (a fresh block, or one whose count was reset by a later claim
// into a new stage; see the claim step's own comment in drive_bead.go).
func blockedRetryCountFromLabels(labels []string) int {
	for _, l := range labels {
		if strings.HasPrefix(l, blockedRetryLabelPrefix) {
			n, err := strconv.Atoi(strings.TrimPrefix(l, blockedRetryLabelPrefix))
			if err != nil {
				return 0
			}
			return n
		}
	}
	return 0
}
