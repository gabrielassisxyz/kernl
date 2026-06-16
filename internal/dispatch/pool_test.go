package dispatch

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

func makeAgents(names ...string) map[string]config.AgentConfig {
	m := make(map[string]config.AgentConfig, len(names))
	for _, n := range names {
		m[n] = config.AgentConfig{Command: n, Type: n}
	}
	return m
}

func makePool(entries ...config.WeightedAgent) config.PoolConfig {
	return config.PoolConfig{Agents: entries}
}

func we(id string, w int) config.WeightedAgent {
	return config.WeightedAgent{AgentID: id, Weight: w}
}

func TestSelectFromPool_SingleAgent(t *testing.T) {
	agents := makeAgents("claude")
	pool := makePool(we("claude", 1))

	got, err := SelectFromPool(pool, agents)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Command != "claude" {
		t.Errorf("expected agent command 'claude', got %q", got.Command)
	}
}

func TestSelectFromPool_EmptyPool(t *testing.T) {
	agents := makeAgents("claude")
	pool := makePool()

	_, err := SelectFromPool(pool, agents)
	if err == nil {
		t.Fatal("expected error for empty pool")
	}
	if _, ok := err.(*DispatchFailureError); !ok {
		t.Errorf("expected DispatchFailureError, got %T", err)
	}
}

func TestSelectFromPool_AllExcluded(t *testing.T) {
	agents := makeAgents("claude")
	pool := makePool(we("claude", 1))

	_, err := SelectFromPool(pool, agents, "claude")
	if err == nil {
		t.Fatal("expected error when all agents excluded")
	}
}

func TestSelectFromPool_ExclusionPrevails(t *testing.T) {
	agents := makeAgents("claude", "codex")
	pool := makePool(we("claude", 1), we("codex", 1))

	got, err := SelectFromPool(pool, agents, "claude")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Command != "codex" {
		t.Errorf("expected 'codex' after excluding 'claude', got %q", got.Command)
	}
}

func TestSelectFromPool_ZeroWeightSkipped(t *testing.T) {
	agents := makeAgents("claude", "codex")
	pool := makePool(we("claude", 0), we("codex", 1))

	got, err := SelectFromPool(pool, agents)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Command != "codex" {
		t.Errorf("expected 'codex' (only positive weight), got %q", got.Command)
	}
}

func TestSelectFromPool_AllZeroWeight(t *testing.T) {
	agents := makeAgents("claude", "codex")
	pool := makePool(we("claude", 0), we("codex", 0))

	_, err := SelectFromPool(pool, agents)
	if err == nil {
		t.Fatal("expected error for all-zero weights")
	}
}

func TestSelectFromPool_DanglingRetriesOtherAgents(t *testing.T) {
	agents := makeAgents("claude")
	pool := makePool(we("missing", 5), we("claude", 1))

	got, err := SelectFromPool(pool, agents)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Command != "claude" {
		t.Errorf("expected 'claude', got %q", got.Command)
	}
}

func TestSelectFromPool_AllDangling(t *testing.T) {
	agents := makeAgents("claude")
	pool := makePool(we("missing1", 5), we("missing2", 3))

	_, err := SelectFromPool(pool, agents)
	if err == nil {
		t.Fatal("expected error for all dangling references")
	}
}

func TestSelectFromPool_WeightedDistribution(t *testing.T) {
	pool := makePool(we("claude", 3), we("codex", 1))

	rng := rand.New(rand.NewSource(42))
	counts := map[string]int{"claude": 0, "codex": 0}

	for i := 0; i < 10000; i++ {
		eligible := pool.Agents
		selectedID := selectByWeight(eligible, rng)
		if selectedID == "" {
			t.Fatal("selectByWeight returned empty")
		}
		counts[selectedID]++
	}

	claudeRatio := float64(counts["claude"]) / 10000.0
	if claudeRatio < 0.65 || claudeRatio > 0.80 {
		t.Errorf("expected claude ratio ~0.75, got %.3f (claude=%d, codex=%d)", claudeRatio, counts["claude"], counts["codex"])
	}
}

func TestSelectFromPoolStrict_ExcludesOne(t *testing.T) {
	agents := makeAgents("claude", "codex")
	pool := makePool(we("claude", 1), we("codex", 1))

	got, err := SelectFromPoolStrict(pool, agents, "claude")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Command != "codex" {
		t.Errorf("expected 'codex', got %q", got.Command)
	}
}

func TestSelectFromPoolStrict_SingleEntryPool(t *testing.T) {
	agents := makeAgents("claude")
	pool := makePool(we("claude", 1))

	_, err := SelectFromPoolStrict(pool, agents, "claude")
	if err == nil {
		t.Fatal("expected error when single entry is the excluded agent")
	}
}

func TestSelectFromPoolStrict_AllOthersZero(t *testing.T) {
	agents := makeAgents("claude", "codex")
	pool := makePool(we("claude", 1), we("codex", 0))

	_, err := SelectFromPoolStrict(pool, agents, "claude")
	if err == nil {
		t.Fatal("expected error when only alternative has zero weight")
	}
}

func TestResolveDispatchAgent_HappyPath(t *testing.T) {
	agents := makeAgents("claude")
	qa := map[string]string{"ready_to_evaluate": "evaluating"}
	pools := map[string]config.PoolConfig{
		"evaluating": makePool(we("claude", 1)),
	}

	got, err := ResolveDispatchAgent(qa, "ready_to_evaluate", agents, pools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Config.Command != "claude" {
		t.Errorf("expected 'claude', got %q", got.Config.Command)
	}
	if got.ID != "claude" {
		t.Errorf("expected agent ID 'claude', got %q", got.ID)
	}
}

func TestResolveDispatchAgent_MissingState(t *testing.T) {
	agents := makeAgents("claude")
	qa := map[string]string{"implementation": "implementation"}
	pools := map[string]config.PoolConfig{
		"implementation": makePool(we("claude", 1)),
	}

	_, err := ResolveDispatchAgent(qa, "unknown_state", agents, pools)
	if err == nil {
		t.Fatal("expected error for missing state")
	}
	if _, ok := err.(*DispatchFailureError); !ok {
		t.Errorf("expected DispatchFailureError, got %T", err)
	}
}

func TestResolveDispatchAgent_MissingPool(t *testing.T) {
	agents := makeAgents("claude")
	qa := map[string]string{"ready_to_evaluate": "evaluating"}
	pools := map[string]config.PoolConfig{}

	_, err := ResolveDispatchAgent(qa, "ready_to_evaluate", agents, pools)
	if err == nil {
		t.Fatal("expected error for missing pool")
	}
	de, ok := err.(*DispatchFailureError)
	if !ok {
		t.Fatalf("expected DispatchFailureError, got %T", err)
	}
	if de.PoolKey != "evaluating" {
		t.Errorf("expected pool key 'evaluating', got %q", de.PoolKey)
	}
}

func TestResolveDispatchAgent_DanglingAgent(t *testing.T) {
	agents := makeAgents("claude")
	qa := map[string]string{"ready": "work"}
	pools := map[string]config.PoolConfig{
		"work": makePool(we("nonexistent", 5)),
	}

	_, err := ResolveDispatchAgent(qa, "ready", agents, pools)
	if err == nil {
		t.Fatal("expected error for dangling agent reference")
	}
}

func TestDispatchFailureError_Message(t *testing.T) {
	e := NewDispatchFailureError("implementation", "bead-1", "pool agents", "add agents to pool")
	msg := e.Error()
	if msg == "" {
		t.Fatal("error message should not be empty")
	}
	if !containsStr(msg, "KERNL DISPATCH FAILURE") {
		t.Errorf("expected marker in error message, got %q", msg)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestSelectByWeight_Empty(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	if got := selectByWeight(nil, rng); got != "" {
		t.Errorf("expected empty string for empty input, got %q", got)
	}
}

func TestSelectByWeight_TotalWeightZero(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	entries := []config.WeightedAgent{we("a", 0), we("b", 0)}
	if got := selectByWeight(entries, rng); got != "" {
		t.Errorf("expected empty string for zero total weight, got %q", got)
	}
}

func TestSelectByWeight_Single(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	entries := []config.WeightedAgent{we("only", 5)}
	if got := selectByWeight(entries, rng); got != "only" {
		t.Errorf("expected 'only', got %q", got)
	}
}

func TestSelectFromPool_NeverPicksUnregistered(t *testing.T) {
	agents := makeAgents("claude")
	pool := makePool(we("ghost", 10), we("claude", 1))

	for i := 0; i < 100; i++ {
		got, err := SelectFromPool(pool, agents)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Command != "claude" {
			t.Errorf("iteration %d: expected 'claude', got %q", i, got.Command)
		}
	}
}

func TestSelectFromPool_NeverReturnsFallbackSilently(t *testing.T) {
	agents := map[string]config.AgentConfig{}
	pool := makePool(we("anything", 1))

	_, err := SelectFromPool(pool, agents)
	if err == nil {
		t.Fatal("must fail loudly, not return fallback")
	}
	de, ok := err.(*DispatchFailureError)
	if !ok {
		t.Fatalf("expected DispatchFailureError, got %T", err)
	}
	if de.PoolKey == "" && de.Missing == "" {
		t.Error("failure error should name the missing thing")
	}
}

func BenchmarkSelectFromWeighted(b *testing.B) {
	agents := makeAgents("a", "b", "c", "d", "e")
	pool := makePool(we("a", 10), we("b", 5), we("c", 3), we("d", 2), we("e", 1))

	for i := 0; i < b.N; i++ {
		_, _ = SelectFromPool(pool, agents)
	}
	_ = fmt.Sprintf("bench complete")
}
