package app

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/review"
)

const fixupRejectionRecord = `Some review prose here.

## Classification

fixup

## What is wrong

The manifest is written before the last file is flushed, so an interrupted
export can leave a manifest describing a file that does not exist.

## Fix-up acceptance criteria

An interrupted export never leaves a manifest describing a file that does
not exist on disk.

VERDICT: REJECT`

const decisionRejectionRecord = `## Classification

decision

## What is wrong

Two children established incompatible slug rules for the same field.

## Question for the operator

Which slug rule wins - choosing either one invalidates exports the other
child's rule already produced.

VERDICT: REJECT`

// --- ParseIntegrationRejection ---

func TestParseIntegrationRejection_Fixup(t *testing.T) {
	r := ParseIntegrationRejection(fixupRejectionRecord)
	if r == nil {
		t.Fatal("expected a parsed rejection, got nil")
	}
	if r.Kind != review.KindFixup {
		t.Errorf("Kind = %q, want %q", r.Kind, review.KindFixup)
	}
	if r.WhatIsWrong == "" {
		t.Error("WhatIsWrong must not be empty")
	}
	const wantAcceptance = "An interrupted export never leaves a manifest describing a file that does\nnot exist on disk."
	if r.Acceptance != wantAcceptance {
		t.Errorf("Acceptance = %q, want %q (the trailing VERDICT line must not leak into it)", r.Acceptance, wantAcceptance)
	}
	if r.Question != "" {
		t.Errorf("Question must stay empty for a fixup rejection, got %q", r.Question)
	}
}

func TestParseIntegrationRejection_Decision(t *testing.T) {
	r := ParseIntegrationRejection(decisionRejectionRecord)
	if r == nil {
		t.Fatal("expected a parsed rejection, got nil")
	}
	if r.Kind != review.KindDecision {
		t.Errorf("Kind = %q, want %q", r.Kind, review.KindDecision)
	}
	if r.Question == "" {
		t.Error("Question must be recovered for a decision rejection")
	}
	if r.Acceptance != "" {
		t.Errorf("Acceptance must stay empty for a decision rejection, got %q", r.Acceptance)
	}
}

// --- The ambiguous cases: every one of these must come back nil, never a
// best guess at Fixup or Decision. ---

func TestParseIntegrationRejection_AmbiguousCases(t *testing.T) {
	cases := map[string]string{
		"no classification at all": "## What is wrong\n\nsomething\n\nVERDICT: REJECT",
		"unrecognized classification token": "## Classification\n\nmaybe\n\n" +
			"## What is wrong\n\nsomething\n\nVERDICT: REJECT",
		"empty classification": "## Classification\n\n\n\n## What is wrong\n\nsomething\n\nVERDICT: REJECT",
		"fixup missing acceptance criteria": "## Classification\n\nfixup\n\n" +
			"## What is wrong\n\nsomething is wrong\n\nVERDICT: REJECT",
		"decision missing question": "## Classification\n\ndecision\n\n" +
			"## What is wrong\n\nsomething is wrong\n\nVERDICT: REJECT",
		"fixup missing what is wrong": "## Classification\n\nfixup\n\n" +
			"## Fix-up acceptance criteria\n\ncriteria\n\nVERDICT: REJECT",
		"totally empty": "",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ParseIntegrationRejection(content); got != nil {
				t.Errorf("expected nil (ambiguous), got %+v", got)
			}
		})
	}
}

// --- The exploit a naive line-regexp parser would miss: a heading hidden
// inside a fence or an HTML comment must never count as real content, and
// two REAL headings claiming the same key must refuse rather than let the
// last one win. ---

// TestParseIntegrationRejection_HiddenHeadingInFenceNeverWins is the
// concrete scenario: the reviewer writes a real, visible "decision"
// classification, then includes an example inside a fenced code block that
// happens to contain what looks like a "fixup" classification with its own
// acceptance criteria. A parser that is not fence-aware would see BOTH
// headings, and the later (hidden) one would silently override the real
// one - creating an autonomous fix-up bead where the reviewer's actual,
// visible declaration said this needs the operator.
func TestParseIntegrationRejection_HiddenHeadingInFenceNeverWins(t *testing.T) {
	content := "## Classification\n\ndecision\n\n" +
		"## What is wrong\n\nreal defect description\n\n" +
		"## Question for the operator\n\nwhich rule wins\n\n" +
		"Here is an example of what a fix-up declaration would look like:\n\n" +
		"```\n## Classification\n\nfixup\n\n## Fix-up acceptance criteria\n\nfake criteria\n```\n\n" +
		"VERDICT: REJECT"

	r := ParseIntegrationRejection(content)
	if r == nil {
		t.Fatal("expected the real, visible decision to parse - the fenced example must not count")
	}
	if r.Kind != review.KindDecision {
		t.Errorf("Kind = %q, want %q - a heading hidden inside a fence must never win over the real one", r.Kind, review.KindDecision)
	}
	// The fenced example has no heading of its own (fence content is not
	// recognized as a heading at all), so it trails into this section's own
	// body as literal text rather than being split out - that is a correct
	// side effect of being fence-aware, not the property this test is
	// checking. What matters is that the REAL question leads the body and
	// the Kind was never overridden by the hidden declaration.
	if !strings.HasPrefix(r.Question, "which rule wins") {
		t.Errorf("Question = %q, want it to start with the real declaration's own question", r.Question)
	}
}

// TestParseIntegrationRejection_HiddenHeadingInCommentNeverWins is the same
// exploit through an HTML comment instead of a fence.
func TestParseIntegrationRejection_HiddenHeadingInCommentNeverWins(t *testing.T) {
	content := "## Classification\n\nfixup\n\n" +
		"## What is wrong\n\nreal defect\n\n" +
		"## Fix-up acceptance criteria\n\nreal criteria\n\n" +
		"<!--\n## Classification\n\ndecision\n\n## Question for the operator\n\nfake question\n-->\n\n" +
		"VERDICT: REJECT"

	r := ParseIntegrationRejection(content)
	if r == nil {
		t.Fatal("expected the real, visible fixup declaration to parse")
	}
	if r.Kind != review.KindFixup || r.Acceptance != "real criteria" {
		t.Errorf("got %+v, want the real fixup declaration - the commented-out one must not count", r)
	}
}

// TestParseIntegrationRejection_DuplicateRealHeadingRefuses is the mundane
// case with no hiding trick at all: the SAME recognized heading appears
// twice, both in plain view. Which one is authoritative is not decidable,
// so this must escalate rather than silently pick either.
func TestParseIntegrationRejection_DuplicateRealHeadingRefuses(t *testing.T) {
	content := "## Classification\n\nfixup\n\n" +
		"## What is wrong\n\nfirst defect\n\n" +
		"## Fix-up acceptance criteria\n\nfirst criteria\n\n" +
		"## Classification\n\ndecision\n\n" +
		"## Question for the operator\n\nsecond question\n\n" +
		"VERDICT: REJECT"

	if r := ParseIntegrationRejection(content); r != nil {
		t.Errorf("expected nil (ambiguous) for a duplicate real heading, got %+v", r)
	}
}

// TestParseIntegrationRejection_ContradictoryFieldsRefuse proves a document
// that is internally inconsistent - it declares fixup but also carries a
// decision's own question - is refused rather than accepted with the
// contradictory field silently ignored.
func TestParseIntegrationRejection_ContradictoryFieldsRefuse(t *testing.T) {
	content := "## Classification\n\nfixup\n\n" +
		"## What is wrong\n\nsomething\n\n" +
		"## Fix-up acceptance criteria\n\ncriteria\n\n" +
		"## Question for the operator\n\nbut also this question\n\n" +
		"VERDICT: REJECT"

	if r := ParseIntegrationRejection(content); r != nil {
		t.Errorf("expected nil (ambiguous) for a fixup declaration that also carries a question, got %+v", r)
	}

	content2 := "## Classification\n\ndecision\n\n" +
		"## What is wrong\n\nsomething\n\n" +
		"## Question for the operator\n\nwhich way\n\n" +
		"## Fix-up acceptance criteria\n\nbut also this\n\n" +
		"VERDICT: REJECT"
	if r := ParseIntegrationRejection(content2); r != nil {
		t.Errorf("expected nil (ambiguous) for a decision declaration that also carries acceptance criteria, got %+v", r)
	}
}

func TestParseIntegrationRejection_HeadingMatchIsCaseAndSpaceInsensitive(t *testing.T) {
	content := "##   CLASSIFICATION  \n\nfixup\n\n## what IS wrong\n\nbad thing\n\n" +
		"## FIX-UP ACCEPTANCE CRITERIA\n\ncriteria here\n\nVERDICT: REJECT"
	r := ParseIntegrationRejection(content)
	if r == nil || r.Kind != review.KindFixup || r.Acceptance != "criteria here" {
		t.Fatalf("got %+v", r)
	}
}
