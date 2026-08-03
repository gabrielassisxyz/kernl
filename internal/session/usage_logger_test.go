package session

import "testing"

func i64(v int64) *int64     { return &v }
func f64(v float64) *float64 { return &v }
func str(v string) *string   { return &v }

// The numbers here are the shape of a real pi stage: one agent_end carrying
// the whole conversation, then one per internal retry carrying an empty
// assistant message. Keeping the most recent report recorded that run as
// having cost zero.
func TestCapturingUsageLogger_RetryEnvelopesDoNotErasePriorUsage(t *testing.T) {
	logger := NewCapturingUsageLogger()

	logger.LogTokenUsage("kb-1", TokenUsageCounts{
		InputTokens:     1014610,
		OutputTokens:    4212,
		TotalTokens:     1018822,
		ReasoningTokens: i64(340),
		Model:           str("kimi-k2.7"),
	})
	for i := 0; i < 3; i++ {
		logger.LogTokenUsage("kb-1", TokenUsageCounts{
			InputTokens:     0,
			OutputTokens:    0,
			TotalTokens:     0,
			ReasoningTokens: i64(0),
			Model:           str("kimi-k2.7"),
		})
	}

	got := logger.Usage()
	if got == nil {
		t.Fatal("Usage() = nil, want the accumulated counts")
	}
	if got.InputTokens != 1014610 || got.OutputTokens != 4212 || got.TotalTokens != 1018822 {
		t.Errorf("counts = in %d / out %d / total %d, want 1014610 / 4212 / 1018822 - the retry envelopes must not erase the run's real usage",
			got.InputTokens, got.OutputTokens, got.TotalTokens)
	}
	if got.ReasoningTokens == nil || *got.ReasoningTokens != 340 {
		t.Errorf("ReasoningTokens = %v, want 340", got.ReasoningTokens)
	}
}

// Disjoint segments, which is what pi actually emits: the first agent_end
// holds the run's opening user message and a later one holds none, so summing
// counts nothing twice.
func TestCapturingUsageLogger_SumsDisjointSegments(t *testing.T) {
	logger := NewCapturingUsageLogger()

	logger.LogTokenUsage("kb-1", TokenUsageCounts{InputTokens: 561294, OutputTokens: 1200, TotalTokens: 562494})
	logger.LogTokenUsage("kb-1", TokenUsageCounts{InputTokens: 0, OutputTokens: 0, TotalTokens: 0})
	logger.LogTokenUsage("kb-1", TokenUsageCounts{InputTokens: 4554910, OutputTokens: 14749, TotalTokens: 4569659})

	got := logger.Usage()
	if got.InputTokens != 5116204 {
		t.Errorf("InputTokens = %d, want 5116204 (561294 + 0 + 4554910)", got.InputTokens)
	}
	if got.TotalTokens != 5132153 {
		t.Errorf("TotalTokens = %d, want 5132153", got.TotalTokens)
	}
}

// A field no event ever reported must stay nil. Recording zero there would
// claim the dialect measured it and found none, which is the fiction the
// ledger's nullable columns exist to prevent.
func TestCapturingUsageLogger_UnreportedFieldsStayNil(t *testing.T) {
	logger := NewCapturingUsageLogger()

	logger.LogTokenUsage("kb-1", TokenUsageCounts{InputTokens: 10, OutputTokens: 2, TotalTokens: 12})
	logger.LogTokenUsage("kb-1", TokenUsageCounts{InputTokens: 5, OutputTokens: 1, TotalTokens: 6})

	got := logger.Usage()
	if got.CacheReadTokens != nil {
		t.Errorf("CacheReadTokens = %v, want nil - no event reported it", got.CacheReadTokens)
	}
	if got.CostUSD != nil {
		t.Errorf("CostUSD = %v, want nil - no event reported it", got.CostUSD)
	}
	if got.Turns != nil {
		t.Errorf("Turns = %v, want nil - no event reported it", got.Turns)
	}
}

// One event reporting a field and another staying silent must not average the
// silence in: "did not report" contributes nothing rather than dragging a real
// measurement toward zero.
func TestCapturingUsageLogger_SilenceDoesNotDiluteAReportedField(t *testing.T) {
	logger := NewCapturingUsageLogger()

	logger.LogTokenUsage("kb-1", TokenUsageCounts{InputTokens: 10, CacheReadTokens: i64(900), CostUSD: f64(1.5)})
	logger.LogTokenUsage("kb-1", TokenUsageCounts{InputTokens: 5})

	got := logger.Usage()
	if got.CacheReadTokens == nil || *got.CacheReadTokens != 900 {
		t.Errorf("CacheReadTokens = %v, want 900", got.CacheReadTokens)
	}
	if got.CostUSD == nil || *got.CostUSD != 1.5 {
		t.Errorf("CostUSD = %v, want 1.5", got.CostUSD)
	}
}

// claude and codex report exactly one terminal usage event, so accumulation
// must be indistinguishable from the retain-the-latest behaviour this
// replaced. This is the regression guard for the dialects the change was not
// about.
func TestCapturingUsageLogger_SingleEventIsUnchanged(t *testing.T) {
	logger := NewCapturingUsageLogger()

	logger.LogTokenUsage("kb-1", TokenUsageCounts{
		InputTokens: 100, OutputTokens: 20, TotalTokens: 120,
		CacheReadTokens: i64(80), CostUSD: f64(0.42), Turns: i64(3), Model: str("claude-opus-5"),
	})

	got := logger.Usage()
	if got.InputTokens != 100 || got.OutputTokens != 20 || got.TotalTokens != 120 {
		t.Errorf("counts = %d / %d / %d, want 100 / 20 / 120", got.InputTokens, got.OutputTokens, got.TotalTokens)
	}
	if got.CostUSD == nil || *got.CostUSD != 0.42 {
		t.Errorf("CostUSD = %v, want 0.42", got.CostUSD)
	}
	if got.Model == nil || *got.Model != "claude-opus-5" {
		t.Errorf("Model = %v, want claude-opus-5", got.Model)
	}
}

func TestCapturingUsageLogger_NoEventLeavesUsageNil(t *testing.T) {
	if got := NewCapturingUsageLogger().Usage(); got != nil {
		t.Errorf("Usage() = %v, want nil - a run that reported nothing must not read as a measured zero", got)
	}
}
