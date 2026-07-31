package session

import (
	"math"

	"github.com/gabrielassisxyz/kernl/internal/adapter"
)

// TokenUsageCounts carries every accounting field a dispatched CLI's stream
// can report for one turn. InputTokens/OutputTokens/TotalTokens are required
// (normalizeCodexUsage and normalizeClaudeResultUsage both return nil rather
// than a partially-populated struct when either is missing), everything else
// is a pointer because dialects disagree on what they report at all -
// leaving a field nil is how a caller (the stage-attempt ledger) tells "the
// CLI did not report this" apart from "the CLI reported zero".
type TokenUsageCounts struct {
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	// CacheReadTokens is claude's cache_read_input_tokens / codex's
	// cached_input_tokens - tokens served from a previously written cache
	// entry.
	CacheReadTokens *int64
	// CacheWriteTokens is claude's cache_creation_input_tokens / codex's
	// cache_write_input_tokens - tokens spent writing a new cache entry.
	CacheWriteTokens *int64
	// ReasoningTokens is codex's reasoning_output_tokens. Claude does not
	// report a separate reasoning count.
	ReasoningTokens *int64
	// CostUSD is claude's total_cost_usd. Codex reports no cost.
	CostUSD *float64
	// Turns is claude's num_turns. Codex reports no turn count.
	Turns *int64
	// Model is the concrete model identifier the CLI itself reported (e.g.
	// claude's "model" field on the result event). Nil means the dialect did
	// not report one - codex never does - and the caller must fall back to
	// whatever the operator configured, recording that it is a fallback.
	Model *string
}

type TokenUsageLogger interface {
	LogTokenUsage(beadID string, usage TokenUsageCounts)
}

func readCount(value any) int64 {
	f, ok := value.(float64)
	if !ok || math.IsInf(f, 0) || math.IsNaN(f) || f < 0 {
		return -1
	}
	return int64(math.Trunc(f))
}

// readOptionalCount is readCount for fields a dialect may simply not send:
// missing, wrong-typed, negative, or non-finite all mean "not reported"
// (nil), never a coerced zero that would misrepresent silence as a real
// measurement of no tokens.
func readOptionalCount(value any) *int64 {
	if value == nil {
		return nil
	}
	f, ok := value.(float64)
	if !ok || math.IsInf(f, 0) || math.IsNaN(f) || f < 0 {
		return nil
	}
	v := int64(math.Trunc(f))
	return &v
}

// readOptionalFloat mirrors readOptionalCount for dollar amounts (total_cost_usd),
// which are fractional and legitimately zero, so unlike readOptionalCount it
// does not reject a zero value - only missing/wrong-typed/non-finite.
func readOptionalFloat(value any) *float64 {
	if value == nil {
		return nil
	}
	f, ok := value.(float64)
	if !ok || math.IsInf(f, 0) || math.IsNaN(f) {
		return nil
	}
	return &f
}

func readOptionalString(value any) *string {
	s, ok := value.(string)
	if !ok || s == "" {
		return nil
	}
	return &s
}

func normalizeCodexUsage(usage any) *TokenUsageCounts {
	obj, ok := usage.(map[string]any)
	if !ok || obj == nil {
		return nil
	}
	inputTokens := readCount(obj["input_tokens"])
	outputTokens := readCount(obj["output_tokens"])
	if inputTokens < 0 || outputTokens < 0 {
		return nil
	}
	totalTokens := readCount(obj["total_tokens"])
	if totalTokens < 0 {
		totalTokens = inputTokens + outputTokens
	}
	return &TokenUsageCounts{
		InputTokens:      inputTokens,
		OutputTokens:     outputTokens,
		TotalTokens:      totalTokens,
		CacheReadTokens:  readOptionalCount(obj["cached_input_tokens"]),
		CacheWriteTokens: readOptionalCount(obj["cache_write_input_tokens"]),
		ReasoningTokens:  readOptionalCount(obj["reasoning_output_tokens"]),
	}
}

// normalizeClaudeResultUsage reads claude's terminal "result" event: token
// counts nested under "usage" (mirroring codex's shape), cost/turn/model as
// top-level fields alongside it.
func normalizeClaudeResultUsage(obj map[string]any) *TokenUsageCounts {
	usageObj, _ := obj["usage"].(map[string]any)
	var inputTokens, outputTokens int64 = -1, -1
	if usageObj != nil {
		inputTokens = readCount(usageObj["input_tokens"])
		outputTokens = readCount(usageObj["output_tokens"])
	}
	if inputTokens < 0 || outputTokens < 0 {
		return nil
	}
	counts := &TokenUsageCounts{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  inputTokens + outputTokens,
		CostUSD:      readOptionalFloat(obj["total_cost_usd"]),
		Turns:        readOptionalCount(obj["num_turns"]),
		Model:        readOptionalString(obj["model"]),
	}
	if usageObj != nil {
		counts.CacheReadTokens = readOptionalCount(usageObj["cache_read_input_tokens"])
		counts.CacheWriteTokens = readOptionalCount(usageObj["cache_creation_input_tokens"])
	}
	return counts
}

// ExtractTokenUsageFromEvent picks the one event per dialect that carries a
// turn's final usage totals: codex's "turn.completed", claude's "result".
// Every other dialect (and every other event type) yields nil - there is
// nothing to log yet, not a zero-usage measurement.
func ExtractTokenUsageFromEvent(dialect adapter.AgentDialect, parsed map[string]any) *TokenUsageCounts {
	evtType, _ := parsed["type"].(string)
	switch dialect {
	case adapter.DialectCodex:
		if evtType != "turn.completed" {
			return nil
		}
		return normalizeCodexUsage(parsed["usage"])
	case adapter.DialectClaude:
		if evtType != "result" {
			return nil
		}
		return normalizeClaudeResultUsage(parsed)
	default:
		return nil
	}
}

func LogTokenUsageForEvent(logger TokenUsageLogger, dialect adapter.AgentDialect, parsed map[string]any, beadID string) {
	usage := ExtractTokenUsageFromEvent(dialect, parsed)
	if usage == nil {
		return
	}
	logger.LogTokenUsage(beadID, *usage)
}
