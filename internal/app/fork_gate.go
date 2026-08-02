package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// forkHandoverArtifactPath is the one, well-known place an implementer's
// fork handover lives - inside this bead's own artifact directory, resolved
// through backend.ResolveArtifactPath exactly as renderDecisionRecord
// already resolves the decision record's own path (see renderForkHandover in
// prompt.go, the section that tells an implementer to write here).
const forkHandoverArtifactPath = "<artifact_dir>/fork.md"

// forkAnswerArtifactPath is where the DA's decision is written once a fork is
// decided, and read back at the next stage entry so the implementer sees its
// own answer (see renderForkAnswer in prompt.go) - mirroring how
// readRejectedReview feeds RejectedReview from implementation-review.md.
const forkAnswerArtifactPath = "<artifact_dir>/fork-answer.md"

// forkHandoverLimit bounds how many fork handovers ONE call to
// DriveBeadToTerminal will decide (re-entering the same stage each time)
// before refusing to ask the DA again and escalating instead.
//
// Greater than one, unlike implementationReviewRewindLimit (which is
// deliberately 1): a rewind answers a REVIEWER's rejection of work already
// done, and a second rejection of the same work is a signal for a human, not
// a third attempt. A fork handover is the opposite shape - an implementer
// can legitimately meet more than one genuine fork while writing a single
// stage's code (a storage choice, then an unrelated naming choice), and each
// is a fresh, independent question, not the same objection repeated. A limit
// of one would turn a second, unrelated fork into a block. Three matches
// config.DefaultFixupBudget's own magnitude and reasoning: room for a
// stage that is genuinely encountering distinct forks, while still
// surfacing an implementer that will not stop asking within the same call.
const forkHandoverLimit = 3

// ForkCauseBudgetSpent names an escalation this package's own loop produces,
// not DecideForkAction's - the fork-handover budget is a property of one
// DriveBeadToTerminal call (see forkHandoverLimit), not of the pure policy
// fork_handover.go implements, the same reason implementationReviewRewindLimit
// lives in review_rewind.go rather than inside a decision function.  It is
// declared here, as its own ForkGateCause value, rather than added to
// fork_handover.go: that file's causes are all produced by DecideForkAction
// itself, and this one never is.
const ForkCauseBudgetSpent ForkGateCause = "fork_gate_budget_spent"

// forkHandoverArmed reports whether this stage may hand a fork to the DA at
// all: a DA has to be configured to ask, AND the state must declare a
// decision_record exit gate - the same gate that already requires every
// OTHER fork the implementer resolves alone to be written down. A reviewer
// stage never declares decision_record, so it is never armed, and is never
// told in its own prompt that it may hand anything over (see
// StagePromptInput.ForkHandoverPath).
func forkHandoverArmed(wf backend.WorkflowDescriptor, state string, da DA) bool {
	if da == nil {
		return false
	}
	_, ok := backend.FindExitGateByType(wf, state, "decision_record")
	return ok
}

// resolvedForkHandoverPath and resolvedForkAnswerPath expand the two
// well-known artifact names against one bead's own artifact directory.
func resolvedForkHandoverPath(beadID, artifactDir string) string {
	return backend.ResolveArtifactPath(forkHandoverArtifactPath, beadID, artifactDir)
}

func resolvedForkAnswerPath(beadID, artifactDir string) string {
	return backend.ResolveArtifactPath(forkAnswerArtifactPath, beadID, artifactDir)
}

// readForkHandoverArtifact reads the handover an implementer wrote, or
// reports present=false - the normal case for every attempt where the
// implementer simply wrote code and committed instead of handing a fork
// over.
func readForkHandoverArtifact(beadID, artifactDir string) (content string, present bool) {
	data, err := os.ReadFile(resolvedForkHandoverPath(beadID, artifactDir))
	if err != nil {
		return "", false
	}
	return string(data), true
}

// consumeForkHandover renames the handover artifact to an archived name so
// it can never fire twice, rather than deleting it, so the record survives
// for the run report.
//
// This differs deliberately from readRejectedReview, which leaves a stale
// REJECT verdict in place on purpose: a rejection describes a state of the
// WORK that is still true until a new review overwrites it, so re-reading it
// is correct even on a retry. A fork handover describes an EVENT - "the
// implementer stopped and asked" - that this call is in the middle of
// answering. Leaving fork.md in place after it has been decided or escalated
// would make the very next stage entry (the re-run this same gate causes,
// or an unrelated later retry) discover the identical file and treat an
// already-answered question as a brand new one, forever.
//
// attempt is folded into the archived name so a SECOND handover in the same
// call (see forkHandoverLimit) does not overwrite the first one's record.
func consumeForkHandover(beadID, artifactDir string, attempt int) error {
	from := resolvedForkHandoverPath(beadID, artifactDir)
	to := fmt.Sprintf("%s.answered.%d", from, attempt)
	if err := os.Rename(from, to); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: consuming fork handover %s for bead %s: %w - Fix: kernl needs write access to this bead's own artifact directory", from, beadID, err)
	}
	return nil
}

// forkAnswerEntrySeparator divides two decided forks recorded in the same
// fork-answer.md - see writeForkAnswerArtifact's own doc comment for why this
// file is appended to, never overwritten.
const forkAnswerEntrySeparator = "\n---\n"

// writeForkAnswerArtifact APPENDS the DA's decision so every later stage
// entry can carry EVERY fork this call has decided, in the order they were
// decided (see readForkAnswerArtifact). Only ever called for a decided fork:
// escalating leaves nothing for a future prompt to carry, because the bead
// is blocked for the operator instead of being re-entered.
//
// This used to call os.WriteFile, which overwrites. forkHandoverLimit is
// deliberately greater than one (see that constant's own doc comment)
// precisely because an implementer can genuinely meet more than one distinct
// fork while writing a single stage's code - and the moment a second fork
// was decided in the same call, os.WriteFile erased the first answer
// entirely. The next re-entered attempt's prompt then carried only the
// SECOND answer; the first fork's own decision was gone, so that attempt
// re-derived and silently re-decided it alone - the exact failure this whole
// unit exists to prevent, happening inside the case the code already planned
// for. Appending, with a separator between entries, is what makes
// readForkAnswerArtifact return all of them.
func writeForkAnswerArtifact(beadID, artifactDir string, decision ForkDecision) error {
	entry := fmt.Sprintf("CHOSEN: %s\n\n%s\n", decision.ChosenOption, strings.TrimSpace(decision.Reason))
	path := resolvedForkAnswerPath(beadID, artifactDir)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: writing the DA's fork answer to %s for bead %s: %w", path, beadID, err)
	}
	defer f.Close()

	if info, statErr := f.Stat(); statErr == nil && info.Size() > 0 {
		if _, err := f.WriteString(forkAnswerEntrySeparator); err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: writing the DA's fork answer to %s for bead %s: %w", path, beadID, err)
		}
	}
	if _, err := f.WriteString(entry); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: writing the DA's fork answer to %s for bead %s: %w", path, beadID, err)
	}
	return nil
}

// readForkAnswerArtifact returns EVERY decision this bead's fork-answer.md
// has accumulated so far, in the order they were decided (see
// writeForkAnswerArtifact), or "" for a bead that never handed a fork over.
func readForkAnswerArtifact(beadID, artifactDir string) string {
	data, err := os.ReadFile(resolvedForkAnswerPath(beadID, artifactDir))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// forkGateAttemptContext is everything one dispatch path (subprocess or
// native) already knows about the stage attempt that just exited zero,
// handed to handleForkGate so both paths write exactly one ledger row for a
// fork-gate outcome through the same code, rather than each improvising its
// own shape - the same reuse EvaluateExitGate's own two call sites already
// follow for the ordinary gate-failure ledger row.
type forkGateAttemptContext struct {
	Deps DriveBeadDeps
	WF   backend.WorkflowDescriptor
	// Bead is the freshly re-fetched bead each dispatch path already reads
	// right before building its own ExitGateContext (gateDesc/freshBead) -
	// passed in rather than re-fetched here, so this check costs no second
	// read of the same row.
	Bead        *backend.Bead
	ActiveState string
	ArtifactDir string
	EpicID      string
	// CallsUsed is how many fork handovers THIS call to DriveBeadToTerminal
	// has already decided (re-entered the stage for) before this attempt -
	// the budget forkHandoverLimit bounds.
	CallsUsed int
	// LedgerInput carries every field the caller's own dispatch path already
	// resolved for the ordinary ledger row (agent/dialect/timing/SHAs/...).
	// GatePassed and GateFailureReason are overwritten by handleForkGate
	// itself; every other field is used exactly as given.
	LedgerInput StageAttemptInput
}

// forkGateHandled is what handleForkGate decided.
type forkGateHandled struct {
	// Applied is false when there was nothing to handle at all - the stage
	// is not armed, or the implementer did not hand a fork over this
	// attempt. The caller proceeds to backend.EvaluateExitGate exactly as it
	// always has; nothing about this bead's normal gate evaluation changes.
	Applied bool
	// Reenter is true when a fork was decided: the caller must skip its own
	// state-transition/blocking logic entirely and `continue` its loop
	// without touching the bead's state, so the very next iteration
	// re-dispatches the same stage (see DriveBeadToTerminal's own doc
	// comment on why a plain continue is enough here).
	Reenter bool
	// Result is set only when Applied is true and Reenter is false - an
	// escalation. The caller returns this immediately, exactly as it would
	// any other unresolved gate failure.
	Result RunBeadResult
}

// handleForkGate is the one place that reads a fork handover, decides or
// escalates it, records the attempt, and tells the caller what to do next.
// Both dispatch paths in drive_bead.go call this in the same spot: after the
// agent has exited zero, and BEFORE backend.EvaluateExitGate is ever called.
// That ordering is load-bearing, not incidental: an implementer that handed
// a fork over stopped deliberately without committing, so the ordinary
// commit_marker/decision_record gates would fail it - turning the handover
// into exactly the interruption this gate exists to prevent. Evaluating the
// exit gate at all for this attempt would be asking a question that has
// already been overtaken by a different, more specific one.
func handleForkGate(ctx context.Context, in forkGateAttemptContext) (forkGateHandled, error) {
	if !forkHandoverArmed(in.WF, in.ActiveState, in.Deps.DA) {
		return forkGateHandled{}, nil
	}
	content, present := readForkHandoverArtifact(in.Deps.BeadID, in.ArtifactDir)
	if !present {
		return forkGateHandled{}, nil
	}

	// Consumed immediately, before any decision is made: whatever happens
	// next (a clean decision, an escalation, even a failure gathering
	// facts), this same handover must never be read as a fresh one again.
	// See consumeForkHandover's own doc comment for why this differs from
	// readRejectedReview.
	if err := consumeForkHandover(in.Deps.BeadID, in.ArtifactDir, in.CallsUsed+1); err != nil {
		return forkGateHandled{}, err
	}

	decision, err := decideForkAttempt(ctx, in, content)
	if err != nil {
		// A genuine infrastructure failure (the tracker could not be asked
		// which siblings depend on this bead) is not this fork's own
		// escalation - it is the same shape as every other "could not even
		// look" failure in this loop (e.g. resolveArtifactDir), and it fails
		// the whole call rather than being folded into a swallowed
		// escalation the operator would have to notice was a proxy for an
		// outage.
		return forkGateHandled{}, err
	}

	in.LedgerInput.GatePassed = false
	in.LedgerInput.GateFailureReason = fmt.Sprintf("fork_gate:%s: %s", decision.Cause, decision.Reason)
	if err := AppendStageAttempt(in.Deps.StateDir, in.EpicID, BuildStageAttemptRecord(in.LedgerInput)); err != nil {
		// Not recording this attempt would bias the ledger toward stages
		// that happened not to hit a fork - the same argument the
		// subprocess-failure branch above this one in drive_bead.go already
		// makes for its own ledger write.
		slog.Error("DRIVE_TRACE fork gate attempt ledger write failed", "bead", in.Deps.BeadID, "err", err)
	}

	if decision.Action == ForkActionDecided {
		if err := writeForkAnswerArtifact(in.Deps.BeadID, in.ArtifactDir, decision); err != nil {
			return forkGateHandled{}, err
		}
		_ = in.Deps.Backend.Comment(in.Deps.BeadID, fmt.Sprintf("fork_decided: chose %q [%s]: %s", decision.ChosenOption, decision.Cause, decision.Reason), in.Deps.RepoPath)
		return forkGateHandled{Applied: true, Reenter: true}, nil
	}

	// Escalate: block the bead exactly like any other unresolved gate
	// failure, but naming the fork gate's own vocabulary verbatim so an
	// operator can grep for it - the same reason epic_fixup.go's escalate()
	// names its own cause verbatim, so one vocabulary covers both gates (and
	// a third, review_decision_gate.go's own escalation - see
	// blockBeadForDecision, shared by both).
	return forkGateHandled{
		Applied: true,
		Result:  blockBeadForDecision(in.Deps, in.ActiveState, "fork_gate_escalated", decision),
	}, nil
}

// decideForkAttempt gathers the measured scope facts and asks
// DecideForkAction, unless this call's own budget (forkHandoverLimit) is
// already spent - in which case it escalates without asking the DA again,
// the same shape rewindAfterReviewRejection's own budget check takes.
//
// The only error this returns is a genuine infrastructure failure measuring
// scope (GatherForkScopeFacts could not ask the tracker who depends on this
// bead) - never a fork gate escalation, which is a ForkDecision, not an
// error.
func decideForkAttempt(ctx context.Context, in forkGateAttemptContext, handoverContent string) (ForkDecision, error) {
	if in.CallsUsed >= forkHandoverLimit {
		slog.Info("DRIVE_TRACE fork gate escalating: budget spent", "bead", in.Deps.BeadID, "limit", forkHandoverLimit)
		return ForkDecision{
			Action: ForkActionEscalate,
			Cause:  ForkCauseBudgetSpent,
			Reason: fmt.Sprintf("this run already spent its budget of %d fork handover(s) - Fix: resolve the pending fork by hand, or raise it deliberately if this bead genuinely needs more", forkHandoverLimit),
		}, nil
	}

	facts, err := gatherForkScopeFactsForBead(ctx, in.Deps, in.Bead, in.EpicID)
	if err != nil {
		return ForkDecision{}, err
	}

	return DecideForkAction(ctx, ParseForkHandover(handoverContent), facts, in.Deps.DA), nil
}
