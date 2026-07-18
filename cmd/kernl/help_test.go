package main

import (
	"strings"
	"testing"
)

// stubAllVerbs replaces every verb fn with one that fails the test if called.
// Help interception must fire BEFORE any verb dispatch — a help request that
// reaches a verb fn is the exact bug class this guards against (capture once
// stored "--help" as a note; sweep ran a live tick).
func stubAllVerbs(t *testing.T) {
	t.Helper()
	fail := func(name string) func(string, []string) error {
		return func(string, []string) error {
			t.Fatalf("verb %s was dispatched on a help request", name)
			return nil
		}
	}
	doctorFn = func(string, []string) error { t.Fatal("doctor dispatched on a help request"); return nil }
	serveFn = func(string, int, bool) error { t.Fatal("serve dispatched on a help request"); return nil }
	epicFn = fail("epic")
	beadFn = fail("bead")
	sweepFn = fail("sweep")
	bookmarkFn = fail("bookmark")
	captureFn = fail("capture")
	planFn = fail("plan")
	t.Cleanup(func() {
		doctorFn, serveFn = runDoctor, runServe
		epicFn, beadFn, sweepFn = runEpic, runBead, runSweep
		bookmarkFn, captureFn, planFn = runBookmark, runCapture, runPlan
	})
}

func TestHelpInterceptsEveryVerb(t *testing.T) {
	stubAllVerbs(t)
	invocations := [][]string{
		{"serve", "--help"}, {"doctor", "--help"}, {"epic", "--help"},
		{"bead", "--help"}, {"sweep", "--help"}, {"bookmark", "--help"},
		{"capture", "--help"}, {"plan", "--help"}, {"version", "--help"},
		{"epic", "run", "--help"}, {"epic", "list", "-h"}, {"epic", "merge", "--help"},
		{"epic", "abort", "--help"}, {"bead", "run", "--help"},
		{"bookmark", "add", "--help"}, {"bookmark", "import", "-h"},
		{"capture", "-h"}, {"sweep", "-h"},
		{"help", "epic"}, {"help", "sweep"}, {"help"},
	}
	for _, argv := range invocations {
		if err := Dispatch(argv); err != nil {
			t.Errorf("Dispatch(%v) on a help request must succeed, got: %v", argv, err)
		}
	}
}

func TestHelpWorksWithGlobalFlagsPresent(t *testing.T) {
	stubAllVerbs(t)
	if err := Dispatch([]string{"--config", "does-not-exist.yaml", "epic", "--help"}); err != nil {
		t.Fatalf("help must not require a loadable config, got: %v", err)
	}
}

func TestHelpUnknownTopicFailsLoud(t *testing.T) {
	err := printHelpFor([]string{"frobnicate"})
	if err == nil || !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Fatalf("unknown help topic must fail loud, got: %v", err)
	}
	if !strings.Contains(err.Error(), "capture") {
		t.Errorf("unknown-topic error should list valid topics, got: %v", err)
	}
	err = printHelpFor([]string{"epic", "frobnicate"})
	if err == nil || !strings.Contains(err.Error(), "list") {
		t.Fatalf("unknown sub-topic error should list valid sub-verbs, got: %v", err)
	}
}

func TestHelpTopicDetection(t *testing.T) {
	cases := []struct {
		args  []string
		topic []string
		ok    bool
	}{
		{[]string{"epic", "--help"}, []string{"epic"}, true},
		{[]string{"epic", "run", "--help"}, []string{"epic", "run"}, true},
		{[]string{"-h"}, nil, true},
		{[]string{"help", "epic"}, []string{"epic"}, true},
		{[]string{"capture", "--", "--help"}, nil, false},
		{[]string{"capture", "real", "text"}, nil, false},
	}
	for _, c := range cases {
		topic, ok := helpTopic(c.args)
		if ok != c.ok || strings.Join(topic, " ") != strings.Join(c.topic, " ") {
			t.Errorf("helpTopic(%v) = (%v, %v), want (%v, %v)", c.args, topic, ok, c.topic, c.ok)
		}
	}
}

func TestCommandTableCoversDispatch(t *testing.T) {
	// Every dispatchable verb must have a help entry; the table is the single
	// source of truth and this pins them together.
	for _, verb := range []string{"serve", "doctor", "epic", "bead", "sweep", "bookmark", "capture", "plan", "capabilities", "robot-docs", "version"} {
		if findCommand(commandTable, verb) == nil {
			t.Errorf("verb %q missing from commandTable", verb)
		}
	}
}
