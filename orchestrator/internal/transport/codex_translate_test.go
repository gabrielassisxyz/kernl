package transport

import (
	"strings"
	"testing"
)

func stripANSI(s string) string {
	var b strings.Builder
	esc := false
	for _, r := range s {
		if r == '\x1b' {
			esc = true
			continue
		}
		if esc {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
				esc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ── Section 7: Codex Event Formatting ──────────────────────

func TestFormatCodexEvent_TurnStarted(t *testing.T) {
	result := FormatCodexEvent(map[string]any{"type": "turn.started"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsDetail {
		t.Error("turn.started should be detail")
	}
	if !strings.Contains(stripANSI(result.Text), "▷ turn started") {
		t.Errorf("unexpected text: %s", stripANSI(result.Text))
	}
}

func TestFormatCodexEvent_TurnCompleted(t *testing.T) {
	result := FormatCodexEvent(map[string]any{"type": "turn.completed"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsDetail {
		t.Error("turn.completed should be detail")
	}
	if !strings.Contains(stripANSI(result.Text), "▷ turn completed") {
		t.Errorf("unexpected text: %s", stripANSI(result.Text))
	}
}

func TestFormatCodexEvent_TurnFailed_WithMessage(t *testing.T) {
	result := FormatCodexEvent(map[string]any{
		"type":  "turn.failed",
		"error": map[string]any{"message": "rate limited"},
	})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.IsDetail {
		t.Error("turn.failed must NOT be detail — it's always visible")
	}
	stripped := stripANSI(result.Text)
	if !strings.Contains(stripped, "✗ turn failed: rate limited") {
		t.Errorf("unexpected text: %s", stripped)
	}
}

func TestFormatCodexEvent_TurnFailed_NoMessage(t *testing.T) {
	result := FormatCodexEvent(map[string]any{"type": "turn.failed"})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	stripped := stripANSI(result.Text)
	if !strings.Contains(stripped, "no error message") {
		t.Errorf("should contain fallback message, got: %s", stripped)
	}
}

func TestFormatCodexEvent_AgentMessageCompleted(t *testing.T) {
	result := FormatCodexEvent(map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"type": "agent_message",
			"id":   "msg-1",
			"text": "All done",
		},
	})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.IsDetail {
		t.Error("agent_message completed should not be detail")
	}
	if result.Text != "All done\n" {
		t.Errorf("expected 'All done\\n', got %q", result.Text)
	}
}

func TestFormatCodexEvent_AgentMessageStarted_Dropped(t *testing.T) {
	result := FormatCodexEvent(map[string]any{
		"type": "item.started",
		"item": map[string]any{"type": "agent_message", "id": "msg-1"},
	})
	if result != nil {
		t.Error("item.started for agent_message should be dropped")
	}
}

func TestFormatCodexEvent_AgentMessageDelta(t *testing.T) {
	result := FormatCodexEvent(map[string]any{
		"type": "item.delta",
		"item": map[string]any{"type": "agent_message"},
		"text": "streaming...",
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result.IsDetail {
		t.Error("agent_message delta should not be detail")
	}
	if result.Text != "streaming..." {
		t.Errorf("expected 'streaming...', got %q", result.Text)
	}
}

func TestFormatCodexEvent_AgentMessageEmpty(t *testing.T) {
	result := FormatCodexEvent(map[string]any{
		"type": "item.completed",
		"item": map[string]any{"type": "agent_message", "id": "msg-1", "text": ""},
	})
	if result != nil {
		t.Error("empty agent_message completion should be dropped")
	}
}

func TestFormatCodexEvent_CommandStarted(t *testing.T) {
	result := FormatCodexEvent(map[string]any{
		"type": "item.started",
		"item": map[string]any{
			"type":    "command_execution",
			"id":      "call-1",
			"command": "ls -la",
		},
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if !result.IsDetail {
		t.Error("command started should be detail")
	}
	stripped := stripANSI(result.Text)
	if !strings.Contains(stripped, "ls -la") {
		t.Errorf("expected command in output, got: %s", stripped)
	}
}

func TestFormatCodexEvent_CommandCompleted(t *testing.T) {
	result := FormatCodexEvent(map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"type":              "command_execution",
			"id":                "call-1",
			"command":           "echo hi",
			"aggregated_output": "hi\n",
			"status":            "completed",
		},
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	stripped := stripANSI(result.Text)
	if !strings.Contains(stripped, "hi") {
		t.Errorf("expected output in rendered text, got: %s", stripped)
	}
}

func TestFormatCodexEvent_CommandCompleted_FailedStatus(t *testing.T) {
	result := FormatCodexEvent(map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"type":              "command_execution",
			"id":                "call-1",
			"command":           "false",
			"aggregated_output": "",
			"status":            "failed",
		},
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	stripped := stripANSI(result.Text)
	if !strings.Contains(stripped, "[failed]") {
		t.Errorf("expected [failed] tag in output, got: %s", stripped)
	}
}

func TestFormatCodexEvent_CommandDelta(t *testing.T) {
	result := FormatCodexEvent(map[string]any{
		"type": "item.delta",
		"item": map[string]any{"type": "command_execution"},
		"text": "hello\n",
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	stripped := stripANSI(result.Text)
	if !strings.Contains(stripped, "hello") {
		t.Errorf("expected 'hello' in output, got: %s", stripped)
	}
}

func TestFormatCodexEvent_LongCommandClipped(t *testing.T) {
	longCmd := strings.Repeat("x", 500)
	result := FormatCodexEvent(map[string]any{
		"type": "item.started",
		"item": map[string]any{
			"type":    "command_execution",
			"id":      "c",
			"command": longCmd,
		},
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if len(result.Text) > 300 {
		t.Errorf("long command should be clipped, got length %d", len(result.Text))
	}
	stripped := stripANSI(result.Text)
	if !strings.Contains(stripped, "...") {
		t.Errorf("clipped command should contain '...', got: %s", stripped[:50])
	}
}

func TestFormatCodexEvent_Reasoning(t *testing.T) {
	result := FormatCodexEvent(map[string]any{
		"type": "item.completed",
		"item": map[string]any{"type": "reasoning", "text": "thinking"},
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if !result.IsDetail {
		t.Error("reasoning should be detail")
	}
	stripped := stripANSI(result.Text)
	if !strings.Contains(stripped, "thinking") {
		t.Errorf("expected 'thinking', got: %s", stripped)
	}
}

func TestFormatCodexEvent_ReasoningEmpty_Dropped(t *testing.T) {
	result := FormatCodexEvent(map[string]any{
		"type": "item.completed",
		"item": map[string]any{"type": "reasoning", "text": ""},
	})
	if result != nil {
		t.Error("empty reasoning should be dropped")
	}
}

func TestFormatCodexEvent_TerminalInteraction(t *testing.T) {
	result := FormatCodexEvent(map[string]any{
		"type": "command_execution.terminal_interaction",
		"item":      map[string]any{"type": "command_execution", "id": "call-1"},
		"processId": "12345",
		"stdin":     "y\n",
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	stripped := stripANSI(result.Text)
	if !strings.Contains(stripped, "terminal interaction") {
		t.Errorf("expected 'terminal interaction', got: %s", stripped)
	}
	if !strings.Contains(stripped, "id=call-1") {
		t.Errorf("expected id=call-1, got: %s", stripped)
	}
	if !strings.Contains(stripped, "pid=12345") {
		t.Errorf("expected pid=12345, got: %s", stripped)
	}
}

func TestFormatCodexEvent_TerminalInteractionEmptyStdin(t *testing.T) {
	result := FormatCodexEvent(map[string]any{
		"type": "command_execution.terminal_interaction",
		"item":      map[string]any{"type": "command_execution", "id": "c"},
		"processId": "1",
		"stdin":     "",
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	stripped := stripANSI(result.Text)
	if !strings.Contains(stripped, "stdin=(empty)") {
		t.Errorf("expected 'stdin=(empty)', got: %s", stripped)
	}
}

func TestFormatCodexEvent_Unrecognized(t *testing.T) {
	if FormatCodexEvent(map[string]any{"type": "assistant"}) != nil {
		t.Error("non-codex type should return nil")
	}
	if FormatCodexEvent(map[string]any{"type": "stream_event"}) != nil {
		t.Error("non-codex type should return nil")
	}
	if FormatCodexEvent(map[string]any{}) != nil {
		t.Error("empty object should return nil")
	}
}

func TestFormatCodexEvent_UnknownItemType(t *testing.T) {
	if FormatCodexEvent(map[string]any{
		"type": "item.completed",
		"item": map[string]any{"type": "unknown_thing"},
	}) != nil {
		t.Error("unknown item type should return nil")
	}
}

// ── Section 7 spec: Codex JSON-RPC Translation ──────────────

func TestIsTranslatedMethod(t *testing.T) {
	if !IsTranslatedMethod("item/commandExecution/terminalInteraction") {
		t.Error("should be translated")
	}
	if !IsTranslatedMethod("item/commandExecution/outputDelta") {
		t.Error("should be translated")
	}
	if !IsTranslatedMethod("item/agentMessage/delta") {
		t.Error("should be translated")
	}
	if !IsTranslatedMethod("item/reasoning/textDelta") {
		t.Error("should be translated")
	}
	if !IsTranslatedMethod("item/reasoning/summaryTextDelta") {
		t.Error("should be translated")
	}
	if !IsTranslatedMethod("turn/started") {
		t.Error("should be translated")
	}
	if !IsTranslatedMethod("turn/completed") {
		t.Error("should be translated")
	}
	if !IsTranslatedMethod("item/started") {
		t.Error("should be translated")
	}
	if !IsTranslatedMethod("item/completed") {
		t.Error("should be translated")
	}
	if IsTranslatedMethod("mcpServer/startupStatus/updated") {
		t.Error("should NOT be translated")
	}
	if IsTranslatedMethod("thread/started") {
		t.Error("should NOT be translated")
	}
}

func TestTranslateAgentMessageDelta_DeltaField(t *testing.T) {
	result := TranslateAgentMessageDelta(map[string]any{
		"threadId": "t-1", "turnId": "turn-1",
		"itemId": "msg-1", "delta": "Hello world",
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result["text"] != "Hello world" {
		t.Errorf("expected text=Hello world, got %v", result["text"])
	}
	item := result["item"].(map[string]any)
	if item["type"] != "agent_message" {
		t.Errorf("expected type=agent_message, got %v", item["type"])
	}
	if item["id"] != "msg-1" {
		t.Errorf("expected id=msg-1, got %v", item["id"])
	}
}

func TestTranslateAgentMessageDelta_TextFallback(t *testing.T) {
	result := TranslateAgentMessageDelta(map[string]any{
		"itemId": "msg-1", "text": "fallback shape",
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result["text"] != "fallback shape" {
		t.Errorf("expected text=fallback shape, got %v", result["text"])
	}
}

func TestTranslateAgentMessageDelta_Empty(t *testing.T) {
	if TranslateAgentMessageDelta(map[string]any{}) != nil {
		t.Error("empty params should return nil")
	}
	if TranslateAgentMessageDelta(map[string]any{"delta": ""}) != nil {
		t.Error("empty delta should return nil")
	}
}

func TestTranslateAgentMessageDelta_NoIdOmitted(t *testing.T) {
	result := TranslateAgentMessageDelta(map[string]any{"delta": "x"})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	item := result["item"].(map[string]any)
	if _, ok := item["id"]; ok {
		t.Error("id should be omitted when missing")
	}
}

func TestTranslateOutputDelta(t *testing.T) {
	result := TranslateOutputDelta(map[string]any{
		"itemId": "call-1", "delta": "stdout chunk\n",
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result["text"] != "stdout chunk\n" {
		t.Errorf("expected text='stdout chunk\\n', got %v", result["text"])
	}
	item := result["item"].(map[string]any)
	if item["type"] != "command_execution" {
		t.Errorf("expected type=command_execution, got %v", item["type"])
	}
	if item["id"] != "call-1" {
		t.Errorf("expected id=call-1, got %v", item["id"])
	}
}

func TestTranslateOutputDelta_Empty(t *testing.T) {
	if TranslateOutputDelta(map[string]any{}) != nil {
		t.Error("empty should return nil")
	}
}

func TestTranslateReasoningDelta(t *testing.T) {
	result := TranslateReasoningDelta(map[string]any{
		"itemId": "rs-1", "delta": "thinking...",
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	item := result["item"].(map[string]any)
	if item["type"] != "reasoning" {
		t.Errorf("expected type=reasoning, got %v", item["type"])
	}
	if result["text"] != "thinking..." {
		t.Errorf("expected text='thinking...', got %v", result["text"])
	}
}

func TestTranslateReasoningDelta_Empty(t *testing.T) {
	if TranslateReasoningDelta(map[string]any{}) != nil {
		t.Error("empty should return nil")
	}
}

func TestTranslateTerminalInteraction(t *testing.T) {
	result := TranslateTerminalInteraction(map[string]any{
		"itemId": "call-1", "processId": "12345", "stdin": "y\n",
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result["type"] != "command_execution.terminal_interaction" {
		t.Errorf("unexpected type: %v", result["type"])
	}
	if result["stdin"] != "y\n" {
		t.Errorf("expected stdin='y\\n', got %v", result["stdin"])
	}
	if result["processId"] != "12345" {
		t.Errorf("expected processId=12345, got %v", result["processId"])
	}
}

func TestTranslateTerminalInteraction_EmptyStdin(t *testing.T) {
	result := TranslateTerminalInteraction(map[string]any{
		"itemId": "call-1", "processId": "12345", "stdin": "",
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result["stdin"] != "" {
		t.Errorf("expected empty string stdin, got %v", result["stdin"])
	}
}

func TestTranslateTerminalInteraction_NothingUseful(t *testing.T) {
	if TranslateTerminalInteraction(map[string]any{}) != nil {
		t.Error("should return nil when nothing useful")
	}
}

// ── Item notification translation ────────────────────────────

func TestTranslateItemNotification_FilterUserMessage(t *testing.T) {
	if TranslateItemNotification("item/started", map[string]any{
		"item": map[string]any{
			"type": "userMessage", "id": "u-1",
			"content": []any{map[string]any{"type": "text", "text": "x"}},
		},
	}) != nil {
		t.Error("userMessage on item/started should be filtered")
	}
	if TranslateItemNotification("item/completed", map[string]any{
		"item": map[string]any{"type": "userMessage", "id": "u-1"},
	}) != nil {
		t.Error("userMessage on item/completed should be filtered")
	}
}

func TestTranslateItemNotification_EmptyReasoning(t *testing.T) {
	if TranslateItemNotification("item/completed", map[string]any{
		"item": map[string]any{
			"type": "reasoning", "id": "rs-1",
			"summary": []any{}, "content": []any{},
		},
	}) != nil {
		t.Error("empty reasoning should be dropped")
	}
}

func TestTranslateItemNotification_ReasoningWithSummary(t *testing.T) {
	result := TranslateItemNotification("item/completed", map[string]any{
		"item": map[string]any{
			"type": "reasoning", "id": "rs-1",
			"summary": []any{
				map[string]any{"type": "summary_text", "text": "step 1"},
				map[string]any{"type": "summary_text", "text": "step 2"},
			},
		},
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	item := result["item"].(map[string]any)
	if item["text"] != "step 1\nstep 2" {
		t.Errorf("expected joined summary, got %v", item["text"])
	}
}

func TestTranslateItemNotification_ReasoningSummaryPartsLegacy(t *testing.T) {
	result := TranslateItemNotification("item/completed", map[string]any{
		"item": map[string]any{
			"type": "reasoning", "id": "rs-1",
			"summaryParts": []any{
				map[string]any{"type": "text", "text": "legacy"},
			},
		},
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	item := result["item"].(map[string]any)
	if item["text"] != "legacy" {
		t.Errorf("expected 'legacy', got %v", item["text"])
	}
}

func TestTranslateItemNotification_AgentMessageStarted(t *testing.T) {
	result := TranslateItemNotification("item/started", map[string]any{
		"item": map[string]any{
			"type": "agentMessage", "id": "msg-1", "text": "",
		},
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result["type"] != "item.started" {
		t.Errorf("expected type=item.started, got %v", result["type"])
	}
	item := result["item"].(map[string]any)
	if item["type"] != "agent_message" {
		t.Errorf("expected item type=agent_message, got %v", item["type"])
	}
}

func TestTranslateItemNotification_CommandExecution(t *testing.T) {
	result := TranslateItemNotification("item/completed", map[string]any{
		"item": map[string]any{
			"type":              "commandExecution",
			"id":                "call-1",
			"command":           "echo hi",
			"aggregatedOutput":  "hi\n",
			"status":            "completed",
		},
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result["type"] != "item.completed" {
		t.Errorf("expected type=item.completed, got %v", result["type"])
	}
	item := result["item"].(map[string]any)
	if item["type"] != "command_execution" {
		t.Errorf("expected command_execution, got %v", item["type"])
	}
	if item["command"] != "echo hi" {
		t.Errorf("expected command=echo hi, got %v", item["command"])
	}
	if item["aggregated_output"] != "hi\n" {
		t.Errorf("expected aggregated_output=hi\\n, got %v", item["aggregated_output"])
	}
	if item["status"] != "completed" {
		t.Errorf("expected status=completed, got %v", item["status"])
	}
}

func TestTranslateItemNotification_CommandFailed(t *testing.T) {
	result := TranslateItemNotification("item/completed", map[string]any{
		"item": map[string]any{
			"type":             "commandExecution",
			"id":               "call-1",
			"command":          "false",
			"aggregatedOutput": "",
			"status":           "failed",
		},
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	item := result["item"].(map[string]any)
	if item["status"] != "failed" {
		t.Errorf("expected status=failed, got %v", item["status"])
	}
}

// ── Turn lifecycle ───────────────────────────────────────────

func TestTranslateTurnCompleted_Success(t *testing.T) {
	r := TranslateTurnCompleted(map[string]any{
		"turn": map[string]any{"id": "tu-1", "status": "completed"},
	})
	if r.TurnFailed {
		t.Error("expected turnFailed=false")
	}
	if r.Event["type"] != "turn.completed" {
		t.Errorf("expected type=turn.completed, got %v", r.Event["type"])
	}
}

func TestTranslateTurnCompleted_Failed(t *testing.T) {
	r := TranslateTurnCompleted(map[string]any{
		"turn": map[string]any{
			"id": "tu-1", "status": "failed",
			"error": map[string]any{"message": "rate limit"},
		},
	})
	if !r.TurnFailed {
		t.Error("expected turnFailed=true")
	}
	if r.Event["type"] != "turn.failed" {
		t.Errorf("expected type=turn.failed, got %v", r.Event["type"])
	}
	errorObj := r.Event["error"].(map[string]any)
	if errorObj["message"] != "rate limit" {
		t.Errorf("expected message='rate limit', got %v", errorObj["message"])
	}
}

func TestTranslateTurnCompleted_DefaultErrorMessage(t *testing.T) {
	r := TranslateTurnCompleted(map[string]any{
		"turn": map[string]any{"status": "failed"},
	})
	errorObj := r.Event["error"].(map[string]any)
	if errorObj["message"] != "Turn failed" {
		t.Errorf("expected default message, got %v", errorObj["message"])
	}
}

// ── Agent message text collection ────────────────────────────

func TestTranslateItemNotification_AgentMessageFragments(t *testing.T) {
	result := TranslateItemNotification("item/completed", map[string]any{
		"item": map[string]any{
			"type": "agentMessage",
			"id":   "msg-1",
			"fragments": []any{
				map[string]any{"type": "text", "text": "hello"},
				map[string]any{"type": "text", "text": "world"},
			},
		},
	})
	if result == nil {
		t.Fatal("expected non-nil")
	}
	item := result["item"].(map[string]any)
	if item["text"] != "hello\nworld" {
		t.Errorf("expected 'hello\\nworld', got %q", item["text"])
	}
}

