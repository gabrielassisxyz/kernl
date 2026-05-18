package session

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestRuntime(dialect string, interactive bool) *SessionRuntime {
	return NewSessionRuntimeWithCapabilities("bead-1", "/repo", dialect, interactive)
}

func pipeReaderWriter() (*strings.Reader, *strings.Builder) {
	return nil, &strings.Builder{}
}

type pipeWriter struct {
	mu   sync.Mutex
	data []byte
}

func (p *pipeWriter) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data = append(p.data, b...)
	return len(b), nil
}

func (p *pipeWriter) String() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return string(p.data)
}

func TestCapabilitiesForDialect_Claude(t *testing.T) {
	caps := CapabilitiesForDialect("claude", false)
	if !caps.Interactive {
		t.Error("claude should be interactive")
	}
	if caps.PromptTransport != TransportStdioStreamJSON {
		t.Errorf("expected stdin-stream-json, got %s", caps.PromptTransport)
	}
	if !caps.SupportsFollowUp {
		t.Error("claude should support follow-up")
	}
	if !caps.SupportsAskUserAutoResp {
		t.Error("claude should support AskUser auto-response")
	}
	if caps.StdinDrainPolicy != DrainCloseAfterResult {
		t.Errorf("expected close-after-result, got %s", caps.StdinDrainPolicy)
	}
	if caps.ResultDetection != ResultDetectionTypeResult {
		t.Errorf("expected type-result, got %s", caps.ResultDetection)
	}
	if caps.SupportsInteractive {
		t.Error("claude doesn't have an interactive variant override")
	}
}

func TestCapabilitiesForDialect_CodexOneShot(t *testing.T) {
	caps := CapabilitiesForDialect("codex", false)
	if caps.Interactive {
		t.Error("codex one-shot should not be interactive")
	}
	if caps.SupportsFollowUp {
		t.Error("codex one-shot should not support follow-up")
	}
	if caps.StdinDrainPolicy != DrainNeverOpened {
		t.Errorf("expected never-opened, got %s", caps.StdinDrainPolicy)
	}
	if !caps.SupportsInteractive {
		t.Error("codex supports interactive mode")
	}
}

func TestCapabilitiesForDialect_CodexInteractive(t *testing.T) {
	caps := CapabilitiesForDialect("codex", true)
	if !caps.Interactive {
		t.Error("codex interactive should be interactive")
	}
	if !caps.SupportsFollowUp {
		t.Error("codex interactive should support follow-up")
	}
	if caps.PromptTransport != TransportJSONRPCStdio {
		t.Errorf("expected jsonrpc-stdio, got %s", caps.PromptTransport)
	}
}

func TestCapabilitiesForDialect_OpenCodeInteractive(t *testing.T) {
	caps := CapabilitiesForDialect("opencode", true)
	if !caps.Interactive {
		t.Error("opencode interactive should be interactive")
	}
	if caps.PromptTransport != TransportHTTPServer {
		t.Errorf("expected http-server, got %s", caps.PromptTransport)
	}
}

func TestCapabilitiesForDialect_GeminiOneShot(t *testing.T) {
	caps := CapabilitiesForDialect("gemini", false)
	if caps.ResultDetection != ResultDetectionStatusResult {
		t.Errorf("expected status-result, got %s", caps.ResultDetection)
	}
}

func TestSessionRuntime_InitialState_Interactive(t *testing.T) {
	r := newTestRuntime("claude", true)
	if r.StdinClosed() {
		t.Error("interactive sessions should start with open stdin")
	}
}

func TestSessionRuntime_InitialState_OneShot(t *testing.T) {
	r := newTestRuntime("codex", false)
	if !r.StdinClosed() {
		t.Error("one-shot sessions should start with closed stdin")
	}
}

func TestSessionRuntime_SendUserTurn_Interactive(t *testing.T) {
	r := newTestRuntime("claude", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	result := r.SendUserTurn("hello world")
	if !result {
		t.Error("interactive sendUserTurn should return true")
	}

	written := pw.String()
	if !strings.Contains(written, "hello world") {
		t.Errorf("expected prompt in stdin, got: %s", written)
	}
	if !strings.Contains(written, `"type":"user_message"`) {
		t.Errorf("expected user_message type, got: %s", written)
	}
}

func TestSessionRuntime_SendUserTurn_OneShot(t *testing.T) {
	r := newTestRuntime("codex", false)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	result := r.SendUserTurn("hello")
	if result {
		t.Error("one-shot sendUserTurn should return false")
	}
}

func TestSessionRuntime_SendUserTurn_StdinClosed(t *testing.T) {
	r := newTestRuntime("claude", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)
	r.CloseInput()

	result := r.SendUserTurn("hello")
	if result {
		t.Error("sendUserTurn after stdin closed should return false")
	}
}

func TestSessionRuntime_CloseInput_Idempotent(t *testing.T) {
	r := newTestRuntime("claude", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	r.CloseInput()
	r.CloseInput()
	if !r.StdinClosed() {
		t.Error("stdin should be closed after CloseInput")
	}
}

func TestSessionRuntime_ScheduleInputClose(t *testing.T) {
	r := newTestRuntime("claude", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	r.ScheduleInputClose(50 * time.Millisecond)
	time.Sleep(150 * time.Millisecond)

	if !r.StdinClosed() {
		t.Error("stdin should be closed after grace period")
	}
}

func TestSessionRuntime_CancelInputClose(t *testing.T) {
	r := newTestRuntime("claude", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	r.ScheduleInputClose(50 * time.Millisecond)
	r.CancelInputClose()
	time.Sleep(150 * time.Millisecond)

	if r.StdinClosed() {
		t.Error("stdin should remain open after cancelInputClose")
	}
}

func TestSessionRuntime_Dispose_CancelTimer(t *testing.T) {
	r := newTestRuntime("claude", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	r.ScheduleInputClose(5 * time.Minute)
	r.Dispose()
	time.Sleep(50 * time.Millisecond)

	if !r.StdinClosed() {
		t.Error("dispose should mark stdin as closed")
	}
}

func TestSessionRuntime_PromptDeliveryHooks(t *testing.T) {
	r := newTestRuntime("claude", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	var attempted, succeeded bool
	r.SetPromptHooks(PromptDeliveryHook{
		OnAttempted: func(transport string) { attempted = true },
		OnSucceeded: func(transport string) { succeeded = true },
	})

	r.SendUserTurn("hello")
	if !attempted {
		t.Error("onAttempted should have been called")
	}
	if !succeeded {
		t.Error("onSucceeded should have been called")
	}
}

func TestSessionRuntime_PromptDeliveryHooks_OneShotFailure(t *testing.T) {
	r := newTestRuntime("codex", false)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	var attempted, failed bool
	r.SetPromptHooks(PromptDeliveryHook{
		OnAttempted: func(transport string) { attempted = true },
		OnFailed: func(transport string, err error) { failed = true },
	})

	r.SendUserTurn("hello")
	if !attempted {
		t.Error("onAttempted should have been called")
	}
	if !failed {
		t.Error("onFailed should have been called for one-shot")
	}
}

func TestSessionRuntime_MarkResultObserved(t *testing.T) {
	r := newTestRuntime("claude", true)
	if r.ResultObserved() {
		t.Error("should start false")
	}
	r.MarkResultObserved("turn_ended")
	if !r.ResultObserved() {
		t.Error("should be true after marking")
	}
}

func TestSessionRuntime_SendUserTurnResetsResultObserved(t *testing.T) {
	r := newTestRuntime("claude", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	r.MarkResultObserved("turn_ended")
	if !r.ResultObserved() {
		t.Fatal("should be observed")
	}

	r.SendUserTurn("follow up")
	if r.ResultObserved() {
		t.Error("resultObserved should be reset after sending a turn")
	}
}

func TestSessionRuntime_ClaudeResultDetection(t *testing.T) {
	r := newTestRuntime("claude", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdout := strings.NewReader(`{"type":"result","content":"done"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	time.Sleep(100 * time.Millisecond)

	if !r.ResultObserved() {
		t.Error("claude result event should set resultObserved")
	}
}

func TestSessionRuntime_CodexTurnCompleted(t *testing.T) {
	r := newTestRuntime("codex", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdout := strings.NewReader(`{"type":"turn.completed","status":"completed"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	time.Sleep(100 * time.Millisecond)

	if !r.ResultObserved() {
		t.Error("codex turn.completed should set resultObserved")
	}
}

func TestSessionRuntime_CodexTurnFailed(t *testing.T) {
	r := newTestRuntime("codex", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdout := strings.NewReader(`{"type":"turn.failed","error":"boom"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	time.Sleep(100 * time.Millisecond)

	if !r.ResultObserved() {
		t.Error("codex turn.failed should set resultObserved")
	}
	if !r.IsError() {
		t.Error("codex turn.failed should set isError")
	}
}

func TestSessionRuntime_CopilotTaskComplete(t *testing.T) {
	r := newTestRuntime("copilot", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdout := strings.NewReader(`{"type":"session.task_complete"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	time.Sleep(100 * time.Millisecond)

	if !r.ResultObserved() {
		t.Error("copilot session.task_complete should set resultObserved")
	}
}

func TestSessionRuntime_CopilotError(t *testing.T) {
	r := newTestRuntime("copilot", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdout := strings.NewReader(`{"type":"session.error","error":"fail"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	time.Sleep(100 * time.Millisecond)

	if !r.ResultObserved() {
		t.Error("copilot session.error should set resultObserved")
	}
	if !r.IsError() {
		t.Error("copilot session.error should set isError")
	}
}

func TestSessionRuntime_GeminiSuccess(t *testing.T) {
	r := newTestRuntime("gemini", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdout := strings.NewReader(`{"type":"result","status":"success"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	time.Sleep(100 * time.Millisecond)

	if !r.ResultObserved() {
		t.Error("gemini result with success status should set resultObserved")
	}
	if r.IsError() {
		t.Error("gemini success result should not set isError")
	}
}

func TestSessionRuntime_GeminiError(t *testing.T) {
	r := newTestRuntime("gemini", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdout := strings.NewReader(`{"type":"result","status":"error"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	time.Sleep(100 * time.Millisecond)

	if !r.ResultObserved() {
		t.Error("gemini result should set resultObserved even on error status")
	}
	if !r.IsError() {
		t.Error("gemini error result should set isError")
	}
}

func TestSessionRuntime_OpenCodeSessionIdle(t *testing.T) {
	r := newTestRuntime("opencode", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdout := strings.NewReader(`{"type":"session_idle"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	time.Sleep(100 * time.Millisecond)

	if !r.ResultObserved() {
		t.Error("opencode session_idle should set resultObserved")
	}
}

func TestSessionRuntime_OpenCodeStepFinishStopIsNotTurnEnd(t *testing.T) {
	r := newTestRuntime("opencode", true)

	var events []TerminalEvent
	done := make(chan struct{})
	go func() {
		for evt := range r.Events() {
			events = append(events, evt)
			if len(events) > 4 {
				close(done)
				return
			}
		}
	}()

	ctx := context.Background()
	stdout := strings.NewReader(
		`{"type":"step_finish","reason":"stop"}` + "\n" +
			`{"type":"session_idle"}` + "\n")
	stderr := strings.NewReader("")

	r.Start(ctx, stdout, stderr)

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
	}

	if !r.ResultObserved() {
		t.Error("session_idle should set resultObserved")
	}
}

func TestSessionRuntime_OpenCodeStepFinishError(t *testing.T) {
	r := newTestRuntime("opencode", true)

	var events []TerminalEvent
	done := make(chan struct{})
	go func() {
		for evt := range r.Events() {
			events = append(events, evt)
			if len(events) > 2 {
				close(done)
				return
			}
		}
	}()

	ctx := context.Background()
	stdout := strings.NewReader(`{"type":"step_finish","reason":"error"}` + "\n")
	stderr := strings.NewReader("")
	r.Start(ctx, stdout, stderr)

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
	}

	if !r.ResultObserved() {
		t.Error("step_finish with reason=error should set resultObserved")
	}
	if !r.IsError() {
		t.Error("step_finish with reason=error should set isError")
	}
}

func TestSessionRuntime_OnTurnEndedCallback(t *testing.T) {
	r := newTestRuntime("claude", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	turnEndedCalled := make(chan string, 1)
	r.SetOnTurnEnded(func(reason string) bool {
		turnEndedCalled <- reason
		return false
	})

	ctx := context.Background()
	stdout := strings.NewReader(`{"type":"result","content":"done"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)

	select {
	case reason := <-turnEndedCalled:
		if reason != "turn_ended" {
			t.Errorf("expected onTurnEnded called with turn_ended, got %s", reason)
		}
	case <-time.After(1 * time.Second):
		t.Error("onTurnEnded was not called")
	}
}

func TestSessionRuntime_OnTurnEndedPreventClose(t *testing.T) {
	r := newTestRuntime("claude", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	r.SetOnTurnEnded(func(reason string) bool {
		return true
	})

	ctx := context.Background()
	stdout := strings.NewReader(`{"type":"result"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	time.Sleep(200 * time.Millisecond)

	if r.StdinClosed() {
		t.Error("onTurnEnded returning true should prevent input close")
	}
}

func TestSessionRuntime_ClaudeAutoAnswer(t *testing.T) {
	r := newTestRuntime("claude", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	ctx := context.Background()
	stdout := strings.NewReader(`{"type":"tool_use","name":"AskUserQuestion","id":"tu-1","input":{"question":"continue?"}}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	time.Sleep(200 * time.Millisecond)

	written := pw.String()
	if !strings.Contains(written, "auto-response") {
		t.Errorf("claude should auto-answer AskUserQuestion, got: %s", written)
	}
	if !strings.Contains(written, "tu-1") {
		t.Errorf("auto-answer should reference tool_use_id, got: %s", written)
	}
}

func TestSessionRuntime_ClaudeAutoAnswerDedup(t *testing.T) {
	r := newTestRuntime("claude", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	ctx := context.Background()
	stdout := strings.NewReader(
		`{"type":"tool_use","name":"AskUserQuestion","id":"tu-1"}` + "\n" +
			`{"type":"tool_use","name":"AskUserQuestion","id":"tu-1"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	time.Sleep(200 * time.Millisecond)

	autoAnswerCount := strings.Count(pw.String(), "auto-response")
	if autoAnswerCount != 1 {
		t.Errorf("AskUserQuestion should be auto-answered exactly once, got %d", autoAnswerCount)
	}
}

func TestSessionRuntime_CopilotAutoAnswer(t *testing.T) {
	r := newTestRuntime("copilot", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	ctx := context.Background()
	stdout := strings.NewReader(`{"type":"user_input.requested"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	time.Sleep(200 * time.Millisecond)

	written := pw.String()
	if !strings.Contains(written, "auto-response") {
		t.Errorf("copilot should auto-answer user_input.requested, got: %s", written)
	}
}

func TestCloseDiagnostics_Normal(t *testing.T) {
	r := newTestRuntime("claude", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	ctx := context.Background()
	stdout := strings.NewReader(`{"type":"result"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	time.Sleep(200 * time.Millisecond)

	d := CaptureChildCloseDiagnostics(r, 0, "")
	if d.ExitReason != "turn_ended" {
		t.Errorf("expected turn_ended, got %s", d.ExitReason)
	}
	if d.LastEventType != "result" {
		t.Errorf("expected result, got %s", d.LastEventType)
	}
	if d.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", d.ExitCode)
	}
}

func TestCloseDiagnostics_NilRuntime(t *testing.T) {
	r := NewSessionRuntime("bead-1", "/repo")
	d := CaptureChildCloseDiagnostics(r, 1, "SIGTERM")
	if d.ExitReason != "normal" {
		t.Errorf("expected normal for nil exitReason, got %s", d.ExitReason)
	}
	if d.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", d.ExitCode)
	}
	if d.Signal != "SIGTERM" {
		t.Errorf("expected SIGTERM, got %s", d.Signal)
	}
}

func TestFormatDiagnosticsForLog(t *testing.T) {
	d := CloseDiagnostics{
		ExitReason:      "turn_ended",
		LastEventType:   "result",
		Signal:          "",
		ExitCode:        0,
		MsSinceLastStdout: 500,
		TurnError:       "",
	}
	logLine := FormatDiagnosticsForLog(d)
	if !strings.Contains(logLine, "exitReason=turn_ended") {
		t.Errorf("expected exitReason=turn_ended in log, got: %s", logLine)
	}
	if !strings.Contains(logLine, "lastEventType=result") {
		t.Errorf("expected lastEventType=result in log, got: %s", logLine)
	}
	if !strings.Contains(logLine, "msSinceLastStdout=500") {
		t.Errorf("expected msSinceLastStdout=500 in log, got: %s", logLine)
	}
	if !strings.Contains(logLine, "turnError=null") {
		t.Errorf("expected turnError=null in log, got: %s", logLine)
	}
}

func TestFormatDiagnosticsForLog_AllNull(t *testing.T) {
	d := CloseDiagnostics{
		ExitReason:      "normal",
		MsSinceLastStdout: -1,
	}
	logLine := FormatDiagnosticsForLog(d)
	if !strings.Contains(logLine, "msSinceLastStdout=null") {
		t.Errorf("expected null for -1, got: %s", logLine)
	}
	if !strings.Contains(logLine, "turnError=null") {
		t.Errorf("expected turnError=null, got: %s", logLine)
	}
}

func TestShouldTreatTurnEndedSignalAsClean(t *testing.T) {
	tests := []struct {
		name     string
		d        CloseDiagnostics
		expected bool
	}{
		{"clean exit with turn_ended", CloseDiagnostics{ExitReason: "turn_ended"}, true},
		{"clean exit with result event", CloseDiagnostics{LastEventType: "result"}, true},
		{"signal present", CloseDiagnostics{Signal: "SIGTERM"}, false},
		{"non-zero exit", CloseDiagnostics{ExitCode: 1}, false},
		{"turn.failed event", CloseDiagnostics{LastEventType: "turn.failed"}, false},
		{"turn error present", CloseDiagnostics{TurnError: "something failed"}, false},
		{"null signal no evidence", CloseDiagnostics{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldTreatTurnEndedSignalAsClean(tt.d)
			if got != tt.expected {
				t.Errorf("ShouldTreatTurnEndedSignalAsClean(%+v) = %v, want %v", tt.d, got, tt.expected)
			}
		})
	}
}

func TestSessionRuntime_StderrPassthrough(t *testing.T) {
	r := NewSessionRuntime("bead-1", "/repo")
	ctx := context.Background()

	stdout := strings.NewReader("")
	stderr := strings.NewReader("some error output")

	var events []TerminalEvent
	done := make(chan struct{})
	go func() {
		for evt := range r.Events() {
			events = append(events, evt)
			if len(events) > 0 {
				close(done)
				return
			}
		}
	}()

	r.Start(ctx, stdout, stderr)

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for stderr event")
	}

	found := false
	for _, evt := range events {
		if evt.Type == "stderr" && strings.Contains(evt.Content, "some error output") {
			found = true
		}
	}
	if !found {
		t.Error("stderr should be pushed as stderr events")
	}
}

func TestSessionRuntime_NonJSONStdout(t *testing.T) {
	r := NewSessionRuntime("bead-1", "/repo")
	ctx := context.Background()

	stdout := strings.NewReader("plain text output\n")
	stderr := strings.NewReader("")

	var events []TerminalEvent
	done := make(chan struct{})
	go func() {
		for evt := range r.Events() {
			events = append(events, evt)
			if len(events) > 0 {
				close(done)
				return
			}
		}
	}()

	r.Start(ctx, stdout, stderr)

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for stdout event")
	}

	found := false
	for _, evt := range events {
		if evt.Type == "stdout" && strings.Contains(evt.Content, "plain text") {
			found = true
		}
	}
	if !found {
		t.Error("non-JSON stdout should be pushed as raw stdout events")
	}
}

func TestSessionRuntime_OpenCodeSessionError(t *testing.T) {
	r := newTestRuntime("opencode", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	ctx := context.Background()
	stdout := strings.NewReader(`{"type":"session_error","error":"crash"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	time.Sleep(100 * time.Millisecond)

	if !r.ResultObserved() {
		t.Error("opencode session_error should set resultObserved")
	}
	if !r.IsError() {
		t.Error("opencode session_error should set isError")
	}
}

func TestSessionRuntime_TurnEndDedup(t *testing.T) {
	r := newTestRuntime("claude", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	turnEndedCount := 0
	r.SetOnTurnEnded(func(reason string) bool {
		turnEndedCount++
		return true
	})

	ctx := context.Background()
	stdout := strings.NewReader(
		`{"type":"result","content":"first"}` + "\n" +
			`{"type":"result","content":"second"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	time.Sleep(200 * time.Millisecond)

	r.MarkResultObserved("turn_ended")
	if turnEndedCount != 1 {
		t.Errorf("onTurnEnded should fire exactly once, fired %d times", turnEndedCount)
	}
}