package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/review"
)

// IntegrationRejection is what an integration-review.md must declare once
// its trailing verdict is REJECT (see backend.evaluateSingleExitGate's
// "verdict_reject" reason and internal/prompt's integration_review prompt,
// which asks for exactly this shape). Per the orchestrator-autonomy decision
// model §7, the reviewer - not the orchestrator - decides which of the two
// kinds a rejection is: "the rule falls on its author, not its
// implementer." This type is that declaration, once parsed.
type IntegrationRejection struct {
	Kind review.Kind
	// WhatIsWrong is the defect the reviewer found, always required.
	WhatIsWrong string
	// Acceptance is the fix-up's own acceptance criteria - required when
	// Kind is KindFixup, the same criteria the model's §7 test asks for:
	// "can this be written without choosing anything?"
	Acceptance string
	// Question is what the operator must decide - required when Kind is
	// KindDecision.
	Question string
}

// integrationRejectionHeadingKeys are the only headings this parser
// recognizes - anything else (an unrelated preamble, prose) still closes
// the previous section but contributes nothing.
var integrationRejectionHeadingKeys = map[string]string{
	"classification":             "classification",
	"what is wrong":              "what is wrong",
	"fix-up acceptance criteria": "fix-up acceptance criteria",
	"question for the operator":  "question for the operator",
}

// integrationRejectionSections splits content on its ATX/setext headings via
// backend.MarkdownSectionsByHeading - fence and HTML-comment aware, the same
// hardened distinction the decision_record exit gate already relies on - and
// returns ok=false when either the split found two real headings for the
// same key, or content is otherwise not decidable. A naive line-regexp
// parser (this package's first draft) would recognize a heading hidden
// inside a code fence or an HTML comment: a reviewer could write
// "## Classification / decision" in plain view, then a later fenced example
// containing "## Classification / fixup" - and the hidden one, appearing
// later, would silently win. Being fence/comment-aware closes that: a hidden
// heading is never a heading at all, and two REAL headings for the same key
// are refused outright rather than resolved by "last one wins."
func integrationRejectionSections(content string) (sections map[string]string, ok bool) {
	sections, dupKey := backend.MarkdownSectionsByHeading(content, func(headingText string) (string, bool) {
		key, recognized := integrationRejectionHeadingKeys[strings.ToLower(strings.TrimSpace(headingText))]
		return key, recognized
	})
	return sections, dupKey == ""
}

// stripTrailingVerdictLine removes the gate's own trailing sentinel line
// ("VERDICT: PASS" or "VERDICT: REJECT") before section-splitting. Without
// this, the last heading in the file - normally "Fix-up acceptance
// criteria" or "Question for the operator" - absorbs that line into its own
// body, since nothing marks a boundary after it; the reviewer's real content
// would still be there, just with the gate's sentinel appended to the end
// of it undetected.
func stripTrailingVerdictLine(content string) string {
	trimmed := strings.TrimRight(content, "\n\r\t ")
	for _, suffix := range []string{"VERDICT: PASS", "VERDICT: REJECT"} {
		if strings.HasSuffix(trimmed, suffix) {
			return trimmed[:len(trimmed)-len(suffix)]
		}
	}
	return content
}

// ParseIntegrationRejection extracts the reviewer's declaration from an
// integration-review.md whose trailing verdict is REJECT. It returns nil -
// not an error - when the declaration is missing, names a Kind outside
// review.All(), or is missing the one field its own Kind requires: none of
// those is a usable declaration, and per §7 an unusable declaration must be
// treated exactly like an explicit "decision" (ambiguous escalates, it is
// never guessed into "fixup"). See DecideFixupAction, the caller that acts
// on this.
func ParseIntegrationRejection(content string) *IntegrationRejection {
	sections, ok := integrationRejectionSections(stripTrailingVerdictLine(content))
	if !ok {
		// Two real headings claimed the same recognized key - which one is
		// authoritative is not decidable content (see
		// integrationRejectionSections's own doc comment).
		return nil
	}

	kind, ok := review.Parse(strings.ToLower(strings.TrimSpace(sections["classification"])))
	if !ok {
		return nil
	}

	whatIsWrong := strings.TrimSpace(sections["what is wrong"])
	if whatIsWrong == "" {
		return nil
	}

	acceptance := strings.TrimSpace(sections["fix-up acceptance criteria"])
	question := strings.TrimSpace(sections["question for the operator"])

	r := &IntegrationRejection{Kind: kind, WhatIsWrong: whatIsWrong}
	switch kind {
	case review.KindFixup:
		// A document carrying both a fix-up's acceptance criteria AND a
		// decision's question is contradictory - the reviewer declared
		// fixup but also wrote down an open question, which is exactly the
		// shape an ambiguous declaration takes. Refusing it rather than
		// picking the declared Kind and ignoring the other section is what
		// keeps "declared cleanly" meaning something.
		if question != "" {
			return nil
		}
		if acceptance == "" {
			return nil
		}
		r.Acceptance = acceptance
	case review.KindDecision:
		if acceptance != "" {
			return nil
		}
		if question == "" {
			return nil
		}
		r.Question = question
	}
	return r
}

// FixupAction is what driveEpic does in response to one integration_review
// rejection, once the artifact has been parsed and the branch's own
// reversibility facts are known.
type FixupAction int

const (
	// FixupActionCreateBead: dispatch a normal fix-up bead, with normal
	// autonomy.
	FixupActionCreateBead FixupAction = iota
	// FixupActionEscalate: hand this rejection to the operator. Which of the
	// gates fired is in GateCause, not in a constant of its own: every
	// caller does the same thing with all of them, and the difference is
	// something to report, not to branch on.
	FixupActionEscalate
)

// GateCause names which gate decided one rejection, on both paths: the one
// that stopped it, or the one that let it continue. It is a string, and it is
// printed verbatim, so a decision can be found later by grepping a run's own
// output for the gate that fired rather than by re-deriving it from prose.
type GateCause string

const (
	// GateCheapToReverse: the mayor judged undoing this later to be cheap,
	// which is the only cause that continues.
	GateCheapToReverse GateCause = "cheap_to_reverse"
	// GatePublished: this branch already exists outside the machine. Undoing
	// published work is not a branch operation any more.
	GatePublished GateCause = "published"
	// GateIrreversibleSurface: the branch changed a path this repository
	// declares expensive to undo.
	GateIrreversibleSurface GateCause = "irreversible_surface"
	// GateBudgetExhausted: this epic has spent its fix-up budget. "Cheap to
	// reverse" is otherwise an unbounded loop, since a reviewer can always
	// find something.
	GateBudgetExhausted GateCause = "fixup_budget_exhausted"
	// GateDecisionOrAmbiguous: the reviewer declared a decision, or declared
	// nothing this package can parse - either way something here needs the
	// operator's judgment, not a bead any pool may pick up.
	GateDecisionOrAmbiguous GateCause = "decision_or_ambiguous"
	// GateExpensiveToReverse: the mayor judged undoing this later to be
	// expensive.
	GateExpensiveToReverse GateCause = "expensive_to_reverse"
	// GateReversibilityUnknown: the question could not be answered at all - no
	// mayor is configured, or the one configured failed or answered something
	// undecidable. An unanswered gate stops; it never continues.
	GateReversibilityUnknown GateCause = "reversibility_unknown"
)

// FixupDecision is the outcome of the policy, with the reason that produced
// it. Every path sets Reason, including the one that continues: the point of
// moving the human gate from "the budget ran out" to "this would be expensive
// to undo" is lost if the new gate leaves no record of what it decided.
type FixupDecision struct {
	Action FixupAction
	Cause  GateCause
	Reason string
}

// DecideFixupAction is the policy for one integration_review rejection.
//
// The gate for human attention is how expensive the change would be to undo,
// not whether an automatic budget ran out. A two-way door the orchestrator
// walks through on its own; a one-way door is what the operator is for. Before
// shipment almost everything is a two-way door - it is a branch, nothing is
// published - so this deliberately hands far fewer rejections to a human than
// the one-fix-up cap it replaces.
//
// Three layers, in this order, and the order is the point: the cheap, certain
// answers come first, and the mayor is asked only what nothing else can
// answer. Among the escalating layers the order decides only which reason gets
// reported, since all of them stop; the layer that must come before the mayor
// is the budget, because exhausting it escalates whatever the mayor thinks.
func DecideFixupAction(ctx context.Context, rejection *IntegrationRejection, facts ReversibilityFacts, judge ReversibilityJudge) FixupDecision {
	if facts.Published {
		return FixupDecision{
			Action: FixupActionEscalate,
			Cause:  GatePublished,
			Reason: fmt.Sprintf("this branch is already published (%s), so undoing it is no longer a branch operation", facts.PublishedDetail),
		}
	}
	if len(facts.IrreversibleSurfacesTouched) > 0 {
		return FixupDecision{
			Action: FixupActionEscalate,
			Cause:  GateIrreversibleSurface,
			Reason: fmt.Sprintf("the branch changed %s, which this repository declares irreversible in registry.repos[].irreversibleSurfaces", strings.Join(facts.IrreversibleSurfacesTouched, ", ")),
		}
	}
	if facts.Budget > 0 && facts.FixupsSpent >= facts.Budget {
		return FixupDecision{
			Action: FixupActionEscalate,
			Cause:  GateBudgetExhausted,
			Reason: fmt.Sprintf("this epic has spent %d of its %d fix-up rounds, and a loop that keeps repairing itself because each round is cheap to reverse never ends on its own", facts.FixupsSpent, facts.Budget),
		}
	}
	// Not a reversibility question at all: the reviewer either asked for a
	// choice to be made, or declared something this package cannot read, and
	// per the decision model an unreadable declaration is treated exactly like
	// an explicit "decision" rather than guessed into a fix-up.
	if rejection == nil {
		return FixupDecision{
			Action: FixupActionEscalate,
			Cause:  GateDecisionOrAmbiguous,
			Reason: "the reviewer's declaration could not be parsed as a fixup or a decision, and an ambiguous rejection escalates rather than being guessed into a fix-up",
		}
	}
	if rejection.Kind != review.KindFixup {
		return FixupDecision{
			Action: FixupActionEscalate,
			Cause:  GateDecisionOrAmbiguous,
			Reason: "the reviewer raised a decision only the operator can make: " + rejection.Question,
		}
	}

	if judge == nil {
		return FixupDecision{
			Action: FixupActionEscalate,
			Cause:  GateReversibilityUnknown,
			Reason: "nothing is wired to judge how expensive this would be to undo - Fix: set llm.provider and llm.endpoint, or llm.agent, in kernl.yaml",
		}
	}
	verdict, err := judge.JudgeReversibility(ctx, ReversibilityQuestion{
		EpicID:        facts.EpicID,
		Objection:     rejection.WhatIsWrong,
		ChangeSummary: facts.ChangeSummary,
	})
	if err != nil {
		return FixupDecision{
			Action: FixupActionEscalate,
			Cause:  GateReversibilityUnknown,
			Reason: fmt.Sprintf("how expensive this would be to undo could not be established: %v", err),
		}
	}
	if verdict.Expensive {
		return FixupDecision{Action: FixupActionEscalate, Cause: GateExpensiveToReverse, Reason: verdict.Reason}
	}
	return FixupDecision{Action: FixupActionCreateBead, Cause: GateCheapToReverse, Reason: verdict.Reason}
}
