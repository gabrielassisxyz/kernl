package app

import (
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

func TestParseIntegrationRejection_HeadingMatchIsCaseAndSpaceInsensitive(t *testing.T) {
	content := "##   CLASSIFICATION  \n\nfixup\n\n## what IS wrong\n\nbad thing\n\n" +
		"## FIX-UP ACCEPTANCE CRITERIA\n\ncriteria here\n\nVERDICT: REJECT"
	r := ParseIntegrationRejection(content)
	if r == nil || r.Kind != review.KindFixup || r.Acceptance != "criteria here" {
		t.Fatalf("got %+v", r)
	}
}

// --- DecideFixupAction: the pure §7 policy, including the two rules that
// matter most - the ambiguous case escalates, and the cap wins even over a
// clean fixup declaration. ---

func TestDecideFixupAction(t *testing.T) {
	fixup := &IntegrationRejection{Kind: review.KindFixup, WhatIsWrong: "x", Acceptance: "y"}
	decision := &IntegrationRejection{Kind: review.KindDecision, WhatIsWrong: "x", Question: "y"}

	cases := []struct {
		name               string
		rejection          *IntegrationRejection
		epicAlreadyFixedUp bool
		want               FixupAction
	}{
		{"fixup, no prior fix-up -> create a bead", fixup, false, FixupActionCreateBead},
		{"decision, no prior fix-up -> escalate", decision, false, FixupActionEscalateDecisionOrAmbiguous},
		{"ambiguous (nil), no prior fix-up -> escalate", nil, false, FixupActionEscalateDecisionOrAmbiguous},
		// The cap: a second rejection on an epic that already spawned one
		// fix-up bead escalates NO MATTER what this new rejection declares -
		// "a fix-up cannot spawn a fix-up" is a rule about the epic's own
		// history, not about how well this rejection classifies itself.
		{"fixup declared, but epic already fixed up -> escalate (the cap)", fixup, true, FixupActionEscalateAlreadyFixedUp},
		{"decision declared, epic already fixed up -> escalate (the cap)", decision, true, FixupActionEscalateAlreadyFixedUp},
		{"ambiguous, epic already fixed up -> escalate (the cap)", nil, true, FixupActionEscalateAlreadyFixedUp},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DecideFixupAction(tc.rejection, tc.epicAlreadyFixedUp); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
