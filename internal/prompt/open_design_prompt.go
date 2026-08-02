package prompt

import (
	"fmt"
	"strings"
)

// The DA's answer format for an open-design bead, shared between this file's
// own template and app.ParseOpenDesignAnswer for the same reason
// ForkDecideLine is shared with app.ParseForkAnswer: naming the same tokens
// in two places by hand is how a prompt and its parser drift apart the
// moment one of them is edited alone.
//
// Deliberately DISTINCT tokens from the fork handover's, rather than reused
// ones: the two answers are not interchangeable, and an answer parsed by the
// wrong reader would be read as a decision it is not. See
// RenderOpenDesign's own doc comment for why this is a separate question at
// all.
const (
	// DesignDecideLine opens an answer that settles the bead's shape.
	DesignDecideLine = "DESIGN: DECIDE"
	// DesignEscalateLine opens an answer that hands the bead to the operator.
	DesignEscalateLine = "DESIGN: ESCALATE"
	// DesignShapePrefix opens the line stating the shape the work should
	// take, immediately below DesignDecideLine.
	DesignShapePrefix = "SHAPE:"
)

// OpenDesignInput feeds the question the DA answers for a bead whose own
// text says its design is still open - before any implementer has run.
//
// This is NOT ForkHandoverInput with different field names, and the
// difference is the whole reason it exists. A fork handover is asked when an
// implementer already weighed real alternatives and could not pick one, so
// the answer is a choice among an enumerated list. An open-design bead has
// been weighed by nobody: it was flagged from its own title and description,
// before any agent ran, so there is no list to choose from and asking for
// one verbatim would be asking for something that does not exist.
type OpenDesignInput struct {
	EpicID string
	BeadID string
	// Title and Description are the bead's own text, which is the entire
	// evidence this question has. There is no implementer's analysis to
	// pass along, because no implementer has run.
	Title       string
	Description string
	// WhyOpen is the classifier's own reason - which marker in the bead's
	// text declared the design open (see dispatch.ClassifyDepth). Passed so
	// the DA can judge the classification itself rather than take it on
	// faith: a bead caught by a phrase used incidentally is one the DA
	// should be able to wave through.
	WhyOpen string
	// Acceptance is the bead's acceptance criteria, when it has any. A bead
	// that states what "correct" means has narrowed its own design more
	// than its prose suggests.
	Acceptance string
	// RelatedDecisions is prose already rendered from
	// app.FetchRelevantDecisions - a recorded preference is an input to this
	// judgment, never a short-circuit. Empty renders an explicit absence
	// line rather than being omitted, the same rule the fork handover
	// follows and for the same reason.
	RelatedDecisions string
	// RepositoryContext is app.AssembleContext's output over this
	// repository's registry.repos[].contextDocs. Empty renders an explicit
	// absence line.
	RepositoryContext string
}

const openDesignTemplate = forkScopeLock + `You are settling the shape of a piece of work on the operator's behalf, in the operator's own repository. Nothing else is being asked of you.

Bead %[1]s, inside epic %[2]s, was flagged before any implementer ran: its own text says the shape of the work is still being chosen, not just how to implement it. Left alone, it would be handed to an implementer who would pick an approach nobody committed to. You are being asked first, so that does not happen.

You are NOT looking at the target repository and must not go re-inspect it: your job is to know what the operator has already decided, written down, or would want, and judge from that.

Why this bead was flagged:
%[3]s

The bead's title:
%[4]s

The bead's own description:
%[5]s

What the bead already states as its acceptance criteria:
%[6]s

## Decisions already recorded in this repository

%[7]s

A recorded preference above is an input to your judgment, not a rule that decides this for you: it should weigh heavily toward proceeding when it applies, but the choice to proceed or escalate is yours to make.

## The operator's own context

%[8]s

Two different answers are available to you, and picking the right one matters more than the content of either.

Settle the shape when you can state, concretely enough for an implementer to act on without guessing, what the work should be: the approach to take, the boundary it should respect, or the option that wins. You are allowed to state a shape nobody wrote down yet - that is what is being asked. What you may not do is restate the bead back as a shape, or answer so vaguely that the implementer is left making the same choice you were asked to make.

Escalate when settling it would commit the operator to something they have given you no basis to choose: a direction that contradicts a recorded decision, a trade-off between things only they can rank, or a bead whose text is too thin to judge at all.

Note that a bead is sometimes flagged by a phrase it uses only in passing, while its design is in fact already settled by its own description. If that is what you find, settle it and say so - that is a decision, not an escalation.

When you are genuinely unsure between two defensible shapes, decide and record why, rather than escalate. Being interrupted is the failure that costs the operator the most, and rare rework the operator did not ask for is the trade this project makes on purpose. This leaning is about your judgment between defensible shapes; it is not licence to guess when you have no basis at all, and it has nothing to do with the ANSWER FORMAT below. Reply in exactly one of the two shapes given, every time, whether you settle or escalate. A reply outside that shape is not read as a decision no matter what it says.

Answer in exactly one of these two shapes, and nothing else:

1. You settle the shape:
%[9]s
%[10]s <one line stating the shape the work should take>
<one or more lines saying why - name what convinced you, and what the implementer must respect>

2. You escalate:
%[11]s
<one or more lines saying why this needs the operator>

Do not ask a clarifying question, and do not review any code.`

// RenderOpenDesign renders the question asked of the DA for a bead whose
// design its own text declares still open.
//
// REJECTED ALTERNATIVE, recorded here so the choice can be revisited rather
// than rediscovered: enumerate first, then decide. An agent would read the
// bead, produce a "## Options Considered" list, and the EXISTING fork
// handover machinery (RenderForkHandover, app.DecideForkAction,
// app.ParseForkAnswer) would then choose among them, unchanged. That keeps
// one question shape in the system instead of two, and it was rejected on
// two counts. It spends an extra agent invocation before the DA is asked
// anything. And the enumeration is itself the judgment being delegated: an
// agent that decides which three options exist has already made the
// operator's decision, quietly, in a step that records no decision at all -
// the DA would then be choosing among alternatives nobody reviewed. If the
// extra call ever becomes worth it (say, because open-design beads turn out
// to need options written down for the run report anyway), this is the path
// back.
//
// Like RenderForkHandover, this refuses to render without the evidence it
// judges from: an answer produced from nothing is a guess with a reason
// attached.
func RenderOpenDesign(in OpenDesignInput) (string, error) {
	if strings.TrimSpace(in.Title) == "" && strings.TrimSpace(in.Description) == "" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: the open-design question for bead %s has neither a title nor a description to judge from - Fix: this question is built from the bead's own text, so the bead must be fetched from the tracker before it is asked", in.BeadID)
	}
	if strings.TrimSpace(in.WhyOpen) == "" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: the open-design question for bead %s does not say why the bead was flagged - Fix: pass dispatch.DepthProposal.Reason, so the DA can judge the classification instead of taking it on faith", in.BeadID)
	}
	return fmt.Sprintf(openDesignTemplate,
		in.BeadID, in.EpicID, in.WhyOpen,
		in.Title,
		orExplicitAbsence(in.Description, "The bead has no description beyond its title."),
		orExplicitAbsence(in.Acceptance, "The bead states no acceptance criteria."),
		orExplicitAbsence(in.RelatedDecisions, "No related decisions were found recorded in this repository."),
		orExplicitAbsence(in.RepositoryContext, "No repository context is available for this run."),
		DesignDecideLine, DesignShapePrefix, DesignEscalateLine,
	), nil
}
