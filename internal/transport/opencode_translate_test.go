package transport

import (
	"strings"
	"testing"
)

func TestTranslateOpenCodePartStepStart(t *testing.T) {
	out := TranslateOpenCodePart(map[string]any{"type": "step-start"})
	if len(out) != 1 {
		t.Fatalf("expected 1 event, got %d", len(out))
	}
	if out[0]["type"] != "step_start" {
		t.Errorf("expected step_start, got %v", out[0]["type"])
	}
}

func TestTranslateOpenCodePartText(t *testing.T) {
	out := TranslateOpenCodePart(map[string]any{"type": "text", "text": "hello"})
	if len(out) != 1 {
		t.Fatalf("expected 1 event, got %d", len(out))
	}
	if out[0]["type"] != "text" {
		t.Errorf("expected text, got %v", out[0]["type"])
	}
	part, _ := out[0]["part"].(map[string]any)
	if part["text"] != "hello" {
		t.Errorf("expected part.text=hello, got %v", part["text"])
	}
}

func TestTranslateOpenCodePartStepFinishWithReason(t *testing.T) {
	out := TranslateOpenCodePart(map[string]any{"type": "step-finish", "reason": "stop"})
	part, _ := out[0]["part"].(map[string]any)
	if part["reason"] != "stop" {
		t.Errorf("expected reason=stop, got %v", part["reason"])
	}
}

func TestTranslateOpenCodePartStepFinishDefaultReason(t *testing.T) {
	out := TranslateOpenCodePart(map[string]any{"type": "step-finish"})
	part, _ := out[0]["part"].(map[string]any)
	if part["reason"] != "stop" {
		t.Errorf("expected default reason=stop, got %v", part["reason"])
	}
}

func TestTranslateOpenCodePartUnknown(t *testing.T) {
	out := TranslateOpenCodePart(map[string]any{"type": "totally-unknown"})
	if len(out) != 0 {
		t.Errorf("expected empty for unknown part, got %d events", len(out))
	}
}

func TestTranslateOpenCodePartToolRunning(t *testing.T) {
	out := TranslateOpenCodePart(map[string]any{
		"type": "tool",
		"id":   "call_1",
		"tool": "bash",
		"state": map[string]any{
			"status": "running",
			"input":  map[string]any{"command": "ls -la"},
		},
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 event for running tool, got %d", len(out))
	}
	if out[0]["type"] != "tool_use" {
		t.Errorf("expected tool_use, got %v", out[0]["type"])
	}
	if out[0]["name"] != "bash" {
		t.Errorf("expected name=bash, got %v", out[0]["name"])
	}
	if out[0]["input"].(map[string]any)["command"] != "ls -la" {
		t.Errorf("expected input.command=ls -la")
	}
}

func TestTranslateOpenCodePartToolCompleted(t *testing.T) {
	out := TranslateOpenCodePart(map[string]any{
		"type": "tool",
		"id":   "call_2",
		"tool": "read",
		"state": map[string]any{
			"status": "completed",
			"input":  map[string]any{"file_path": "/tmp/x"},
			"output": "file contents",
		},
	})
	if len(out) != 2 {
		t.Fatalf("expected 2 events for completed tool, got %d", len(out))
	}
	if out[0]["type"] != "tool_use" {
		t.Errorf("first event expected tool_use, got %v", out[0]["type"])
	}
	if out[0]["id"] != "call_2" {
		t.Errorf("expected id=call_2, got %v", out[0]["id"])
	}
	if out[1]["type"] != "tool_result" {
		t.Errorf("second event expected tool_result, got %v", out[1]["type"])
	}
	if out[1]["tool_use_id"] != "call_2" {
		t.Errorf("expected tool_use_id=call_2, got %v", out[1]["tool_use_id"])
	}
}

func TestTranslateOpenCodePartToolPending(t *testing.T) {
	out := TranslateOpenCodePart(map[string]any{
		"type":  "tool",
		"id":    "call_3",
		"tool":  "edit",
		"state": map[string]any{"status": "pending"},
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 event for pending tool, got %d", len(out))
	}
	if out[0]["type"] != "tool_use" {
		t.Errorf("expected tool_use, got %v", out[0]["type"])
	}
	if out[0]["status"] != "pending" {
		t.Errorf("expected status=pending, got %v", out[0]["status"])
	}
}

func TestTranslateOpenCodePartReasoning(t *testing.T) {
	out := TranslateOpenCodePart(map[string]any{"type": "reasoning", "text": "thinking…"})
	if len(out) != 1 || out[0]["type"] != "reasoning" {
		t.Errorf("expected reasoning, got %v", out)
	}
	if out[0]["text"] != "thinking…" {
		t.Errorf("expected text=thinking…, got %v", out[0]["text"])
	}
}

func TestTranslateOpenCodePartFile(t *testing.T) {
	out := TranslateOpenCodePart(map[string]any{
		"type":     "file",
		"filename": "/tmp/a.png",
		"mime":     "image/png",
		"source":   "agent",
	})
	if len(out) != 1 || out[0]["type"] != "file" {
		t.Fatalf("expected file event")
	}
	if out[0]["filename"] != "/tmp/a.png" {
		t.Errorf("expected filename=/tmp/a.png, got %v", out[0]["filename"])
	}
	if out[0]["mime"] != "image/png" {
		t.Errorf("expected mime=image/png, got %v", out[0]["mime"])
	}
	if out[0]["source"] != "agent" {
		t.Errorf("expected source=agent, got %v", out[0]["source"])
	}
}

func TestTranslateOpenCodePartSnapshot(t *testing.T) {
	out := TranslateOpenCodePart(map[string]any{"type": "snapshot", "snapshot": "abc123"})
	if len(out) != 1 || out[0]["type"] != "snapshot" {
		t.Fatalf("expected snapshot event")
	}
	if out[0]["snapshot"] != "abc123" {
		t.Errorf("expected snapshot=abc123, got %v", out[0]["snapshot"])
	}
}

func TestTranslateOpenCodeEventPermissionAskedTopLevel(t *testing.T) {
	event := map[string]any{
		"type":      "permission.asked",
		"sessionID": "ses_1",
		"requestID": "req_1",
	}
	out := TranslateOpenCodeEvent(event)
	if len(out) != 1 {
		t.Fatalf("expected 1 event, got %d", len(out))
	}
	if out[0]["type"] != "permission.asked" {
		t.Errorf("expected permission.asked, got %v", out[0]["type"])
	}
	if out[0]["sessionID"] != "ses_1" {
		t.Errorf("expected sessionID=ses_1, got %v", out[0]["sessionID"])
	}
}

func TestTranslateOpenCodeEventPermissionUpdatedFromName(t *testing.T) {
	out := TranslateOpenCodeEvent(map[string]any{
		"event":     "permission.updated",
		"sessionID": "ses_1",
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 event, got %d", len(out))
	}
	if out[0]["type"] != "permission.updated" {
		t.Errorf("expected type=permission.updated, got %v", out[0]["type"])
	}
}

func TestTranslateOpenCodeEventMessagePartUpdatedTool(t *testing.T) {
	out := TranslateOpenCodeEvent(map[string]any{
		"type": "message.part.updated",
		"properties": map[string]any{
			"part": map[string]any{
				"type": "tool",
				"id":   "call_a",
				"tool": "grep",
				"state": map[string]any{
					"status": "completed",
					"input":  map[string]any{"pattern": "foo"},
					"output": "match",
				},
			},
		},
	})
	if len(out) != 2 {
		t.Fatalf("expected 2 events, got %d", len(out))
	}
	if out[0]["type"] != "tool_use" {
		t.Errorf("first event expected tool_use, got %v", out[0]["type"])
	}
	if out[1]["type"] != "tool_result" {
		t.Errorf("second event expected tool_result, got %v", out[1]["type"])
	}
}

func TestTranslateOpenCodeEventDataPartFallback(t *testing.T) {
	out := TranslateOpenCodeEvent(map[string]any{
		"type": "message.part.updated",
		"data": map[string]any{
			"part": map[string]any{"type": "text", "text": "hi"},
		},
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 event, got %d", len(out))
	}
	if out[0]["type"] != "text" {
		t.Errorf("expected text, got %v", out[0])
	}
}

func TestTranslateOpenCodeEventMessageUpdated(t *testing.T) {
	out := TranslateOpenCodeEvent(map[string]any{
		"type": "message.updated",
		"properties": map[string]any{
			"info": map[string]any{"time": map[string]any{"completed": 12345}},
		},
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 event, got %d", len(out))
	}
	if out[0]["type"] != "message_updated" {
		t.Errorf("expected message_updated, got %v", out[0]["type"])
	}
}

func TestTranslateOpenCodeEventStepUpdated(t *testing.T) {
	out := TranslateOpenCodeEvent(map[string]any{
		"type": "step.updated",
		"properties": map[string]any{
			"step": map[string]any{"name": "review", "status": "running"},
		},
	})
	if len(out) != 1 || out[0]["type"] != "step_updated" {
		t.Fatalf("expected step_updated")
	}
	step, _ := out[0]["step"].(map[string]any)
	if step["name"] != "review" {
		t.Errorf("expected step.name=review, got %v", step["name"])
	}
}

func TestTranslateOpenCodeEventSessionIdle(t *testing.T) {
	out := TranslateOpenCodeEvent(map[string]any{
		"type": "session.idle",
		"properties": map[string]any{
			"sessionID": "ses_idle_1",
		},
	})
	if len(out) != 1 || out[0]["type"] != "session_idle" {
		t.Fatalf("expected session_idle")
	}
	if out[0]["sessionID"] != "ses_idle_1" {
		t.Errorf("expected sessionID=ses_idle_1, got %v", out[0]["sessionID"])
	}
}

func TestTranslateOpenCodeEventSessionStatusIdle(t *testing.T) {
	out := TranslateOpenCodeEvent(map[string]any{
		"type": "session.status",
		"properties": map[string]any{
			"sessionID": "ses_status_1",
			"status":    map[string]any{"type": "idle"},
		},
	})
	if len(out) != 1 || out[0]["type"] != "session_idle" {
		t.Fatalf("expected session_idle from session.status idle, got %v", out)
	}
}

func TestTranslateOpenCodeEventSessionStatusBusy(t *testing.T) {
	out := TranslateOpenCodeEvent(map[string]any{
		"type": "session.status",
		"properties": map[string]any{
			"sessionID": "ses_status_2",
			"status":    map[string]any{"type": "busy"},
		},
	})
	if len(out) != 0 {
		t.Errorf("expected no events for session.status busy, got %d", len(out))
	}
}

func TestTranslateOpenCodeEventSessionError(t *testing.T) {
	out := TranslateOpenCodeEvent(map[string]any{
		"type": "session.error",
		"properties": map[string]any{
			"error": map[string]any{"message": "rate limited"},
		},
	})
	if len(out) != 1 || out[0]["type"] != "session_error" {
		t.Fatalf("expected session_error")
	}
	if out[0]["message"] != "rate limited" {
		t.Errorf("expected message=rate limited, got %v", out[0]["message"])
	}
}

func TestTranslateOpenCodeEventUnknown(t *testing.T) {
	out := TranslateOpenCodeEvent(map[string]any{"type": "totally.new"})
	if len(out) != 0 {
		t.Errorf("expected empty for unknown SSE envelope, got %d", len(out))
	}
}

func TestTranslateOpenCodeEventNonObject(t *testing.T) {
	out := TranslateOpenCodeEvent("not an object")
	if len(out) != 0 {
		t.Errorf("expected empty for non-object, got %d", len(out))
	}
	out = TranslateOpenCodeEvent(nil)
	if len(out) != 0 {
		t.Errorf("expected empty for nil, got %d", len(out))
	}
}

func TestTranslateOpenCodeResponseMixedParts(t *testing.T) {
	out := TranslateOpenCodeResponse(map[string]any{
		"parts": []any{
			map[string]any{"type": "step-start"},
			map[string]any{"type": "text", "text": "ok"},
			map[string]any{
				"type": "tool",
				"id":   "t1",
				"tool": "bash",
				"state": map[string]any{
					"status": "completed",
					"input":  map[string]any{"command": "echo"},
					"output": "echo\n",
				},
			},
			map[string]any{"type": "step-finish", "reason": "stop"},
		},
	})
	types := make([]string, len(out))
	for i, e := range out {
		types[i] = e["type"].(string)
	}
	expected := []string{"step_start", "text", "tool_use", "tool_result", "step_finish"}
	if len(types) != len(expected) {
		t.Fatalf("expected %d events, got %d: %v", len(expected), len(types), types)
	}
	for i, want := range expected {
		if types[i] != want {
			t.Errorf("event[%d]: expected %s, got %s", i, want, types[i])
		}
	}
}

func TestTranslateOpenCodeResponseWithEvents(t *testing.T) {
	out := TranslateOpenCodeResponse(map[string]any{
		"parts": []any{
			map[string]any{"type": "text", "text": "hi"},
		},
		"events": []any{
			map[string]any{
				"type":       "session.idle",
				"properties": map[string]any{"sessionID": "X"},
			},
		},
	})
	types := make([]string, len(out))
	for i, e := range out {
		types[i] = e["type"].(string)
	}
	expected := []string{"text", "session_idle"}
	if len(types) != len(expected) {
		t.Fatalf("expected %d events, got %d: %v", len(expected), len(types), types)
	}
	for i, want := range expected {
		if types[i] != want {
			t.Errorf("event[%d]: expected %s, got %s", i, want, types[i])
		}
	}
}

func TestHasOpenCodeMessagePayload(t *testing.T) {
	if !HasOpenCodeMessagePayload(map[string]any{"parts": []any{}}) {
		t.Error("expected true for parts present")
	}
	if !HasOpenCodeMessagePayload(map[string]any{"events": []any{}}) {
		t.Error("expected true for events present")
	}
	if !HasOpenCodeMessagePayload(map[string]any{"type": "permission.asked"}) {
		t.Error("expected true for permission event")
	}
	if HasOpenCodeMessagePayload(map[string]any{}) {
		t.Error("expected false for empty record")
	}
}

func TestTranslateOpenCodePartPermissionInPart(t *testing.T) {
	out := TranslateOpenCodePart(map[string]any{
		"type":  "permission.asked",
		"event": "permission.asked",
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 permission event, got %d", len(out))
	}
	if out[0]["type"] != "permission.asked" {
		t.Errorf("expected permission.asked, got %v", out[0]["type"])
	}
}

func TestTranslateOpenCodePartPermissionUpdatedInPart(t *testing.T) {
	out := TranslateOpenCodePart(map[string]any{
		"type": "permission.updated",
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 permission event, got %d", len(out))
	}
	if out[0]["type"] != "permission.updated" {
		t.Errorf("expected permission.updated, got %v", out[0]["type"])
	}
}

func TestTranslateOpenCodeEventPermissionFromPart(t *testing.T) {
	out := TranslateOpenCodeEvent(map[string]any{
		"type": "message.part.updated",
		"part": map[string]any{
			"type": "permission.asked",
		},
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 event, got %d", len(out))
	}
	if out[0]["type"] != "permission.asked" {
		t.Errorf("expected permission.asked, got %v", out[0]["type"])
	}
}

func TestTranslateOpenCodeEventSessionErrorDefaultMessage(t *testing.T) {
	out := TranslateOpenCodeEvent(map[string]any{
		"type": "session.error",
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 event")
	}
	if out[0]["message"] != "OpenCode session error" {
		t.Errorf("expected default message, got %v", out[0]["message"])
	}
}

func TestTranslateOpenCodeEventSessionStatusFromTopLevel(t *testing.T) {
	out := TranslateOpenCodeEvent(map[string]any{
		"type":   "session.status",
		"status": map[string]any{"type": "idle"},
	})
	if len(out) != 1 || out[0]["type"] != "session_idle" {
		t.Errorf("expected session_idle from top-level session.status, got %v", out)
	}
}

// --- Formatter tests ---

func stripAnsi(s string) string {
	result := make([]rune, 0, len(s))
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
				inEscape = false
			}
			continue
		}
		result = append(result, r)
	}
	return string(result)
}

func TestFormatOpenCodeEventReasoning(t *testing.T) {
	ev := FormatOpenCodeEvent(map[string]any{"type": "reasoning", "text": "thinking…"})
	if ev == nil {
		t.Fatal("expected non-nil event")
	}
	if !ev.IsDetail {
		t.Error("expected IsDetail=true for reasoning")
	}
	plain := stripAnsi(ev.Text)
	if !strings.Contains(plain, "thinking…") {
		t.Errorf("expected thinking… in text, got %s", plain)
	}
}

func TestFormatOpenCodeEventReasoningEmpty(t *testing.T) {
	ev := FormatOpenCodeEvent(map[string]any{"type": "reasoning", "text": ""})
	if ev != nil {
		t.Error("expected nil for empty reasoning")
	}
}

func TestFormatOpenCodeEventStepUpdated(t *testing.T) {
	ev := FormatOpenCodeEvent(map[string]any{
		"type": "step_updated",
		"step": map[string]any{"name": "review", "status": "running"},
	})
	if ev == nil {
		t.Fatal("expected non-nil event")
	}
	plain := stripAnsi(ev.Text)
	if !strings.Contains(plain, "step review running") {
		t.Errorf("expected 'step review running' in text, got %s", plain)
	}
}

func TestFormatOpenCodeEventStepUpdatedEmpty(t *testing.T) {
	ev := FormatOpenCodeEvent(map[string]any{
		"type": "step_updated",
		"step": map[string]any{},
	})
	if ev != nil {
		t.Error("expected nil for empty step")
	}
}

func TestFormatOpenCodeEventSessionIdle(t *testing.T) {
	ev := FormatOpenCodeEvent(map[string]any{
		"type":      "session_idle",
		"sessionID": "ses_X",
	})
	plain := stripAnsi(ev.Text)
	if !strings.Contains(plain, "session idle ses_X") {
		t.Errorf("expected 'session idle ses_X' in text, got %s", plain)
	}
}

func TestFormatOpenCodeEventSessionError(t *testing.T) {
	ev := FormatOpenCodeEvent(map[string]any{
		"type":    "session_error",
		"message": "rate limited",
	})
	if ev.IsDetail {
		t.Error("expected IsDetail=false for session_error")
	}
	plain := stripAnsi(ev.Text)
	if !strings.Contains(plain, "rate limited") {
		t.Errorf("expected 'rate limited' in text, got %s", plain)
	}
}

func TestFormatOpenCodeEventFile(t *testing.T) {
	ev := FormatOpenCodeEvent(map[string]any{
		"type":     "file",
		"filename": "/tmp/x.png",
		"mime":     "image/png",
	})
	plain := stripAnsi(ev.Text)
	if !strings.Contains(plain, "/tmp/x.png") {
		t.Errorf("expected filename in text, got %s", plain)
	}
	if !strings.Contains(plain, "image/png") {
		t.Errorf("expected mime in text, got %s", plain)
	}
}

func TestFormatOpenCodeEventFileNoFilename(t *testing.T) {
	ev := FormatOpenCodeEvent(map[string]any{
		"type":     "file",
		"filename": "",
	})
	if ev != nil {
		t.Error("expected nil for file without filename")
	}
}

func TestFormatOpenCodeEventSnapshot(t *testing.T) {
	long := strings.Repeat("a", 100)
	ev := FormatOpenCodeEvent(map[string]any{
		"type":     "snapshot",
		"snapshot": long,
	})
	plain := stripAnsi(ev.Text)
	if !strings.Contains(plain, "snapshot ") {
		t.Errorf("expected 'snapshot ' in text, got %s", plain)
	}
	if len(plain) >= len(long) {
		t.Errorf("expected truncation, got len=%d >= original=%d", len(plain), len(long))
	}
}

func TestFormatOpenCodeEventMessageUpdatedCompleted(t *testing.T) {
	ev := FormatOpenCodeEvent(map[string]any{
		"type": "message_updated",
		"info": map[string]any{"time": map[string]any{"completed": 999}},
	})
	if ev == nil {
		t.Fatal("expected non-nil event")
	}
	plain := stripAnsi(ev.Text)
	if !strings.Contains(plain, "turn complete") {
		t.Errorf("expected 'turn complete' in text, got %s", plain)
	}
}

func TestFormatOpenCodeEventMessageUpdatedNoCompleted(t *testing.T) {
	ev := FormatOpenCodeEvent(map[string]any{
		"type": "message_updated",
		"info": map[string]any{"time": map[string]any{}},
	})
	if ev != nil {
		t.Error("expected nil for message_updated without completed")
	}
}

func TestFormatOpenCodeEventUnknownType(t *testing.T) {
	ev := FormatOpenCodeEvent(map[string]any{
		"type": "tool_use",
		"name": "Bash",
	})
	if ev != nil {
		t.Error("expected nil for unrelated event type")
	}
}

func TestFormatOpenCodeEventNilType(t *testing.T) {
	ev := FormatOpenCodeEvent(map[string]any{})
	if ev != nil {
		t.Error("expected nil for empty type")
	}
}

func TestClip(t *testing.T) {
	short := "abc"
	if clip(short, 64) != short {
		t.Errorf("expected short string unchanged")
	}
	long := strings.Repeat("x", 100)
	result := clip(long, 64)
	if !strings.HasSuffix(result, "...") {
		t.Errorf("expected truncation with ...")
	}
	if len(result) > 67 {
		t.Errorf("expected clipped string <= 67, got %d", len(result))
	}
}
