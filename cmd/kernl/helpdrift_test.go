package main

import (
	"strings"
	"testing"
)

// R2-008: help text must not drift from code behavior. These two topics each
// shipped a claim the code contradicts — an agent that reads the help writes
// the wrong branch. The tests pin the help to the observed behavior so the
// drift cannot silently reopen.

func subDetails(t *testing.T, verb, sub string) string {
	t.Helper()
	cmd := findCommand(commandTable, verb)
	if cmd == nil {
		t.Fatalf("no %q command in the table", verb)
	}
	s := findCommand(cmd.Subs, sub)
	if s == nil {
		t.Fatalf("no %q sub under %q", sub, verb)
	}
	return s.Details
}

func TestGraphBriefingHelpMatchesExitZeroBehavior(t *testing.T) {
	// runGraphBriefing uses getOptional: a missing briefing exits 0 with
	// {"briefing":null}, it does NOT answer 404/exit 2.
	d := subDetails(t, "graph", "briefing")
	if strings.Contains(d, "404") || strings.Contains(d, "exits 2") {
		t.Errorf("briefing help still claims 404/exit 2, but the code exits 0: %q", d)
	}
	if !strings.Contains(d, "exits 0") || !strings.Contains(d, "briefing\":null") {
		t.Errorf("briefing help must document the real exit-0 / null-body answer: %q", d)
	}
}

func TestHealthUpdateCheckHelpMatchesResponseShape(t *testing.T) {
	// appUpdateHandler returns {"status":"unknown","checked":false,...} — never
	// "up_to_date". The field to branch on is "checked".
	d := subDetails(t, "health", "update-check")
	if strings.Contains(d, "up_to_date") {
		t.Errorf("update-check help still claims up_to_date, but the endpoint returns unknown/checked:false: %q", d)
	}
	if !strings.Contains(d, "checked") {
		t.Errorf("update-check help must name the 'checked' field an agent branches on: %q", d)
	}
}
