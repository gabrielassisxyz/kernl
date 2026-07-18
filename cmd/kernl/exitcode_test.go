package main

import (
	"errors"
	"fmt"
	"testing"
)

func TestExitCodeDictionary(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil is success", nil, 0},
		{"plain error is internal", errors.New("boom"), 1},
		{"usage error is 2", usagef("bad invocation"), 2},
		{"wrapped usage error stays 2", fmt.Errorf("outer: %w", usagef("bad")), 2},
	}
	for _, c := range cases {
		if got := exitCode(c.err); got != c.want {
			t.Errorf("%s: exitCode = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestUsageErrorsClassifiedAcrossVerbs(t *testing.T) {
	// Every wrong-invocation path must exit 2, not 1, so scripts can branch.
	assertUsage := func(label string, err error) {
		t.Helper()
		if err == nil {
			t.Errorf("%s: should fail", label)
			return
		}
		if exitCode(err) != 2 {
			t.Errorf("%s: should be a usage error (exit 2), got %d: %v", label, exitCode(err), err)
		}
	}
	assertUsage("unknown verb", Dispatch([]string{"frobnicate"}))
	assertUsage("epic no sub", runEpicWithApp(nil, nil, func(string) {}))
	assertUsage("epic unknown sub", runEpicWithApp(nil, []string{"frobnicate"}, func(string) {}))
	assertUsage("bead no sub", runBeadWithApp(nil, nil))
	assertUsage("bead unknown sub", runBeadWithApp(nil, []string{"frobnicate"}))
	assertUsage("help unknown topic", printHelpFor([]string{"frobnicate"}))
}
