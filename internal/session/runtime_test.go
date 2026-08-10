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

func TestCapabilitiesForDialect_ClaudeInteractive(t *testing.T) {
	caps := CapabilitiesForDialect("claude", true)
	if !caps.Interactive {
		t.Error("claude interactive should be interactive")
	}
	if caps.PromptTransport != TransportStdioStreamJSON {
		t.Errorf("expected stdin-stream-json, got %s", caps.PromptTransport)
	}
	if !caps.SupportsFollowUp {
		t.Error("claude interactive should support follow-up")
	}
	if !caps.SupportsAskUserAutoResp {
		t.Error("claude interactive should support AskUser auto-response")
	}
	if caps.StdinDrainPolicy != DrainCloseAfterResult {
		t.Errorf("expected close-after-result, got %s", caps.StdinDrainPolicy)
	}
	if caps.ResultDetection != ResultDetectionTypeResult {
		t.Errorf("expected type-result, got %s", caps.ResultDetection)
	}
}

// TestCapabilitiesForDialect_ClaudeOneShot pins the profile actually used by
// the take-loop: claude dispatched with `-p <prompt>` on the CLI arg
// (adapter.BuildClaudePromptModeArgs), which never opens the stdin-stream-json
// channel the interactive profile relies on for a follow-up.
func TestCapabilitiesForDialect_ClaudeOneShot(t *testing.T) {
	caps := CapabilitiesForDialect("claude", false)
	if caps.Interactive {
		t.Error("claude one-shot should not be interactive")
	}
	if caps.PromptTransport != TransportCLIArg {
		t.Errorf("expected cli-arg, got %s", caps.PromptTransport)
	}
	if caps.SupportsFollowUp {
		t.Error("claude one-shot should not support follow-up - -p <prompt> never opens a channel to deliver one")
	}
	if caps.SupportsAskUserAutoResp {
		t.Error("claude one-shot should not support AskUser auto-response - no stdin to write to")
	}
	if caps.StdinDrainPolicy != DrainNeverOpened {
		t.Errorf("expected never-opened, got %s", caps.StdinDrainPolicy)
	}
	if !caps.SupportsInteractive {
		t.Error("claude supports interactive mode")
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
		OnFailed:    func(transport string, err error) { failed = true },
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

// TestSessionRuntime_ClaudeResultCleanIsNotError pins the clean-completion
// half of claude's is_error handling: a result with is_error:false (or the
// field simply absent, as in TestSessionRuntime_ClaudeResultDetection above)
// must never mark the turn as failed. This is the case the take-loop's
// follow-up gate (internal/terminal.HandleTakeLoopTurnEnded) relies on to
// tell a stage that finished from one that didn't.
func TestSessionRuntime_ClaudeResultCleanIsNotError(t *testing.T) {
	r := newTestRuntime("claude", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdout := strings.NewReader(`{"type":"result","is_error":false,"result":"done"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	r.WaitDrained()

	if !r.ResultObserved() {
		t.Error("claude result event should set resultObserved")
	}
	if r.IsError() {
		t.Error("claude result with is_error:false should not set isError")
	}
}

// TestSessionRuntime_ClaudeResultErrorSetsIsError closes the gap this fix
// found: processClaudeEvent used to discard the result event's own is_error
// field entirely, so a claude turn that genuinely failed (max turns hit, a
// tool error surfaced as the terminal event, etc.) was indistinguishable
// from a clean completion at the exact call site that needs to tell them
// apart (internal/terminal.HandleTakeLoopTurnEnded via TakeLoopContext.
// TurnFailed). codex, gemini, copilot and opencode already set isError from
// their own dialect-native failure signal; claude alone did not.
func TestSessionRuntime_ClaudeResultErrorSetsIsError(t *testing.T) {
	r := newTestRuntime("claude", true)
	pw := &pipeWriter{}
	r.SetStdin(pw)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdout := strings.NewReader(`{"type":"result","is_error":true,"result":"error_max_turns"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	r.WaitDrained()

	if !r.ResultObserved() {
		t.Error("claude result event should set resultObserved even when is_error is true")
	}
	if !r.IsError() {
		t.Error("claude result with is_error:true should set isError")
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

// TestSessionRuntime_WaitDrainedBlocksUntilReadersFinish pins the mechanism a
// caller like RunBead depends on to avoid the exec.Cmd StdoutPipe/StderrPipe
// race: WaitDrained must not return before both readStdout and readStderr
// have consumed their stream to EOF.
func TestSessionRuntime_WaitDrainedBlocksUntilReadersFinish(t *testing.T) {
	r := newTestRuntime("claude", false)

	ctx := context.Background()
	stdout := strings.NewReader(`{"type":"result"}` + "\n")
	stderr := strings.NewReader("")

	r.Start(ctx, stdout, stderr)

	drained := make(chan struct{})
	go func() {
		r.WaitDrained()
		close(drained)
	}()

	select {
	case <-drained:
	case <-time.After(1 * time.Second):
		t.Fatal("WaitDrained did not return after both readers reached EOF")
	}
}

// TestSessionRuntime_WaitDrainedHappensAfterOnTurnEnded pins the ordering
// guarantee the take-loop's follow-up gate depends on: readStdout runs
// onTurnEnded synchronously before it returns, so by the time WaitDrained
// unblocks, any turn-ending event in the stream has already been handled.
// A caller that reaps the process (proc.Wait()) only after WaitDrained can
// therefore trust that a follow-up refusal, if any, has already been
// recorded - this is what makes the RunBead-level fix for a dialect with no
// follow-up path deterministic instead of racing process exit.
func TestSessionRuntime_WaitDrainedHappensAfterOnTurnEnded(t *testing.T) {
	r := newTestRuntime("claude", false)

	var mu sync.Mutex
	var turnEndedCalled bool
	r.SetOnTurnEnded(func(reason string) bool {
		mu.Lock()
		turnEndedCalled = true
		mu.Unlock()
		return false
	})

	ctx := context.Background()
	stdout := strings.NewReader(`{"type":"result"}` + "\n")
	stderr := strings.NewReader("")

	r.Start(ctx, stdout, stderr)

	drained := make(chan struct{})
	go func() {
		r.WaitDrained()
		close(drained)
	}()

	select {
	case <-drained:
	case <-time.After(1 * time.Second):
		t.Fatal("WaitDrained did not return")
	}

	mu.Lock()
	defer mu.Unlock()
	if !turnEndedCalled {
		t.Error("expected onTurnEnded to have already run by the time WaitDrained returned")
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
		ExitReason:        "turn_ended",
		LastEventType:     "result",
		Signal:            "",
		ExitCode:          0,
		MsSinceLastStdout: 500,
		TurnError:         "",
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
		ExitReason:        "normal",
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

func TestSessionRuntime_PiAgentSettledEndsTheTurn(t *testing.T) {
	r := newTestRuntime("pi", false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The real order from `pi -p --mode json`: the session line first, the
	// per-turn boundary next, and agent_settled last.
	stdout := strings.NewReader(
		`{"type":"session","version":3,"id":"019fbd83-f97c-7137-aa40-7ff3f1d2445e"}` + "\n" +
			`{"type":"turn_end"}` + "\n" +
			`{"type":"agent_settled"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	time.Sleep(100 * time.Millisecond)

	if !r.ResultObserved() {
		t.Error("pi agent_settled should set resultObserved")
	}
}

func TestSessionRuntime_PiTurnEndAloneIsNotTheEnd(t *testing.T) {
	r := newTestRuntime("pi", false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// turn_end closes a turn, not the process: pi can open another one. A
	// stage treated as finished here would be read as complete while the
	// agent is still working.
	stdout := strings.NewReader(`{"type":"turn_end"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	time.Sleep(100 * time.Millisecond)

	if r.ResultObserved() {
		t.Error("pi turn_end alone should not set resultObserved")
	}
}

func TestSessionRuntime_PiCapturesItsSessionID(t *testing.T) {
	r := newTestRuntime("pi", false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdout := strings.NewReader(
		`{"type":"session","version":3,"id":"019fbd83-f97c-7137-aa40-7ff3f1d2445e"}` + "\n" +
			`{"type":"agent_settled"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	time.Sleep(100 * time.Millisecond)

	if got := r.CapturedSessionID(); got != "019fbd83-f97c-7137-aa40-7ff3f1d2445e" {
		t.Errorf("CapturedSessionID() = %q, want pi's own session id", got)
	}
}

// The stream shape here is trimmed from a real `pi -p --mode json` run: an
// agent_end carrying the conversation's usage, then the empty agent_end pi
// emits per internal retry, then agent_settled. It guards two gaps at once -
// pi's usage was parsed correctly and never logged, because processPiEvent
// read only the last line; and once it was logged, the retry envelope
// overwrote it with zero, which recorded a million-token run as free.
func TestSessionRuntime_PiAgentEndReachesTheTokenLogger(t *testing.T) {
	r := newTestRuntime("pi", false)
	logger := NewCapturingUsageLogger()
	r.SetTokenUsageLogger(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdout := strings.NewReader(
		`{"type":"session","version":3,"id":"019fbd83-f97c-7137-aa40-7ff3f1d2445e"}` + "\n" +
			`{"type":"agent_end","willRetry":false,"messages":[` +
			`{"model":"kimi-k2.7","usage":{"input":100,"output":7,"cacheRead":0,"cacheWrite":0,"reasoning":3,"totalTokens":107,"cost":{"total":0}}},` +
			`{"model":"kimi-k2.7","usage":{"input":40,"output":5,"cacheRead":0,"cacheWrite":0,"reasoning":1,"totalTokens":45,"cost":{"total":0}}}` +
			`]}` + "\n" +
			`{"type":"agent_end","willRetry":true,"messages":[` +
			`{"model":"kimi-k2.7","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"reasoning":0,"totalTokens":0,"cost":{"total":0}}}` +
			`]}` + "\n" +
			`{"type":"agent_settled"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	time.Sleep(100 * time.Millisecond)

	usage := logger.Usage()
	if usage == nil {
		t.Fatal("pi agent_end should reach the token logger, got no usage")
	}
	// Summed across both messages, not read off the last one: a multi-step
	// run would otherwise report one model call as the whole stage's cost.
	if usage.InputTokens != 140 || usage.OutputTokens != 12 || usage.TotalTokens != 152 {
		t.Errorf("usage = in %d / out %d / total %d, want 140 / 12 / 152",
			usage.InputTokens, usage.OutputTokens, usage.TotalTokens)
	}
	if usage.Model == nil || *usage.Model != "kimi-k2.7" {
		t.Errorf("Model = %v, want the identifier pi reported so the ledger row is modelResolved", usage.Model)
	}
	if usage.ReasoningTokens == nil || *usage.ReasoningTokens != 4 {
		t.Errorf("ReasoningTokens = %v, want 4 - pi reports reasoning and the ledger has a column for it", usage.ReasoningTokens)
	}
}

// A dispatch that stays under the ceiling must be untouched: the guard only
// exists for the runaway case, and a stage stopped early is a stage whose
// work is thrown away.
func TestSessionRuntime_TurnsUnderTheCeilingAreNotStopped(t *testing.T) {
	r := newTestRuntime("pi", false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var stream strings.Builder
	for i := 0; i < MaxTurnsPerDispatch; i++ {
		stream.WriteString(`{"type":"turn_end"}` + "\n")
	}
	stream.WriteString(`{"type":"agent_settled"}` + "\n")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, strings.NewReader(stream.String()), strings.NewReader(""))
	time.Sleep(200 * time.Millisecond)

	if exceeded, turns := r.TurnCeilingExceeded(); exceeded {
		t.Errorf("TurnCeilingExceeded() = true at %d turns, want false at exactly the limit", turns)
	}
	if !r.ResultObserved() {
		t.Error("the run should have ended normally on agent_settled")
	}
}

// The run that motivated this crossed 422 turns while producing output the
// whole time, which is precisely why the silence-based watchdog never fired.
func TestSessionRuntime_TurnCeilingStopsARunawayDispatch(t *testing.T) {
	r := newTestRuntime("pi", false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var stream strings.Builder
	for i := 0; i < MaxTurnsPerDispatch+40; i++ {
		stream.WriteString(`{"type":"turn_end"}` + "\n")
	}

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, strings.NewReader(stream.String()), strings.NewReader(""))
	time.Sleep(200 * time.Millisecond)

	exceeded, turns := r.TurnCeilingExceeded()
	if !exceeded {
		t.Fatalf("TurnCeilingExceeded() = false after %d turns, want true", turns)
	}
	if turns <= MaxTurnsPerDispatch {
		t.Errorf("turns = %d, want more than the %d limit", turns, MaxTurnsPerDispatch)
	}
	if !r.IsError() {
		t.Error("a dispatch stopped for looping must be marked an error, not a clean finish")
	}
	if !strings.Contains(r.LastError(), "turn ceiling exceeded") {
		t.Errorf("LastError() = %q, want it to name the turn ceiling rather than a signal", r.LastError())
	}
}

// opencode's step_finish is a mid-run boundary exactly like pi's turn_end,
// so it is counted too; session_idle is what ends that dialect's dispatch.
func TestSessionRuntime_TurnCeilingCountsOpenCodeSteps(t *testing.T) {
	r := newTestRuntime("opencode", false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var stream strings.Builder
	for i := 0; i < MaxTurnsPerDispatch+10; i++ {
		stream.WriteString(`{"type":"step_finish","reason":"stop"}` + "\n")
	}

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, strings.NewReader(stream.String()), strings.NewReader(""))
	time.Sleep(200 * time.Millisecond)

	if exceeded, _ := r.TurnCeilingExceeded(); !exceeded {
		t.Error("opencode step_finish should count toward the turn ceiling")
	}
}

// claude's result and codex's turn.completed END the dispatch, so counting
// them could never reach a ceiling. Pinning that keeps a later change from
// "harmonising" the dialects and making a one-shot run trip the guard.
func TestSessionRuntime_TerminalDialectsNeverTripTheCeiling(t *testing.T) {
	for _, tc := range []struct{ dialect, line string }{
		{"claude", `{"type":"result","usage":{"input_tokens":1,"output_tokens":1}}`},
		{"codex", `{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}`},
	} {
		r := newTestRuntime(tc.dialect, false)
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			for evt := range r.Events() {
				_ = evt
			}
		}()

		r.Start(ctx, strings.NewReader(tc.line+"\n"), strings.NewReader(""))
		time.Sleep(100 * time.Millisecond)

		if exceeded, turns := r.TurnCeilingExceeded(); exceeded {
			t.Errorf("%s: TurnCeilingExceeded() = true at %d turns, want false", tc.dialect, turns)
		}
		cancel()
	}
}

// CountedTurns is what persists the live count into the stage-attempt
// ledger (see internal/app.RunBeadResult.Turns). Tool calls and plain
// messages vastly outnumber turn boundaries in a real stream, so the count
// must track boundaries crossed, not lines seen.
func TestSessionRuntime_CountedTurnsIgnoresToolAndMessageVolume(t *testing.T) {
	r := newTestRuntime("pi", false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var stream strings.Builder
	for i := 0; i < 3; i++ {
		for j := 0; j < 5; j++ {
			stream.WriteString(`{"type":"tool_use","name":"Read"}` + "\n")
			stream.WriteString(`{"type":"message","role":"assistant"}` + "\n")
		}
		stream.WriteString(`{"type":"turn_end"}` + "\n")
	}
	stream.WriteString(`{"type":"agent_settled"}` + "\n")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, strings.NewReader(stream.String()), strings.NewReader(""))
	time.Sleep(200 * time.Millisecond)

	turns, counted := r.CountedTurns()
	if !counted {
		t.Fatal("pi should count turn boundaries")
	}
	if turns != 3 {
		t.Errorf("CountedTurns() turns = %d, want 3 - 30 tool/message lines must not inflate the count", turns)
	}
}

// A dialect whose per-turn boundary IS the end of the run (claude, codex,
// copilot, gemini) or which emits no event stream at all (agy) reports no
// turn count - counted must be false, never a fabricated number.
func TestSessionRuntime_CountedTurnsUnsupportedDialectStaysUncounted(t *testing.T) {
	for _, dialect := range []string{"claude", "codex", "copilot", "gemini", "agy"} {
		r := newTestRuntime(dialect, false)
		if _, counted := r.CountedTurns(); counted {
			t.Errorf("%s: CountedTurns() counted = true, want false - this dialect's turn boundary is the end of the run", dialect)
		}
	}
}

// A run that genuinely crossed zero counted boundaries must report turns=0,
// counted=true - distinct from an unsupported dialect's turns=0,
// counted=false. Collapsing the two would persist a fabricated 0 for a
// dialect that never measured anything, or drop a real zero as if it were
// unmeasured.
func TestSessionRuntime_CountedTurnsMeasuredZeroIsNotUncounted(t *testing.T) {
	for _, tc := range []struct{ dialect, line string }{
		{"pi", `{"type":"agent_settled"}`},
		{"opencode", `{"type":"session_idle"}`},
	} {
		r := newTestRuntime(tc.dialect, false)
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			for evt := range r.Events() {
				_ = evt
			}
		}()

		r.Start(ctx, strings.NewReader(tc.line+"\n"), strings.NewReader(""))
		time.Sleep(100 * time.Millisecond)

		turns, counted := r.CountedTurns()
		if !counted {
			t.Errorf("%s: counted = false, want true even with zero boundaries crossed", tc.dialect)
		}
		if turns != 0 {
			t.Errorf("%s: turns = %d, want 0", tc.dialect, turns)
		}
		cancel()
	}
}

func TestSessionRuntime_PiErrorSetsIsErrorAndLastError(t *testing.T) {
	r := newTestRuntime("pi", false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdout := strings.NewReader(
		`{"type":"session","version":3,"id":"019fc4ff-74ee-79d1-910b-734b253f6668"}` + "\n" +
			`{"type":"message","id":"9d37a587","message":{"role":"assistant","stopReason":"error","errorMessage":"429: litellm.RateLimitError: Model rate limit exceeded"}}` + "\n" +
			`{"type":"agent_settled"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	r.WaitDrained()

	if !r.IsError() {
		t.Error("pi message with stopReason: error should set IsError()")
	}
	if !strings.Contains(r.LastError(), "RateLimitError") {
		t.Errorf("LastError() = %q, want it to contain RateLimitError", r.LastError())
	}
}

func TestSessionRuntime_PiTransientErrorFollowedByStopIsSuccess(t *testing.T) {
	r := newTestRuntime("pi", false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdout := strings.NewReader(
		`{"type":"session","version":3,"id":"019fbea3-abd9-7000-8000-000000000000"}` + "\n" +
			`{"type":"message","id":"msg1","message":{"role":"assistant","stopReason":"toolUse"}}` + "\n" +
			`{"type":"message","id":"msg2","message":{"role":"assistant","stopReason":"error","errorMessage":"429: litellm.RateLimitError: Model rate limit exceeded"}}` + "\n" +
			`{"type":"message","id":"msg3","message":{"role":"assistant","stopReason":"toolUse"}}` + "\n" +
			`{"type":"message","id":"msg4","message":{"role":"assistant","stopReason":"stop"}}` + "\n" +
			`{"type":"agent_settled"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	r.WaitDrained()

	if r.IsError() {
		t.Errorf("pi session with transient 429 that ended in stopReason: stop should NOT set IsError(), got LastError=%q", r.LastError())
	}
}

func TestSessionRuntime_PiUnfinishedToolUseIsFailure(t *testing.T) {
	r := newTestRuntime("pi", false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdout := strings.NewReader(
		`{"type":"session","version":3,"id":"019fbea3-abd9-7000-8000-000000000000"}` + "\n" +
			`{"type":"message","id":"msg1","message":{"role":"assistant","stopReason":"toolUse"}}` + "\n" +
			`{"type":"agent_settled"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	r.WaitDrained()

	if !r.IsError() {
		t.Error("pi session ending in stopReason: toolUse without stop should set IsError()")
	}
	if !strings.Contains(r.LastError(), "pi agent_incomplete") {
		t.Errorf("LastError() = %q, want it to contain pi agent_incomplete", r.LastError())
	}
}

func TestSessionRuntime_PiAbortedIsFailure(t *testing.T) {
	r := newTestRuntime("pi", false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdout := strings.NewReader(
		`{"type":"session","version":3,"id":"019fbea3-abd9-7000-8000-000000000000"}` + "\n" +
			`{"type":"message","id":"msg1","message":{"role":"assistant","stopReason":"aborted"}}` + "\n" +
			`{"type":"agent_settled"}` + "\n")
	stderr := strings.NewReader("")

	go func() {
		for evt := range r.Events() {
			_ = evt
		}
	}()

	r.Start(ctx, stdout, stderr)
	r.WaitDrained()

	if !r.IsError() {
		t.Error("pi session ending in stopReason: aborted should set IsError()")
	}
	if !strings.Contains(r.LastError(), "pi agent_aborted") {
		t.Errorf("LastError() = %q, want it to contain pi agent_aborted", r.LastError())
	}
}
