package main

import (
	"strings"
	"testing"
)

func TestDamerauDistanceCatchesRealTypoClasses(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"epic", "epic", 0},
		{"epci", "epic", 1},             // transposition
		{"swep", "sweep", 1},            // deletion
		{"captur", "capture", 1},        // deletion
		{"--jsno", "--json", 1},         // transposition
		{"autnoomous", "autonomous", 1}, // adjacent swap
	}
	for _, c := range cases {
		if got := damerauDistance(c.a, c.b); got != c.want {
			t.Errorf("damerauDistance(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSuggestThresholds(t *testing.T) {
	verbs := []string{"serve", "doctor", "epic", "bead", "sweep", "bookmark", "capture", "plan", "version"}
	if got := suggest("captur", verbs); got != "capture" {
		t.Errorf("suggest(captur) = %q, want capture", got)
	}
	if got := suggest("boockmark", verbs); got != "bookmark" {
		t.Errorf("suggest(boockmark) = %q, want bookmark", got)
	}
	if got := suggest("zzz", verbs); got != "" {
		t.Errorf("suggest(zzz) = %q, want no suggestion", got)
	}
	// Short inputs never get distance-2 guesses (too noisy).
	if got := suggest("pl", []string{"plan"}); got != "" {
		t.Errorf("suggest(pl) = %q, want no suggestion for short input", got)
	}
}

func TestDispatchUnknownVerbSuggests(t *testing.T) {
	err := Dispatch([]string{"captur", "some text"})
	if err == nil || !strings.Contains(err.Error(), `did you mean "capture"?`) {
		t.Fatalf("expected did-you-mean hint, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("typo'd verb should be a usage error, got exit %d", exitCode(err))
	}
}

func TestDispatchUnknownFlagSuggests(t *testing.T) {
	err := Dispatch([]string{"--cofnig", "x.yaml", "doctor"})
	if err == nil || !strings.Contains(err.Error(), `did you mean "--config"?`) {
		t.Fatalf("expected flag did-you-mean hint, got: %v", err)
	}
}

func TestEpicRunRejectsStrayFlags(t *testing.T) {
	// A typo'd flag must error, not silently become the epic ID.
	err := runEpicRun(nil, []string{"--autonomos", "epic-42"}, func(string) {})
	if err == nil || !strings.Contains(err.Error(), `did you mean "--autonomous"?`) {
		t.Fatalf("expected stray-flag rejection with hint, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("stray flag should be a usage error, got exit %d", exitCode(err))
	}
}

func TestEpicSubcommandSuggests(t *testing.T) {
	err := runEpicWithApp(nil, []string{"lsit"}, func(string) {})
	if err == nil || !strings.Contains(err.Error(), `did you mean "list"?`) {
		t.Fatalf("expected epic sub-verb hint, got: %v", err)
	}
}
