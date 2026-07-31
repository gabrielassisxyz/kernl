package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// AgentAttemptStats aggregates the stage-attempt ledger's rows for one agent
// over whatever stage/time-window filter the caller applied. Every rate and
// median field is nil, not zero, whenever no row in the group carried the
// underlying measurement - a zero would be indistinguishable from "this
// agent scored zero", which is exactly the fiction the ledger's write side
// was built to avoid (see StageAttemptRecord's field comments). The paired
// *Observations field says how many rows fed the number, so a rate computed
// over one attempt is never confused with one computed over a hundred.
type AgentAttemptStats struct {
	AgentID  string
	Attempts int

	// FirstPassGateRate is the share of this agent's attemptNumber==1 rows
	// whose exit gate passed. GatePassed is never null on a row, so this is
	// nil only when the group has no attemptNumber==1 rows at all (e.g. a
	// --since window that excludes every first attempt but keeps a retry).
	FirstPassGateRate         *float64
	FirstPassGateObservations int

	// ReviewRejectionRate is the share of rows carrying a reviewVerdict that
	// is not "PASS". reviewVerdict is null on any row from a non-review
	// stage, or when the verdict artifact could not be read - those rows are
	// excluded from both the numerator and the denominator, not counted as
	// passes.
	ReviewRejectionRate         *float64
	ReviewRejectionObservations int

	// MedianDurationMs is the median of durationMs, which the ledger always
	// records as a real elapsed measurement - this is nil only when the
	// group itself is empty.
	MedianDurationMs     *float64
	DurationObservations int

	// MedianDiffLines is the median of diffLinesAdded+diffLinesRemoved, over
	// only the rows where both are non-null (a stage that produced no commit
	// reports neither, and a partial pair is not a diff size).
	MedianDiffLines  *float64
	DiffObservations int
}

// AttemptLedgerPaths lists every stage-attempt ledger file across every epic
// under stateDir. An empty, non-nil result (no error) means literally no
// ledger exists yet - the orchestrator has never recorded an attempt - which
// callers report as "nothing recorded yet", not as a failure.
func AttemptLedgerPaths(stateDir string) ([]string, error) {
	pattern := filepath.Join(stateDir, "run", "*", "attempts.jsonl")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: listing attempt ledgers matching %s: %w", pattern, err)
	}
	return paths, nil
}

// AggregateAttemptStats answers "which agent should run this stage" from the
// stage-attempt ledger: every epic's attempts.jsonl under stateDir, grouped
// by AgentID, optionally narrowed to one stage and to rows started at or
// after since (a zero since applies no time filter). Results are sorted by
// AgentID for a deterministic report.
//
// It reads through parseLedgerBytes - the same parser AppendStageAttempt
// uses to derive AttemptNumber - rather than a second implementation that
// could disagree with the writer about what a valid row is.
func AggregateAttemptStats(stateDir, stage string, since time.Time) ([]AgentAttemptStats, error) {
	paths, err := AttemptLedgerPaths(stateDir)
	if err != nil {
		return nil, err
	}

	records, err := readAttemptLedgers(paths)
	if err != nil {
		return nil, err
	}

	byAgent := map[string][]StageAttemptRecord{}
	for _, rec := range records {
		if stage != "" && rec.Stage != stage {
			continue
		}
		if !since.IsZero() && rec.StartedAt.Before(since) {
			continue
		}
		byAgent[rec.AgentID] = append(byAgent[rec.AgentID], rec)
	}

	agentIDs := make([]string, 0, len(byAgent))
	for id := range byAgent {
		agentIDs = append(agentIDs, id)
	}
	sort.Strings(agentIDs)

	stats := make([]AgentAttemptStats, 0, len(agentIDs))
	for _, id := range agentIDs {
		stats = append(stats, computeAgentAttemptStats(id, byAgent[id]))
	}
	return stats, nil
}

// readAttemptLedgers reads and parses every ledger file named by paths. A
// file that cannot be read, or whose content parseLedgerBytes rejects as
// corrupt, halts the whole read and names the offending file - an
// aggregation over a ledger it could not fully read would silently
// undercount an agent's attempts.
func readAttemptLedgers(paths []string) ([]StageAttemptRecord, error) {
	var all []StageAttemptRecord
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("KERNL DISPATCH FAILURE: reading attempt ledger %s: %w", path, err)
		}
		records, _, err := parseLedgerBytes(path, data)
		if err != nil {
			return nil, err
		}
		all = append(all, records...)
	}
	return all, nil
}

func computeAgentAttemptStats(agentID string, recs []StageAttemptRecord) AgentAttemptStats {
	stats := AgentAttemptStats{AgentID: agentID, Attempts: len(recs)}

	var firstPassTotal, firstPassPassed int
	var reviewTotal, reviewRejected int
	durations := make([]float64, 0, len(recs))
	diffs := make([]float64, 0, len(recs))

	for _, r := range recs {
		if r.AttemptNumber == 1 {
			firstPassTotal++
			if r.GatePassed {
				firstPassPassed++
			}
		}
		if r.ReviewVerdict != nil {
			reviewTotal++
			if *r.ReviewVerdict != "PASS" {
				reviewRejected++
			}
		}
		durations = append(durations, float64(r.DurationMs))
		if r.DiffLinesAdded != nil && r.DiffLinesRemoved != nil {
			diffs = append(diffs, float64(*r.DiffLinesAdded+*r.DiffLinesRemoved))
		}
	}

	stats.FirstPassGateObservations = firstPassTotal
	if firstPassTotal > 0 {
		rate := float64(firstPassPassed) / float64(firstPassTotal)
		stats.FirstPassGateRate = &rate
	}

	stats.ReviewRejectionObservations = reviewTotal
	if reviewTotal > 0 {
		rate := float64(reviewRejected) / float64(reviewTotal)
		stats.ReviewRejectionRate = &rate
	}

	stats.DurationObservations = len(durations)
	if med, ok := median(durations); ok {
		stats.MedianDurationMs = &med
	}

	stats.DiffObservations = len(diffs)
	if med, ok := median(diffs); ok {
		stats.MedianDiffLines = &med
	}

	return stats
}

// median returns the midpoint of a sorted copy of values (the average of the
// two middle values for an even count), and false when values is empty - the
// caller must be able to tell "no observations" apart from "the observations
// median to zero".
func median(values []float64) (float64, bool) {
	if len(values) == 0 {
		return 0, false
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid], true
	}
	return (sorted[mid-1] + sorted[mid]) / 2, true
}
