package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeAttemptFixture appends one attempt to stateDir/run/<epic>/attempts.jsonl
// through the real writer (BuildStageAttemptRecord + AppendStageAttempt)
// rather than hand-typed JSON, so a fixture can never drift from the shape
// the writer actually produces.
func writeAttemptFixture(t *testing.T, stateDir, epic string, in StageAttemptInput) {
	t.Helper()
	if in.DiffStats == nil {
		in.DiffStats = fakeDiffStatter{}
	}
	rec := BuildStageAttemptRecord(in)
	if err := AppendStageAttempt(stateDir, epic, rec); err != nil {
		t.Fatalf("writeAttemptFixture: %v", err)
	}
}

func TestAggregateAttemptStats_NoLedgerFilesIsNotAnError(t *testing.T) {
	stateDir := t.TempDir()

	stats, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("expected no error for an empty state dir, got: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("expected no agent stats, got %+v", stats)
	}

	paths, err := AttemptLedgerPaths(stateDir)
	if err != nil {
		t.Fatalf("AttemptLedgerPaths: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected zero ledger paths, got %v", paths)
	}
}

func TestAggregateAttemptStats_AggregatesAcrossMultipleEpics(t *testing.T) {
	stateDir := t.TempDir()
	now := time.Now()

	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: now, Duration: 10 * time.Second, GatePassed: true,
	})
	writeAttemptFixture(t, stateDir, "epic-b", StageAttemptInput{
		AgentID: "claude", BeadID: "bead-2", Stage: "implementation",
		StartedAt: now, Duration: 20 * time.Second, GatePassed: true,
	})

	stats, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected one agent aggregated across both epics, got %d: %+v", len(stats), stats)
	}
	if stats[0].Attempts != 2 {
		t.Errorf("expected 2 attempts pooled from epic-a and epic-b, got %d", stats[0].Attempts)
	}
}

func TestAggregateAttemptStats_StageFilter(t *testing.T) {
	stateDir := t.TempDir()
	now := time.Now()

	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation", StartedAt: now, GatePassed: true,
	})
	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "review", StartedAt: now, GatePassed: true,
	})

	stats, err := AggregateAttemptStats(stateDir, "implementation", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}
	if len(stats) != 1 || stats[0].Attempts != 1 {
		t.Fatalf("expected exactly one implementation-stage attempt, got %+v", stats)
	}
}

func TestAggregateAttemptStats_SinceFilter(t *testing.T) {
	stateDir := t.TempDir()
	now := time.Now()

	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation", StartedAt: now.Add(-48 * time.Hour), GatePassed: true,
	})
	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "claude", BeadID: "bead-2", Stage: "implementation", StartedAt: now.Add(-1 * time.Hour), GatePassed: true,
	})

	cutoff := now.Add(-24 * time.Hour)
	stats, err := AggregateAttemptStats(stateDir, "", cutoff)
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}
	if len(stats) != 1 || stats[0].Attempts != 1 {
		t.Fatalf("expected the --since cutoff to drop the 48h-old row, got %+v", stats)
	}
}

func TestAggregateAttemptStats_MedianOverOddAndEvenCounts(t *testing.T) {
	stateDir := t.TempDir()
	now := time.Now()

	// Odd count (3): durations 10s, 20s, 30s -> median 20s.
	for _, d := range []time.Duration{10 * time.Second, 30 * time.Second, 20 * time.Second} {
		writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
			AgentID: "odd-agent", BeadID: "bead-odd", Stage: "implementation", StartedAt: now, Duration: d, GatePassed: true,
		})
	}
	// Even count (4): durations 10s, 20s, 30s, 40s -> median (20+30)/2 = 25s.
	for _, d := range []time.Duration{10 * time.Second, 40 * time.Second, 20 * time.Second, 30 * time.Second} {
		writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
			AgentID: "even-agent", BeadID: "bead-even", Stage: "implementation", StartedAt: now, Duration: d, GatePassed: true,
		})
	}

	stats, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}

	var odd, even *AgentAttemptStats
	for i := range stats {
		switch stats[i].AgentID {
		case "odd-agent":
			odd = &stats[i]
		case "even-agent":
			even = &stats[i]
		}
	}
	if odd == nil || even == nil {
		t.Fatalf("expected both agents in the result, got %+v", stats)
	}
	if odd.MedianDurationMs == nil || *odd.MedianDurationMs != 20000 {
		t.Errorf("odd-count median duration = %v, want 20000ms", odd.MedianDurationMs)
	}
	if even.MedianDurationMs == nil || *even.MedianDurationMs != 25000 {
		t.Errorf("even-count median duration = %v, want 25000ms", even.MedianDurationMs)
	}
}

func TestAggregateAttemptStats_UnmeasuredGroupReportsNilNotZero(t *testing.T) {
	stateDir := t.TempDir()
	now := time.Now()

	// A stage with no diff stats and no review verdict at all: DiffLinesAdded/
	// Removed stay nil (fakeDiffStatter here returns nil, nil), ReviewVerdict
	// stays nil - nothing to measure for either field on any row.
	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation", StartedAt: now, GatePassed: true,
	})

	stats, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected one agent, got %+v", stats)
	}
	s := stats[0]
	if s.MedianDiffLines != nil {
		t.Errorf("MedianDiffLines must be nil when no row carried a diff size, got %v", *s.MedianDiffLines)
	}
	if s.DiffObservations != 0 {
		t.Errorf("DiffObservations must be 0, got %d", s.DiffObservations)
	}
	if s.ReviewRejectionRate != nil {
		t.Errorf("ReviewRejectionRate must be nil when no row carried a verdict, got %v", *s.ReviewRejectionRate)
	}
	if s.ReviewRejectionObservations != 0 {
		t.Errorf("ReviewRejectionObservations must be 0, got %d", s.ReviewRejectionObservations)
	}
}

func TestAggregateAttemptStats_CorruptLedgerHaltsAndNamesTheFile(t *testing.T) {
	stateDir := t.TempDir()
	epicDir := filepath.Join(stateDir, "run", "epic-a")
	if err := os.MkdirAll(epicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ledgerPath := filepath.Join(epicDir, "attempts.jsonl")
	// A malformed row NOT at the end of the file - parseLedgerBytes treats
	// only the trailing, newline-less segment as a possibly-interrupted
	// write; anything earlier is a real corruption.
	if err := os.WriteFile(ledgerPath, []byte("{not valid json}\n{\"agentId\":\"claude\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err == nil {
		t.Fatal("expected a corrupt ledger to halt aggregation")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error must carry the marker, got: %v", err)
	}
	if !strings.Contains(err.Error(), ledgerPath) {
		t.Errorf("error must name the offending file %q, got: %v", ledgerPath, err)
	}
}

func TestAggregateAttemptStats_UnreadableLedgerHaltsAndNamesTheFile(t *testing.T) {
	stateDir := t.TempDir()
	epicDir := filepath.Join(stateDir, "run", "epic-a")
	if err := os.MkdirAll(epicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ledgerPath := filepath.Join(epicDir, "attempts.jsonl")
	if err := os.WriteFile(ledgerPath, []byte(`{"agentId":"claude"}`+"\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(ledgerPath, 0o644); err != nil {
			t.Errorf("restoring ledger permissions: %v", err)
		}
	})

	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}

	_, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err == nil {
		t.Fatal("expected an unreadable ledger to halt aggregation")
	}
	if !strings.Contains(err.Error(), ledgerPath) {
		t.Errorf("error must name the offending file %q, got: %v", ledgerPath, err)
	}
}

func TestAggregateAttemptStats_SortedByAgentID(t *testing.T) {
	stateDir := t.TempDir()
	now := time.Now()

	for _, agent := range []string{"zeta", "alpha", "mid"} {
		writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
			AgentID: agent, BeadID: "bead-" + agent, Stage: "implementation", StartedAt: now, GatePassed: true,
		})
	}

	stats, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}
	if len(stats) != 3 {
		t.Fatalf("expected 3 agents, got %+v", stats)
	}
	if stats[0].AgentID != "alpha" || stats[1].AgentID != "mid" || stats[2].AgentID != "zeta" {
		t.Errorf("expected alphabetical order, got %s, %s, %s", stats[0].AgentID, stats[1].AgentID, stats[2].AgentID)
	}
}
