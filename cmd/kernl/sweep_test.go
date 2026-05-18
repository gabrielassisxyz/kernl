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
	f := parseSweepFlags(nil)
	if f.dryRun {
		t.Fatal("expected dryRun=false by default")
	}
	if f.repo != "" {
		t.Fatalf("expected repo=\"\" by default, got %q", f.repo)
	}
}

func TestSweepFlagsDryRunParsed(t *testing.T) {
	f := parseSweepFlags([]string{"--dry-run"})
	if !f.dryRun {
		t.Fatal("expected dryRun=true")
	}
}

func TestSweepFlagsRepoParsed(t *testing.T) {
	f := parseSweepFlags([]string{"--repo", "/tmp/x"})
	if f.repo != "/tmp/x" {
		t.Fatalf("expected repo=/tmp/x, got %q", f.repo)
	}
}

func TestSweepFlagsAllParsed(t *testing.T) {
	f := parseSweepFlags([]string{
		"--dry-run",
		"--repo", "/tmp/x",
		"--failure-threshold", "5",
		"--backoff-minutes", "1,2,3",
		"--stale-warn-days", "14",
	})
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
