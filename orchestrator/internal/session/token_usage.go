package session

import (
	"math"

	"github.com/gabrielassisxyz/kernl/internal/adapter"
)

type TokenUsageCounts struct {
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
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
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  totalTokens,
	}
}

func ExtractTokenUsageFromEvent(dialect adapter.AgentDialect, parsed map[string]any) *TokenUsageCounts {
	if dialect != adapter.DialectCodex {
		return nil
	}
	evtType, _ := parsed["type"].(string)
	if evtType != "turn.completed" {
		return nil
	}
	return normalizeCodexUsage(parsed["usage"])
}

func LogTokenUsageForEvent(logger TokenUsageLogger, dialect adapter.AgentDialect, parsed map[string]any, beadID string) {
	usage := ExtractTokenUsageFromEvent(dialect, parsed)
	if usage == nil {
		return
	}
	logger.LogTokenUsage(beadID, *usage)
}