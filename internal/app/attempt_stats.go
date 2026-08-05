package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

	// ReworkAttempts counts this agent's rows carrying causedBy: attempts
	// redoing work a reviewer rejected. Not every attempt that follows a
	// rejection qualifies - see findCausedBy, which rules out a review
	// re-running on unchanged work and a stage running for the first time,
	// neither of which is redoing anything.
	//
	// It is charged to the agent that REDID the work, not to the reviewer
	// that rejected it. The reviewer's own behaviour is already
	// ReviewRejectionRate above, computed from its verdict rows; asking one
	// number to carry both would make neither answerable.
	ReworkAttempts int
	// ReworkRate is ReworkAttempts over Attempts - the share of what this
	// agent did that was redoing work. Nil only when the agent has no rows
	// at all, which AggregateAttemptStats never produces (an agent is
	// listed because it has some).
	ReworkRate *float64

	// ReworkGatePassRate is the share of the rework attempts whose own exit
	// gate passed: how often the second try actually landed. Nil when there
	// was no rework to observe, which is not the same as rework that never
	// recovered.
	ReworkGatePassRate     *float64
	ReworkGateObservations int

	// ReworkDurationMs is the wall-clock the rework attempts cost, summed.
	// Always measured (the ledger records a real elapsed duration on every
	// row), so this is 0 only when there was no rework.
	ReworkDurationMs int64

	// ReworkOutputTokens and ReworkCostUSD are what the rework cost where
	// the dialect reported it, and nil - never 0 - where none of the rework
	// rows reported anything. codex reports neither, so an epic implemented
	// by codex has real rework with no price on it, and a zero here would
	// read as "rework was free" rather than "nobody measured it". The
	// observation counts say how many rows each sum is actually built from.
	ReworkOutputTokens      *int64
	ReworkTokenObservations int
	ReworkCostUSD           *float64
	ReworkCostObservations  int
	ReworkCostIsCeiling     bool

	// BeadsStopped counts the beads whose last recorded attempt failed its
	// gate: the run got no further and someone has to look. It is the other
	// half of what a rejection costs, and the half that is not paid in quota
	// - rework is the agent fixing its own work, this is a person's day.
	//
	// Charged like rework, to whoever produced the work rather than to
	// whoever judged it: a review stage that fails is the reviewer's row, so
	// the stop goes to the implementer of the attempt it was judging.
	BeadsStopped int
	// BeadsImplemented is the denominator: distinct beads this agent
	// produced work on (any stage that is not a review). A count of stops
	// with no denominator is not comparable between agents - eight stops out
	// of sixteen beads and one out of one are not the same fact - and the
	// two populations must match, which is why this counts beads the agent
	// worked rather than beads it merely touched: a stop can only ever be
	// charged to an agent that produced something.
	BeadsImplemented int
	// BeadsStoppedRate is BeadsStopped over BeadsImplemented, nil when the
	// agent produced work on nothing (a pure reviewer).
	BeadsStoppedRate *float64
	// StoppedByReason breaks the stops down by the gate that failed, keyed
	// by the reason's prefix (commit_marker_missing, verdict_reject,
	// fork_gate, ...). It is the result rather than a detail: "stopped
	// because a reviewer rejected it twice" and "stopped without ever
	// producing a commit" are different failures with different fixes, and a
	// single total hides which one an agent actually has.
	StoppedByReason map[string]int
}

// AttemptLedgerPaths lists every stage-attempt ledger file across every epic
// under stateDir. A missing <stateDir>/run is the genuine "nothing recorded
// yet" case: an empty, non-nil result with no error, since the orchestrator
// has never recorded an attempt.
//
// Any other failure to read <stateDir>/run, or to reach a ledger inside one
// of its epic subdirectories (a chmod'd directory, a permission error deeper
// in the tree), halts loudly instead of being folded into that same "empty"
// result - filepath.Glob's own contract silently treats an unreadable
// directory as "no match", which would make the reader unable to tell "I
// looked and found nothing" from "I could not look," and report the wrong
// one as success.
func AttemptLedgerPaths(stateDir string) ([]string, error) {
	runRoot := filepath.Join(stateDir, "run")
	entries, err := os.ReadDir(runRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: listing attempt ledger directory %s: %w - Fix: check permissions on %s", runRoot, err, runRoot)
	}

	var paths []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		ledgerPath := filepath.Join(runRoot, entry.Name(), "attempts.jsonl")
		if _, err := os.Stat(ledgerPath); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("KERNL DISPATCH FAILURE: checking attempt ledger %s: %w - Fix: check permissions on %s", ledgerPath, err, filepath.Dir(ledgerPath))
		}
		paths = append(paths, ledgerPath)
	}
	sort.Strings(paths)
	return paths, nil
}

// AttemptStatsResult is AggregateAttemptStats's answer: the per-agent
// numbers, plus which ledger files (if any) had their most recent row
// dropped because it was still mid-write. DanglingLedgers is not a failure -
// parseLedgerBytes already treats an unterminated trailing line as a write
// the writer's own flock+Truncate protocol is built to recover from, not
// corruption - but it does mean one attempt is missing from the count below
// it, and a caller that never surfaces that fact would report a clean number
// that quietly undercounts.
type AttemptStatsResult struct {
	Agents          []AgentAttemptStats
	DanglingLedgers []string
}

// AggregateAttemptStats answers "which agent should run this stage" from the
// stage-attempt ledger: every epic's attempts.jsonl under stateDir, grouped
// by AgentID, optionally narrowed to one stage and to rows started at or
// after since (a zero since applies no time filter). Agents are sorted by
// AgentID for a deterministic report.
//
// It reads through parseLedgerBytes - the same parser AppendStageAttempt
// uses to derive AttemptNumber - rather than a second implementation that
// could disagree with the writer about what a valid row is.
func AggregateAttemptStats(stateDir, stage string, since time.Time) (AttemptStatsResult, error) {
	paths, err := AttemptLedgerPaths(stateDir)
	if err != nil {
		return AttemptStatsResult{}, err
	}

	records, dangling, err := readAttemptLedgers(paths)
	if err != nil {
		return AttemptStatsResult{}, err
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

	stopped, implemented := computeBeadOutcomes(records, byAgent)

	stats := make([]AgentAttemptStats, 0, len(agentIDs))
	for _, id := range agentIDs {
		agent := computeAgentAttemptStats(id, byAgent[id])
		agent.BeadsImplemented = implemented[id]
		agent.StoppedByReason = stopped[id]
		for _, n := range stopped[id] {
			agent.BeadsStopped += n
		}
		if agent.BeadsImplemented > 0 {
			rate := float64(agent.BeadsStopped) / float64(agent.BeadsImplemented)
			agent.BeadsStoppedRate = &rate
		}
		stats = append(stats, agent)
	}
	return AttemptStatsResult{Agents: stats, DanglingLedgers: dangling}, nil
}

// isReviewStage reports whether a stage judges work rather than producing it.
// The project's workflows name every such stage <thing>_review (plan_review,
// implementation_review, integration_review, shipment_review), and this reads
// that convention rather than listing the four: a custom workflow that follows
// it is classified correctly without being enumerated here.
func isReviewStage(stage string) bool {
	return strings.HasSuffix(stage, "_review")
}

// computeBeadOutcomes answers, per agent, which beads it produced work on and
// which of those ended on a failed gate - the two halves of the stopped rate,
// computed together so the numerator can never be drawn from a population the
// denominator does not contain.
//
// A bead is "stopped" when its LAST recorded attempt failed. That is a
// snapshot, not a lifetime count: a bead that runs again stops counting the
// moment its next row lands. It also cannot see a bead someone repaired by
// hand outside kernl - the ledger records what kernl ran, and a manual rescue
// leaves no row - so this undercounts, and undercounts older runs most. The
// tracker's own bead status is what would close that gap; the ledger cannot.
func computeBeadOutcomes(records []StageAttemptRecord, byAgent map[string][]StageAttemptRecord) (stopped map[string]map[string]int, implemented map[string]int) {
	stopped = map[string]map[string]int{}
	implemented = map[string]int{}

	// Only rows that survived the caller's stage/time filter take part, so a
	// windowed report describes the window rather than quietly reading rows
	// it excluded from every other number.
	kept := map[string]bool{}
	for agentID, rows := range byAgent {
		for range rows {
			kept[agentID] = true
		}
	}

	byBead := map[string][]StageAttemptRecord{}
	order := []string{}
	for _, rec := range records {
		if !kept[rec.AgentID] {
			continue
		}
		key := rec.EpicID + "\x00" + rec.BeadID
		if _, seen := byBead[key]; !seen {
			order = append(order, key)
		}
		byBead[key] = append(byBead[key], rec)
	}

	for _, key := range order {
		rows := byBead[key]
		producers := map[string]bool{}
		for _, r := range rows {
			if !isReviewStage(r.Stage) {
				producers[r.AgentID] = true
			}
		}
		for agentID := range producers {
			implemented[agentID]++
		}

		last := rows[len(rows)-1]
		if last.GatePassed {
			continue
		}
		culprit := last.AgentID
		if isReviewStage(last.Stage) {
			// The row belongs to the reviewer; the bead stopped because the
			// work it was judging was not accepted. Walk back to the attempt
			// that produced that work.
			for i := len(rows) - 2; i >= 0; i-- {
				if rows[i].Stage != last.Stage {
					culprit = rows[i].AgentID
					break
				}
			}
		}
		if stopped[culprit] == nil {
			stopped[culprit] = map[string]int{}
		}
		stopped[culprit][gateFailureCategory(last.GateFailureReason)]++
	}
	return stopped, implemented
}

// gateFailureCategory is the part of a gate reason before its colon - the
// kind of failure, without the artifact path that makes every instance
// unique. "unknown" covers a failed attempt that recorded no reason at all,
// which is a gap in the record rather than a category of failure.
func gateFailureCategory(reason *string) string {
	if reason == nil || *reason == "" {
		return "unknown"
	}
	if idx := strings.IndexByte(*reason, ':'); idx != -1 {
		return (*reason)[:idx]
	}
	return *reason
}

// readAttemptLedgers reads and parses every ledger file named by paths.
//
// Two failure shapes inside a file are handled differently, matching how the
// writer (appendStageAttempt) itself distinguishes them. A malformed row
// that is NOT the file's trailing segment is real mid-file corruption -
// parseLedgerBytes rejects it, and that halts the whole read here too,
// naming the offending file, because aggregating over a ledger it could not
// fully parse would silently misrepresent every row after the break. A
// trailing segment with no newline is different: it is exactly the state a
// process killed between the write syscall and AppendStageAttempt's
// truncate-back leaves behind, which the writer's own flock+Truncate
// protocol is designed to tolerate and repair on its next call.
// parseLedgerBytes reports that case via a shorter validSize rather than an
// error, and this function surfaces it as a dangling path rather than
// silently dropping the row it belongs to - halting on it would make `stats`
// unusable after any crash mid-append, stricter than the writer is about a
// state the writer designed for.
func readAttemptLedgers(paths []string) (records []StageAttemptRecord, dangling []string, err error) {
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, nil, fmt.Errorf("KERNL DISPATCH FAILURE: reading attempt ledger %s: %w", path, readErr)
		}
		parsed, validSize, parseErr := parseLedgerBytes(path, data)
		if parseErr != nil {
			return nil, nil, parseErr
		}
		if validSize != int64(len(data)) {
			dangling = append(dangling, path)
		}
		for i := range parsed {
			DeriveAttemptCost(&parsed[i])
		}
		records = append(records, parsed...)
	}
	return records, dangling, nil
}

func computeAgentAttemptStats(agentID string, recs []StageAttemptRecord) AgentAttemptStats {
	stats := AgentAttemptStats{AgentID: agentID, Attempts: len(recs)}

	var firstPassTotal, firstPassPassed int
	var reviewTotal, reviewRejected int
	var reworkTotal, reworkPassed int
	var reworkTokens int64
	var reworkCost float64
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
		if r.CausedBy != nil {
			reworkTotal++
			if r.GatePassed {
				reworkPassed++
			}
			stats.ReworkDurationMs += r.DurationMs
			if r.OutputTokens != nil {
				reworkTokens += *r.OutputTokens
				stats.ReworkTokenObservations++
			}
			if r.CostUSD != nil {
				reworkCost += *r.CostUSD
				stats.ReworkCostObservations++
				if r.CostSource == "derived_ceiling" {
					stats.ReworkCostIsCeiling = true
				}
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

	stats.ReworkAttempts = reworkTotal
	if len(recs) > 0 {
		rate := float64(reworkTotal) / float64(len(recs))
		stats.ReworkRate = &rate
	}

	stats.ReworkGateObservations = reworkTotal
	if reworkTotal > 0 {
		rate := float64(reworkPassed) / float64(reworkTotal)
		stats.ReworkGatePassRate = &rate
	}
	if stats.ReworkTokenObservations > 0 {
		stats.ReworkOutputTokens = &reworkTokens
	}
	if stats.ReworkCostObservations > 0 {
		stats.ReworkCostUSD = &reworkCost
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
