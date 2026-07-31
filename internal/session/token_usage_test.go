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
