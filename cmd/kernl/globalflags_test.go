package main

import (
	"strings"
	"testing"
)

func TestParseStringFlagAcceptsGnuEqualsForm(t *testing.T) {
	v, rest, err := parseConfigPath([]string{"--config=x.yaml", "doctor"})
	if err != nil || v != "x.yaml" || len(rest) != 1 || rest[0] != "doctor" {
		t.Fatalf("--config=x.yaml: v=%q rest=%v err=%v", v, rest, err)
	}
	v, rest, err = parseConfigPath([]string{"--config", "y.yaml", "doctor"})
	if err != nil || v != "y.yaml" || len(rest) != 1 {
		t.Fatalf("--config y.yaml: v=%q rest=%v err=%v", v, rest, err)
	}
}

func TestParseStringFlagDuplicateLastWins(t *testing.T) {
	v, rest, err := parseConfigPath([]string{"--config", "a.yaml", "--config", "b.yaml", "doctor"})
	if err != nil || v != "b.yaml" {
		t.Fatalf("duplicate --config must be last-wins, got v=%q err=%v", v, err)
	}
	if len(rest) != 1 || rest[0] != "doctor" {
		t.Fatalf("both occurrences must be stripped, rest=%v", rest)
	}
}

func TestParseStringFlagMissingValueFailsLoud(t *testing.T) {
	_, _, err := parseConfigPath([]string{"doctor", "--config"})
	if err == nil || exitCode(err) != 2 {
		t.Fatalf("--config without value must be usage error, got: %v", err)
	}
}

func TestParsePortInvalidValueFailsLoud(t *testing.T) {
	// A non-numeric --port used to be silently dropped, falling back to the
	// default port with no diagnostic at all.
	_, _, err := parsePort([]string{"--port", "abc", "serve"})
	if err == nil || !strings.Contains(err.Error(), `got "abc"`) {
		t.Fatalf("--port abc must fail naming the value, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("invalid port is a usage error, got exit %d", exitCode(err))
	}
	p, rest, err := parsePort([]string{"--port=9090", "serve"})
	if err != nil || p != 9090 || len(rest) != 1 {
		t.Fatalf("--port=9090: p=%d rest=%v err=%v", p, rest, err)
	}
}

func TestSentinelProtectsPayloadFromGlobalParsing(t *testing.T) {
	// Text after "--" must never be stolen by global flag stripping.
	captured := []string(nil)
	captureFn = func(configPath string, args []string) error { captured = args; return nil }
	t.Cleanup(func() { captureFn = runCapture })
	if err := Dispatch([]string{"capture", "--", "--config", "x.yaml"}); err != nil {
		t.Fatalf("capture with sentinel payload failed: %v", err)
	}
	want := []string{"--", "--config", "x.yaml"}
	if strings.Join(captured, " ") != strings.Join(want, " ") {
		t.Fatalf("payload was altered: got %v, want %v", captured, want)
	}
}

func TestEpicAbortRejectsMultipleIDs(t *testing.T) {
	err := runEpicAbort(nil, []string{"epic-a", "epic-b", "--yes"}, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "exactly one epic ID") {
		t.Fatalf("two IDs must be rejected, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("want usage error, got exit %d", exitCode(err))
	}
}

func TestWorkflowUsageErrorsExitTwo(t *testing.T) {
	for _, args := range [][]string{
		{"--workflow"}, {"--workflow="}, {"--workflow", ""},
	} {
		err := runEpicRun(nil, "kernl.yaml", args, func(string) {})
		if err == nil || exitCode(err) != 2 {
			t.Errorf("epic run %v: want usage error (exit 2), got %d: %v", args, exitCode(err), err)
		}
	}
}
