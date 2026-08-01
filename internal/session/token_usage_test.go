package session

import (
	"math"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/adapter"
)

func TestExtractTokenUsageFromEvent_CodexTurnCompleted(t *testing.T) {
	parsed := map[string]any{
		"type": "turn.completed",
		"usage": map[string]any{
			"input_tokens":  float64(100),
			"output_tokens": float64(20),
		},
	}
	got := ExtractTokenUsageFromEvent(adapter.DialectCodex, parsed)
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", got.InputTokens)
	}
	if got.OutputTokens != 20 {
		t.Errorf("OutputTokens = %d, want 20", got.OutputTokens)
	}
	if got.TotalTokens != 120 {
		t.Errorf("TotalTokens = %d, want 120", got.TotalTokens)
	}
}

func TestExtractTokenUsageFromEvent_CodexTurnCompleted_WithTotal(t *testing.T) {
	parsed := map[string]any{
		"type": "turn.completed",
		"usage": map[string]any{
			"input_tokens":  float64(100),
			"output_tokens": float64(20),
			"total_tokens":  float64(150),
		},
	}
	got := ExtractTokenUsageFromEvent(adapter.DialectCodex, parsed)
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, want 150 (explicit value should be used)", got.TotalTokens)
	}
}

func TestExtractTokenUsageFromEvent_IncompleteUsage(t *testing.T) {
	parsed := map[string]any{
		"type": "turn.completed",
		"usage": map[string]any{
			"total_tokens": float64(120),
		},
	}
	got := ExtractTokenUsageFromEvent(adapter.DialectCodex, parsed)
	if got != nil {
		t.Errorf("expected nil for incomplete usage, got %+v", got)
	}
}

func TestExtractTokenUsageFromEvent_CodexTurnCompleted_CacheAndReasoningFields(t *testing.T) {
	parsed := map[string]any{
		"type": "turn.completed",
		"usage": map[string]any{
			"input_tokens":             float64(100),
			"output_tokens":            float64(20),
			"cached_input_tokens":      float64(40),
			"cache_write_input_tokens": float64(15),
			"reasoning_output_tokens":  float64(8),
		},
	}
	got := ExtractTokenUsageFromEvent(adapter.DialectCodex, parsed)
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.CacheReadTokens == nil || *got.CacheReadTokens != 40 {
		t.Errorf("CacheReadTokens = %v, want 40", got.CacheReadTokens)
	}
	if got.CacheWriteTokens == nil || *got.CacheWriteTokens != 15 {
		t.Errorf("CacheWriteTokens = %v, want 15", got.CacheWriteTokens)
	}
	if got.ReasoningTokens == nil || *got.ReasoningTokens != 8 {
		t.Errorf("ReasoningTokens = %v, want 8", got.ReasoningTokens)
	}
	// Codex never reports cost or a resolved model - both must stay nil, not
	// a coerced zero/empty value.
	if got.CostUSD != nil {
		t.Errorf("CostUSD = %v, want nil (codex does not report cost)", got.CostUSD)
	}
	if got.Model != nil {
		t.Errorf("Model = %v, want nil (codex does not report a resolved model)", got.Model)
	}
}

func TestExtractTokenUsageFromEvent_ClaudeResult(t *testing.T) {
	parsed := map[string]any{
		"type":           "result",
		"model":          "claude-opus-5",
		"total_cost_usd": 0.0456,
		"num_turns":      float64(3),
		"session_id":     "sess-123",
		"usage": map[string]any{
			"input_tokens":                float64(500),
			"output_tokens":               float64(120),
			"cache_creation_input_tokens": float64(300),
			"cache_read_input_tokens":     float64(200),
		},
	}
	got := ExtractTokenUsageFromEvent(adapter.DialectClaude, parsed)
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.InputTokens != 500 {
		t.Errorf("InputTokens = %d, want 500", got.InputTokens)
	}
	if got.OutputTokens != 120 {
		t.Errorf("OutputTokens = %d, want 120", got.OutputTokens)
	}
	if got.CacheWriteTokens == nil || *got.CacheWriteTokens != 300 {
		t.Errorf("CacheWriteTokens = %v, want 300 (from cache_creation_input_tokens)", got.CacheWriteTokens)
	}
	if got.CacheReadTokens == nil || *got.CacheReadTokens != 200 {
		t.Errorf("CacheReadTokens = %v, want 200 (from cache_read_input_tokens)", got.CacheReadTokens)
	}
	if got.CostUSD == nil || *got.CostUSD != 0.0456 {
		t.Errorf("CostUSD = %v, want 0.0456", got.CostUSD)
	}
	if got.Turns == nil || *got.Turns != 3 {
		t.Errorf("Turns = %v, want 3", got.Turns)
	}
	if got.Model == nil || *got.Model != "claude-opus-5" {
		t.Errorf("Model = %v, want claude-opus-5", got.Model)
	}
}

func TestExtractTokenUsageFromEvent_ClaudeNonResultEvent(t *testing.T) {
	parsed := map[string]any{
		"type": "assistant",
		"usage": map[string]any{
			"input_tokens":  float64(10),
			"output_tokens": float64(5),
		},
	}
	got := ExtractTokenUsageFromEvent(adapter.DialectClaude, parsed)
	if got != nil {
		t.Errorf("expected nil for non-result claude event, got %+v", got)
	}
}

func TestExtractTokenUsageFromEvent_ClaudeResultMissingUsage(t *testing.T) {
	parsed := map[string]any{
		"type":  "result",
		"model": "claude-opus-5",
	}
	got := ExtractTokenUsageFromEvent(adapter.DialectClaude, parsed)
	if got != nil {
		t.Errorf("expected nil when claude result carries no usage block, got %+v", got)
	}
}

func TestExtractTokenUsageFromEvent_NonCodexDialect(t *testing.T) {
	parsed := map[string]any{
		"type": "turn.completed",
		"usage": map[string]any{
			"input_tokens":  float64(100),
			"output_tokens": float64(20),
		},
	}
	for _, dialect := range []adapter.AgentDialect{adapter.DialectClaude, adapter.DialectCopilot, adapter.DialectOpenCode, adapter.DialectGemini} {
		got := ExtractTokenUsageFromEvent(dialect, parsed)
		if got != nil {
			t.Errorf("expected nil for dialect %s, got %+v", dialect, got)
		}
	}
}

func TestExtractTokenUsageFromEvent_NonTurnCompleted(t *testing.T) {
	parsed := map[string]any{
		"type": "turn.started",
		"usage": map[string]any{
			"input_tokens":  float64(100),
			"output_tokens": float64(20),
		},
	}
	got := ExtractTokenUsageFromEvent(adapter.DialectCodex, parsed)
	if got != nil {
		t.Errorf("expected nil for non-turn.completed event, got %+v", got)
	}
}

func TestExtractTokenUsageFromEvent_NilUsage(t *testing.T) {
	parsed := map[string]any{
		"type": "turn.completed",
	}
	got := ExtractTokenUsageFromEvent(adapter.DialectCodex, parsed)
	if got != nil {
		t.Errorf("expected nil for nil usage, got %+v", got)
	}
}

func TestReadCount_NegativeValue(t *testing.T) {
	got := readCount(float64(-5))
	if got >= 0 {
		t.Errorf("readCount(-5) = %d, want negative", got)
	}
}

func TestReadCount_Infinity(t *testing.T) {
	got := readCount(math.Inf(1))
	if got >= 0 {
		t.Errorf("readCount(+Inf) = %d, want negative", got)
	}
}

func TestReadCount_FloatTruncation(t *testing.T) {
	got := readCount(float64(100.9))
	if got != 100 {
		t.Errorf("readCount(100.9) = %d, want 100", got)
	}
}

func TestReadCount_Zero(t *testing.T) {
	got := readCount(float64(0))
	if got != 0 {
		t.Errorf("readCount(0) = %d, want 0", got)
	}
}

func TestReadCount_WrongType(t *testing.T) {
	got := readCount("not a number")
	if got >= 0 {
		t.Errorf("readCount(string) = %d, want negative", got)
	}
}

type mockTokenUsageLogger struct {
	calls []tokenUsageCall
}

type tokenUsageCall struct {
	beadID string
	usage  TokenUsageCounts
}

func (m *mockTokenUsageLogger) LogTokenUsage(beadID string, usage TokenUsageCounts) {
	m.calls = append(m.calls, tokenUsageCall{beadID: beadID, usage: usage})
}

func TestLogTokenUsageForEvent_SingleBead(t *testing.T) {
	logger := &mockTokenUsageLogger{}
	parsed := map[string]any{
		"type": "turn.completed",
		"usage": map[string]any{
			"input_tokens":  float64(10),
			"output_tokens": float64(5),
			"total_tokens":  float64(15),
		},
	}
	LogTokenUsageForEvent(logger, adapter.DialectCodex, parsed, "bead-a")
	if len(logger.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(logger.calls))
	}
	if logger.calls[0].beadID != "bead-a" {
		t.Errorf("beadID = %q, want %q", logger.calls[0].beadID, "bead-a")
	}
	if logger.calls[0].usage.InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", logger.calls[0].usage.InputTokens)
	}
	if logger.calls[0].usage.OutputTokens != 5 {
		t.Errorf("OutputTokens = %d, want 5", logger.calls[0].usage.OutputTokens)
	}
	if logger.calls[0].usage.TotalTokens != 15 {
		t.Errorf("TotalTokens = %d, want 15", logger.calls[0].usage.TotalTokens)
	}
}

func TestLogTokenUsageForEvent_NoExtraction(t *testing.T) {
	logger := &mockTokenUsageLogger{}
	parsed := map[string]any{
		"type": "turn.started",
	}
	LogTokenUsageForEvent(logger, adapter.DialectCodex, parsed, "bead-a")
	if len(logger.calls) != 0 {
		t.Errorf("expected 0 calls for non-matching event, got %d", len(logger.calls))
	}
}

// piAgentEndFixture is the tail of a real `pi -p --mode json` run (pi 0.81,
// litellm/kimi-k2.7), trimmed to the fields this code reads. Keeping the
// captured shape rather than an invented one is what makes this a contract
// test: pi nests counts under "usage" with camelCase keys and its own
// spelling ("input"/"output", not codex's "input_tokens"), which is exactly
// the kind of drift a hand-written fixture would paper over.
func piAgentEndFixture() map[string]any {
	return map[string]any{
		"type": "agent_end",
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": []any{map[string]any{"type": "text", "text": "Reply with exactly: HELLO"}},
			},
			map[string]any{
				"role":     "assistant",
				"provider": "litellm",
				"model":    "kimi-k2.7",
				"usage": map[string]any{
					"input":       float64(4901),
					"output":      float64(22),
					"cacheRead":   float64(0),
					"cacheWrite":  float64(0),
					"reasoning":   float64(18),
					"totalTokens": float64(4923),
					"cost":        map[string]any{"total": float64(0)},
				},
				"stopReason": "stop",
			},
		},
		"willRetry": false,
	}
}

func TestExtractTokenUsageFromEvent_PiAgentEnd(t *testing.T) {
	got := ExtractTokenUsageFromEvent(adapter.DialectPi, piAgentEndFixture())
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.InputTokens != 4901 {
		t.Errorf("InputTokens = %d, want 4901", got.InputTokens)
	}
	if got.OutputTokens != 22 {
		t.Errorf("OutputTokens = %d, want 22", got.OutputTokens)
	}
	if got.TotalTokens != 4923 {
		t.Errorf("TotalTokens = %d, want 4923", got.TotalTokens)
	}
	if got.ReasoningTokens == nil || *got.ReasoningTokens != 18 {
		t.Errorf("ReasoningTokens = %v, want 18", got.ReasoningTokens)
	}
	// pi reports the concrete model, unlike codex - this is the field that
	// keeps the stage-attempt ledger from recording modelResolved: false.
	if got.Model == nil || *got.Model != "kimi-k2.7" {
		t.Errorf("Model = %v, want kimi-k2.7", got.Model)
	}
	// A zero cost is a reported zero, not silence: the proxy bills nothing,
	// and nil here would mean "pi did not say", which is a different fact.
	if got.CostUSD == nil || *got.CostUSD != 0 {
		t.Errorf("CostUSD = %v, want a reported 0", got.CostUSD)
	}
}

func TestExtractTokenUsageFromEvent_PiSumsEveryAssistantMessage(t *testing.T) {
	parsed := map[string]any{
		"type": "agent_end",
		"messages": []any{
			map[string]any{
				"role":  "assistant",
				"model": "kimi-k2.7",
				"usage": map[string]any{"input": float64(100), "output": float64(10), "totalTokens": float64(110)},
			},
			map[string]any{
				"role":  "assistant",
				"model": "glm-5.2",
				"usage": map[string]any{"input": float64(200), "output": float64(20), "totalTokens": float64(220)},
			},
		},
	}
	got := ExtractTokenUsageFromEvent(adapter.DialectPi, parsed)
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.InputTokens != 300 || got.OutputTokens != 30 || got.TotalTokens != 330 {
		t.Errorf("counts = %d/%d/%d, want 300/30/330", got.InputTokens, got.OutputTokens, got.TotalTokens)
	}
	// Last model wins: pi can cycle models mid-session, and the one that
	// produced the final answer is what "what ran here" means.
	if got.Model == nil || *got.Model != "glm-5.2" {
		t.Errorf("Model = %v, want glm-5.2", got.Model)
	}
}

func TestExtractTokenUsageFromEvent_PiIgnoresNonTerminalEvents(t *testing.T) {
	parsed := piAgentEndFixture()
	parsed["type"] = "message_end"
	if got := ExtractTokenUsageFromEvent(adapter.DialectPi, parsed); got != nil {
		t.Errorf("message_end yielded %+v, want nil - only agent_end carries the run's totals", got)
	}
}

func TestExtractTokenUsageFromEvent_PiUserOnlyConversationReportsNothing(t *testing.T) {
	parsed := map[string]any{
		"type": "agent_end",
		"messages": []any{
			map[string]any{"role": "user", "content": []any{}},
		},
	}
	if got := ExtractTokenUsageFromEvent(adapter.DialectPi, parsed); got != nil {
		t.Errorf("got %+v, want nil - no assistant usage is silence, not a zero-token measurement", got)
	}
}

func TestCapabilitiesForDialect_PiAndAgyAreOneShotOnly(t *testing.T) {
	for _, dialect := range []string{"pi", "agy"} {
		for _, interactive := range []bool{false, true} {
			caps := CapabilitiesForDialect(dialect, interactive)
			if caps.SupportsInteractive {
				t.Errorf("%s: SupportsInteractive = true, want false - no interactive transport is wired for it", dialect)
			}
			if caps.PromptTransport != TransportCLIArg {
				t.Errorf("%s: PromptTransport = %q, want %q", dialect, caps.PromptTransport, TransportCLIArg)
			}
			if caps.SupportsFollowUp {
				t.Errorf("%s: SupportsFollowUp = true, want false - a one-shot dispatch opens no channel to nudge", dialect)
			}
		}
	}
}

func TestCapabilitiesForDialect_PiResultDetectionIsAgentSettled(t *testing.T) {
	if got := CapabilitiesForDialect("pi", false).ResultDetection; got != ResultDetectionAgentSettled {
		t.Errorf("ResultDetection = %q, want %q - pi emits no \"result\" event", got, ResultDetectionAgentSettled)
	}
	if got := CapabilitiesForDialect("agy", false).ResultDetection; got != ResultDetectionNone {
		t.Errorf("agy ResultDetection = %q, want none - it prints plain text", got)
	}
}
