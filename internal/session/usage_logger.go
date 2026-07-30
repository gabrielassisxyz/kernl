package session

import "sync"

// CapturingUsageLogger is the production TokenUsageLogger: it retains the
// most recent usage a dispatched CLI reported, so the caller can read it
// back once the runtime's reader goroutines have drained (see
// SessionRuntime.WaitDrained). Before this, TokenUsageLogger had no
// production implementation - only a test mock - so every measured run's
// token usage was parsed and discarded.
//
// One instance is meant to back a single RunBead invocation: a dispatched
// agent reports at most one terminal usage event (codex's turn.completed,
// claude's result) per run, so "most recent" and "only" coincide in
// practice; a shared instance across concurrent runs would let one bead's
// usage overwrite another's.
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
	u := usage
	l.usage = &u
}

// Usage returns the last captured usage, or nil if the run never emitted a
// recognized terminal usage event. Only safe to call after the runtime's
// reader goroutines have drained - calling earlier can race the write in
// LogTokenUsage above.
func (l *CapturingUsageLogger) Usage() *TokenUsageCounts {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.usage
}
