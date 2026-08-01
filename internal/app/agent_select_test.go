package app

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

// configWithAgent builds the smallest config that resolves a single-agent
// pool, so each test varies only the command under examination.
func configWithAgent(command string) *config.Config {
	cfg := &config.Config{}
	cfg.Settings.Agents = map[string]config.AgentConfig{
		"the-agent": {Command: command, Model: "some-model"},
	}
	cfg.Settings.Pools = map[string]config.PoolConfig{
		"implementation": {Agents: []config.WeightedAgent{{AgentID: "the-agent", Weight: 1}}},
	}
	return cfg
}

// A command no dialect speaks used to resolve to claude and be spawned with
// claude's flags, so the only evidence was the other CLI rejecting arguments
// it never declared - in an agent log, naming neither the agent entry nor the
// pool that chose it. The failure belongs here, before any stage starts.
func TestResolveAgentForPool_RejectsCommandWithNoDialect(t *testing.T) {
	_, err := ResolveAgentForPool(configWithAgent("some-other-cli"), "implementation")
	if err == nil {
		t.Fatal("expected a dispatch failure for a command no dialect speaks")
	}
	msg := err.Error()
	for _, want := range []string{"KERNL DISPATCH FAILURE", "some-other-cli", "the-agent", "implementation", "Fix:"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %q", msg, want)
		}
	}
}

func TestResolveAgentForPool_AcceptsEveryKnownDialect(t *testing.T) {
	for _, command := range []string{"claude", "codex", "copilot", "opencode", "gemini", "pi", "agy"} {
		got, err := ResolveAgentForPool(configWithAgent(command), "implementation")
		if err != nil {
			t.Fatalf("ResolveAgentForPool with command %q: %v", command, err)
		}
		if got.Command != command {
			t.Errorf("Command = %q, want %q", got.Command, command)
		}
		if got.Model != "some-model" {
			t.Errorf("Model = %q, want the configured model to survive resolution", got.Model)
		}
	}
}

// An absolute path is how an agent is configured when the binary is not on
// PATH, and the dialect check must read the base name rather than reject it.
func TestResolveAgentForPool_AcceptsAnAbsolutePath(t *testing.T) {
	if _, err := ResolveAgentForPool(configWithAgent("/home/user/.local/bin/pi"), "implementation"); err != nil {
		t.Fatalf("absolute path rejected: %v", err)
	}
}
