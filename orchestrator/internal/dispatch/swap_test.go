package dispatch

import (
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

func TestSwapPoolAgent_ReplacesSingle(t *testing.T) {
	pools := map[string]config.PoolConfig{
		"evaluating": {Agents: []config.WeightedAgent{we("claude", 3), we("codex", 1)}},
	}
	result := SwapPoolAgent(pools, "evaluating", "claude", "gemini")
	if !result.Changed {
		t.Fatal("expected Changed=true")
	}
	if result.Affected != 1 {
		t.Errorf("expected Affected=1, got %d", result.Affected)
	}
	found := false
	for _, a := range result.Updated.Agents {
		if a.AgentID == "gemini" {
			found = true
			if a.Weight != 3 {
				t.Errorf("expected gemini weight=3, got %d", a.Weight)
			}
		}
	}
	if !found {
		t.Error("expected gemini in updated pool")
	}
}

func TestSwapPoolAgent_MergesWeights(t *testing.T) {
	pools := map[string]config.PoolConfig{
		"working": {Agents: []config.WeightedAgent{we("claude", 3), we("codex", 2), we("gemini", 1)}},
	}
	result := SwapPoolAgent(pools, "working", "claude", "gemini")
	if !result.Changed {
		t.Fatal("expected Changed=true")
	}
	for _, a := range result.Updated.Agents {
		if a.AgentID == "gemini" {
			if a.Weight != 4 {
				t.Errorf("expected merged gemini weight=4 (3+1), got %d", a.Weight)
			}
		}
		if a.AgentID == "claude" {
			t.Error("claude should have been replaced, not kept")
		}
	}
}

func TestSwapPoolAgent_SameAgent(t *testing.T) {
	pools := map[string]config.PoolConfig{
		"evaluating": {Agents: []config.WeightedAgent{we("claude", 3)}},
	}
	result := SwapPoolAgent(pools, "evaluating", "claude", "claude")
	if result.Changed {
		t.Error("expected Changed=false for same agent swap")
	}
}

func TestSwapPoolAgent_AgentNotInPool(t *testing.T) {
	pools := map[string]config.PoolConfig{
		"evaluating": {Agents: []config.WeightedAgent{we("claude", 3)}},
	}
	result := SwapPoolAgent(pools, "evaluating", "missing", "codex")
	if result.Changed {
		t.Error("expected Changed=false when agent not in pool")
	}
	if result.Affected != 0 {
		t.Errorf("expected Affected=0, got %d", result.Affected)
	}
}

func TestSwapPoolAgent_PoolNotFound(t *testing.T) {
	pools := map[string]config.PoolConfig{
		"evaluating": {Agents: []config.WeightedAgent{we("claude", 3)}},
	}
	result := SwapPoolAgent(pools, "nonexistent", "claude", "codex")
	if result.Changed {
		t.Error("expected Changed=false for missing pool key")
	}
}

func TestSwapPoolAgent_PreservesOtherPools(t *testing.T) {
	pools := map[string]config.PoolConfig{
		"evaluating": {Agents: []config.WeightedAgent{we("claude", 3)}},
		"working":    {Agents: []config.WeightedAgent{we("codex", 2)}},
	}
	result := SwapPoolAgent(pools, "evaluating", "claude", "gemini")
	if result.Pools["working"].Agents[0].AgentID != "codex" {
		t.Error("other pool should be unchanged")
	}
}

func TestSwapActionsAgent_Replaces(t *testing.T) {
	actions := map[string]string{
		"take":            "claude",
		"scene":           "claude",
		"scopeRefinement": "codex",
		"staleGrooming":   "claude",
	}
	result := SwapActionsAgent(actions, "claude", "gemini")
	if result.Affected != 3 {
		t.Errorf("expected Affected=3, got %d", result.Affected)
	}
	if result.Actions["take"] != "gemini" {
		t.Errorf("expected take=gemini, got %q", result.Actions["take"])
	}
	if result.Actions["scene"] != "gemini" {
		t.Errorf("expected scene=gemini, got %q", result.Actions["scene"])
	}
	if result.Actions["scopeRefinement"] != "codex" {
		t.Errorf("expected scopeRefinement=codex (unchanged), got %q", result.Actions["scopeRefinement"])
	}
	if len(result.Updates) != 3 {
		t.Errorf("expected 3 updates, got %d", len(result.Updates))
	}
}

func TestSwapActionsAgent_NoMatch(t *testing.T) {
	actions := map[string]string{
		"take": "codex",
	}
	result := SwapActionsAgent(actions, "claude", "gemini")
	if result.Affected != 0 {
		t.Errorf("expected Affected=0, got %d", result.Affected)
	}
	if result.Actions["take"] != "codex" {
		t.Error("actions should be unchanged")
	}
}

func TestSwapActionsAgent_SameAgent(t *testing.T) {
	actions := map[string]string{
		"take": "claude",
	}
	result := SwapActionsAgent(actions, "claude", "claude")
	if result.Affected != 0 {
		t.Errorf("expected Affected=0 for same agent, got %d", result.Affected)
	}
}

func TestSwapPoolsAgent_ReplacesAcrossSteps(t *testing.T) {
	pools := map[string]config.PoolConfig{
		"evaluating": {Agents: []config.WeightedAgent{we("claude", 3), we("codex", 1)}},
		"working":    {Agents: []config.WeightedAgent{we("claude", 2)}},
		"reviewing":  {Agents: []config.WeightedAgent{we("codex", 4)}},
	}
	result := SwapPoolsAgent(pools, "claude", "gemini")
	if result.AffectedEntries != 2 {
		t.Errorf("expected AffectedEntries=2, got %d", result.AffectedEntries)
	}
	if result.AffectedSteps != 2 {
		t.Errorf("expected AffectedSteps=2, got %d", result.AffectedSteps)
	}
	if len(result.Updates) != 2 {
		t.Errorf("expected Updates with 2 keys, got %d", len(result.Updates))
	}
	for _, a := range result.Pools["evaluating"].Agents {
		if a.AgentID == "claude" {
			t.Error("claude should be replaced in evaluating pool")
		}
	}
	for _, a := range result.Pools["working"].Agents {
		if a.AgentID == "gemini" && a.Weight != 2 {
			t.Errorf("expected gemini weight=2 in working, got %d", a.Weight)
		}
	}
}

func TestSwapPoolsAgent_SameAgent(t *testing.T) {
	pools := map[string]config.PoolConfig{
		"evaluating": {Agents: []config.WeightedAgent{we("claude", 3)}},
	}
	result := SwapPoolsAgent(pools, "claude", "claude")
	if result.AffectedEntries != 0 || result.AffectedSteps != 0 {
		t.Error("expected zero affected for same-agent swap")
	}
}

func TestSwapPoolsAgent_NotPresent(t *testing.T) {
	pools := map[string]config.PoolConfig{
		"evaluating": {Agents: []config.WeightedAgent{we("codex", 2)}},
	}
	result := SwapPoolsAgent(pools, "claude", "gemini")
	if result.AffectedEntries != 0 {
		t.Errorf("expected AffectedEntries=0, got %d", result.AffectedEntries)
	}
}

func TestCountDispatchAgentOccurrences(t *testing.T) {
	actions := map[string]string{
		"take":           "claude",
		"scene":          "codex",
		"staleGrooming":  "claude",
	}
	pools := map[string]config.PoolConfig{
		"evaluating": {Agents: []config.WeightedAgent{we("claude", 3), we("codex", 1)}},
		"working":    {Agents: []config.WeightedAgent{we("claude", 2)}},
		"reviewing":  {Agents: []config.WeightedAgent{we("gemini", 5)}},
	}
	result := CountDispatchAgentOccurrences(actions, pools, "claude")
	if result.AffectedActions != 2 {
		t.Errorf("expected AffectedActions=2, got %d", result.AffectedActions)
	}
	if result.AffectedEntries != 2 {
		t.Errorf("expected AffectedEntries=2, got %d", result.AffectedEntries)
	}
	if result.AffectedSteps != 2 {
		t.Errorf("expected AffectedSteps=2, got %d", result.AffectedSteps)
	}
}

func TestCountDispatchAgentOccurrences_NoMatch(t *testing.T) {
	actions := map[string]string{"take": "claude"}
	pools := map[string]config.PoolConfig{
		"evaluating": {Agents: []config.WeightedAgent{we("codex", 1)}},
	}
	result := CountDispatchAgentOccurrences(actions, pools, "missing")
	if result.AffectedActions != 0 || result.AffectedEntries != 0 || result.AffectedSteps != 0 {
		t.Errorf("expected all zeros for missing agent, got %+v", result)
	}
}

func TestGetSwappableSourceAgentIDs_Basic(t *testing.T) {
	used := []string{"claude", "codex"}
	avail := []string{"claude", "codex", "gemini"}
	result := GetSwappableSourceAgentIDs(used, avail)
	if len(result) != 2 {
		t.Errorf("expected 2 swappable ids, got %d: %v", len(result), result)
	}
}

func TestGetSwappableSourceAgentIDs_SingleIdentical(t *testing.T) {
	used := []string{"claude"}
	avail := []string{"claude"}
	result := GetSwappableSourceAgentIDs(used, avail)
	if len(result) != 0 {
		t.Errorf("expected 0 swappable ids for identical single-element sets, got %d", len(result))
	}
}

func TestGetSwappableSourceAgentIDs_EmptyUsed(t *testing.T) {
	result := GetSwappableSourceAgentIDs(nil, []string{"claude"})
	if len(result) != 0 {
		t.Error("expected empty result for empty used")
	}
}

func TestGetSwappableSourceAgentIDs_EmptyAvail(t *testing.T) {
	result := GetSwappableSourceAgentIDs([]string{"claude"}, nil)
	if len(result) != 0 {
		t.Error("expected empty result for empty available")
	}
}

func TestSwapPoolAgent_MultipleOccurrences(t *testing.T) {
	pools := map[string]config.PoolConfig{
		"evaluating": {Agents: []config.WeightedAgent{we("claude", 2), we("codex", 1), we("claude", 3)}},
	}
	result := SwapPoolAgent(pools, "evaluating", "claude", "gemini")
	if !result.Changed {
		t.Fatal("expected Changed=true")
	}
	if result.Affected != 2 {
		t.Errorf("expected Affected=2 (two occurrences), got %d", result.Affected)
	}
	found := 0
	totalWeight := 0
	for _, a := range result.Updated.Agents {
		if a.AgentID == "gemini" {
			found++
			totalWeight += a.Weight
		}
		if a.AgentID == "claude" {
			t.Error("claude should be fully replaced")
		}
	}
	if found != 1 {
		t.Errorf("expected 1 gemini entry (merged), got %d", found)
	}
	if totalWeight != 5 {
		t.Errorf("expected total gemini weight=5 (2+3), got %d", totalWeight)
	}
}

func TestSwapPoolsAgent_MergesWhenReplacementExists(t *testing.T) {
	pools := map[string]config.PoolConfig{
		"evaluating": {Agents: []config.WeightedAgent{we("claude", 3), we("gemini", 2)}},
	}
	result := SwapPoolsAgent(pools, "claude", "gemini")
	if result.AffectedEntries != 1 {
		t.Errorf("expected AffectedEntries=1, got %d", result.AffectedEntries)
	}
	totalWeight := 0
	geminiCount := 0
	for _, a := range result.Pools["evaluating"].Agents {
		if a.AgentID == "gemini" {
			totalWeight += a.Weight
			geminiCount++
		}
	}
	if geminiCount != 1 {
		t.Errorf("expected 1 gemini entry after merge, got %d", geminiCount)
	}
	if totalWeight != 5 {
		t.Errorf("expected gemini total weight=5 (3+2), got %d", totalWeight)
	}
}

func TestStepAgentTracker_RecordAndGet(t *testing.T) {
	tracker := NewStepAgentTracker()
	tracker.Record("beat-1", "implementation", "claude")
	got, ok := tracker.Get("beat-1", "implementation")
	if !ok {
		t.Fatal("expected to find recorded agent")
	}
	if got != "claude" {
		t.Errorf("expected 'claude', got %q", got)
	}
}

func TestStepAgentTracker_Missing(t *testing.T) {
	tracker := NewStepAgentTracker()
	_, ok := tracker.Get("beat-1", "implementation")
	if ok {
		t.Error("expected not found for unrecorded key")
	}
}

func TestStepAgentTracker_Overwrite(t *testing.T) {
	tracker := NewStepAgentTracker()
	tracker.Record("beat-1", "implementation", "claude")
	tracker.Record("beat-1", "implementation", "codex")
	got, ok := tracker.Get("beat-1", "implementation")
	if !ok {
		t.Fatal("expected to find recorded agent")
	}
	if got != "codex" {
		t.Errorf("expected 'codex' (overwritten), got %q", got)
	}
}