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
	err := runEpicRun(nil, "kernl.yaml", []string{"--autonomos", "epic-42"}, func(string) {})
	if err == nil || !strings.Contains(err.Error(), `did you mean "--autonomous"?`) {
		t.Fatalf("expected stray-flag rejection with hint, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("stray flag should be a usage error, got exit %d", exitCode(err))
	}
}

func TestEpicSubcommandSuggests(t *testing.T) {
	err := runEpicWithApp(nil, "kernl.yaml", []string{"lsit"}, func(string) {})
	if err == nil || !strings.Contains(err.Error(), `did you mean "list"?`) {
		t.Fatalf("expected epic sub-verb hint, got: %v", err)
	}
}

func TestRootFlagHintRedirectsSubcommandFlags(t *testing.T) {
	// A sub-verb flag typed at the root must not be a dead end — for the
	// EXACT spelling above all (the common case; a prior test only covered
	// typos and masked a dead exact-match path).
	for flag, wantOwner := range map[string]string{
		"--dry-run":    "kernl sweep --dry-run",
		"--yes":        "kernl sweep --yes",
		"--autonomous": "kernl epic run --autonomous",
		"--workflow":   "kernl epic run --workflow",
		"--json":       "after the subcommand: e.g. kernl task list --json",
		"--dryrun":     "kernl sweep --dry-run",
		"--autonomos":  "kernl epic run --autonomous",
	} {
		err := Dispatch([]string{flag})
		if err == nil || !strings.Contains(err.Error(), wantOwner) {
			t.Errorf("root %s must redirect to %q, got: %v", flag, wantOwner, err)
		}
	}
}

func TestVerbAliasHints(t *testing.T) {
	for verb, want := range map[string]string{"list": "kernl epic list", "ls": "kernl epic list"} {
		err := Dispatch([]string{verb})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("alias %q must hint %q, got: %v", verb, want, err)
		}
	}
}

func TestEpicSubcommandValidatedBeforeConfig(t *testing.T) {
	// Typo diagnosis must not depend on a loadable config.
	err := runEpic(verbContext{configPath: "definitely-missing.yaml"}, []string{"staus"})
	if err == nil || !strings.Contains(err.Error(), "unknown epic subcommand") {
		t.Fatalf("epic sub-verb typo must be diagnosed before config, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("want usage error, got exit %d", exitCode(err))
	}
}

func TestEpicHelpSecondTokenIntercepted(t *testing.T) {
	topic, ok := helpTopic([]string{"epic", "help"})
	if !ok || len(topic) != 1 || topic[0] != "epic" {
		t.Fatalf("epic help must resolve to epic help topic, got (%v, %v)", topic, ok)
	}
	topic, ok = helpTopic([]string{"epic", "help", "run"})
	if !ok || strings.Join(topic, " ") != "epic run" {
		t.Fatalf("epic help run must resolve to epic run, got (%v, %v)", topic, ok)
	}
	if _, ok := helpTopic([]string{"capture", "help"}); ok {
		t.Fatal("capture help must NOT be intercepted (capture takes free text)")
	}
}

func TestWrapLoudSingleMarker(t *testing.T) {
	inner := usagef("KERNL DISPATCH FAILURE: inner")
	if got := wrapLoud("ctx", inner).Error(); strings.Count(got, "KERNL DISPATCH FAILURE") != 1 {
		t.Errorf("marker must appear once, got: %s", got)
	}
	plain := wrapLoud("ctx", errNotFound)
	if !strings.Contains(plain.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("plain error must gain the marker, got: %s", plain)
	}
}
