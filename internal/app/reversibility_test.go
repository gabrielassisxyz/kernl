package app

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// countingJudge is a named fake (AGENTS.md §4) that records how often it was
// consulted. The count is the assertion that matters most here: the mechanical
// layers exist precisely so a model is not asked what facts already settle,
// and a judge that answers correctly but is asked anyway has failed the design
// even though every verdict comes out right.
type countingJudge struct {
	verdict ReversibilityVerdict
	err     error
	calls   int
	asked   ReversibilityQuestion
}

func (j *countingJudge) JudgeReversibility(_ context.Context, q ReversibilityQuestion) (ReversibilityVerdict, error) {
	j.calls++
	j.asked = q
	return j.verdict, j.err
}

func cheapJudge() *countingJudge {
	return &countingJudge{verdict: ReversibilityVerdict{Expensive: false, Reason: "it is a branch nobody has pulled"}}
}

func fixupRejection() *IntegrationRejection {
	return &IntegrationRejection{Kind: "fixup", WhatIsWrong: "the manifest is written before the last file is flushed", Acceptance: "the manifest is written last"}
}

func cleanFacts() ReversibilityFacts {
	return ReversibilityFacts{EpicID: "ep-1", ChangeSummary: " 3 files changed, 40 insertions(+)", Budget: FixupBudget}
}

// --- The mechanical layers: certain answers, decided without a model. ---

func TestDecideFixupAction_PublishedEscalatesWithoutAskingTheMayor(t *testing.T) {
	judge := cheapJudge()
	facts := cleanFacts()
	facts.Published = true
	facts.PublishedDetail = "refs/remotes/origin/feat/ep-1"

	got := DecideFixupAction(context.Background(), fixupRejection(), facts, judge)

	if got.Action != FixupActionEscalate || got.Cause != GatePublished {
		t.Errorf("got %+v, want an escalation caused by %q", got, GatePublished)
	}
	if judge.calls != 0 {
		t.Errorf("the mayor was asked %d times about work that is already published - a fact settles this, not an opinion", judge.calls)
	}
	if !strings.Contains(got.Reason, "refs/remotes/origin/feat/ep-1") {
		t.Errorf("Reason = %q, want it to name what was found published", got.Reason)
	}
}

func TestDecideFixupAction_IrreversibleSurfaceEscalatesWithoutAskingTheMayor(t *testing.T) {
	judge := cheapJudge()
	facts := cleanFacts()
	facts.IrreversibleSurfacesTouched = []string{"migrations/0007_drop_column.sql"}

	got := DecideFixupAction(context.Background(), fixupRejection(), facts, judge)

	if got.Action != FixupActionEscalate || got.Cause != GateIrreversibleSurface {
		t.Errorf("got %+v, want an escalation caused by %q", got, GateIrreversibleSurface)
	}
	if judge.calls != 0 {
		t.Errorf("the mayor was asked %d times about a surface the repository already declared irreversible", judge.calls)
	}
	if !strings.Contains(got.Reason, "migrations/0007_drop_column.sql") {
		t.Errorf("Reason = %q, want it to name the path that fired", got.Reason)
	}
}

// The budget is the hard stop that keeps "cheap to reverse" from being an
// unbounded loop, so it must win over a mayor that would happily keep going.
func TestDecideFixupAction_ExhaustedBudgetEscalatesEvenWhenReversalIsCheap(t *testing.T) {
	judge := cheapJudge()
	facts := cleanFacts()
	facts.FixupsSpent = FixupBudget

	got := DecideFixupAction(context.Background(), fixupRejection(), facts, judge)

	if got.Action != FixupActionEscalate || got.Cause != GateBudgetExhausted {
		t.Errorf("got %+v, want an escalation caused by %q", got, GateBudgetExhausted)
	}
	if judge.calls != 0 {
		t.Errorf("the mayor was asked %d times after the budget was already spent", judge.calls)
	}
}

func TestDecideFixupAction_DecisionAndAmbiguousNeverReachTheMayor(t *testing.T) {
	decision := &IntegrationRejection{Kind: "decision", WhatIsWrong: "two children disagree", Question: "which slug rule wins?"}

	for name, rejection := range map[string]*IntegrationRejection{
		"a declared decision": decision,
		"nothing parseable":   nil,
	} {
		t.Run(name, func(t *testing.T) {
			judge := cheapJudge()
			got := DecideFixupAction(context.Background(), rejection, cleanFacts(), judge)
			if got.Action != FixupActionEscalate || got.Cause != GateDecisionOrAmbiguous {
				t.Errorf("got %+v, want an escalation caused by %q", got, GateDecisionOrAmbiguous)
			}
			if judge.calls != 0 {
				t.Errorf("the mayor was asked %d times - how expensive this is to undo is not the question when the reviewer asked for a judgment call", judge.calls)
			}
			if got.Reason == "" {
				t.Error("an escalation with no reason is exactly what this gate must not produce")
			}
		})
	}
}

// --- The residual: what nothing mechanical can answer. ---

func TestDecideFixupAction_CheapReversalContinuesAndRecordsTheReason(t *testing.T) {
	judge := cheapJudge()
	facts := cleanFacts()
	// One round already spent, well inside the budget: this is the case the
	// old one-fix-up cap sent to a human even though nothing was published.
	facts.FixupsSpent = 1

	got := DecideFixupAction(context.Background(), fixupRejection(), facts, judge)

	if got.Action != FixupActionCreateBead {
		t.Fatalf("got %+v, want the run to continue with a second fix-up bead", got)
	}
	if got.Cause != GateCheapToReverse {
		t.Errorf("Cause = %q, want %q so a continue is as greppable as an escalation", got.Cause, GateCheapToReverse)
	}
	if got.Reason != "it is a branch nobody has pulled" {
		t.Errorf("Reason = %q, want the mayor's own words recorded", got.Reason)
	}
	if judge.calls != 1 {
		t.Fatalf("the mayor was asked %d times, want exactly once", judge.calls)
	}
	if judge.asked.Objection != fixupRejection().WhatIsWrong {
		t.Errorf("the mayor was asked about %q, want the reviewer's objection", judge.asked.Objection)
	}
	if judge.asked.ChangeSummary != facts.ChangeSummary {
		t.Errorf("the mayor was given %q as the change summary, want the measured one", judge.asked.ChangeSummary)
	}
}

func TestDecideFixupAction_ExpensiveReversalEscalates(t *testing.T) {
	judge := &countingJudge{verdict: ReversibilityVerdict{Expensive: true, Reason: "it rewrites rows in place"}}

	got := DecideFixupAction(context.Background(), fixupRejection(), cleanFacts(), judge)

	if got.Action != FixupActionEscalate || got.Cause != GateExpensiveToReverse {
		t.Errorf("got %+v, want an escalation caused by %q", got, GateExpensiveToReverse)
	}
	if got.Reason != "it rewrites rows in place" {
		t.Errorf("Reason = %q, want the mayor's own words", got.Reason)
	}
}

// An unanswerable question must land on the side that stops. "Cheap" is the
// answer that keeps a run going without a human, so nothing that failed is
// allowed to produce it.
func TestDecideFixupAction_AnUnansweredQuestionEscalates(t *testing.T) {
	cases := map[string]ReversibilityJudge{
		"no judge configured at all": nil,
		"the judge failed":           &countingJudge{err: errors.New("the endpoint refused the connection")},
	}
	for name, judge := range cases {
		t.Run(name, func(t *testing.T) {
			got := DecideFixupAction(context.Background(), fixupRejection(), cleanFacts(), judge)
			if got.Action != FixupActionEscalate || got.Cause != GateReversibilityUnknown {
				t.Errorf("got %+v, want an escalation caused by %q", got, GateReversibilityUnknown)
			}
			if got.Reason == "" {
				t.Error("Reason is empty, so the operator is told a gate fired but not which one or why")
			}
		})
	}
}

// --- Reading the mayor's answer. ---

func TestParseReversibilityAnswer(t *testing.T) {
	cases := []struct {
		name          string
		answer        string
		wantErr       bool
		wantExpensive bool
		wantReason    string
	}{
		{
			name:       "cheap, with its reason",
			answer:     "REVERSAL: CHEAP\nNothing has been published and the change is confined to one package.",
			wantReason: "Nothing has been published and the change is confined to one package.",
		},
		{
			name:          "expensive, with its reason",
			answer:        "REVERSAL: EXPENSIVE\nIt migrates stored records in place.",
			wantExpensive: true,
			wantReason:    "It migrates stored records in place.",
		},
		{
			name:       "styled verdict line still parses",
			answer:     "**REVERSAL: CHEAP**\nStill a branch.",
			wantReason: "Still a branch.",
		},
		// The three refusals below all mean the same thing: nothing here can
		// be acted on, and the caller escalates rather than defaulting.
		{name: "no verdict line", answer: "I think this is fine, honestly.", wantErr: true},
		{name: "a verdict outside the enum", answer: "REVERSAL: MAYBE\nHard to say.", wantErr: true},
		{name: "a verdict with no reason", answer: "REVERSAL: CHEAP", wantErr: true},
		{name: "nothing at all", answer: "  \n ", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseReversibilityAnswer(tc.answer)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Expensive != tc.wantExpensive || got.Reason != tc.wantReason {
				t.Errorf("got %+v, want expensive=%v reason=%q", got, tc.wantExpensive, tc.wantReason)
			}
		})
	}
}

// --- Measuring the facts. ---

// fakeInspector stands in for git.
type fakeInspector struct {
	refs    []string
	changed []string
	summary string
	err     error
}

func (f fakeInspector) PublishedRefs(_, _ string) ([]string, error) {
	return f.refs, f.err
}
func (f fakeInspector) ChangedPaths(_, _, _ string) ([]string, error) {
	return f.changed, f.err
}
func (f fakeInspector) ChangeSummary(_, _, _ string) (string, error) {
	return f.summary, f.err
}

func TestGatherReversibilityFacts_MeasuresTheBranch(t *testing.T) {
	facts, err := GatherReversibilityFacts(GatherReversibilityFactsInput{
		EpicID:               "ep-1",
		RepoPath:             "/repos/example",
		BaseBranch:           "master",
		EpicBranch:           "feat/ep-1",
		IrreversibleSurfaces: []string{"migrations/", "go.sum", "docs/api/*.yaml"},
		FixupsSpent:          2,
		Inspector: fakeInspector{
			refs:    []string{"refs/remotes/origin/feat/ep-1"},
			changed: []string{"internal/app/thing.go", "migrations/0007.sql", "docs/api/public.yaml", "go.sum"},
			summary: " 4 files changed",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !facts.Published || facts.PublishedDetail != "refs/remotes/origin/feat/ep-1" {
		t.Errorf("published = %v (%q), want the ref that was found", facts.Published, facts.PublishedDetail)
	}
	want := []string{"migrations/0007.sql", "docs/api/public.yaml", "go.sum"}
	if strings.Join(facts.IrreversibleSurfacesTouched, ",") != strings.Join(want, ",") {
		t.Errorf("touched = %v, want %v - a subtree pattern, a glob and an exact path all match", facts.IrreversibleSurfacesTouched, want)
	}
	if facts.FixupsSpent != 2 || facts.Budget != FixupBudget {
		t.Errorf("spent/budget = %d/%d, want 2/%d", facts.FixupsSpent, facts.Budget, FixupBudget)
	}
}

// A repository that declares nothing declares NOTHING. Reading the empty list
// as "everything is irreversible" would send every rejection to a human and
// look exactly like the gate working.
func TestGatherReversibilityFacts_NoDeclaredSurfacesMatchesNothing(t *testing.T) {
	facts, err := GatherReversibilityFacts(GatherReversibilityFactsInput{
		EpicBranch: "feat/ep-1",
		Inspector:  fakeInspector{changed: []string{"migrations/0007.sql", "go.sum"}, summary: "2 files changed"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(facts.IrreversibleSurfacesTouched) != 0 {
		t.Errorf("touched = %v, want none declared to mean none touched", facts.IrreversibleSurfacesTouched)
	}
	if facts.Published {
		t.Error("no remote ref was found, so nothing is published")
	}
}

// Facts that could not be measured must not come back as a zero value: "not
// published, nothing irreversible touched" are the two answers that let a run
// continue without a human, and neither may be produced by a failure to look.
func TestGatherReversibilityFacts_GitFailureIsAnError(t *testing.T) {
	_, err := GatherReversibilityFacts(GatherReversibilityFactsInput{
		EpicBranch: "feat/ep-1",
		Inspector:  fakeInspector{err: errors.New("not a git repository")},
	})
	if err == nil {
		t.Fatal("expected an error when git cannot answer")
	}
	if !strings.Contains(err.Error(), "feat/ep-1") {
		t.Errorf("error = %q, want it to name the branch it could not measure", err)
	}
}

func TestMatchesSurface(t *testing.T) {
	cases := []struct {
		path, pattern string
		want          bool
	}{
		{"migrations/0001.sql", "migrations/", true},
		{"internal/migrations/0001.sql", "migrations/", false},
		{"go.sum", "go.sum", true},
		{"web/go.sum", "go.sum", false},
		{"docs/api/public.yaml", "docs/api/*.yaml", true},
		{"docs/api/v2/public.yaml", "docs/api/*.yaml", false},
		{"anything", "", false},
		// A malformed pattern matches nothing rather than failing the run:
		// it is configuration, and refusing a whole run over a stray bracket
		// costs more than a surface that visibly never fires.
		{"anything", "[", false},
	}
	for _, tc := range cases {
		if got := matchesSurface(tc.path, tc.pattern); got != tc.want {
			t.Errorf("matchesSurface(%q, %q) = %v, want %v", tc.path, tc.pattern, got, tc.want)
		}
	}
}
