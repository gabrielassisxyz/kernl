package dispatch

import (
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// Depth is how much of the pipeline an autonomous run commits to before a
// human has looked at any of it: DepthGate defers the decision back to the
// operator entirely, DepthFullPipeline runs a planner before an implementer
// touches code, and DepthShortFlow skips the planner because there is
// nothing left for one to decide.
type Depth string

const (
	// DepthGate marks a candidate the orchestrator refuses to run
	// autonomously: the bead's own text says the shape of the work is still
	// being chosen, so running it now would have the agent pick an approach
	// nobody committed to, rather than implement one that was. This is the
	// spike case - it belongs to the operator, never to a queue.
	DepthGate Depth = "gate"

	// DepthFullPipeline runs a planner before an implementer: what "correct"
	// means is settled, but how to get there is not, so a plan is what has
	// to exist before code does.
	DepthFullPipeline Depth = "full_pipeline"

	// DepthShortFlow skips the planner: a single implementer goes straight
	// to work because a failing test (or an equivalent, already-stated
	// acceptance criterion) already says what "correct" means for a
	// localized defect.
	DepthShortFlow Depth = "short_flow"
)

// DepthProposal is the routing decision for one candidate bead: the depth
// chosen and the reason for it, so the reason can be checked against the
// bead's own title and description before anything runs.
type DepthProposal struct {
	ID     string
	Depth  Depth
	Reason string
}

// openDesignMarkers are phrases that, appearing in a bead's own title or
// description, announce that the bead is not yet a decided piece of work -
// the shape of the fix, not just how to implement it, is still being
// chosen. A bead like that is a decision gate, not autonomous work.
//
// This is a short, literal list, not a scored or weighted heuristic: the
// one measured case this project has - a bead that disqualified itself with
// "the shape of a fix, not yet chosen" and "worth measuring before it is
// built" - is caught by exact, ordinary phrasing. Growing this into fuzzy
// matching before a second real case shows the first list wrong would be
// premature generality; the list grows when a real gate bead slips through
// it, not before.
var openDesignMarkers = []string{
	"spike",
	"not yet chosen",
	"not yet decided",
	"to be decided",
	"worth measuring",
	"open question",
}

// openDesignReason reports the first marker found in the candidate's title
// or description, and a human-readable reason built from it.
func openDesignReason(b backend.Bead) (string, bool) {
	haystack := strings.ToLower(b.Title + "\n" + b.Description)
	for _, marker := range openDesignMarkers {
		if strings.Contains(haystack, marker) {
			return fmt.Sprintf("description declares its own design still open (matches %q) - that is a decision for the operator, not autonomous work", marker), true
		}
	}
	return "", false
}

// ClassifyDepth proposes a depth for one candidate bead. ProposeDepths below
// is the list version every real caller uses; this is exported on its own
// so a single classification can be tested and reasoned about in isolation.
func ClassifyDepth(b backend.Bead) DepthProposal {
	if reason, ok := openDesignReason(b); ok {
		return DepthProposal{ID: b.ID, Depth: DepthGate, Reason: reason}
	}

	// "bug" is br's issue_type for a defect report, not this package's own
	// vocabulary - see brIssue.IssueType in internal/backend/brcli.go. A bug
	// with acceptance criteria already on file means a failing test already
	// defines correct, which is exactly the short-flow case: nothing left
	// for a planner to decide.
	if b.Type == "bug" && strings.TrimSpace(b.Acceptance) != "" {
		return DepthProposal{
			ID:     b.ID,
			Depth:  DepthShortFlow,
			Reason: "a bug with acceptance criteria already stated - a failing test already defines what correct means, so one implementer can go straight to it, no planner needed",
		}
	}

	return DepthProposal{
		ID:     b.ID,
		Depth:  DepthFullPipeline,
		Reason: "no open-design language and no ready-made acceptance criteria for a localized defect - a planner should settle how before an implementer commits to an approach",
	}
}

// ProposeDepths classifies every candidate independently, in order. This is
// what answers "what can be worked on in this repository today?": a set of
// candidates (from BackendPort.ListReady, typically) paired with the depth
// proposed for each, for the operator to confirm or override before any of
// them run.
func ProposeDepths(candidates []backend.Bead) []DepthProposal {
	proposals := make([]DepthProposal, 0, len(candidates))
	for _, c := range candidates {
		proposals = append(proposals, ClassifyDepth(c))
	}
	return proposals
}
