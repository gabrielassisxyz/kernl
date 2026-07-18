package main

import (
	"strings"
	"testing"
)

func TestDispatchSweepRunsTick(t *testing.T) {
	var ran bool
	sweepFn = func(configPath string, args []string) error { ran = true; return nil }
	t.Cleanup(func() { sweepFn = runSweep })
	if err := Dispatch([]string{"sweep"}); err != nil || !ran {
		t.Fatalf("sweep not dispatched: ran=%v err=%v", ran, err)
	}
}

func TestDispatchSweepPassesArgs(t *testing.T) {
	var captured []string
	sweepFn = func(configPath string, args []string) error {
		captured = args
		return nil
	}
	t.Cleanup(func() { sweepFn = runSweep })
	_ = Dispatch([]string{"sweep", "--dry-run", "--repo", "/tmp/x"})
	if !stringsContain(captured, "--dry-run") {
		t.Fatalf("expected --dry-run in args, got %v", captured)
	}
	if !stringsContain(captured, "--repo") {
		t.Fatalf("expected --repo in args, got %v", captured)
	}
}

func TestSweepFlagsDefault(t *testing.T) {
	f, err := parseSweepFlags(nil)
	if err != nil {
		t.Fatalf("parseSweepFlags(nil): %v", err)
	}
	if f.dryRun {
		t.Fatal("expected dryRun=false by default")
	}
	if f.repo != "" {
		t.Fatalf("expected repo=\"\" by default, got %q", f.repo)
	}
}

func TestSweepFlagsDryRunParsed(t *testing.T) {
	f, err := parseSweepFlags([]string{"--dry-run"})
	if err != nil {
		t.Fatalf("parseSweepFlags: %v", err)
	}
	if !f.dryRun {
		t.Fatal("expected dryRun=true")
	}
}

func TestSweepFlagsRepoParsed(t *testing.T) {
	f, err := parseSweepFlags([]string{"--repo", "/tmp/x"})
	if err != nil {
		t.Fatalf("parseSweepFlags: %v", err)
	}
	if f.repo != "/tmp/x" {
		t.Fatalf("expected repo=/tmp/x, got %q", f.repo)
	}
}

func TestSweepFlagsAllParsed(t *testing.T) {
	f, err := parseSweepFlags([]string{
		"--dry-run",
		"--repo", "/tmp/x",
		"--failure-threshold", "5",
		"--backoff-minutes", "1,2,3",
		"--stale-warn-days", "14",
	})
	if err != nil {
		t.Fatalf("parseSweepFlags: %v", err)
	}
	if !f.dryRun {
		t.Fatal("expected dryRun=true")
	}
	if f.repo != "/tmp/x" {
		t.Fatalf("expected repo=/tmp/x, got %q", f.repo)
	}
	if f.failureThreshold != 5 {
		t.Fatalf("expected failureThreshold=5, got %d", f.failureThreshold)
	}
	if len(f.backoffMinutes) != 3 || f.backoffMinutes[0] != 1 || f.backoffMinutes[1] != 2 || f.backoffMinutes[2] != 3 {
		t.Fatalf("expected backoffMinutes=[1,2,3], got %v", f.backoffMinutes)
	}
	if f.staleWarnDays != 14 {
		t.Fatalf("expected staleWarnDays=14, got %d", f.staleWarnDays)
	}
}

func TestPrintHelpIncludesSweep(t *testing.T) {
	data := captureHelpOutput()
	if !strings.Contains(data, "sweep") {
		t.Fatalf("help output should include 'sweep', got:\n%s", data)
	}
}

func captureHelpOutput() string {
	origHelp := helpFn
	defer func() { helpFn = origHelp }()

	var out strings.Builder
	helpFn = func() error {
		printHelpString(&out)
		return nil
	}
	_ = Dispatch([]string{"--help"})
	return out.String()
}

func printHelpString(out *strings.Builder) {
	out.WriteString("kernl — multi-agent orchestration runner\n\n")
	out.WriteString("Subcommands:\n")
	out.WriteString("  serve        Start the HTTP API server\n")
	out.WriteString("  doctor       Run system checks\n")
	out.WriteString("  epic         Manage epics\n")
	out.WriteString("  bead         Manage individual beads\n")
	out.WriteString("  sweep        Close epics whose PRs are merged\n")
}

func stringsContain(args []string, target string) bool {
	for _, a := range args {
		if a == target {
			return true
		}
	}
	return false
}

func TestSweepFlagsUnknownFlagFailsLoud(t *testing.T) {
	_, err := parseSweepFlags([]string{"--dryrun"})
	if err == nil || !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Fatalf("unknown sweep flag must fail loud, got: %v", err)
	}
	if !strings.Contains(err.Error(), `did you mean "--dry-run"?`) {
		t.Errorf("expected did-you-mean hint, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("unknown flag should be usage error, got exit %d", exitCode(err))
	}
}

func TestSweepFlagsInvalidIntFailsLoud(t *testing.T) {
	_, err := parseSweepFlags([]string{"--failure-threshold", "abc"})
	if err == nil || !strings.Contains(err.Error(), `got "abc"`) {
		t.Fatalf("invalid int must fail loud naming the value, got: %v", err)
	}
	_, err = parseSweepFlags([]string{"--backoff-minutes", "5,x,60"})
	if err == nil || !strings.Contains(err.Error(), "comma-separated integers") {
		t.Fatalf("invalid backoff list must fail loud, got: %v", err)
	}
}

func TestSweepFlagsYesParsed(t *testing.T) {
	f, err := parseSweepFlags([]string{"--yes"})
	if err != nil || !f.yes {
		t.Fatalf("expected yes=true, got %+v err=%v", f, err)
	}
}
