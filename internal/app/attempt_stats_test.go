package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/session"
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

	result, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("expected no error for an empty state dir, got: %v", err)
	}
	if len(result.Agents) != 0 {
		t.Errorf("expected no agent stats, got %+v", result.Agents)
	}
	if len(result.DanglingLedgers) != 0 {
		t.Errorf("expected no dangling ledgers, got %v", result.DanglingLedgers)
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

	result, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}
	if len(result.Agents) != 1 {
		t.Fatalf("expected one agent aggregated across both epics, got %d: %+v", len(result.Agents), result.Agents)
	}
	if result.Agents[0].Attempts != 2 {
		t.Errorf("expected 2 attempts pooled from epic-a and epic-b, got %d", result.Agents[0].Attempts)
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

	result, err := AggregateAttemptStats(stateDir, "implementation", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}
	if len(result.Agents) != 1 || result.Agents[0].Attempts != 1 {
		t.Fatalf("expected exactly one implementation-stage attempt, got %+v", result.Agents)
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
	result, err := AggregateAttemptStats(stateDir, "", cutoff)
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}
	if len(result.Agents) != 1 || result.Agents[0].Attempts != 1 {
		t.Fatalf("expected the --since cutoff to drop the 48h-old row, got %+v", result.Agents)
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

	result, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}

	var odd, even *AgentAttemptStats
	for i := range result.Agents {
		switch result.Agents[i].AgentID {
		case "odd-agent":
			odd = &result.Agents[i]
		case "even-agent":
			even = &result.Agents[i]
		}
	}
	if odd == nil || even == nil {
		t.Fatalf("expected both agents in the result, got %+v", result.Agents)
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

	result, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}
	if len(result.Agents) != 1 {
		t.Fatalf("expected one agent, got %+v", result.Agents)
	}
	s := result.Agents[0]
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

	result, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}
	if len(result.Agents) != 3 {
		t.Fatalf("expected 3 agents, got %+v", result.Agents)
	}
	if result.Agents[0].AgentID != "alpha" || result.Agents[1].AgentID != "mid" || result.Agents[2].AgentID != "zeta" {
		t.Errorf("expected alphabetical order, got %s, %s, %s", result.Agents[0].AgentID, result.Agents[1].AgentID, result.Agents[2].AgentID)
	}
}

// --- second-review findings: dangling trailing row, unreadable run dir ---

func TestAggregateAttemptStats_DanglingTrailingRowIsSurfacedNotDropped(t *testing.T) {
	stateDir := t.TempDir()
	now := time.Now()

	// A real, complete attempt through the writer, then a hand-appended
	// trailing row with no newline - the exact state a process killed
	// between the write syscall and appendStageAttempt's truncate-back
	// leaves behind, which flock+Truncate is built to recover from on the
	// next real append, not something this reader should treat as corrupt.
	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation", StartedAt: now, GatePassed: true,
	})
	ledgerPath := filepath.Join(stateDir, "run", "epic-a", "attempts.jsonl")
	f, err := os.OpenFile(ledgerPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"agentId":"claude","beadId":"bead-2","stage":"implementation"}`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("a dangling trailing row must not halt aggregation, got: %v", err)
	}
	if len(result.Agents) != 1 || result.Agents[0].Attempts != 1 {
		t.Fatalf("expected the dangling row excluded from the count (1 complete attempt), got %+v", result.Agents)
	}
	if len(result.DanglingLedgers) != 1 || result.DanglingLedgers[0] != ledgerPath {
		t.Errorf("expected the ledger to be reported as dangling, got %v", result.DanglingLedgers)
	}
}

func TestAggregateAttemptStats_NoDanglingLedgersWhenEveryRowIsComplete(t *testing.T) {
	stateDir := t.TempDir()
	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation", StartedAt: time.Now(), GatePassed: true,
	})

	result, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}
	if len(result.DanglingLedgers) != 0 {
		t.Errorf("a ledger with only complete rows must not be reported dangling, got %v", result.DanglingLedgers)
	}
}

func TestAttemptLedgerPaths_UnreadableRunDirHaltsRatherThanReportingEmpty(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}

	stateDir := t.TempDir()
	runDir := filepath.Join(stateDir, "run")
	if err := os.MkdirAll(filepath.Join(runDir, "epic-a"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation", StartedAt: time.Now(), GatePassed: true,
	})
	if err := os.Chmod(runDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(runDir, 0o755); err != nil {
			t.Errorf("restoring run dir permissions: %v", err)
		}
	})

	_, err := AttemptLedgerPaths(stateDir)
	if err == nil {
		t.Fatal("expected an unreadable run directory to be a loud failure, not an empty result")
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error must carry the marker, got: %v", err)
	}
	if !strings.Contains(err.Error(), runDir) {
		t.Errorf("error must name the run directory %q, got: %v", runDir, err)
	}
}

func TestAttemptLedgerPaths_MissingRunDirIsGenuinelyEmpty(t *testing.T) {
	stateDir := t.TempDir() // <stateDir>/run never created

	paths, err := AttemptLedgerPaths(stateDir)
	if err != nil {
		t.Fatalf("a run directory that was never created is not an error, got: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected no ledger paths, got %v", paths)
	}
}

// --- rework: attempts that exist because a reviewer rejected the previous one ---

// rejectingReview writes the review row whose deliberate rejection makes the
// NEXT attempt for that bead rework. It is the shape backend's artifact_verdict
// gate produces for a "VERDICT: REJECT" at a state that has somewhere to send
// the work back to.
func rejectingReview(t *testing.T, stateDir, epic, bead string, at time.Time) {
	t.Helper()
	verdict := "REJECT"
	writeAttemptFixture(t, stateDir, epic, StageAttemptInput{
		AgentID: "reviewer", BeadID: bead, Stage: "implementation_review",
		StartedAt: at, Duration: time.Second, GatePassed: false,
		GateFailureReason: "verdict_reject: /artifacts/" + bead + "/implementation-review.md",
		ReviewVerdict:     &verdict,
	})
}

func agentByID(t *testing.T, result AttemptStatsResult, id string) AgentAttemptStats {
	t.Helper()
	for _, a := range result.Agents {
		if a.AgentID == id {
			return a
		}
	}
	t.Fatalf("no stats for agent %q in %+v", id, result.Agents)
	return AgentAttemptStats{}
}

func TestAggregateAttemptStats_ReworkIsChargedToWhoeverRedidTheWork(t *testing.T) {
	stateDir := t.TempDir()
	now := time.Now()
	cost := 0.25
	usage := func() *session.TokenUsageCounts {
		c := cost
		return &session.TokenUsageCounts{OutputTokens: 1000, CostUSD: &c}
	}

	// bead-1: implementation, rejected, redone (the redo passes).
	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: now, Duration: 10 * time.Second, GatePassed: true, Usage: usage(),
	})
	rejectingReview(t, stateDir, "epic-a", "bead-1", now.Add(time.Minute))
	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: now.Add(2 * time.Minute), Duration: 30 * time.Second, GatePassed: true, Usage: usage(),
	})

	result, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}

	claude := agentByID(t, result, "claude")
	if claude.ReworkAttempts != 1 {
		t.Errorf("ReworkAttempts = %d, want 1", claude.ReworkAttempts)
	}
	if claude.ReworkRate == nil || *claude.ReworkRate != 0.5 {
		t.Errorf("ReworkRate = %v, want 0.5 - one of claude's two attempts was a redo", claude.ReworkRate)
	}
	if claude.ReworkGatePassRate == nil || *claude.ReworkGatePassRate != 1 {
		t.Errorf("ReworkGatePassRate = %v, want 1 - the redo passed its gate", claude.ReworkGatePassRate)
	}
	if claude.ReworkDurationMs != (30 * time.Second).Milliseconds() {
		t.Errorf("ReworkDurationMs = %d, want only the redo's own 30s", claude.ReworkDurationMs)
	}
	if claude.ReworkOutputTokens == nil || *claude.ReworkOutputTokens != 1000 {
		t.Errorf("ReworkOutputTokens = %v, want 1000 - the first attempt's tokens are not rework", claude.ReworkOutputTokens)
	}
	if claude.ReworkCostUSD == nil || *claude.ReworkCostUSD != cost {
		t.Errorf("ReworkCostUSD = %v, want %v", claude.ReworkCostUSD, cost)
	}

	// The reviewer rejected; it did not redo anything. Charging the rework to
	// it would double-count the same rejection - it already carries it as a
	// review verdict.
	reviewer := agentByID(t, result, "reviewer")
	if reviewer.ReworkAttempts != 0 {
		t.Errorf("reviewer ReworkAttempts = %d, want 0 - rejecting is not redoing", reviewer.ReworkAttempts)
	}
	if reviewer.ReviewRejectionRate == nil || *reviewer.ReviewRejectionRate != 1 {
		t.Errorf("reviewer ReviewRejectionRate = %v, want 1 - its own rejection still counts there", reviewer.ReviewRejectionRate)
	}
}

func TestAggregateAttemptStats_ReworkThatFailedAgainIsNotRecovered(t *testing.T) {
	stateDir := t.TempDir()
	now := time.Now()

	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: now, Duration: time.Second, GatePassed: true,
	})
	rejectingReview(t, stateDir, "epic-a", "bead-1", now.Add(time.Minute))
	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: now.Add(2 * time.Minute), Duration: time.Second, GatePassed: false,
		GateFailureReason: "commit_marker_missing: implementation",
	})

	result, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}
	claude := agentByID(t, result, "claude")
	if claude.ReworkGatePassRate == nil || *claude.ReworkGatePassRate != 0 {
		t.Errorf("ReworkGatePassRate = %v, want 0 - the redo failed its own gate", claude.ReworkGatePassRate)
	}
	if claude.ReworkGateObservations != 1 {
		t.Errorf("ReworkGateObservations = %d, want 1", claude.ReworkGateObservations)
	}
}

// A dialect that reports no usage (codex reports neither model nor cost)
// leaves the rework sums nil rather than zero: rework nobody priced must not
// read as rework that was free.
func TestAggregateAttemptStats_ReworkCostIsNilWhenNothingReportedIt(t *testing.T) {
	stateDir := t.TempDir()
	now := time.Now()

	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "codex", BeadID: "bead-1", Stage: "implementation",
		StartedAt: now, Duration: time.Second, GatePassed: true,
	})
	rejectingReview(t, stateDir, "epic-a", "bead-1", now.Add(time.Minute))
	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "codex", BeadID: "bead-1", Stage: "implementation",
		StartedAt: now.Add(2 * time.Minute), Duration: 5 * time.Second, GatePassed: true,
	})

	result, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}
	codex := agentByID(t, result, "codex")
	if codex.ReworkAttempts != 1 {
		t.Fatalf("ReworkAttempts = %d, want 1", codex.ReworkAttempts)
	}
	if codex.ReworkCostUSD != nil {
		t.Errorf("ReworkCostUSD = %v, want nil - codex reports no cost", *codex.ReworkCostUSD)
	}
	if codex.ReworkOutputTokens != nil {
		t.Errorf("ReworkOutputTokens = %v, want nil", *codex.ReworkOutputTokens)
	}
	if codex.ReworkCostObservations != 0 || codex.ReworkTokenObservations != 0 {
		t.Errorf("observation counts must say the sums rest on nothing, got cost=%d tokens=%d", codex.ReworkCostObservations, codex.ReworkTokenObservations)
	}
	// The duration is always measured, even when the dialect reports no usage.
	if codex.ReworkDurationMs != (5 * time.Second).Milliseconds() {
		t.Errorf("ReworkDurationMs = %d, want the redo's real elapsed time", codex.ReworkDurationMs)
	}
}

// A run with no rejection has no rework, and the rate must say "none of what
// this agent did was a redo" rather than being absent.
func TestAggregateAttemptStats_NoRejectionMeansZeroReworkNotUnmeasured(t *testing.T) {
	stateDir := t.TempDir()
	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, GatePassed: true,
	})

	result, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}
	claude := agentByID(t, result, "claude")
	if claude.ReworkRate == nil || *claude.ReworkRate != 0 {
		t.Errorf("ReworkRate = %v, want 0", claude.ReworkRate)
	}
	if claude.ReworkGatePassRate != nil {
		t.Errorf("ReworkGatePassRate = %v, want nil - there was no redo to recover", *claude.ReworkGatePassRate)
	}
}

// --- stopped beads: the cost that leaves a person's day rather than a quota ---

func TestAggregateAttemptStats_AStoppedBeadIsChargedToWhoeverProducedTheWork(t *testing.T) {
	stateDir := t.TempDir()
	now := time.Now()
	verdict := "REJECT"

	// bead-1: the implementer works, the reviewer rejects, and nothing runs
	// after - the bead is sitting there waiting for a person.
	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "pi-kimi", BeadID: "bead-1", Stage: "implementation",
		StartedAt: now, Duration: time.Second, GatePassed: true,
	})
	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "codex", BeadID: "bead-1", Stage: "implementation_review",
		StartedAt: now.Add(time.Minute), Duration: time.Second, GatePassed: false,
		GateFailureReason: "verdict_reject: /artifacts/implementation-review.md",
		ReviewVerdict:     &verdict,
	})

	result, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}

	kimi := agentByID(t, result, "pi-kimi")
	if kimi.BeadsStopped != 1 {
		t.Errorf("BeadsStopped = %d, want 1 - the work that was not accepted is the implementer's", kimi.BeadsStopped)
	}
	if kimi.StoppedByReason["verdict_reject"] != 1 {
		t.Errorf("StoppedByReason = %v, want one verdict_reject", kimi.StoppedByReason)
	}
	if kimi.BeadsImplemented != 1 || kimi.BeadsStoppedRate == nil || *kimi.BeadsStoppedRate != 1 {
		t.Errorf("rate = %v over %d beads, want 1/1", kimi.BeadsStoppedRate, kimi.BeadsImplemented)
	}

	codex := agentByID(t, result, "codex")
	if codex.BeadsStopped != 0 {
		t.Errorf("codex BeadsStopped = %d, want 0 - it judged the work, it did not produce it", codex.BeadsStopped)
	}
	// A pure reviewer has no denominator, so it has no rate - not a zero,
	// which would read as "it worked beads and never stopped one".
	if codex.BeadsImplemented != 0 || codex.BeadsStoppedRate != nil {
		t.Errorf("codex rate = %v over %d beads, want no rate at all", codex.BeadsStoppedRate, codex.BeadsImplemented)
	}
}

// A stop at a stage that is not implementation still belongs to the agent
// that ran it, and its denominator has to include that bead - otherwise the
// numerator is drawn from a population the denominator does not contain, and
// an agent that only ever integrated reads as one stop out of zero beads.
func TestAggregateAttemptStats_TheDenominatorCoversEveryStageThatProducesWork(t *testing.T) {
	stateDir := t.TempDir()
	now := time.Now()

	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "claude", BeadID: "epic-a", Stage: "integration",
		StartedAt: now, Duration: time.Second, GatePassed: false,
		GateFailureReason: "commit_marker_missing: integration",
	})

	result, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}
	claude := agentByID(t, result, "claude")
	if claude.BeadsImplemented != 1 {
		t.Errorf("BeadsImplemented = %d, want 1 - integration produces work too", claude.BeadsImplemented)
	}
	if claude.StoppedByReason["commit_marker_missing"] != 1 {
		t.Errorf("StoppedByReason = %v, want one commit_marker_missing", claude.StoppedByReason)
	}
}

// A bead that ran again is not stopped any more. The number is a snapshot of
// what is sitting there now, not a count of every time something failed.
func TestAggregateAttemptStats_ABeadThatRanAgainIsNoLongerStopped(t *testing.T) {
	stateDir := t.TempDir()
	now := time.Now()

	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: now, Duration: time.Second, GatePassed: false,
		GateFailureReason: "commit_marker_missing: implementation",
	})
	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: now.Add(time.Minute), Duration: time.Second, GatePassed: true,
	})

	result, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}
	claude := agentByID(t, result, "claude")
	if claude.BeadsStopped != 0 {
		t.Errorf("BeadsStopped = %d, want 0 - the bead moved on", claude.BeadsStopped)
	}
	if claude.BeadsStoppedRate == nil || *claude.BeadsStoppedRate != 0 {
		t.Errorf("rate = %v, want 0 over one bead worked", claude.BeadsStoppedRate)
	}
}

// The breakdown separates two failures with different fixes: work that was
// judged and sent back, and work that never produced a commit at all. A total
// that merges them reads the second as evidence about the first.
func TestAggregateAttemptStats_StopsAreBrokenDownByWhatFailed(t *testing.T) {
	stateDir := t.TempDir()
	now := time.Now()

	for i, bead := range []string{"bead-1", "bead-2"} {
		writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
			AgentID: "pi-kimi", BeadID: bead, Stage: "implementation",
			StartedAt: now.Add(time.Duration(i) * time.Minute), Duration: time.Second,
			GatePassed: false, GateFailureReason: "commit_marker_missing: implementation",
		})
	}
	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "pi-kimi", BeadID: "bead-3", Stage: "implementation",
		StartedAt: now.Add(2 * time.Minute), Duration: time.Second, GatePassed: true,
	})
	verdict := "REJECT"
	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "codex", BeadID: "bead-3", Stage: "implementation_review",
		StartedAt: now.Add(3 * time.Minute), Duration: time.Second, GatePassed: false,
		GateFailureReason: "verdict_reject: /artifacts/implementation-review.md",
		ReviewVerdict:     &verdict,
	})

	result, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}
	kimi := agentByID(t, result, "pi-kimi")
	if kimi.BeadsStopped != 3 {
		t.Fatalf("BeadsStopped = %d, want 3", kimi.BeadsStopped)
	}
	if kimi.StoppedByReason["commit_marker_missing"] != 2 || kimi.StoppedByReason["verdict_reject"] != 1 {
		t.Errorf("StoppedByReason = %v, want 2 commit_marker_missing and 1 verdict_reject", kimi.StoppedByReason)
	}
}

// A failed attempt that recorded no reason is a gap in the record, not a
// category of failure - and must not be silently folded into a real one.
func TestAggregateAttemptStats_AStopWithNoRecordedReasonIsNamedUnknown(t *testing.T) {
	stateDir := t.TempDir()
	writeAttemptFixture(t, stateDir, "epic-a", StageAttemptInput{
		AgentID: "claude", BeadID: "bead-1", Stage: "implementation",
		StartedAt: time.Now(), Duration: time.Second, GatePassed: false,
	})

	result, err := AggregateAttemptStats(stateDir, "", time.Time{})
	if err != nil {
		t.Fatalf("AggregateAttemptStats: %v", err)
	}
	if got := agentByID(t, result, "claude").StoppedByReason; got["unknown"] != 1 {
		t.Errorf("StoppedByReason = %v, want one unknown", got)
	}
}
