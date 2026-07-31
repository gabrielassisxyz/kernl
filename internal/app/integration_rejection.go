package app

import (
	"regexp"
	"strings"

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

// integrationRejectionHeadingRe matches one ATX markdown heading line
// ("## Text", 1-6 hashes) and captures its text. Deliberately simpler than
// backend.DecisionRecordSectionBodies's fence/comment-aware parser: that
// parser exists to resist an agent gaming a hard gate (a decision_record
// exit gate that blocks the bead outright), where a heading hidden in a
// fence or a comment must not count as real content. Here, a declaration
// this parser cannot make sense of - for any reason, hidden or otherwise -
// already resolves to nil, and nil already means "escalate to the operator"
// (see ParseIntegrationRejection and DecideFixupAction). Escalating is the
// safe fallback in both cases, so a second copy of that heavier parser would
// buy nothing.
var integrationRejectionHeadingRe = regexp.MustCompile(`(?m)^[ \t]{0,3}#{1,6}[ \t]+(.+?)[ \t]*$`)

// integrationRejectionSections splits content on its ATX headings and
// returns each heading's body, keyed by the heading text lowercased and
// trimmed - so "## Classification", "## classification " and "##
// CLASSIFICATION" all key the same entry.
func integrationRejectionSections(content string) map[string]string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	locs := integrationRejectionHeadingRe.FindAllStringSubmatchIndex(content, -1)
	sections := make(map[string]string, len(locs))
	for i, loc := range locs {
		key := strings.ToLower(strings.TrimSpace(content[loc[2]:loc[3]]))
		bodyStart := loc[1]
		bodyEnd := len(content)
		if i+1 < len(locs) {
			bodyEnd = locs[i+1][0]
		}
		sections[key] = strings.TrimSpace(content[bodyStart:bodyEnd])
	}
	return sections
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
	sections := integrationRejectionSections(stripTrailingVerdictLine(content))

	kind, ok := review.Parse(strings.ToLower(strings.TrimSpace(sections["classification"])))
	if !ok {
		return nil
	}

	whatIsWrong := strings.TrimSpace(sections["what is wrong"])
	if whatIsWrong == "" {
		return nil
	}

	r := &IntegrationRejection{Kind: kind, WhatIsWrong: whatIsWrong}
	switch kind {
	case review.KindFixup:
		r.Acceptance = strings.TrimSpace(sections["fix-up acceptance criteria"])
		if r.Acceptance == "" {
			return nil
		}
	case review.KindDecision:
		r.Question = strings.TrimSpace(sections["question for the operator"])
		if r.Question == "" {
			return nil
		}
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
