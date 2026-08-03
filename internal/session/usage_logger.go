package session

import "sync"

// CapturingUsageLogger is the production TokenUsageLogger: it ACCUMULATES the
// usage a dispatched CLI reports, so the caller can read one run's total back
// once the runtime's reader goroutines have drained (see
// SessionRuntime.WaitDrained). Before this, TokenUsageLogger had no
// production implementation - only a test mock - so every measured run's
// token usage was parsed and discarded.
//
// It accumulates rather than retaining the most recent report, and that is
// load-bearing rather than defensive. It originally kept the most recent, on
// the belief that a dispatched agent reports at most one terminal usage event
// per run - true of codex's turn.completed and claude's result, and FALSE of
// pi, which emits one agent_end per internal retry (carrying willRetry) and
// then a final one. Observed on a real stage: four agent_end events whose
// input tokens were 1,014,610 / 0 / 0 / 0, where keeping the last recorded
// the run as having cost ZERO. That is worse than recording nothing at all,
// which is the distinction the whole ledger is built on - a nil column says
// "the dialect did not report this", a zero says "this was measured and it
// was free."
//
// Summing is correct because the events carry DISJOINT segments of the
// conversation, not cumulative snapshots of it. Measured across the recorded
// runs that retried: the first agent_end holds the one user message that
// opened the run and a later one holds none, so no message - and therefore no
// usage - is counted twice. A dialect that ever reports running totals
// instead would need its own handling here; none of the three does today.
//
// The counts-always-present fields sum plainly. The optional ones stay nil
// until some event reports them, so "no event mentioned reasoning tokens"
// still reads as nil rather than as a measured zero. Model takes the most
// recent non-empty report, matching normalizePiAgentEndUsage's own rule that
// the model which produced the final answer is the one that answers "what
// ran here".
//
// One instance is meant to back a single RunBead invocation; a shared
// instance across concurrent runs would fold one bead's usage into another's.
type CapturingUsageLogger struct {
	mu    sync.Mutex
	usage *TokenUsageCounts
}

func NewCapturingUsageLogger() *CapturingUsageLogger {
	return &CapturingUsageLogger{}
}

func (l *CapturingUsageLogger) LogTokenUsage(_ string, usage TokenUsageCounts) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.usage == nil {
		u := usage
		l.usage = &u
		return
	}

	l.usage.InputTokens += usage.InputTokens
	l.usage.OutputTokens += usage.OutputTokens
	l.usage.TotalTokens += usage.TotalTokens
	l.usage.CacheReadTokens = addOptionalCount(l.usage.CacheReadTokens, usage.CacheReadTokens)
	l.usage.CacheWriteTokens = addOptionalCount(l.usage.CacheWriteTokens, usage.CacheWriteTokens)
	l.usage.ReasoningTokens = addOptionalCount(l.usage.ReasoningTokens, usage.ReasoningTokens)
	l.usage.Turns = addOptionalCount(l.usage.Turns, usage.Turns)
	l.usage.CostUSD = addOptionalFloat(l.usage.CostUSD, usage.CostUSD)
	if usage.Model != nil && *usage.Model != "" {
		l.usage.Model = usage.Model
	}
}

// addOptionalCount sums two fields a dialect may not report at all. Two nils
// stay nil - the field was never measured, and a zero there would claim it
// was. One nil contributes nothing, because "did not report" is not "reported
// zero" and must not drag a real measurement down to one.
func addOptionalCount(acc, next *int64) *int64 {
	if next == nil {
		return acc
	}
	if acc == nil {
		v := *next
		return &v
	}
	sum := *acc + *next
	return &sum
}

// addOptionalFloat is addOptionalCount for dollar amounts, which are
// fractional and legitimately zero (see readOptionalFloat in token_usage.go).
func addOptionalFloat(acc, next *float64) *float64 {
	if next == nil {
		return acc
	}
	if acc == nil {
		v := *next
		return &v
	}
	sum := *acc + *next
	return &sum
}

// Usage returns the run's accumulated usage, or nil if the run never emitted
// a recognized terminal usage event. Only safe to call after the runtime's
// reader goroutines have drained - calling earlier can race the write in
// LogTokenUsage above.
func (l *CapturingUsageLogger) Usage() *TokenUsageCounts {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.usage
}
