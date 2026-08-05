package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPriceTableDate_IsPresent(t *testing.T) {
	if PriceTableDate != "2026-08-05" {
		t.Errorf("PriceTableDate = %q, want %q", PriceTableDate, "2026-08-05")
	}
}

func TestDeriveAttemptCost_ReportedCostIsPreservedAndLabelledReported(t *testing.T) {
	cost := 0.45
	rec := StageAttemptRecord{
		AgentID: "claude",
		CostUSD: &cost,
	}
	DeriveAttemptCost(&rec)
	if rec.CostSource != "reported" {
		t.Errorf("CostSource = %q, want %q", rec.CostSource, "reported")
	}
	if rec.CostUSD == nil || *rec.CostUSD != 0.45 {
		t.Errorf("CostUSD = %v, want 0.45", rec.CostUSD)
	}
}

func TestDeriveAttemptCost_DerivedCostFromTokensAndModel(t *testing.T) {
	inp, out := int64(100000), int64(10000)
	cread := int64(0)
	model := "gpt-5.6-sol"
	rec := StageAttemptRecord{
		AgentID:         "codex",
		Model:           &model,
		InputTokens:     &inp,
		OutputTokens:    &out,
		CacheReadTokens: &cread,
	}
	DeriveAttemptCost(&rec)
	if rec.CostSource != "derived" {
		t.Errorf("CostSource = %q, want %q", rec.CostSource, "derived")
	}
	// gpt-5.6-sol: in=$5.00/1M, out=$30.00/1M => (100k*5 + 10k*30)/1e6 = 0.5 + 0.3 = 0.8
	expected := 0.80
	if rec.CostUSD == nil || *rec.CostUSD != expected {
		t.Errorf("CostUSD = %v, want %v", rec.CostUSD, expected)
	}
}

func TestDeriveAttemptCost_NormalizedModelAlias(t *testing.T) {
	inp, out := int64(100000), int64(10000)
	cread := int64(0)
	model := "opus"
	rec := StageAttemptRecord{
		AgentID:         "claude",
		Model:           &model,
		InputTokens:     &inp,
		OutputTokens:    &out,
		CacheReadTokens: &cread,
	}
	DeriveAttemptCost(&rec)
	if rec.CostSource != "derived" {
		t.Errorf("CostSource = %q, want %q", rec.CostSource, "derived")
	}
	// opus -> claude-opus-5: in=$5.00/1M, out=$25.00/1M => (100k*5 + 10k*25)/1e6 = 0.5 + 0.25 = 0.75
	expected := 0.75
	if rec.CostUSD == nil || *rec.CostUSD != expected {
		t.Errorf("CostUSD = %v, want %v", rec.CostUSD, expected)
	}
}

func TestDeriveAttemptCost_UnknownModelYieldsUnavailableAndNil(t *testing.T) {
	inp, out := int64(1000), int64(1000)
	model := "unknown-future-model-99"
	rec := StageAttemptRecord{
		AgentID:      "test-agent",
		Model:        &model,
		InputTokens:  &inp,
		OutputTokens: &out,
	}
	DeriveAttemptCost(&rec)
	if rec.CostSource != "unavailable" {
		t.Errorf("CostSource = %q, want %q", rec.CostSource, "unavailable")
	}
	if rec.CostUSD != nil {
		t.Errorf("CostUSD = %v, want nil for unknown model", *rec.CostUSD)
	}
}

func TestDeriveAttemptCost_PiSessionLookupYieldsDerivedCeiling(t *testing.T) {
	tempDir := t.TempDir()
	sessionSubdir := filepath.Join(tempDir, "test-repo")
	if err := os.MkdirAll(sessionSubdir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionID := "sess12345"
	sessionFile := filepath.Join(sessionSubdir, "2026-08-01T12-00-00Z_"+sessionID+".jsonl")

	sessionContent := `{"type":"session","id":"` + sessionID + `"}` + "\n" +
		`{"type":"message","message":{"role":"assistant","usage":{"input":100000,"output":10000,"cacheRead":0,"cacheWrite":0}}}` + "\n"

	if err := os.WriteFile(sessionFile, []byte(sessionContent), 0o644); err != nil {
		t.Fatal(err)
	}

	model := "litellm/kimi-k2.7"
	rec := StageAttemptRecord{
		AgentID:   "pi-kimi",
		Dialect:   "pi",
		Model:     &model,
		SessionID: sessionID,
	}

	DeriveAttemptCostInDir(&rec, tempDir)

	if rec.InputTokens == nil || *rec.InputTokens != 100000 {
		t.Errorf("InputTokens = %v, want 100000", rec.InputTokens)
	}
	if rec.OutputTokens == nil || *rec.OutputTokens != 10000 {
		t.Errorf("OutputTokens = %v, want 10000", rec.OutputTokens)
	}
	if rec.CostSource != "derived_ceiling" {
		t.Errorf("CostSource = %q, want %q (pi does not report cache read tokens)", rec.CostSource, "derived_ceiling")
	}
	// kimi-k2.7: in=$0.95/1M, out=$4.00/1M => (100k*0.95 + 10k*4.00)/1e6 = (0.095 + 0.04) = 0.135
	expected := 0.135
	if rec.CostUSD == nil || *rec.CostUSD != expected {
		t.Errorf("CostUSD = %v, want %v", rec.CostUSD, expected)
	}
}

func TestDeriveAttemptCost_ZeroTokenPlaceholderTriggersFallback(t *testing.T) {
	tempDir := t.TempDir()
	sessionSubdir := filepath.Join(tempDir, "test-repo")
	if err := os.MkdirAll(sessionSubdir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionID := "sess67890"
	sessionFile := filepath.Join(sessionSubdir, "2026-08-01T12-00-00Z_"+sessionID+".jsonl")

	sessionContent := `{"type":"session","id":"` + sessionID + `"}` + "\n" +
		`{"type":"message","message":{"role":"assistant","usage":{"input":50000,"output":5000}}}` + "\n"

	if err := os.WriteFile(sessionFile, []byte(sessionContent), 0o644); err != nil {
		t.Fatal(err)
	}

	zeroIn, zeroOut := int64(0), int64(0)
	zeroCost := 0.0
	model := "kimi-k2.7"
	rec := StageAttemptRecord{
		AgentID:      "pi-kimi",
		Dialect:      "pi",
		Model:        &model,
		SessionID:    sessionID,
		InputTokens:  &zeroIn,
		OutputTokens: &zeroOut,
		CostUSD:      &zeroCost,
	}

	DeriveAttemptCostInDir(&rec, tempDir)

	if rec.InputTokens == nil || *rec.InputTokens != 50000 {
		t.Errorf("InputTokens = %v, want 50000 (recovered from pi session)", rec.InputTokens)
	}
	if rec.CostSource != "derived_ceiling" {
		t.Errorf("CostSource = %q, want %q", rec.CostSource, "derived_ceiling")
	}
	if rec.CostUSD == nil || *rec.CostUSD <= 0 {
		t.Errorf("CostUSD = %v, want non-zero derived cost", rec.CostUSD)
	}
}

func TestNormalizeModel(t *testing.T) {
	tests := []struct {
		rawModel string
		agentID  string
		want     string
	}{
		{"litellm/kimi-k2.7", "pi-kimi", "kimi-k2.7"},
		{"opus", "claude", "claude-opus-5"},
		{"", "codex", "gpt-5.6-sol"},
		{"", "pi-kimi", "kimi-k2.7"},
		{"claude-sonnet-5", "claude-sonnet", "claude-sonnet-5"},
		{"unknown-model", "unknown-agent", "unknown-model"},
	}

	for _, tt := range tests {
		got := NormalizeModel(tt.rawModel, tt.agentID)
		if got != tt.want {
			t.Errorf("NormalizeModel(%q, %q) = %q, want %q", tt.rawModel, tt.agentID, got, tt.want)
		}
	}
}
