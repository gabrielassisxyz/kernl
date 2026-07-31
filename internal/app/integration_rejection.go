package app

import (
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
// rejection, once the artifact has been parsed and the epic's own fix-up
// history is known.
type FixupAction int

const (
	// FixupActionCreateBead: dispatch a normal fix-up bead, with normal
	// autonomy.
	FixupActionCreateBead FixupAction = iota
	// FixupActionEscalateAlreadyFixedUp: this epic already spawned one
	// fix-up bead and integration_review rejected again - a fix-up cannot
	// spawn a fix-up (§7), so this always escalates regardless of what the
	// new rejection declares.
	FixupActionEscalateAlreadyFixedUp
	// FixupActionEscalateDecisionOrAmbiguous: the reviewer declared a
	// decision, or declared nothing this package can parse - either way,
	// something here needs the operator's judgment, not a bead any pool may
	// pick up.
	FixupActionEscalateDecisionOrAmbiguous
)

// DecideFixupAction is the pure §7 policy. It takes no I/O so every branch -
// including the ambiguous one, which is the one a caller is most tempted to
// skip - is a table test, not a fixture.
//
// The cap is checked first and unconditionally: an epic that already spawned
// one fix-up bead escalates no matter how cleanly this new rejection
// classifies itself, because the rule this defends against is "a fix-up
// bead cannot spawn a fix-up bead" - not "a fix-up bead cannot spawn an
// ambiguous one."
func DecideFixupAction(rejection *IntegrationRejection, epicAlreadyFixedUp bool) FixupAction {
	if epicAlreadyFixedUp {
		return FixupActionEscalateAlreadyFixedUp
	}
	if rejection == nil {
		return FixupActionEscalateDecisionOrAmbiguous
	}
	if rejection.Kind == review.KindFixup {
		return FixupActionCreateBead
	}
	return FixupActionEscalateDecisionOrAmbiguous
}
