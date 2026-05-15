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
	BeatID  string `json:"beatId,omitempty"`
	Time    int64  `json:"time"`
}

type CloseDiagnostics struct {
	ExitReason      string `json:"exitReason"`
	LastEventType   string `json:"lastEventType"`
	Signal          string `json:"signal,omitempty"`
	ExitCode        int    `json:"exitCode"`
	MsSinceLastStdout int64 `json:"msSinceLastStdout"`
	TurnError       string `json:"turnError,omitempty"`
}

type PromptDeliveryHook struct {
	OnAttempted func(transport string)
	OnSucceeded func(transport string)
	OnFailed    func(transport string, err error)
	OnDeferred  func()
}

type OnTurnEndedFunc func(exitReason string) bool

type SessionRuntime struct {
	beatID        string
	repoPath       string
	capabilities   DialectCapabilities
	dialect        string
	events        chan TerminalEvent
	resultObserved bool
	exitReason     string
	isError        bool
	lastEventType  string
	lastStdoutAt   *time.Time
	stdinClosed    bool
	autoAnswered   map[string]bool
	mu            sync.Mutex
	cancel        context.CancelFunc
	stdin         io.Writer
	stdinMu       sync.Mutex
	onTurnEnded   OnTurnEndedFunc
	promptHooks   PromptDeliveryHook
	inputCloseTimer *time.Timer
	lastTurnError string
	tokenLogger  TokenUsageLogger
}

func NewSessionRuntime(beatID, repoPath string) *SessionRuntime {
	return &SessionRuntime{
		beatID:       beatID,
		repoPath:      repoPath,
		events:        make(chan TerminalEvent, 5000),
		autoAnswered:  make(map[string]bool),
	}
}

func NewSessionRuntimeWithCapabilities(beatID, repoPath, dialect string, interactive bool) *SessionRuntime {
	r := &SessionRuntime{
		beatID:        beatID,
		repoPath:       repoPath,
		dialect:        dialect,
		capabilities:   CapabilitiesForDialect(dialect, interactive),
		events:        make(chan TerminalEvent, 5000),
		autoAnswered:  make(map[string]bool),
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

func (r *SessionRuntime) SetDialect(dialect string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dialect = dialect
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
	now := time.Now()
	r.lastStdoutAt = &now
	r.mu.Unlock()

	go r.readStdout(ctx, stdout)
	go r.readStderr(ctx, stderr)

	return ctx
}

func (r *SessionRuntime) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
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

func (r *SessionRuntime) BeatID() string {
	return r.beatID
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
		"type": "user_message",
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

func CaptureChildCloseDiagnostics(runtime *SessionRuntime, exitCode int, signal string) CloseDiagnostics {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	d := CloseDiagnostics{
		ExitCode:  exitCode,
		Signal:    signal,
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
		r.mu.Lock()
		r.lastEventType = "result"
		r.mu.Unlock()
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
			r.mu.Unlock()
			r.emit("stdout", rawLine)
			r.handleTurnEnd("turn_ended")
			return
		}
		r.mu.Lock()
		tokenLogger := r.tokenLogger
		dialect := r.dialect
		beatID := r.beatID
		r.lastEventType = "turn.completed"
		r.mu.Unlock()

		if tokenLogger != nil {
			LogTokenUsageForEvent(tokenLogger, adapter.AgentDialect(dialect), obj, beatID)
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

func (r *SessionRuntime) processOpenCodeEvent(evtType string, obj map[string]any, rawLine string, caps DialectCapabilities) {
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
		r.mu.Lock()
		r.lastEventType = "step_finish"
		r.mu.Unlock()
		r.emit("stdout", rawLine)
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
		"type":       "tool_result",
		"tool_use_id": id,
		"content":    "auto-response",
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	data = append(data, '\n')
	r.stdinMu.Lock()
	stdin.Write(data)
	r.stdinMu.Unlock()
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
	stdin.Write(data)
	r.stdinMu.Unlock()
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
		BeatID:  r.beatID,
		Time:    time.Now().UnixMilli(),
	}
	select {
	case r.events <- evt:
	default:
		slog.Warn("session event channel full, dropping event", "type", evtType, "beatId", r.beatID)
	}
}