package dispatch

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

func TestCleanString(t *testing.T) {
	if v := CleanString("  hello  "); v != "hello" {
		t.Errorf("CleanString(`  hello  `) = %q, want %q", v, "hello")
	}
	if v := CleanString(""); v != "" {
		t.Errorf("CleanString(empty) = %q, want empty", v)
	}
	if v := CleanString("   "); v != "" {
		t.Errorf("CleanString(spaces) = %q, want empty", v)
	}
	if v := CleanString("hello world"); v != "hello world" {
		t.Errorf("CleanString(`hello world`) = %q, want %q", v, "hello world")
	}
}

func TestCanonicalizeRuntimeModel_Claude(t *testing.T) {
	tests := []struct {
		command string
		model   string
		want    string
	}{
		{"claude", "claude-opus-4.6", "claude-opus-4-6"},
		{"claude", "claude-sonnet-4-5", "claude-sonnet-4-5"},
		{"claude", "claude-haiku-4-5", "claude-haiku-4-5"},
		{"claude", "sonnet-4-5", "sonnet-4-5"},
		{"codex", "gpt-5.4", "gpt-5.4"},
		{"", "", ""},
		{"claude", "", ""},
		{"claude", "other-model", "other-model"},
	}

	for _, tt := range tests {
		got := CanonicalizeRuntimeModel(tt.command, tt.model)
		if got != tt.want {
			t.Errorf("CanonicalizeRuntimeModel(%q, %q) = %q, want %q", tt.command, tt.model, got, tt.want)
		}
	}
}

func TestNormalizeRegisteredAgentConfig_ClaudeBasic(t *testing.T) {
	agent := config.AgentConfig{
		Command: "claude",
		Model:   "claude-opus-4.6",
	}
	result := NormalizeRegisteredAgentConfig(agent)
	if result.Command != "claude" {
		t.Errorf("command = %q, want %q", result.Command, "claude")
	}
	if result.Provider != "Claude" {
		t.Errorf("provider = %q, want %q", result.Provider, "Claude")
	}
	if result.Model != "claude-opus-4-6" {
		t.Errorf("model = %q, want %q (dot->dash)", result.Model, "claude-opus-4-6")
	}
	if result.Flavor != "Opus" {
		t.Errorf("flavor = %q, want %q", result.Flavor, "Opus")
	}
	if result.Version != "4.6" {
		t.Errorf("version = %q, want %q", result.Version, "4.6")
	}
	if result.Type != "cli" {
		t.Errorf("type = %q, want %q", result.Type, "cli")
	}
	if result.Label == "" {
		t.Error("label should not be empty")
	}
}

func TestNormalizeRegisteredAgentConfig_CodexBasic(t *testing.T) {
	agent := config.AgentConfig{
		Command: "codex",
		Model:   "gpt-5.4-codex",
	}
	result := NormalizeRegisteredAgentConfig(agent)
	if result.Provider != "Codex" {
		t.Errorf("provider = %q, want %q", result.Provider, "Codex")
	}
	if result.AgentName != "Codex" {
		t.Errorf("agent_name = %q, want %q", result.AgentName, "Codex")
	}
}

func TestNormalizeRegisteredAgentConfig_WithApprovalMode(t *testing.T) {
	agent := config.AgentConfig{
		Command:      "claude",
		Model:        "claude-opus-4-7",
		ApprovalMode: "bypass",
	}
	result := NormalizeRegisteredAgentConfig(agent)
	if result.ApprovalMode != "bypass" {
		t.Errorf("approvalMode = %q, want %q", result.ApprovalMode, "bypass")
	}
}

func TestNormalizeRegisteredAgentConfig_EmptyCommand(t *testing.T) {
	agent := config.AgentConfig{
		Command: "claude",
	}
	result := NormalizeRegisteredAgentConfig(agent)
	if result.Provider != "Claude" {
		t.Errorf("provider = %q, want %q", result.Provider, "Claude")
	}
}

func TestNormalizeSettingsAgents_BasicNormalization(t *testing.T) {
	settings := config.Settings{
		Agents: map[string]config.AgentConfig{
			"my-claude": {
				Command: "claude",
				Model:   "claude-opus-4.6",
			},
		},
		Actions: config.ActionsConfig{},
		Pools:   map[string]config.PoolConfig{},
	}

	result := NormalizeSettingsAgents(settings)

	agent := result.Settings.Agents["my-claude"]
	if agent.Model != "claude-opus-4-6" {
		t.Errorf("model should be normalized from dots to dashes, got %q", agent.Model)
	}
	if agent.Provider != "Claude" {
		t.Errorf("provider should be derived, got %q", agent.Provider)
	}
}

func TestNormalizeSettingsAgents_ChangedPaths(t *testing.T) {
	settings := config.Settings{
		Agents: map[string]config.AgentConfig{
			"my-claude": {
				Command: "claude",
				Model:   "claude-opus-4.6",
			},
		},
		Actions: config.ActionsConfig{},
		Pools:   map[string]config.PoolConfig{},
	}

	result := NormalizeSettingsAgents(settings)

	hasModelChange := false
	for _, p := range result.ChangedPaths {
		if strings.Contains(p, "agents.my-claude.model") {
			hasModelChange = true
		}
	}
	if !hasModelChange {
		t.Errorf("expected model change path, got %v", result.ChangedPaths)
	}
}

func TestNormalizeSettingsAgents_ActionOrphanPruning(t *testing.T) {
	settings := config.Settings{
		Agents: map[string]config.AgentConfig{
			"claude-1": {Command: "claude"},
		},
		Actions: config.ActionsConfig{
			Take:            "deleted-agent",
			Scene:           "claude-1",
			ScopeRefinement: "deleted-agent",
			StaleGrooming:   "",
		},
		Pools: map[string]config.PoolConfig{},
	}

	result := NormalizeSettingsAgents(settings)

	if result.Settings.Actions.Take != "" {
		t.Errorf("pruned action 'take' should be empty, got %q", result.Settings.Actions.Take)
	}
	if result.Settings.Actions.Scene != "claude-1" {
		t.Errorf("valid action 'scene' should be kept, got %q", result.Settings.Actions.Scene)
	}
	if result.Settings.Actions.ScopeRefinement != "" {
		t.Errorf("pruned action 'scopeRefinement' should be empty, got %q", result.Settings.Actions.ScopeRefinement)
	}

	actionPruned := false
	for _, p := range result.ChangedPaths {
		if p == "actions.take" || p == "actions.scopeRefinement" {
			actionPruned = true
		}
	}
	if !actionPruned {
		t.Errorf("expected action pruning in changed paths, got %v", result.ChangedPaths)
	}
}

func TestNormalizeSettingsAgents_PoolOrphanPruning(t *testing.T) {
	settings := config.Settings{
		Agents: map[string]config.AgentConfig{
			"claude-1": {Command: "claude"},
		},
		Actions: config.ActionsConfig{},
		Pools: map[string]config.PoolConfig{
			"implementation": {
				Agents: []config.WeightedAgent{
					{AgentID: "claude-1", Weight: 1},
					{AgentID: "deleted-agent", Weight: 2},
				},
			},
			"review": {
				Agents: []config.WeightedAgent{
					{AgentID: "deleted-agent", Weight: 1},
				},
			},
		},
	}

	result := NormalizeSettingsAgents(settings)

	impl := result.Settings.Pools["implementation"]
	if len(impl.Agents) != 1 || impl.Agents[0].AgentID != "claude-1" {
		t.Errorf("implementation pool should have only claude-1, got %v", impl.Agents)
	}

	review := result.Settings.Pools["review"]
	if len(review.Agents) != 0 {
		t.Errorf("review pool should be empty after pruning, got %v", review.Agents)
	}
}

func TestNormalizeSettingsAgents_NoOrphanPruningWhenAgentsEmpty(t *testing.T) {
	settings := config.Settings{
		Agents:  map[string]config.AgentConfig{},
		Actions: config.ActionsConfig{Take: "some-agent"},
		Pools: map[string]config.PoolConfig{
			"step": {
				Agents: []config.WeightedAgent{
					{AgentID: "some-agent", Weight: 1},
				},
			},
		},
	}

	result := NormalizeSettingsAgents(settings)

	if result.Settings.Actions.Take != "some-agent" {
		t.Errorf("no pruning should occur when agents map is empty, got %q", result.Settings.Actions.Take)
	}
}

func TestNormalizeSettingsAgents_KeepsApprovalMode(t *testing.T) {
	settings := config.Settings{
		Agents: map[string]config.AgentConfig{
			"codex-1": {
				Command:      "codex",
				Model:        "gpt-5.4-codex",
				ApprovalMode: "prompt",
			},
		},
		Actions: config.ActionsConfig{},
		Pools:   map[string]config.PoolConfig{},
	}

	result := NormalizeSettingsAgents(settings)
	agent := result.Settings.Agents["codex-1"]

	if agent.ApprovalMode != "prompt" {
		t.Errorf("approvalMode should be preserved, got %q", agent.ApprovalMode)
	}
	if agent.Command != "codex" {
		t.Errorf("command should be preserved, got %q", agent.Command)
	}
}

func TestNormalizeSettingsAgents_DedupChangedPaths(t *testing.T) {
	settings := config.Settings{
		Agents: map[string]config.AgentConfig{
			"a": {Command: "claude", Model: "claude-opus-4.6"},
		},
		Actions: config.ActionsConfig{Take: "deleted"},
		Pools: map[string]config.PoolConfig{
			"step": {
				Agents: []config.WeightedAgent{
					{AgentID: "deleted", Weight: 1},
				},
			},
		},
	}

	result := NormalizeSettingsAgents(settings)

	seen := map[string]bool{}
	for _, p := range result.ChangedPaths {
		if seen[p] {
			t.Errorf("duplicate changed path: %q", p)
		}
		seen[p] = true
	}
}

func TestCanonicalizeRuntimeModel_NonClaude(t *testing.T) {
	got := CanonicalizeRuntimeModel("codex", "gpt-5.4")
	if got != "gpt-5.4" {
		t.Errorf("non-Claude model should pass through unchanged, got %q", got)
	}

	got = CanonicalizeRuntimeModel("opencode", "openrouter/z-ai/glm-5.1")
	if got != "openrouter/z-ai/glm-5.1" {
		t.Errorf("OpenCode model should pass through unchanged, got %q", got)
	}
}

func TestFieldStringValue(t *testing.T) {
	agent := config.AgentConfig{
		Command:  "  claude  ",
		Type:     "  cli  ",
		Provider: "Claude",
		Model:    "claude-opus-4-7",
		Label:    "  Opus 4.7  ",
	}

	if v := fieldStringValue(agent, "command"); v != "claude" {
		t.Errorf("fieldStringValue(command) = %q, want %q", v, "claude")
	}
	if v := fieldStringValue(agent, "provider"); v != "Claude" {
		t.Errorf("fieldStringValue(provider) = %q, want %q", v, "Claude")
	}
	if v := fieldStringValue(agent, "unknown_field"); v != "" {
		t.Errorf("fieldStringValue(unknown) should be empty, got %q", v)
	}
}