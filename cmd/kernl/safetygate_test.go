package main

import (
	"errors"
	"strings"
	"testing"
)

// requireRefusedWithoutYes asserts the shared contract of every --yes gate: the
// verb previewed, wrote nothing, and ended at exit 2 rather than 0.
//
// Exit 0 on a refused mutation is what this pins against. An agent that runs
// `kernl task delete X` and branches on `$?` reads 0 as "deleted" and never
// looks at the prose that says otherwise, so the gate protects the data and
// misinforms the caller at the same time.
func requireRefusedWithoutYes(t *testing.T, err error, verb string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s without --yes must not exit 0 — an agent branching on $? reads that as done", verb)
	}
	if got := exitCode(err); got != 2 {
		t.Fatalf("%s without --yes must exit 2, got %d (%v)", verb, got, err)
	}
	var reported alreadyReported
	if !errors.As(err, &reported) {
		t.Fatalf("%s refusal must be marked already-reported so main does not print past the preview, got %v", verb, err)
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("%s refusal must name the flag that unblocks it, got: %v", verb, err)
	}
}

// TestEveryYesGateRefusesTheSameWay is the drift guard. The gates live in seven
// files; without one test naming them together, the next one added copies
// whichever neighbour it was pasted from and the contract splits again — which
// is exactly how `epic abort` ended up refusing at exit 2 while the other seven
// refused at exit 0.
func TestEveryYesGateRefusesTheSameWay(t *testing.T) {
	for _, verb := range []string{
		"task delete", "note delete", "project delete", "inbox reopen",
		"inbox batch apply", "bead close", "bead set", "approval resolve",
	} {
		err := refusedWithoutYes(verb)
		requireRefusedWithoutYes(t, err, verb)
		if !strings.Contains(err.Error(), verb) {
			t.Errorf("refusal must name the verb, got: %v", err)
		}
		if !strings.Contains(err.Error(), "nothing was changed") {
			t.Errorf("refusal must state that nothing changed, got: %v", err)
		}
	}
}
