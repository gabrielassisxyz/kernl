package app

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// PriceTableDate is the date (YYYY-MM-DD) when the default pricing rates were resolved.
// Recorded alongside rates because model prices rot over time.
const PriceTableDate = "2026-08-05"

// DefaultCacheShare is the measured floor of request turn gaps <=60s for Ollama Cloud (96%).
// Derived as the fraction of turns whose interval to the previous request in the same session
// is <=60s (measured over LiteLLM_SpendLogs, where Ollama Cloud prefix caching applies).
// Used to compute the estimated lower bound for derived_ceiling costs where the provider
// does not report cached_tokens.
const DefaultCacheShare = 0.96

// ModelPrice defines rates per 1,000,000 tokens (in USD) and whether the provider reports cache tokens.
type ModelPrice struct {
	InputPerM          float64
	OutputPerM         float64
	CacheReadPerM      float64
	CacheWritePerM     float64
	ReportsCacheTokens bool
}

// DefaultModelPrices contains the pricing table resolved on PriceTableDate (2026-08-05).
// Sources: tokscale pricing (OpenRouter for moonshot/z-ai, LiteLLM for deepseek/anthropic/openai).
var DefaultModelPrices = map[string]ModelPrice{
	"kimi-k2.7":         {InputPerM: 0.95, OutputPerM: 4.00, CacheReadPerM: 0.19, CacheWritePerM: 0.0, ReportsCacheTokens: false},
	"kimi-k2.6":         {InputPerM: 0.95, OutputPerM: 4.00, CacheReadPerM: 0.16, CacheWritePerM: 0.0, ReportsCacheTokens: false},
	"glm-5.2":           {InputPerM: 1.40, OutputPerM: 4.40, CacheReadPerM: 0.26, CacheWritePerM: 0.0, ReportsCacheTokens: false},
	"glm-5.1":           {InputPerM: 1.40, OutputPerM: 4.40, CacheReadPerM: 0.26, CacheWritePerM: 0.0, ReportsCacheTokens: false},
	"deepseek-v4-pro":   {InputPerM: 0.43, OutputPerM: 0.87, CacheReadPerM: 0.00, CacheWritePerM: 0.0, ReportsCacheTokens: false},
	"deepseek-v4-flash": {InputPerM: 0.14, OutputPerM: 0.28, CacheReadPerM: 0.00, CacheWritePerM: 0.0, ReportsCacheTokens: false},
	"gpt-5.6-sol":       {InputPerM: 5.00, OutputPerM: 30.00, CacheReadPerM: 0.50, CacheWritePerM: 6.25, ReportsCacheTokens: true},
	"claude-opus-5":     {InputPerM: 5.00, OutputPerM: 25.00, CacheReadPerM: 0.50, CacheWritePerM: 6.25, ReportsCacheTokens: true},
	"claude-sonnet-5":   {InputPerM: 2.00, OutputPerM: 10.00, CacheReadPerM: 0.20, CacheWritePerM: 2.50, ReportsCacheTokens: true},
}

// ModelAliases maps bare or vendor-prefixed aliases onto canonical model IDs.
var ModelAliases = map[string]string{
	"opus":          "claude-opus-5",
	"claude-opus":   "claude-opus-5",
	"claude-sonnet": "claude-sonnet-5",
}

// AgentDefaultModels provides the fallback model ID for an agentID when no model string is recorded.
var AgentDefaultModels = map[string]string{
	"codex":         "gpt-5.6-sol",
	"claude":        "claude-opus-5",
	"claude-sonnet": "claude-sonnet-5",
	"pi-kimi":       "kimi-k2.7",
}

// NormalizeModel resolves a raw model string and agentID to a canonical model ID.
func NormalizeModel(rawModel, agentID string) string {
	model := strings.TrimSpace(rawModel)
	if model != "" {
		model = strings.TrimPrefix(model, "litellm/")
		if canonical, ok := ModelAliases[model]; ok {
			return canonical
		}
		return model
	}
	if defaultModel, ok := AgentDefaultModels[agentID]; ok {
		return defaultModel
	}
	return ""
}

// PiSessionUsage carries per-turn token usage counts accumulated from a pi session JSONL log.
type PiSessionUsage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  *int64
	CacheWriteTokens *int64
	ReasoningTokens  *int64
}

// ReadPiSessionUsage reads a pi session log matching sessionID under ~/.pi/agent/sessions/
// and sums per-message turn usage.
func ReadPiSessionUsage(sessionID string) (*PiSessionUsage, error) {
	if sessionID == "" {
		return nil, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	sessionsDir := filepath.Join(home, ".pi", "agent", "sessions")
	return ReadPiSessionUsageInDir(sessionsDir, sessionID)
}

// ReadPiSessionUsageInDir reads a pi session log matching sessionID under sessionsDir.
func ReadPiSessionUsageInDir(sessionsDir, sessionID string) (*PiSessionUsage, error) {
	if sessionID == "" || sessionsDir == "" {
		return nil, nil
	}
	matches, err := filepath.Glob(filepath.Join(sessionsDir, "*", "*_"+sessionID+".jsonl"))
	if err != nil || len(matches) == 0 {
		return nil, nil
	}

	path := matches[0]
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var usage PiSessionUsage
	var creadSum, cwriteSum, reasSum int64
	var sawCread, sawCwrite, sawReas bool
	var found bool

	dec := json.NewDecoder(f)
	for {
		var row map[string]any
		if err := dec.Decode(&row); err != nil {
			if err == io.EOF {
				break
			}
			continue
		}
		if row["type"] == "message" {
			msg, _ := row["message"].(map[string]any)
			if msg != nil {
				uObj, _ := msg["usage"].(map[string]any)
				if uObj != nil {
					if inp, ok := uObj["input"].(float64); ok && inp >= 0 {
						usage.InputTokens += int64(inp)
						found = true
					}
					if out, ok := uObj["output"].(float64); ok && out >= 0 {
						usage.OutputTokens += int64(out)
						found = true
					}
					if cread, ok := uObj["cacheRead"].(float64); ok && cread >= 0 {
						creadSum += int64(cread)
						sawCread = true
					}
					if cwrite, ok := uObj["cacheWrite"].(float64); ok && cwrite >= 0 {
						cwriteSum += int64(cwrite)
						sawCwrite = true
					}
					if reas, ok := uObj["reasoning"].(float64); ok && reas >= 0 {
						reasSum += int64(reas)
						sawReas = true
					}
				}
			}
		}
	}
	if !found {
		return nil, nil
	}
	if sawCread {
		usage.CacheReadTokens = &creadSum
	}
	if sawCwrite {
		usage.CacheWriteTokens = &cwriteSum
	}
	if sawReas {
		usage.ReasoningTokens = &reasSum
	}
	return &usage, nil
}

// DeriveAttemptCost derives or classifies CostUSD and CostSource for a StageAttemptRecord at read time.
func DeriveAttemptCost(rec *StageAttemptRecord) {
	DeriveAttemptCostInDir(rec, "")
}

// DeriveAttemptCostInDir derives or classifies CostUSD and CostSource for a StageAttemptRecord,
// using customPiSessionsDir if provided (falling back to ~/.pi/agent/sessions/ if empty).
func DeriveAttemptCostInDir(rec *StageAttemptRecord, customPiSessionsDir string) {
	// 1. If costUSD is reported and positive (> 0), mark as reported.
	if rec.CostUSD != nil && *rec.CostUSD > 0 {
		rec.CostSource = "reported"
		return
	}

	// 2. If tokens are missing or zero (0/0 placeholder), try reading pi session usage logs.
	tokensMissing := rec.InputTokens == nil || rec.OutputTokens == nil || (*rec.InputTokens == 0 && *rec.OutputTokens == 0)
	if tokensMissing && rec.SessionID != "" {
		if rec.AgentID == "pi-kimi" || rec.Dialect == "pi" {
			var piUsage *PiSessionUsage
			if customPiSessionsDir != "" {
				piUsage, _ = ReadPiSessionUsageInDir(customPiSessionsDir, rec.SessionID)
			} else {
				piUsage, _ = ReadPiSessionUsage(rec.SessionID)
			}
			if piUsage != nil {
				rec.InputTokens = &piUsage.InputTokens
				rec.OutputTokens = &piUsage.OutputTokens
				rec.CacheReadTokens = piUsage.CacheReadTokens
				rec.CacheWriteTokens = piUsage.CacheWriteTokens
				rec.ReasoningTokens = piUsage.ReasoningTokens
			}
		}
	}

	// 3. Normalize model identifier.
	rawModel := ""
	if rec.Model != nil {
		rawModel = *rec.Model
	}
	modelID := NormalizeModel(rawModel, rec.AgentID)

	// 4. Missing tokens, zero tokens, or unknown model yield unavailable (never 0).
	if rec.InputTokens == nil || rec.OutputTokens == nil || (*rec.InputTokens == 0 && *rec.OutputTokens == 0) || modelID == "" {
		rec.CostUSD = nil
		rec.CostFloorUSD = nil
		rec.CostSource = "unavailable"
		return
	}

	price, ok := DefaultModelPrices[modelID]
	if !ok {
		rec.CostUSD = nil
		rec.CostFloorUSD = nil
		rec.CostSource = "unavailable"
		return
	}

	// 5. Calculate derived cost.
	inp := float64(*rec.InputTokens)
	out := float64(*rec.OutputTokens)
	cread := 0.0
	if rec.CacheReadTokens != nil {
		cread = float64(*rec.CacheReadTokens)
	}
	cwrite := 0.0
	if rec.CacheWriteTokens != nil {
		cwrite = float64(*rec.CacheWriteTokens)
	}

	costCeiling := (inp*price.InputPerM + out*price.OutputPerM + cread*price.CacheReadPerM + cwrite*price.CacheWritePerM) / 1000000.0
	rec.CostUSD = &costCeiling

	// 6. Distinguish between derived (cache read accounted for) and derived_ceiling (cache read unobservable).
	// If CacheReadTokens is nil or the model provider does not report cache tokens (ModelPrice.ReportsCacheTokens == false), cost is a ceiling.
	if rec.CacheReadTokens == nil || !price.ReportsCacheTokens {
		rec.CostSource = "derived_ceiling"
		// Calculate estimated floor at DefaultCacheShare (96% cache hit rate for <=60s gaps)
		effectiveInRate := (1.0-DefaultCacheShare)*price.InputPerM + DefaultCacheShare*price.CacheReadPerM
		costFloor := (inp*effectiveInRate + out*price.OutputPerM) / 1000000.0
		rec.CostFloorUSD = &costFloor
	} else {
		rec.CostSource = "derived"
	}
}
