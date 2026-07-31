package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// revertedDecisionMarkerPrefix opens the HTML comment
// RevertDecisionAndReopenBead prepends onto a bead's description, naming the
// decision it reverted. A retry of the whole operation (see that function's
// own doc comment on why steps 2 and 3 can fail independently) must not
// prepend the constraint a second time; this marker is what a retry checks
// for before writing.
const revertedDecisionMarkerPrefix = "<!-- kernl:reverted-decision:"

// RevertDecisionInput is what the operator names when reverting a bead's
// decision: which bead, optionally which decision (required only when the
// bead carries more than one still-active - not yet reverted - decision),
// what state to reopen it into, and why.
type RevertDecisionInput struct {
	BeadID      string
	DecisionID  string
	TargetState string
	Reason      string
}

// RevertDecisionResult reports what actually happened: the operator names a
// bead and a target state, but the decision id may have been resolved on
// their behalf when the bead carried exactly one active decision.
type RevertDecisionResult struct {
	DecisionID  string
	TargetState string
}

// RevertDecisionAndReopenBead is the decision model's §6 middle row: one
// decision was wrong, the work stands, so the bead reopens with that
// decision written in as a constraint the next implementer cannot silently
// re-derive - "reverting a decision without recording it as a constraint
// means the pool re-derives the same answer and the operator reverts it
// twice."
//
// From the operator's side this is one operation. Internally it is three
// writes, deliberately ordered so a failure partway leaves a state that can
// be named and safely retried, rather than one that has to be
// reverse-engineered by hand:
//
//  1. Mark the Decision node reverted (graph). Idempotent for a retry of the
//     exact same revert (same decision, same reason); a second, DIFFERENT
//     revert of an already-reverted decision fails loud instead of silently
//     overwriting the first revert's record.
//  2. Prepend the constraint onto the bead's description (tracker), guarded
//     by revertedDecisionMarkerPrefix so a retry never double-prepends it.
//  3. Rewind the bead to TargetState (tracker) - done LAST. Reopening the
//     bead before its description carries the constraint is exactly the
//     window in which a poller could dispatch an agent that never sees it;
//     doing this step last closes that window rather than merely narrowing
//     it.
//
// A failure at step 2 or 3 is safe to retry with the identical call: step 1
// has already converged, and step 2's own marker check makes it converge
// too. A failure at step 3 leaves the constraint durably written and the
// bead in its prior state - the returned error says exactly that and tells
// the caller to retry the same command.
func RevertDecisionAndReopenBead(ctx context.Context, g *graph.Graph, be backend.BackendPort, repoPath string, in RevertDecisionInput) (*RevertDecisionResult, error) {
	if strings.TrimSpace(in.BeadID) == "" {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: revert-decision requires a bead id - Fix: pass the bead being reopened")
	}
	if strings.TrimSpace(in.TargetState) == "" {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: revert-decision requires --state - Fix: name the queue state (ready_for_*) the bead reopens into")
	}
	if strings.TrimSpace(in.Reason) == "" {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: revert-decision requires --reason - Fix: state why the decision was wrong; it becomes the constraint the next implementer reads")
	}

	decision, err := resolveActiveDecision(ctx, g, in.BeadID, in.DecisionID)
	if err != nil {
		return nil, err
	}

	if err := markDecisionReverted(ctx, g, decision, in.Reason); err != nil {
		return nil, err
	}

	if err := writeConstraintIntoDescription(be, repoPath, in.BeadID, decision, in.Reason); err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: decision %s marked reverted but writing the constraint into bead %s's description failed: %w - Fix: the revert itself is already recorded; retry this exact command to write the constraint and rewind the bead", decision.ID, in.BeadID, err)
	}

	if err := be.Rewind(in.BeadID, in.TargetState, in.Reason, repoPath); err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: decision %s reverted and the constraint written into bead %s's description, but rewinding it to %s failed: %w - Fix: the bead is unchanged in its prior state; retry this exact command once the backend issue is resolved, the constraint will not be duplicated", decision.ID, in.BeadID, in.TargetState, err)
	}

	return &RevertDecisionResult{DecisionID: decision.ID, TargetState: in.TargetState}, nil
}

// resolveActiveDecision finds the Decision this revert applies to.
//
// With an explicit decisionID, it is used directly after confirming it is
// actually linked to beadID via has_decision - a caller passing a decision
// id copied from the wrong bead gets told so, rather than silently reverting
// an unrelated decision.
//
// With no decisionID, the bead's has_decision edges are walked and exactly
// one not-yet-reverted decision must exist: zero means there is nothing
// recorded to revert, and more than one is ambiguous - the bead has been
// through more than one decision-bearing stage - and must be disambiguated
// with --decision.
func resolveActiveDecision(ctx context.Context, g *graph.Graph, beadID, decisionID string) (*nodes.Decision, error) {
	if decisionID != "" {
		var d *nodes.Decision
		err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
			out, err := edges.Outgoing(ctx, tx, beadID, edges.WithType(edges.EdgeTypeHasDecision))
			if err != nil {
				return err
			}
			linked := false
			for _, e := range out {
				if e.Dst == decisionID {
					linked = true
					break
				}
			}
			if !linked {
				return fmt.Errorf("KERNL DISPATCH FAILURE: decision %s is not linked to bead %s - Fix: pass a decision id this bead actually recorded", decisionID, beadID)
			}
			d, err = nodes.GetDecision(ctx, tx, decisionID)
			return err
		})
		if err != nil {
			return nil, err
		}
		return d, nil
	}

	// alreadyReverted is tracked alongside candidates (not merely discarded)
	// so a zero-candidate result can tell "nothing was ever decided here"
	// (in.DecisionID would never resolve to anything, this bead is simply
	// not revertible) apart from "the one decision here was already
	// reverted, likely by a prior call of this same operation that failed
	// after step 1" (see RevertDecisionAndReopenBead's doc comment on step
	// 2/3 failures being safe to retry) - the second case needs to name the
	// id so the retry can pass --decision explicitly, since auto-resolution
	// deliberately will not silently pick an already-reverted decision back
	// up (that would also re-select a genuinely stale, long-since-reverted
	// decision on a bead that has since recorded a new, unrelated one).
	var candidates, alreadyReverted []*nodes.Decision
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		out, err := edges.Outgoing(ctx, tx, beadID, edges.WithType(edges.EdgeTypeHasDecision))
		if err != nil {
			return err
		}
		for _, e := range out {
			d, err := nodes.GetDecision(ctx, tx, e.Dst)
			if err != nil {
				return fmt.Errorf("reading decision %s linked to bead %s: %w", e.Dst, beadID, err)
			}
			if d.RevertedAt == nil {
				candidates = append(candidates, d)
			} else {
				alreadyReverted = append(alreadyReverted, d)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	switch len(candidates) {
	case 0:
		if len(alreadyReverted) > 0 {
			ids := make([]string, len(alreadyReverted))
			for i, d := range alreadyReverted {
				ids[i] = d.ID
			}
			return nil, fmt.Errorf("KERNL DISPATCH FAILURE: bead %s has no active decision - it already has %d reverted (%s) - Fix: if this is a retry of an interrupted revert, pass --decision <id> explicitly", beadID, len(alreadyReverted), strings.Join(ids, ", "))
		}
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: bead %s has no decision recorded - Fix: nothing to revert", beadID)
	case 1:
		return candidates[0], nil
	default:
		ids := make([]string, len(candidates))
		for i, d := range candidates {
			ids[i] = d.ID
		}
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: bead %s has %d active decisions recorded (%s) - Fix: pass --decision <id> to say which one is being reverted", beadID, len(candidates), strings.Join(ids, ", "))
	}
}

// markDecisionReverted writes RevertedAt/RevertReason onto d, or is a no-op
// when a retry of the exact same revert (identical reason) already landed.
// A DIFFERENT reason against an already-reverted decision fails loud rather
// than silently overwriting the first revert's record - this function never
// guesses that a second, distinct revert of the same decision was intended.
func markDecisionReverted(ctx context.Context, g *graph.Graph, d *nodes.Decision, reason string) error {
	if d.RevertedAt != nil {
		if d.RevertReason != nil && *d.RevertReason == reason {
			return nil
		}
		return fmt.Errorf("KERNL DISPATCH FAILURE: decision %s was already reverted on %s with reason %q - Fix: this looks like a second, different revert of the same decision; resolve it by hand before retrying", d.ID, d.RevertedAt.Format(time.RFC3339), stringOrEmpty(d.RevertReason))
	}

	now := time.Now()
	updated := *d
	updated.RevertedAt = &now
	updated.RevertReason = &reason
	return g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.UpdateDecision(ctx, tx, updated, runRecordAuthor)
	})
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// writeConstraintIntoDescription is the read-modify-write half of the
// operation, and the one that actually reaches the next implementer:
// BuildBeadStagePrompt renders a bead's Description verbatim
// (renderBeadData), but never renders Notes at all, so the constraint has to
// land in Description to be read by the agent, not merely stored in the
// graph.
//
// This is a genuine read-then-write over the tracker with no
// compare-and-swap underneath it: Backend.Update / refine-scope replace the
// field wholesale, and the port offers no conditional update. Accepted here
// because this is an operator-triggered, sequential, rare correction rather
// than a hot concurrent path; building real optimistic concurrency into the
// backend port is a larger change this bead does not make.
func writeConstraintIntoDescription(be backend.BackendPort, repoPath, beadID string, d *nodes.Decision, reason string) error {
	bead, err := be.Get(beadID, repoPath)
	if err != nil {
		return fmt.Errorf("reading bead %s before writing its constraint: %w", beadID, err)
	}
	if bead == nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: bead %s not found in %s - Fix: verify the bead id", beadID, repoPath)
	}

	marker := revertedDecisionMarkerPrefix + d.ID + " -->"
	if strings.Contains(bead.Description, marker) {
		return nil
	}

	block := renderRevertedDecisionConstraint(d, reason, marker)
	return be.Update(beadID, backend.UpdateBeadInput{Description: block + "\n\n" + bead.Description}, repoPath)
}

// renderRevertedDecisionConstraint is the prose BuildBeadStagePrompt ends up
// rendering, via renderBeadData's Description block, for the reopened bead -
// framed as a directive rather than a suggestion, because a line the
// implementer is free to disagree with is advice, not a constraint.
//
// It is built from the Decision node itself (Title, Context, Outcome)
// instead of re-typed prose: the entire value of Phase 3's graph write is
// that the reasoning already exists there, structured, and does not need to
// be remembered a second time by whoever runs this revert.
func renderRevertedDecisionConstraint(d *nodes.Decision, reason, marker string) string {
	var b strings.Builder
	b.WriteString(marker)
	b.WriteString("\n## Reverted decision - do not retake\n\n")
	fmt.Fprintf(&b, "This bead previously decided: %q\n\n", d.Title)
	if strings.TrimSpace(d.Context) != "" {
		b.WriteString(strings.TrimSpace(d.Context))
		b.WriteString("\n\n")
	}
	if strings.TrimSpace(d.Outcome) != "" {
		fmt.Fprintf(&b, "It was decided that way because: %s\n\n", strings.TrimSpace(d.Outcome))
	}
	fmt.Fprintf(&b, "This was reverted because: %s\n\n", reason)
	b.WriteString("Do not choose this option again. Treat this as a hard constraint on your implementation, not a suggestion open to reconsideration.\n\n---")
	return b.String()
}
