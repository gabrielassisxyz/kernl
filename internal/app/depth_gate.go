package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/dispatch"
	"github.com/gabrielassisxyz/kernl/internal/prompt"
)

// ForkCauseOpenDesignDecided names the outcome where the DA settled the
// shape of a bead whose own text declared its design still open. Distinct
// from ForkCauseDADecided so a run's output can tell apart a fork an
// implementer handed over mid-stage from a bead that was never dispatched
// until its shape was settled.
const ForkCauseOpenDesignDecided ForkGateCause = "open_design_decided"

// ForkCauseOpenDesignEscalated names the DA's own judgment that a bead's
// open design belongs to the operator.
const ForkCauseOpenDesignEscalated ForkGateCause = "open_design_escalated"

// depthGateRoutedArtifactPath records that this bead's open design has
// already been put to the DA once - whatever the answer was.
//
// A separate file from fork-answer.md, which holds the ANSWERS: this one is
// the fact that the QUESTION was asked. The two differ for the escalating
// case, which writes no answer at all, and without this marker a bead the DA
// declined to settle would be re-asked on every resume, spending a real
// consultation each time to be told the same thing.
const depthGateRoutedArtifactPath = "<artifact_dir>/depth-gate-routed.md"

func resolvedDepthGateRoutedPath(beadID, artifactDir string) string {
	return backend.ResolveArtifactPath(depthGateRoutedArtifactPath, beadID, artifactDir)
}

// depthGateAlreadyRouted reports whether this bead's open design was already
// put to the DA. Any error other than "not there" surfaces rather than being
// read as "not yet asked": treating an unreadable artifact directory as a
// fresh bead is how the same consultation gets spent twice.
func depthGateAlreadyRouted(beadID, artifactDir string) (bool, error) {
	path := resolvedDepthGateRoutedPath(beadID, artifactDir)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("KERNL DISPATCH FAILURE: checking whether bead %s's open design was already put to the DA at %s: %w - Fix: kernl needs read access to this bead's own artifact directory", beadID, path, err)
}

func markDepthGateRouted(beadID, artifactDir, outcome string) error {
	path := resolvedDepthGateRoutedPath(beadID, artifactDir)
	if err := os.WriteFile(path, []byte(outcome), 0o644); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: recording that bead %s's open design was put to the DA at %s: %w - Fix: kernl needs write access to this bead's own artifact directory", beadID, path, err)
	}
	return nil
}

// decideOpenDesign is the policy for one open-design bead, mirroring
// DecideForkAction's layering deliberately: the certain answers first, the
// DA asked only what nothing else can settle.
//
// It does NOT reuse DecideForkAction itself, and that is the decision this
// unit turns on. That function's question requires a "## Options Considered"
// list the DA must choose from verbatim - RenderForkHandover refuses outright
// without one. A bead is classified DepthGate from its own title and
// description, before any agent has run, so no such list exists and none can
// be honestly manufactured. The two questions differ in kind, not in wording:
// one picks among alternatives somebody weighed, the other states a shape
// nobody has. See prompt.RenderOpenDesign for the rejected alternative of
// enumerating options first.
//
// The open-dependents layer is shared reasoning rather than shared code: a
// bead other open beads depend on is one where something outside it has to
// agree, which is as true before an implementer runs as during.
func decideOpenDesign(ctx context.Context, bead *backend.Bead, classification dispatch.DepthProposal, facts ForkScopeFacts, da DA) ForkDecision {
	if len(facts.OpenDependents) > 0 {
		slog.Info("depth gate escalating: sibling beads depend on this one and have not yet closed",
			"epic", facts.EpicID, "bead", facts.BeadID, "openDependents", facts.OpenDependents)
		return ForkDecision{
			Action: ForkActionEscalate,
			Cause:  ForkCauseOpenDependents,
			Reason: fmt.Sprintf("%s in epic %s depend on this bead and have not yet closed, so something outside this bead has to agree with the shape it takes", strings.Join(facts.OpenDependents, ", "), facts.EpicID),
		}
	}
	if da == nil {
		return ForkDecision{
			Action: ForkActionEscalate,
			Cause:  ForkCauseNoDAConfigured,
			Reason: "this bead's own text says its design is still open, and nothing is configured to settle that on the operator's behalf - Fix: set da.agent and da.workDir in kernl.yaml",
		}
	}

	question, err := prompt.RenderOpenDesign(prompt.OpenDesignInput{
		EpicID:            facts.EpicID,
		BeadID:            facts.BeadID,
		Title:             bead.Title,
		Description:       bead.Description,
		WhyOpen:           classification.Reason,
		Acceptance:        bead.Acceptance,
		RelatedDecisions:  facts.RelatedDecisions,
		RepositoryContext: facts.RepositoryContext,
	})
	if err != nil {
		return ForkDecision{Action: ForkActionEscalate, Cause: ForkCauseUnreadableHandover, Reason: err.Error()}
	}

	answer, err := da.Consult(ctx, question)
	if err != nil {
		return ForkDecision{
			Action: ForkActionEscalate,
			Cause:  ForkCauseDAUnavailable,
			Reason: fmt.Sprintf("the DA could not be asked to settle this bead's open design: %v", err),
		}
	}
	decision, err := ParseOpenDesignAnswer(answer)
	if err != nil {
		return ForkDecision{
			Action: ForkActionEscalate,
			Cause:  ForkCauseDAAnswerUnparseable,
			Reason: fmt.Sprintf("the DA's answer could not be read: %v", err),
		}
	}
	return decision
}

// ParseOpenDesignAnswer reads the DA's reply to an open-design question.
//
// Strict in one direction, exactly like ParseForkAnswer and for the same
// stated reason: the failure of a gate must land on the side that stops,
// never the side that proceeds. A missing or unrecognized verdict line, a
// DECIDE with no SHAPE line, an empty shape, or any verdict with no reason
// below it, is the DA not having answered - never a licence to dispatch the
// bead anyway.
//
// Unlike ParseForkAnswer there is no options list to check the answer
// against, because there is none to offer (see decideOpenDesign). What
// replaces that check is the requirement that a shape be stated AND
// justified: an answer that names a shape with no reason under it is the
// shape of a guess, and this gate exists precisely so a guess is not what
// reaches the implementer.
func ParseOpenDesignAnswer(answer string) (ForkDecision, error) {
	trimmed := strings.TrimSpace(answer)
	if trimmed == "" {
		return ForkDecision{}, fmt.Errorf("the DA answered the open-design question with nothing at all")
	}
	firstLine, rest, _ := strings.Cut(trimmed, "\n")
	verdict := strings.ToUpper(strings.Trim(strings.TrimSpace(firstLine), "*`_ "))

	switch verdict {
	case prompt.DesignDecideLine:
		return parseOpenDesignDecideAnswer(rest)
	case prompt.DesignEscalateLine:
		reason := strings.TrimSpace(rest)
		if reason == "" {
			return ForkDecision{}, fmt.Errorf("the DA answered %q but gave no reason", prompt.DesignEscalateLine)
		}
		return ForkDecision{Action: ForkActionEscalate, Cause: ForkCauseOpenDesignEscalated, Reason: reason}, nil
	default:
		return ForkDecision{}, fmt.Errorf("the DA's answer does not begin with a %q or %q line, so its verdict is not decidable: %q", prompt.DesignDecideLine, prompt.DesignEscalateLine, firstLine)
	}
}

func parseOpenDesignDecideAnswer(afterVerdict string) (ForkDecision, error) {
	shapeLine, reasonRest, _ := strings.Cut(strings.TrimLeft(afterVerdict, "\n"), "\n")
	shapeLine = strings.TrimSpace(shapeLine)

	if !strings.HasPrefix(strings.ToUpper(shapeLine), prompt.DesignShapePrefix) {
		return ForkDecision{}, fmt.Errorf("the DA answered %q but its next line is not a %q line: %q", prompt.DesignDecideLine, prompt.DesignShapePrefix, shapeLine)
	}
	shape := strings.TrimSpace(shapeLine[len(prompt.DesignShapePrefix):])
	if shape == "" {
		return ForkDecision{}, fmt.Errorf("the DA answered %q with an empty %q line, so it named no shape at all", prompt.DesignDecideLine, prompt.DesignShapePrefix)
	}
	reason := strings.TrimSpace(reasonRest)
	if reason == "" {
		return ForkDecision{}, fmt.Errorf("the DA settled on %q but gave no reason, and a shape with no reasoning under it is a guess this gate exists to prevent", shape)
	}
	return ForkDecision{
		Action:       ForkActionDecided,
		Cause:        ForkCauseOpenDesignDecided,
		ChosenOption: shape,
		Reason:       reason,
	}, nil
}

// depthGateOutcome is what handleDepthGate hands back to
// DriveBeadToTerminal: whether the caller must stop here, and the updated
// shared fork/decision budget.
type depthGateOutcome struct {
	// Blocked is true when the bead must not be dispatched at all - the
	// operator has to settle its design.
	Blocked bool
	Result  RunBeadResult
	// ForkGateCalls is the shared budget after this gate, incremented only
	// when the DA was actually consulted.
	ForkGateCalls int
}

// handleDepthGate is the check that closes the defect this unit exists for:
// dispatch never read the depth classifier, so a bead the classifier itself
// marks as a decision gate was handed to an implementer that chose an
// approach nobody committed to.
//
// Runs before the stage does, and at most once per bead: the marker written
// below survives both the re-entry loop inside one DriveBeadToTerminal call
// and a bead resumed by a later, separate run.
//
// It consumes the SHARED fork/decision budget (forkHandoverLimit), rather
// than carrying a budget of its own. A bead whose shape was already put to
// the DA and then hands a fork over mid-implementation is one bead consuming
// two consultations, and the budget exists to bound exactly that.
func handleDepthGate(ctx context.Context, deps DriveBeadDeps, bead *backend.Bead, epicID, artifactDir, activeState string, forkGateCalls int) (depthGateOutcome, error) {
	classification := dispatch.ClassifyDepth(*bead)
	if classification.Depth != dispatch.DepthGate {
		return depthGateOutcome{ForkGateCalls: forkGateCalls}, nil
	}

	routed, err := depthGateAlreadyRouted(deps.BeadID, artifactDir)
	if err != nil {
		return depthGateOutcome{}, err
	}
	if routed {
		// Asked once already. Whatever the answer was, it is on disk: a
		// settled shape is in fork-answer.md and reaches the implementer
		// below, and an escalation already blocked this bead once and will
		// have been unblocked deliberately by whoever is resuming it.
		// Re-asking would spend a second consultation to be told the same
		// thing, and would override a human who just decided otherwise.
		return depthGateOutcome{ForkGateCalls: forkGateCalls}, nil
	}

	if forkGateCalls >= forkHandoverLimit {
		slog.Info("DRIVE_TRACE depth gate escalating: shared fork/decision budget spent",
			"bead", deps.BeadID, "limit", forkHandoverLimit)
		decision := ForkDecision{
			Action: ForkActionEscalate,
			Cause:  ForkCauseBudgetSpent,
			Reason: fmt.Sprintf("this run already spent its shared budget of %d fork/decision handover(s) before reaching this bead's open design - Fix: settle the bead's shape by hand, or raise the budget deliberately", forkHandoverLimit),
		}
		return depthGateOutcome{
			Blocked:       true,
			Result:        blockBeadForDecision(deps, activeState, "depth_gate_escalated", decision),
			ForkGateCalls: forkGateCalls,
		}, nil
	}

	facts, err := gatherForkScopeFactsForBead(ctx, deps, bead, epicID)
	if err != nil {
		return depthGateOutcome{}, err
	}

	decision := decideOpenDesign(ctx, bead, classification, facts, deps.DA)

	// Recorded before either branch acts, so a tracker error below can never
	// leave the question askable a second time - the same ordering, and the
	// same reason, as markReviewDecisionRouted.
	if err := markDepthGateRouted(deps.BeadID, artifactDir,
		fmt.Sprintf("open design put to the DA [%s]: %s", decision.Cause, decision.Reason)); err != nil {
		return depthGateOutcome{}, err
	}

	if decision.Action != ForkActionDecided {
		slog.Info("DRIVE_TRACE depth gate blocking: the bead's design is open and was not settled",
			"bead", deps.BeadID, "cause", decision.Cause)
		return depthGateOutcome{
			Blocked:       true,
			Result:        blockBeadForDecision(deps, activeState, "depth_gate_escalated", decision),
			ForkGateCalls: forkGateCalls,
		}, nil
	}

	// The settled shape goes to the same artifact an implementer's own fork
	// answer lands in, so it reaches the implementer through the path that
	// already exists (see StagePromptInput.ForkAnswer) rather than a second
	// delivery mechanism.
	if err := writeForkAnswerArtifact(deps.BeadID, artifactDir, decision); err != nil {
		return depthGateOutcome{}, err
	}
	_ = deps.Backend.Comment(deps.BeadID,
		fmt.Sprintf("depth_gate_decided: shape %q [%s]: %s", decision.ChosenOption, decision.Cause, decision.Reason),
		deps.RepoPath)
	slog.Info("DRIVE_TRACE depth gate decided: the bead's shape was settled before dispatch",
		"bead", deps.BeadID, "shape", decision.ChosenOption)

	return depthGateOutcome{ForkGateCalls: forkGateCalls + 1}, nil
}
