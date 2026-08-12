package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/adapter"
	"github.com/gabrielassisxyz/kernl/internal/transport"
)

type TerminalEvent struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
	BeadID  string `json:"beadId,omitempty"`
	Time    int64  `json:"time"`
}

type CloseDiagnostics struct {
	ExitReason        string `json:"exitReason"`
	LastEventType     string `json:"lastEventType"`
	Signal            string `json:"signal,omitempty"`
	ExitCode          int    `json:"exitCode"`
	MsSinceLastStdout int64  `json:"msSinceLastStdout"`
	TurnError         string `json:"turnError,omitempty"`
}

type PromptDeliveryHook struct {
	OnAttempted func(transport string)
	OnSucceeded func(transport string)
	OnFailed    func(transport string, err error)
	OnDeferred  func()
}

type OnTurnEndedFunc func(exitReason string) bool

type SessionRuntime struct {
	beadID         string
	repoPath       string
	capabilities   DialectCapabilities
	dialect        string
	events         chan TerminalEvent
	resultObserved bool
	exitReason     string
	isError        bool
	lastEventType  string
	lastStdoutAt   *time.Time
	stdinClosed    bool
	autoAnswered   map[string]bool
	mu             sync.Mutex
	cancel         context.CancelFunc
	// killProcess terminates the child this runtime reads from. Start's
	// context is derived FROM the one the caller spawned that child with, so
	// cancelling it stops the readers and can never reach the process itself;
	// without this hook a stopped dispatch leaves its agent running and the
	// caller blocks in Process.Wait() forever. Nil for runtimes driven from
	// plain readers with no process behind them.
	killProcess        func() error
	stdin              io.Writer
	stdinMu            sync.Mutex
	onTurnEnded        OnTurnEndedFunc
	promptHooks        PromptDeliveryHook
	inputCloseTimer    *time.Timer
	lastTurnError      string
	lastTurnFailure    *TurnFailure
	piLastStopReason   string
	piLastErrorMessage string
	tokenLogger        TokenUsageLogger
	capturedSessionID  string
	turnCount          int
	turnCeilingHit     bool
	drainWG            sync.WaitGroup
}

// MaxTurnsPerDispatch is how many turn boundaries one dispatched agent may
// cross before kernl stops it.
//
// It exists because the watchdog cannot see the failure it guards. The
// watchdog measures SILENCE - its timer is re-armed on every event - and a
// runaway loop is the noisiest thing an agent can do, so it re-arms the
// timer thousands of times and never fires. The one failure mode that burns
// tokens without bound was therefore the one nothing caught.
//
// Measured, and the reason for this constant: an implementer finished its
// real work, committed it, and then spent 39 minutes and 79 MB of stream
// trying to remove one trailing blank line from a file, running three
// variants of the same `truncate` command 374 times across 422 turns. It was
// killed by hand. Nothing in kernl would have stopped it.
//
// 150 is a safety margin rather than a target: healthy stages in the same
// repositories cross tens of turns, and the run above was already past 400.
// A stage that legitimately needs more than 150 turns is a stage that should
// have been split into more than one bead.
//
// This binds only on dialects whose per-turn boundary is not also the end of
// the run. claude's "result" and codex's "turn.completed" ARE the end, so
// counting them could never reach a ceiling; pi's "turn_end" and opencode's
// "step_finish" are mid-run boundaries and are what this counts.
const MaxTurnsPerDispatch = 150

// countTurn records one crossed turn boundary and reports whether that
// crossing is the one that broke the ceiling. It answers true EXACTLY once
// per run, so the caller's termination path cannot be entered twice by the
// events that arrive between the kill signal and the stream closing.
func (r *SessionRuntime) countTurn() bool {
	r.mu.Lock()
	r.turnCount++
	if r.turnCount <= MaxTurnsPerDispatch || r.turnCeilingHit {
		r.mu.Unlock()
		return false
	}
	r.turnCeilingHit = true
	r.isError = true
	r.lastTurnError = fmt.Sprintf("turn ceiling exceeded: %d turns, over the %d turn limit", r.turnCount, MaxTurnsPerDispatch)
	r.lastTurnFailure = UnclassifiedTurnFailure(r.lastTurnError)
	r.mu.Unlock()
	return true
}

// stopForTurnCeiling ends a dispatch that has crossed too many turns, through
// Stop: the readers are cancelled and the agent process is killed through the
// hook the caller registered. Cancelling the context Start derives does NOT
// reach that process - it is a child of the one the process was spawned with -
// and a dispatch that stops reading an agent still running blocks its caller
// in Process.Wait() with no ledger row and no state transition.
//
// The agent is given no chance to finish the turn it is in. That is the
// point: an agent that has crossed this many boundaries is repeating itself,
// and one more turn is one more repetition.
func (r *SessionRuntime) stopForTurnCeiling() {
	r.mu.Lock()
	count := r.turnCount
	r.mu.Unlock()

	slog.Warn("KERNL DISPATCH FAILURE: turn ceiling exceeded - stopping the agent",
		"bead", r.beadID, "dialect", r.dialect, "turns", count, "limit", MaxTurnsPerDispatch)
	r.emit("stderr", fmt.Sprintf("[kernl] turn ceiling exceeded: %d turns, over the %d turn limit - stopping this dispatch\n", count, MaxTurnsPerDispatch))
	r.Stop()
}

// TurnCeilingExceeded reports whether this run was stopped for crossing
// MaxTurnsPerDispatch, and how many turns it had crossed. The driver reads
// it back after the stream drains, so a stage killed for looping is reported
// as that rather than as the bare non-zero exit code the kill produces -
// "exit code 143" names the signal, not the reason.
func (r *SessionRuntime) TurnCeilingExceeded() (bool, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.turnCeilingHit, r.turnCount
}

// CountedTurns reports the turn boundaries this run has crossed toward
// MaxTurnsPerDispatch, and whether this dialect counts any at all. Only
// pi's "turn_end" and opencode's "step_finish" ever call countTurn(); every
// other dialect's turnCount field stays at its zero value forever, which is
// not the same fact as a run that measured zero boundaries - counted is
// what a caller (the stage-attempt ledger) needs to tell the two apart
// instead of persisting a fabricated zero for a dialect that reports none.
func (r *SessionRuntime) CountedTurns() (turns int64, counted bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch r.dialect {
	case "pi", "opencode":
		return int64(r.turnCount), true
	default:
		return 0, false
	}
}

func NewSessionRuntime(beadID, repoPath string) *SessionRuntime {
	return &SessionRuntime{
		beadID:       beadID,
		repoPath:     repoPath,
		events:       make(chan TerminalEvent, 5000),
		autoAnswered: make(map[string]bool),
	}
}

func NewSessionRuntimeWithCapabilities(beadID, repoPath, dialect string, interactive bool) *SessionRuntime {
	r := &SessionRuntime{
		beadID:       beadID,
		repoPath:     repoPath,
		dialect:      dialect,
		capabilities: CapabilitiesForDialect(dialect, interactive),
		events:       make(chan TerminalEvent, 5000),
		autoAnswered: make(map[string]bool),
	}
	if !r.capabilities.Interactive {
		r.stdinClosed = true
	}
	return r
}

func (r *SessionRuntime) SetCapabilities(cap DialectCapabilities) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.capabilities = cap
	if !cap.Interactive {
		r.stdinClosed = true
	}
}

func (r *SessionRuntime) Capabilities() DialectCapabilities {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.capabilities
}

// CapturedSessionID returns the opencode session ID observed in the NDJSON
// stream (the "sessionID" field that opencode emits on the first step_start
// and on every subsequent event). Empty if the stream carried no such field.
func (r *SessionRuntime) CapturedSessionID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.capturedSessionID
}

func (r *SessionRuntime) SetDialect(dialect string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dialect = dialect
}

// Dialect returns the dialect this runtime was constructed with (e.g.
// "claude", "codex"), used by the take-loop to name the offending dialect in
// a KERNL DISPATCH FAILURE when it has no follow-up path.
func (r *SessionRuntime) Dialect() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dialect
}

func (r *SessionRuntime) SetStdin(w io.Writer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stdin = w
}

func (r *SessionRuntime) SetOnTurnEnded(fn OnTurnEndedFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onTurnEnded = fn
}

func (r *SessionRuntime) SetPromptHooks(hooks PromptDeliveryHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.promptHooks = hooks
}

func (r *SessionRuntime) Start(ctx context.Context, stdout, stderr io.Reader) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.mu.Lock()
	r.resultObserved = false
	r.exitReason = ""
	r.isError = false
	r.lastEventType = ""
	r.lastTurnError = ""
	r.lastTurnFailure = nil
	now := time.Now()
	r.lastStdoutAt = &now
	r.mu.Unlock()

	r.drainWG.Add(2)
	go func() {
		defer r.drainWG.Done()
		r.readStdout(ctx, stdout)
	}()
	go func() {
		defer r.drainWG.Done()
		r.readStderr(ctx, stderr)
	}()

	return ctx
}

// WaitDrained blocks until the stdout and stderr readers have both returned
// (stream EOF, or ctx cancellation/timeout - never local Dispose/Stop, which
// callers must not invoke before this returns). Callers spawning a real
// child process via exec.Cmd.StdoutPipe/StderrPipe must call this before
// Process.Wait(): Cmd.Wait() closes those pipes as soon as it sees the
// process exit, and reading from them concurrently with that close can
// silently drop the last buffered line - exactly the line carrying the
// turn-ending event (e.g. claude's "result") that onTurnEnded depends on.
func (r *SessionRuntime) WaitDrained() {
	r.drainWG.Wait()
}

// Stop ends this dispatch: it cancels the readers and terminates the agent
// process. Both halves are load-bearing. Cancelling alone returns the reader
// goroutines but leaves the child running, and the caller then blocks in
// Process.Wait() with nothing left to wake it.
func (r *SessionRuntime) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.mu.Lock()
	kill := r.killProcess
	r.mu.Unlock()
	if kill != nil {
		// A process that already exited reports an error here, which is the
		// ordinary case on the Dispose path after Wait() returned.
		_ = kill()
	}
}

// SetProcessKiller registers how to terminate the agent process this runtime
// reads from. Call it before Start.
func (r *SessionRuntime) SetProcessKiller(kill func() error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.killProcess = kill
}

func (r *SessionRuntime) Events() <-chan TerminalEvent {
	return r.events
}

func (r *SessionRuntime) MarkResultObserved(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resultObserved = true
	r.exitReason = reason
}

func (r *SessionRuntime) ResultObserved() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.resultObserved
}

// IsError reports whether the dialect's own stream marked the turn that just
// ended as a failure or incomplete response (claude's result.is_error,
// codex's turn.failed, gemini's non-success result, copilot's session.error,
// opencode's step_finish/session_error) - never inferred from timing or bead
// state. The take-loop's follow-up gate uses this to tell a turn that ended
// without finishing from one that ended because the agent was done.
func (r *SessionRuntime) IsError() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.isError
}

func (r *SessionRuntime) LastStdoutAt() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lastStdoutAt == nil {
		return time.Time{}
	}
	return *r.lastStdoutAt
}

func (r *SessionRuntime) StdinClosed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stdinClosed
}

func (r *SessionRuntime) BeadID() string {
	return r.beadID
}

func (r *SessionRuntime) RepoPath() string {
	return r.repoPath
}

func (r *SessionRuntime) SendUserTurn(prompt string) bool {
	r.mu.Lock()
	caps := r.capabilities
	stdinClosed := r.stdinClosed
	stdin := r.stdin
	hooks := r.promptHooks
	r.mu.Unlock()

	if hooks.OnAttempted != nil {
		hooks.OnAttempted("stdio")
	}

	if !caps.Interactive || stdinClosed || stdin == nil {
		if hooks.OnFailed != nil && !caps.Interactive {
			hooks.OnFailed("stdio", fmt.Errorf("one-shot session cannot accept user input"))
		}
		return false
	}

	msg := map[string]any{
		"type":    "user_message",
		"content": prompt,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		if hooks.OnFailed != nil {
			hooks.OnFailed("stdio", err)
		}
		return false
	}
	data = append(data, '\n')

	r.stdinMu.Lock()
	defer r.stdinMu.Unlock()
	_, err = stdin.Write(data)
	if err != nil {
		if hooks.OnFailed != nil {
			hooks.OnFailed("stdio", err)
		}
		return false
	}

	if hooks.OnSucceeded != nil {
		hooks.OnSucceeded("stdio")
	}

	r.mu.Lock()
	if r.resultObserved {
		r.resultObserved = false
		r.exitReason = ""
		r.isError = false
	}
	r.mu.Unlock()

	return true
}

func (r *SessionRuntime) CloseInput() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stdinClosed {
		return
	}
	r.stdinClosed = true
	if r.stdin != nil {
		if wc, ok := r.stdin.(interface{ Close() error }); ok {
			wc.Close()
		}
	}
}

func (r *SessionRuntime) ScheduleInputClose(grace time.Duration) {
	if grace == 0 {
		grace = 2 * time.Second
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stdinClosed {
		return
	}
	if r.inputCloseTimer != nil {
		r.inputCloseTimer.Stop()
	}
	r.inputCloseTimer = time.AfterFunc(grace, func() {
		r.CloseInput()
	})
}

func (r *SessionRuntime) CancelInputClose() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inputCloseTimer != nil {
		r.inputCloseTimer.Stop()
		r.inputCloseTimer = nil
	}
}

func (r *SessionRuntime) Dispose() {
	r.mu.Lock()
	if r.inputCloseTimer != nil {
		r.inputCloseTimer.Stop()
		r.inputCloseTimer = nil
	}
	r.stdinClosed = true
	r.mu.Unlock()
	r.Stop()
}

func (r *SessionRuntime) SetTokenUsageLogger(logger TokenUsageLogger) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokenLogger = logger
}

func (r *SessionRuntime) LastError() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastTurnError
}

// LastFailure is how the last failed turn failed, or nil when this dispatch
// has not recorded a failure. Every site that sets lastTurnError sets this
// too, so a caller never has to decide which of the two to believe.
func (r *SessionRuntime) LastFailure() *TurnFailure {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lastTurnFailure == nil {
		return nil
	}
	failure := *r.lastTurnFailure
	return &failure
}

func CaptureChildCloseDiagnostics(runtime *SessionRuntime, exitCode int, signal string) CloseDiagnostics {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	d := CloseDiagnostics{
		ExitCode:   exitCode,
		Signal:     signal,
		ExitReason: "normal",
	}
	if runtime.exitReason != "" {
		d.ExitReason = runtime.exitReason
	}
	if runtime.lastEventType != "" {
		d.LastEventType = runtime.lastEventType
	}
	if runtime.lastStdoutAt != nil {
		d.MsSinceLastStdout = time.Since(*runtime.lastStdoutAt).Milliseconds()
	} else {
		d.MsSinceLastStdout = -1
	}
	if runtime.lastTurnError != "" {
		d.TurnError = runtime.lastTurnError
	}
	return d
}

func FormatDiagnosticsForLog(d CloseDiagnostics) string {
	msStr := "null"
	if d.MsSinceLastStdout >= 0 {
		msStr = fmt.Sprintf("%d", d.MsSinceLastStdout)
	}
	turnErrorStr := "null"
	if d.TurnError != "" {
		turnErrorStr = d.TurnError
	}
	lastEventTypeStr := "null"
	if d.LastEventType != "" {
		lastEventTypeStr = d.LastEventType
	}
	return fmt.Sprintf("signal=%s exitReason=%s msSinceLastStdout=%s lastEventType=%s turnError=%s",
		d.Signal, d.ExitReason, msStr, lastEventTypeStr, turnErrorStr)
}

func ShouldTreatTurnEndedSignalAsClean(d CloseDiagnostics) bool {
	if d.Signal != "" {
		return false
	}
	if d.ExitCode != 0 {
		return false
	}
	if d.TurnError != "" {
		return false
	}
	if d.LastEventType == "turn.failed" {
		return false
	}
	return d.ExitReason == "turn_ended" || d.LastEventType == "result"
}

func (r *SessionRuntime) readStdout(ctx context.Context, reader io.Reader) {
	lines := transport.ParseNDJSON(ctx, reader)
	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-lines:
			if !ok {
				return
			}
			r.mu.Lock()
			now := time.Now()
			r.lastStdoutAt = &now
			r.mu.Unlock()

			if line.Err != nil {
				r.emit("stdout", string(line.Line))
				continue
			}

			normalized := r.normalizeEvent(line.Data)
			if normalized == nil {
				r.emit("stdout", line.Line)
				continue
			}

			evtType := (*normalized)["type"]
			if evtType == nil {
				r.emit("stdout", line.Line)
				continue
			}

			evtTypeStr, _ := evtType.(string)
			r.processNormalizedEvent(evtTypeStr, *normalized, line.Line)
		}
	}
}

func (r *SessionRuntime) normalizeEvent(raw json.RawMessage) *map[string]any {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	return &obj
}

func (r *SessionRuntime) processNormalizedEvent(evtType string, obj map[string]any, rawLine string) {
	r.mu.Lock()
	dialect := r.dialect
	caps := r.capabilities
	r.mu.Unlock()

	switch dialect {
	case "claude":
		r.processClaudeEvent(evtType, obj, rawLine, caps)
	case "codex":
		r.processCodexEvent(evtType, obj, rawLine, caps)
	case "copilot":
		r.processCopilotEvent(evtType, obj, rawLine, caps)
	case "gemini":
		r.processGeminiEvent(evtType, obj, rawLine, caps)
	case "opencode":
		r.processOpenCodeEvent(evtType, obj, rawLine, caps)
	case "pi":
		r.processPiEvent(evtType, obj, rawLine, caps)
	case "agy":
		// agy prints plain text, so nothing here ever parses as an event and
		// this case is never reached from readStdout. It is spelled out
		// anyway so the switch enumerates every dialect: falling into
		// default would read as an oversight rather than as the measured
		// fact that agy has no stream to process.
		r.emit("stdout", rawLine)
	default:
		r.emit("stdout", rawLine)
		if evtType == "result" {
			r.handleTurnEnd("turn_ended")
		}
	}
}

func (r *SessionRuntime) processClaudeEvent(evtType string, obj map[string]any, rawLine string, caps DialectCapabilities) {
	switch evtType {
	case "result":
		// Claude's own stream-json result carries is_error, exactly like
		// codex's turn.failed or gemini's non-success status - it is the
		// dialect's own report of whether this turn finished or gave up, not
		// a guess. Recording it here is what lets a consumer (the take-loop's
		// follow-up gate) tell "the agent is done" from "the agent stopped
		// without finishing" at the moment the turn ends, before the bead's
		// own state has had a chance to reflect either outcome.
		isErr, _ := obj["is_error"].(bool)
		r.mu.Lock()
		r.lastEventType = "result"
		if isErr {
			r.isError = true
		}
		tokenLogger := r.tokenLogger
		dialect := r.dialect
		beadID := r.beadID
		r.mu.Unlock()

		if tokenLogger != nil {
			LogTokenUsageForEvent(tokenLogger, adapter.AgentDialect(dialect), obj, beadID)
		}

		r.emit("stdout", rawLine)
		r.handleTurnEnd("turn_ended")
	default:
		r.mu.Lock()
		r.lastEventType = evtType
		r.mu.Unlock()
		if evtType == "tool_use" && caps.SupportsAskUserAutoResp {
			r.maybeAutoAnswerClaude(obj)
		}
		r.emit("stdout", rawLine)
	}
}

func (r *SessionRuntime) processCodexEvent(evtType string, obj map[string]any, rawLine string, caps DialectCapabilities) {
	switch evtType {
	case "turn.completed":
		status, _ := obj["status"].(string)
		if status == "failed" {
			r.mu.Lock()
			r.lastEventType = "turn.failed"
			r.lastTurnError = fmt.Sprintf("codex turn.failed: %v", obj["error"])
			r.lastTurnFailure = UnclassifiedTurnFailure(r.lastTurnError)
			r.mu.Unlock()
			r.emit("stdout", rawLine)
			r.handleTurnEnd("turn_ended")
			return
		}
		r.mu.Lock()
		tokenLogger := r.tokenLogger
		dialect := r.dialect
		beadID := r.beadID
		r.lastEventType = "turn.completed"
		r.mu.Unlock()

		if tokenLogger != nil {
			LogTokenUsageForEvent(tokenLogger, adapter.AgentDialect(dialect), obj, beadID)
		}

		r.emit("stdout", rawLine)
		r.handleTurnEnd("turn_ended")
	case "turn.failed":
		r.mu.Lock()
		r.lastEventType = "turn.failed"
		r.isError = true
		if errObj, ok := obj["error"]; ok {
			r.lastTurnError = fmt.Sprintf("codex turn.failed: %v", errObj)
		} else {
			r.lastTurnError = "codex turn.failed"
		}
		// codex is not classified: no rollout on disk has ever shown a
		// provider fault in this envelope, so there is nothing measured to
		// key on and a guess would read exactly like an observation.
		r.lastTurnFailure = UnclassifiedTurnFailure(r.lastTurnError)
		r.mu.Unlock()
		r.emit("stdout", rawLine)
		r.handleTurnEnd("turn_ended")
	case "item.completed":
		r.mu.Lock()
		r.lastEventType = "item.completed"
		r.mu.Unlock()
		r.emit("stdout", rawLine)
	default:
		r.mu.Lock()
		r.lastEventType = evtType
		r.mu.Unlock()
		r.emit("stdout", rawLine)
	}
}

func (r *SessionRuntime) processCopilotEvent(evtType string, obj map[string]any, rawLine string, caps DialectCapabilities) {
	switch evtType {
	case "session.task_complete":
		r.mu.Lock()
		r.lastEventType = "session.task_complete"
		r.mu.Unlock()
		r.emit("stdout", rawLine)
		r.handleTurnEnd("turn_ended")
	case "session.error":
		r.mu.Lock()
		r.lastEventType = "session.error"
		r.isError = true
		r.mu.Unlock()
		r.emit("stdout", rawLine)
		r.handleTurnEnd("turn_ended")
	case "user_input.requested":
		if caps.SupportsAskUserAutoResp {
			r.maybeAutoAnswerCopilot(obj)
		}
		r.mu.Lock()
		r.lastEventType = "user_input.requested"
		r.mu.Unlock()
		r.emit("stdout", rawLine)
	default:
		r.mu.Lock()
		r.lastEventType = evtType
		r.mu.Unlock()
		r.emit("stdout", rawLine)
	}
}

func (r *SessionRuntime) processGeminiEvent(evtType string, obj map[string]any, rawLine string, caps DialectCapabilities) {
	switch evtType {
	case "result":
		status, _ := obj["status"].(string)
		r.mu.Lock()
		r.lastEventType = "result"
		if status == "success" {
			r.mu.Unlock()
			r.emit("stdout", rawLine)
			r.handleTurnEnd("turn_ended")
			return
		}
		r.isError = true
		r.mu.Unlock()
		r.emit("stdout", rawLine)
		r.handleTurnEnd("turn_ended")
	default:
		r.mu.Lock()
		r.lastEventType = evtType
		r.mu.Unlock()
		r.emit("stdout", rawLine)
	}
}

// processPiEvent reads pi's NDJSON stream (`pi -p --mode json`), measured
// against pi 0.81. Three events carry meaning beyond being printed:
//
//	session       - the first line, carrying the session id pi assigned
//	agent_end     - carries the run's token totals, one line before the last
//	agent_settled - the last line, after which the process exits
//
// pi's per-turn boundary is "turn_end", but a one-shot dispatch has exactly
// one turn and "agent_settled" is the event that means the agent has stopped
// for good, so that is what ends the turn here.
//
// "agent_end" is a separate case from that turn boundary because pi splits
// what claude's "result" event fuses: the usage totals arrive on agent_end
// and the stop signal on agent_settled. Reading only the last line - which is
// what this function did until the totals were found missing from the
// stage-attempt ledger - dropped every token pi ever reported, silently,
// because a dialect that reports nothing and a dialect nobody asks are
// indistinguishable downstream. Measured order on a real run: agent_end
// precedes agent_settled, so the usage is logged before the turn ends.
//
// No error signal is derived: unlike claude's is_error or gemini's status,
// pi's failure shape has not been observed on this stream, and inventing one
func extractPiStopReasonAndError(obj map[string]any) (string, string) {
	if obj == nil {
		return "", ""
	}
	var stopReason, errMsg string

	// 1. Check top-level
	if sr, ok := obj["stopReason"].(string); ok && sr != "" {
		stopReason = sr
	}
	if em, ok := obj["errorMessage"].(string); ok && em != "" {
		errMsg = em
	} else if ev, ok := obj["error"].(string); ok && ev != "" {
		errMsg = ev
	}

	// 2. Check nested "message" object
	if msg, ok := obj["message"].(map[string]any); ok {
		if sr, ok := msg["stopReason"].(string); ok && sr != "" {
			stopReason = sr
		}
		if em, ok := msg["errorMessage"].(string); ok && em != "" {
			errMsg = em
		} else if ev, ok := msg["error"].(string); ok && ev != "" {
			errMsg = ev
		}
	}

	return stopReason, errMsg
}

func (r *SessionRuntime) processPiEvent(evtType string, obj map[string]any, rawLine string, caps DialectCapabilities) {
	if sid, _ := obj["id"].(string); sid != "" && evtType == "session" {
		r.mu.Lock()
		if r.capturedSessionID == "" {
			r.capturedSessionID = sid
		}
		r.mu.Unlock()
	}

	if sr, errMsg := extractPiStopReasonAndError(obj); sr != "" {
		r.mu.Lock()
		r.piLastStopReason = sr
		switch sr {
		case "stop":
			r.isError = false
			r.lastTurnError = ""
			r.lastTurnFailure = nil
		case "error":
			if errMsg == "" {
				errMsg = "pi stopReason=error"
			}
			r.piLastErrorMessage = errMsg
			r.isError = true
			r.lastTurnError = "pi agent_error: " + r.piLastErrorMessage
			// The only site with a provider envelope to read. Classified
			// from the raw message rather than from lastTurnError, so the
			// payload is not behind kernl's own prefix.
			r.lastTurnFailure = ClassifyPiTurnFailure(errMsg)
		case "aborted":
			r.isError = true
			r.lastTurnError = "pi agent_aborted: session ended with stopReason=aborted"
			r.lastTurnFailure = UnclassifiedTurnFailure(r.lastTurnError)
		case "toolUse":
			r.isError = true
			r.lastTurnError = "pi agent_incomplete: session ended unexpectedly mid-tool call (stopReason=toolUse)"
			r.lastTurnFailure = UnclassifiedTurnFailure(r.lastTurnError)
		default:
			r.isError = true
			r.lastTurnError = fmt.Sprintf("pi agent_unknown: session ended with unrecognized stopReason=%q", sr)
			r.lastTurnFailure = UnclassifiedTurnFailure(r.lastTurnError)
		}
		r.mu.Unlock()
	}

	switch evtType {
	case "agent_end":
		r.mu.Lock()
		r.lastEventType = "agent_end"
		tokenLogger := r.tokenLogger
		dialect := r.dialect
		beadID := r.beadID
		r.mu.Unlock()

		if tokenLogger != nil {
			LogTokenUsageForEvent(tokenLogger, adapter.AgentDialect(dialect), obj, beadID)
		}

		r.emit("stdout", rawLine)
	case "agent_settled":
		r.mu.Lock()
		r.lastEventType = "agent_settled"
		r.mu.Unlock()
		r.emit("stdout", rawLine)
		r.handleTurnEnd("turn_ended")
	case "turn_end":
		// pi's mid-run turn boundary, not the end of the dispatch (see
		// TestSessionRuntime_PiTurnEndAloneIsNotTheEnd). This is where a
		// loop shows up: 422 of these crossed in the run that motivated
		// MaxTurnsPerDispatch.
		exceeded := r.countTurn()
		r.mu.Lock()
		r.lastEventType = evtType
		r.mu.Unlock()
		r.emit("stdout", rawLine)
		if exceeded {
			r.stopForTurnCeiling()
		}
	default:
		r.mu.Lock()
		r.lastEventType = evtType
		r.mu.Unlock()
		r.emit("stdout", rawLine)
	}
}

func (r *SessionRuntime) processOpenCodeEvent(evtType string, obj map[string]any, rawLine string, caps DialectCapabilities) {
	// Capture the opencode session ID from the first event that carries it.
	// opencode emits "sessionID" (camelCase) on step_start and most events.
	if sid, _ := obj["sessionID"].(string); sid != "" {
		r.mu.Lock()
		if r.capturedSessionID == "" {
			r.capturedSessionID = sid
		}
		r.mu.Unlock()
	}
	switch evtType {
	case "session_idle":
		r.mu.Lock()
		r.lastEventType = "session_idle"
		r.mu.Unlock()
		r.emit("stdout", rawLine)
		r.handleTurnEnd("turn_ended")
	case "step_finish":
		reason, _ := obj["reason"].(string)
		if reason == "error" {
			r.mu.Lock()
			r.lastEventType = "step_finish"
			r.isError = true
			r.mu.Unlock()
			r.emit("stdout", rawLine)
			r.handleTurnEnd("turn_ended")
			return
		}
		// opencode's mid-run turn boundary: session_idle ends the dispatch,
		// this does not, so this is the one that can run away. Same reason
		// pi's turn_end is counted - see MaxTurnsPerDispatch.
		exceeded := r.countTurn()
		r.mu.Lock()
		r.lastEventType = "step_finish"
		r.mu.Unlock()
		r.emit("stdout", rawLine)
		if exceeded {
			r.stopForTurnCeiling()
		}
	case "session_error":
		r.mu.Lock()
		r.lastEventType = "session_error"
		r.isError = true
		r.mu.Unlock()
		r.emit("stdout", rawLine)
		r.handleTurnEnd("turn_ended")
	default:
		r.mu.Lock()
		r.lastEventType = evtType
		r.mu.Unlock()
		r.emit("stdout", rawLine)
	}
}

func (r *SessionRuntime) handleTurnEnd(reason string) {
	r.mu.Lock()
	resultAlreadyObserved := r.resultObserved
	fn := r.onTurnEnded
	r.mu.Unlock()

	r.MarkResultObserved(reason)

	if resultAlreadyObserved {
		return
	}
	r.emit("turn_ended", reason)

	if fn != nil {
		proceed := fn(reason)
		if !proceed {
			r.ScheduleInputClose(0)
		}
	}
}

func (r *SessionRuntime) maybeAutoAnswerClaude(obj map[string]any) {
	name, _ := obj["name"].(string)
	if name != "AskUserQuestion" {
		return
	}
	r.mu.Lock()
	stdinClosed := r.stdinClosed
	stdin := r.stdin
	r.mu.Unlock()
	if stdinClosed || stdin == nil {
		return
	}
	id, _ := obj["id"].(string)
	if id != "" {
		r.mu.Lock()
		if r.autoAnswered[id] {
			r.mu.Unlock()
			return
		}
		r.autoAnswered[id] = true
		r.mu.Unlock()
	}
	msg := map[string]any{
		"type":        "tool_result",
		"tool_use_id": id,
		"content":     "auto-response",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	data = append(data, '\n')
	r.stdinMu.Lock()
	_, writeErr := stdin.Write(data)
	r.stdinMu.Unlock()
	if writeErr != nil {
		slog.Warn("session: writing to agent stdin failed", "error", writeErr)
	}
}

func (r *SessionRuntime) maybeAutoAnswerCopilot(obj map[string]any) {
	r.mu.Lock()
	stdinClosed := r.stdinClosed
	stdin := r.stdin
	r.mu.Unlock()
	if stdinClosed || stdin == nil {
		return
	}
	msg := map[string]any{
		"type":    "user_message",
		"content": "auto-response",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	data = append(data, '\n')
	r.stdinMu.Lock()
	_, writeErr := stdin.Write(data)
	r.stdinMu.Unlock()
	if writeErr != nil {
		slog.Warn("session: writing to agent stdin failed", "error", writeErr)
	}
}

func (r *SessionRuntime) readStderr(ctx context.Context, reader io.Reader) {
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := reader.Read(buf)
		if n > 0 {
			r.emit("stderr", string(buf[:n]))
		}
		if err != nil {
			if err != io.EOF {
				r.emit("error", err.Error())
			}
			return
		}
	}
}

func (r *SessionRuntime) emit(evtType, content string) {
	evt := TerminalEvent{
		Type:    evtType,
		Content: content,
		BeadID:  r.beadID,
		Time:    time.Now().UnixMilli(),
	}
	select {
	case r.events <- evt:
	default:
		slog.Warn("session event channel full, dropping event", "type", evtType, "beadId", r.beadID)
	}
}
