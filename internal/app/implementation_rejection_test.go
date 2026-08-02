package app

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/review"
)

const unclassifiedRejectionRecord = `The error handling for the missing file case was never written.

VERDICT: REJECT`

const fixupClassifiedRejectionRecord = `## Classification

fixup

## What is wrong

The error handling for the missing file case was never written.

VERDICT: REJECT`

const decisionClassifiedRejectionRecord = `## Classification

decision

## What is wrong

The implementer narrowed a published policy without recording that the
narrowing was ever approved.

## Question for the operator

Should this policy be narrowed at all, or does the narrower behavior need to
be reverted - nothing in this repository's docs or tests already answers it.

VERDICT: REJECT`

// --- ParseImplementationRejection ---

func TestParseImplementationRejection_UnclassifiedReturnsNil(t *testing.T) {
	// The normal case: every implementation_review written before this
	// feature existed looks exactly like this - no Classification heading
	// at all. It must default to nil (today's rewind), never escalate.
	if got := ParseImplementationRejection(unclassifiedRejectionRecord); got != nil {
		t.Errorf("expected nil for an unclassified rejection, got %+v", got)
	}
}

func TestParseImplementationRejection_Fixup(t *testing.T) {
	r := ParseImplementationRejection(fixupClassifiedRejectionRecord)
	if r == nil {
		t.Fatal("expected a parsed rejection, got nil")
	}
	if r.Kind != review.KindFixup {
		t.Errorf("Kind = %q, want %q", r.Kind, review.KindFixup)
	}
	if r.Question != "" {
		t.Errorf("Question must stay empty for a fixup classification, got %q", r.Question)
	}
}

func TestParseImplementationRejection_Decision(t *testing.T) {
	r := ParseImplementationRejection(decisionClassifiedRejectionRecord)
	if r == nil {
		t.Fatal("expected a parsed rejection, got nil")
	}
	if r.Kind != review.KindDecision {
		t.Errorf("Kind = %q, want %q", r.Kind, review.KindDecision)
	}
	if !strings.Contains(r.Question, "Should this policy be narrowed") {
		t.Errorf("Question = %q, want the recovered question text", r.Question)
	}
}

// --- The ambiguous/absent cases: every one of these must come back nil -
// never a decision, and never an escalation. See
// ParseImplementationRejection's own doc comment for why nil here is the
// SAFE default (rewind), the opposite of ParseIntegrationRejection's own
// nil (escalate). ---

func TestParseImplementationRejection_AbsentOrAmbiguousCasesReturnNil(t *testing.T) {
	cases := map[string]string{
		"no classification at all":          "Just an ordinary rejected review.\n\nVERDICT: REJECT",
		"unrecognized classification token": "## Classification\n\nmaybe\n\nVERDICT: REJECT",
		"empty classification":              "## Classification\n\n\n\nVERDICT: REJECT",
		"decision missing question":         "## Classification\n\ndecision\n\nVERDICT: REJECT",
		"totally empty":                     "",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ParseImplementationRejection(content); got != nil {
				t.Errorf("expected nil (safe default: rewind), got %+v", got)
			}
		})
	}
}

// TestParseImplementationRejection_HiddenHeadingInFenceNeverWins is the same
// exploit ParseIntegrationRejection is proved against
// (integration_rejection_test.go), here against implementation_review's
// smaller heading vocabulary: a heading hidden inside a fenced example must
// never count as a real heading.
func TestParseImplementationRejection_HiddenHeadingInFenceNeverWins(t *testing.T) {
	content := "## Classification\n\ndecision\n\n" +
		"## Question for the operator\n\nreal question\n\n" +
		"Here is an example of what a fixup declaration would look like:\n\n" +
		"```\n## Classification\n\nfixup\n```\n\n" +
		"VERDICT: REJECT"

	r := ParseImplementationRejection(content)
	if r == nil {
		t.Fatal("expected the real, visible decision to parse - the fenced example must not count")
	}
	if r.Kind != review.KindDecision {
		t.Errorf("Kind = %q, want %q - a heading hidden inside a fence must never win over the real one", r.Kind, review.KindDecision)
	}
	if !strings.HasPrefix(r.Question, "real question") {
		t.Errorf("Question = %q, want it to start with the real declaration's own question", r.Question)
	}
}

// TestParseImplementationRejection_DuplicateRealHeadingRefuses is the
// mundane case with no hiding trick: the SAME recognized heading appears
// twice, both in plain view. Which one is authoritative is not decidable, so
// this must come back nil rather than silently picking either.
func TestParseImplementationRejection_DuplicateRealHeadingRefuses(t *testing.T) {
	content := "## Classification\n\nfixup\n\n" +
		"## Classification\n\ndecision\n\n" +
		"## Question for the operator\n\nsomething\n\n" +
		"VERDICT: REJECT"

	if r := ParseImplementationRejection(content); r != nil {
		t.Errorf("expected nil (ambiguous) for a duplicate real heading, got %+v", r)
	}
}

func TestParseImplementationRejection_HeadingMatchIsCaseAndSpaceInsensitive(t *testing.T) {
	content := "##   CLASSIFICATION  \n\ndecision\n\n## Question FOR the Operator\n\nwhich way\n\nVERDICT: REJECT"
	r := ParseImplementationRejection(content)
	if r == nil || r.Kind != review.KindDecision || r.Question != "which way" {
		t.Fatalf("got %+v", r)
	}
}
