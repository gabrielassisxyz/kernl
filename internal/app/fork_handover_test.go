package app

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const forkHandoverRecord = `Some preamble prose here.

## Fork

should search rank results by recency or by relevance

## Options Considered

recency-first; relevance-first

## What Would Have To Agree

nothing outside this function - callers receive a list either way
`

// --- ParseForkHandover ---

func TestParseForkHandover_WellFormed(t *testing.T) {
	h := ParseForkHandover(forkHandoverRecord)
	if h == nil {
		t.Fatal("expected a parsed handover, got nil")
	}
	if h.Fork == "" || h.OptionsConsidered == "" || h.WhatWouldHaveToAgree == "" {
		t.Errorf("got %+v, want every field populated", h)
	}
}

func TestParseForkHandover_AmbiguousCasesReturnNil(t *testing.T) {
	cases := map[string]string{
		"missing fork":                     "## Options Considered\n\na; b\n\n## What Would Have To Agree\n\nnothing\n",
		"missing options considered":       "## Fork\n\nq\n\n## What Would Have To Agree\n\nnothing\n",
		"missing what would have to agree": "## Fork\n\nq\n\n## Options Considered\n\na; b\n",
		"empty fork body":                  "## Fork\n\n\n\n## Options Considered\n\na; b\n\n## What Would Have To Agree\n\nnothing\n",
		"totally empty":                    "",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ParseForkHandover(content); got != nil {
				t.Errorf("expected nil (ambiguous), got %+v", got)
			}
		})
	}
}

// A heading hidden inside a fence must never count as a real heading - the
// same exploit integration_rejection_test.go proves against
// ParseIntegrationRejection, here against the fork vocabulary instead.
func TestParseForkHandover_HiddenHeadingInFenceNeverWins(t *testing.T) {
	content := "## Fork\n\nreal fork\n\n" +
		"## Options Considered\n\nreal a; real b\n\n" +
		"## What Would Have To Agree\n\nreal answer\n\n" +
		"Example of a different fork:\n\n```\n## Fork\n\nfake fork\n```\n"

	h := ParseForkHandover(content)
	if h == nil {
		t.Fatal("expected the real, visible fork to parse - the fenced example must not count")
	}
	if h.Fork != "real fork" {
		t.Errorf("Fork = %q, want the real, visible one", h.Fork)
	}
}

func TestParseForkHandover_DuplicateRealHeadingRefuses(t *testing.T) {
	content := "## Fork\n\nfirst fork\n\n" +
		"## Options Considered\n\na; b\n\n" +
		"## What Would Have To Agree\n\nnothing\n\n" +
		"## Fork\n\nsecond fork\n"
	if got := ParseForkHandover(content); got != nil {
		t.Errorf("expected nil (ambiguous) for a duplicate real heading, got %+v", got)
	}
}

// --- DecideForkAction ---

// countingDA is a named fake (AGENTS.md §4) recording how often it was
// consulted and what it was asked - the count is the assertion that matters
// most: the measured layers exist so a model is never asked what a fact
// already settles.
type countingDA struct {
	answer string
	err    error
	calls  int
	asked  string
}

func (d *countingDA) Consult(_ context.Context, question string) (string, error) {
	d.calls++
	d.asked = question
	return d.answer, d.err
}

func decidingDA(chosen string) *countingDA {
	return &countingDA{answer: "FORK: DECIDE\nCHOSEN: " + chosen + "\nnothing outside this bead has to agree with it.\n"}
}

func wellFormedHandover() *ForkHandover {
	return &ForkHandover{
		Fork:                 "should search rank by recency or relevance",
		OptionsConsidered:    "recency-first; relevance-first",
		WhatWouldHaveToAgree: "nothing outside this function",
	}
}

func cleanForkFacts() ForkScopeFacts {
	return ForkScopeFacts{EpicID: "ep-1", BeadID: "ep-1.4"}
}

func TestDecideForkAction_NilHandoverEscalatesWithoutAskingTheDA(t *testing.T) {
	da := decidingDA("relevance-first")
	got := DecideForkAction(context.Background(), nil, cleanForkFacts(), da)

	if got.Action != ForkActionEscalate || got.Cause != ForkCauseUnreadableHandover {
		t.Errorf("got %+v, want an escalation caused by %q", got, ForkCauseUnreadableHandover)
	}
	if da.calls != 0 {
		t.Errorf("the DA was asked %d times about a handover that could not even be read", da.calls)
	}
}

func TestDecideForkAction_OpenDependentsEscalatesWithoutAskingTheDA(t *testing.T) {
	da := decidingDA("relevance-first")
	facts := cleanForkFacts()
	facts.OpenDependents = []string{"ep-1.5", "ep-1.6"}

	got := DecideForkAction(context.Background(), wellFormedHandover(), facts, da)

	if got.Action != ForkActionEscalate || got.Cause != ForkCauseOpenDependents {
		t.Errorf("got %+v, want an escalation caused by %q", got, ForkCauseOpenDependents)
	}
	if da.calls != 0 {
		t.Errorf("the DA was asked %d times - open sibling dependents settle this without a model", da.calls)
	}
	if !strings.Contains(got.Reason, "ep-1.5") || !strings.Contains(got.Reason, "ep-1.6") {
		t.Errorf("Reason = %q, want it to name the open dependents", got.Reason)
	}
}

func TestDecideForkAction_NilDAEscalatesWithoutAskingAnything(t *testing.T) {
	got := DecideForkAction(context.Background(), wellFormedHandover(), cleanForkFacts(), nil)

	if got.Action != ForkActionEscalate || got.Cause != ForkCauseNoDAConfigured {
		t.Errorf("got %+v, want an escalation caused by %q", got, ForkCauseNoDAConfigured)
	}
}

func TestDecideForkAction_NoOpenDependentsReachesTheDAExactlyOnceAndIsDecided(t *testing.T) {
	da := decidingDA("relevance-first")

	got := DecideForkAction(context.Background(), wellFormedHandover(), cleanForkFacts(), da)

	if da.calls != 1 {
		t.Fatalf("the DA was asked %d times, want exactly once", da.calls)
	}
	if got.Action != ForkActionDecided || got.Cause != ForkCauseDADecided {
		t.Errorf("got %+v, want a decision", got)
	}
	if got.ChosenOption != "relevance-first" {
		t.Errorf("ChosenOption = %q, want it copied verbatim from the DA's CHOSEN line", got.ChosenOption)
	}
	if got.Reason == "" {
		t.Error("Reason must not be empty")
	}
}

func TestDecideForkAction_DAEscalates(t *testing.T) {
	da := &countingDA{answer: "FORK: ESCALATE\nthis narrows a policy other beads assumed.\n"}

	got := DecideForkAction(context.Background(), wellFormedHandover(), cleanForkFacts(), da)

	if got.Action != ForkActionEscalate || got.Cause != ForkCauseDAEscalated {
		t.Errorf("got %+v, want an escalation caused by %q", got, ForkCauseDAEscalated)
	}
	if !strings.Contains(got.Reason, "narrows a policy") {
		t.Errorf("Reason = %q, want the DA's own reason", got.Reason)
	}
}

func TestDecideForkAction_UnparseableDAAnswerEscalatesRatherThanDecides(t *testing.T) {
	da := &countingDA{answer: "I think relevance-first is better."}

	got := DecideForkAction(context.Background(), wellFormedHandover(), cleanForkFacts(), da)

	if got.Action != ForkActionEscalate || got.Cause != ForkCauseDAAnswerUnparseable {
		t.Errorf("got %+v, want an escalation caused by %q, never a fabricated decision", got, ForkCauseDAAnswerUnparseable)
	}
}

func TestDecideForkAction_DAErrorEscalates(t *testing.T) {
	da := &countingDA{err: errors.New("agent CLI exited 1")}

	got := DecideForkAction(context.Background(), wellFormedHandover(), cleanForkFacts(), da)

	if got.Action != ForkActionEscalate || got.Cause != ForkCauseDAUnavailable {
		t.Errorf("got %+v, want an escalation caused by %q", got, ForkCauseDAUnavailable)
	}
}

// --- ParseForkAnswer / parseForkDecideAnswer: chosen-option containment ---

// TestParseForkAnswer_ChosenOptionAbsentFromOptionsConsideredEscalates proves
// finding 3 of the fork/decision-gate hardening pass: a DA naming an option
// that was never among the alternatives it was actually offered must not be
// read as a decision. "hybrid" appears nowhere in wellFormedHandover's own
// OptionsConsidered ("recency-first; relevance-first").
func TestParseForkAnswer_ChosenOptionAbsentFromOptionsConsideredEscalates(t *testing.T) {
	_, err := ParseForkAnswer("FORK: DECIDE\nCHOSEN: hybrid\nit balances both signals.\n", wellFormedHandover())
	if err == nil {
		t.Fatal("expected an error: hybrid was never among the options this fork actually offered")
	}
}

// TestDecideForkAction_ChosenOptionNotOfferedEscalatesRatherThanDecides is the
// same failure exercised through DecideForkAction, proving the unusable
// answer surfaces as the SAME escalation cause any other unparseable DA reply
// does (ForkCauseDAAnswerUnparseable) - never a fabricated decision.
func TestDecideForkAction_ChosenOptionNotOfferedEscalatesRatherThanDecides(t *testing.T) {
	da := &countingDA{answer: "FORK: DECIDE\nCHOSEN: hybrid\nit balances both signals.\n"}

	got := DecideForkAction(context.Background(), wellFormedHandover(), cleanForkFacts(), da)

	if got.Action != ForkActionEscalate || got.Cause != ForkCauseDAAnswerUnparseable {
		t.Errorf("got %+v, want an escalation caused by %q - an option nobody weighed must never decide", got, ForkCauseDAAnswerUnparseable)
	}
}

// TestParseForkAnswer_ChosenOptionContainmentIgnoresCaseAndSurroundingWhitespace
// proves the comparison is not a brittle exact match: different case and
// stray whitespace around the option in either the CHOSEN line or the prose
// must not turn a genuinely offered option into a refused one.
func TestParseForkAnswer_ChosenOptionContainmentIgnoresCaseAndSurroundingWhitespace(t *testing.T) {
	h := &ForkHandover{
		Fork:                 "which ranking wins",
		OptionsConsidered:    "  Recency-First ; Relevance-First  ",
		WhatWouldHaveToAgree: "nothing outside this bead",
	}
	got, err := ParseForkAnswer("FORK: DECIDE\nCHOSEN: RELEVANCE-FIRST\nnothing outside this bead has to agree.\n", h)
	if err != nil {
		t.Fatalf("ParseForkAnswer: %v", err)
	}
	if got.ChosenOption != "RELEVANCE-FIRST" {
		t.Errorf("ChosenOption = %q, want the DA's own CHOSEN text preserved verbatim even though the comparison was case-insensitive", got.ChosenOption)
	}
}

// A recorded preference must be visible in the prompt text the DA actually
// receives - it is an input to judgment, never a short-circuit this package
// applies on the DA's behalf (see DecideForkAction's own doc comment).
func TestDecideForkAction_RecordedPreferenceIsVisibleInThePromptTheDAReceives(t *testing.T) {
	da := decidingDA("relevance-first")
	facts := cleanForkFacts()
	facts.RelatedDecisions = "- **search ranking** - relevance-first won, because recency already has its own sort"

	DecideForkAction(context.Background(), wellFormedHandover(), facts, da)

	if !strings.Contains(da.asked, "relevance-first won, because recency already has its own sort") {
		t.Errorf("the DA's own question = %q, want the recorded preference visible in it", da.asked)
	}
}
