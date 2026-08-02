package app

import (
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/review"
)

// ImplementationRejection is the reviewer's OPTIONAL classification of an
// implementation_review rejection - see review.Kind's own package doc and
// local/artifacts/plans/2026-08-01-composer-context-and-fork-gate-plan.md
// §3.3's last bullet: "a reviewer that identifies a decision routes it to
// the same DA." Recording a decision is not consent - some decisions must be
// approved, not merely recorded, and the proactive fork handover (pass 1/2)
// cannot detect those on its own; this is the reviewer backstop.
//
// Its shape follows IntegrationRejection (integration_rejection.go) closely,
// but see ParseImplementationRejection's own doc comment for the one place
// this deliberately does NOT mirror that type: what an unusable declaration
// defaults to.
type ImplementationRejection struct {
	Kind review.Kind
	// Question is what the operator must decide - populated only when Kind
	// is review.KindDecision. A review.KindFixup classification carries no
	// extra field: it changes nothing about today's rewind, so there is
	// nothing further to recover from the document for it.
	Question string
}

// implementationRejectionHeadingKeys are the only headings this parser
// recognizes - anything else still closes the previous section (see
// backend.MarkdownSectionsByHeading's own doc comment) but contributes
// nothing. Deliberately a smaller vocabulary than
// integrationRejectionHeadingKeys: a fixup classification here needs no
// acceptance-criteria section of its own, because it changes nothing about
// the rewind an implementation_review rejection already gets.
var implementationRejectionHeadingKeys = map[string]string{
	"classification":            "classification",
	"question for the operator": "question for the operator",
}

// implementationRejectionSections splits content on its ATX/setext headings
// via backend.MarkdownSectionsByHeading - the same fence- and
// HTML-comment-aware pass integrationRejectionSections already reuses (see
// that function's own doc comment for the concrete exploit a naive
// line-regexp parser would miss: a heading hidden inside a fenced example
// must never count as a real heading, and two REAL headings claiming the
// same key are refused rather than resolved by "last one wins").
func implementationRejectionSections(content string) (sections map[string]string, ok bool) {
	sections, dupKey := backend.MarkdownSectionsByHeading(content, func(headingText string) (string, bool) {
		key, recognized := implementationRejectionHeadingKeys[strings.ToLower(strings.TrimSpace(headingText))]
		return key, recognized
	})
	return sections, dupKey == ""
}

// ParseImplementationRejection extracts the reviewer's OPTIONAL
// classification from an implementation-review.md whose trailing verdict is
// REJECT. It returns nil for an ABSENT OR UNUSABLE declaration: no
// "## Classification" heading at all, an unrecognized token, two real
// headings claiming the same key, or a "decision" classification with no
// question below it.
//
// THE DEFAULT HERE IS THE OPPOSITE OF ParseIntegrationRejection'S, AND THAT
// IS DELIBERATE. Do not "harmonize" the two into agreeing - that is exactly
// the regression this comment exists to prevent.
//
// ParseIntegrationRejection treats an unparseable declaration as a decision
// and escalates, because there the alternative was minting a fix-up bead
// with acceptance criteria nobody wrote. Here, nil means "keep today's
// behavior: rewind to the implementer" - it must NEVER be read as a decision
// and must NEVER reach the DA. The reason: an unclassified rejection is
// exactly what every implementation_review written before this feature
// existed looks like, and the rewind it already gets costs nothing and
// wakes nobody. Classifying KindDecision is strictly additive: it can only
// move a rejection that would have been a wasted rewind into an answered
// question, never a regression. Defaulting an unclassified rejection to
// escalation instead would wake the operator for every review ever written
// before this feature existed, which is the single failure this whole unit
// exists to avoid.
//
// See handleGateFailure (review_decision_gate.go), the caller that acts on
// this.
func ParseImplementationRejection(content string) *ImplementationRejection {
	sections, ok := implementationRejectionSections(stripTrailingVerdictLine(content))
	if !ok {
		// Two real headings claimed the same recognized key - not decidable
		// content, so this falls through to the safe default (nil), not an
		// escalation (see this function's own doc comment).
		return nil
	}

	kind, ok := review.Parse(strings.ToLower(strings.TrimSpace(sections["classification"])))
	if !ok {
		return nil
	}
	if kind == review.KindFixup {
		return &ImplementationRejection{Kind: review.KindFixup}
	}

	question := strings.TrimSpace(sections["question for the operator"])
	if question == "" {
		return nil
	}
	return &ImplementationRejection{Kind: review.KindDecision, Question: question}
}
