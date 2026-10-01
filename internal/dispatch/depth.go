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

	// DepthFullPipeline runs a planner before an implementer: no acceptance
	// criteria are stated, so what "correct" means for this bead was never
	// settled, and that has to happen before an implementer commits to an
	// approach.
	DepthFullPipeline Depth = "full_pipeline"

	// DepthShortFlow skips the planner: a single implementer goes straight
	// to work because the bead already states its own acceptance criteria -
	// a bead that exists has already been planned, so this is the default,
	// not the exception.
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

// acceptanceMarkers are phrases that, appearing in a bead's own text,
// indicate its acceptance criteria are already stated somewhere other than
// the dedicated Acceptance field. A hand-planned backlog often writes the
// criteria inside the description ("Done when:" or an "Acceptance criteria"
// heading) rather than in the separate field. The list is literal and short,
// same discipline as openDesignMarkers.
var acceptanceMarkers = []string{
	"done when:",
	"## acceptance criteria",
}

// hasAcceptanceCriteria reports whether the bead's own text states what
// "correct" means for it. It checks the dedicated Acceptance field first,
// then scans the title, description and notes for markers that show
// acceptance criteria are present in the prose.
func hasAcceptanceCriteria(b backend.Bead) bool {
	if strings.TrimSpace(b.Acceptance) != "" {
		return true
	}
	haystack := strings.ToLower(b.Title + "\n" + b.Description + "\n" + b.Notes)
	for _, marker := range acceptanceMarkers {
		if strings.Contains(haystack, marker) {
			return true
		}
	}
	return false
}

// ClassifyDepth proposes a depth for one candidate bead. ProposeDepths below
// is the list version every real caller uses; this is exported on its own
// so a single classification can be tested and reasoned about in isolation.
//
// The default is DepthShortFlow, not DepthFullPipeline: a bead that exists
// has already been planned - that is what turned an idea into a bead in the
// first place - so the exception that needs a positive reason is the one
// that still runs a planner, not the one that skips it. DepthFullPipeline
// is reached only when acceptance criteria are missing, which is the
// positive, checkable fact that "what correct means" was never settled.
func ClassifyDepth(b backend.Bead) DepthProposal {
	if reason, ok := openDesignReason(b); ok {
		return DepthProposal{ID: b.ID, Depth: DepthGate, Reason: reason}
	}

	if !hasAcceptanceCriteria(b) {
		return DepthProposal{
			ID:     b.ID,
			Depth:  DepthFullPipeline,
			Reason: "no acceptance criteria stated - what correct means for this bead was never settled, so a planner needs to decide it before an implementer commits to an approach",
		}
	}

	return DepthProposal{
		ID:     b.ID,
		Depth:  DepthShortFlow,
		Reason: "a bead that exists has already been planned - acceptance criteria are already stated, so one implementer can go straight to it, no planner needed",
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
