package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kernl.yaml")
	content := []byte(`
settings:
  agents:
    claude:
      command: claude
      type: claude
  pools:
    implementation:
      agents:
        - agentId: claude
          weight: 1
  defaults:
    interactiveSessionTimeoutMinutes: 30
registry:
  repos:
    - path: /tmp/test-repo
      memoryManager: beads
server:
  port: 9090
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Settings.Defaults.InteractiveSessionTimeoutMinutes != 30 {
		t.Errorf("expected timeout 30, got %d", cfg.Settings.Defaults.InteractiveSessionTimeoutMinutes)
	}
	if len(cfg.Settings.Agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(cfg.Settings.Agents))
	}
	if cfg.Settings.Agents["claude"].Command != "claude" {
		t.Errorf("expected command 'claude', got %s", cfg.Settings.Agents["claude"].Command)
	}
}

func TestLoadDefaultsPort(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kernl.yaml")
	content := []byte(`
settings:
  agents:
    stub:
      command: stub
  pools: {}
registry:
  repos: []
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Settings.Defaults.InteractiveSessionTimeoutMinutes != 10 {
		t.Errorf("expected default timeout 10, got %d", cfg.Settings.Defaults.InteractiveSessionTimeoutMinutes)
	}
}

func TestLoadAppliesOrchestratorDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kernl.yaml")
	content := []byte(`
settings:
  agents:
    stub:
      command: stub
  pools: {}
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Orchestrator.MaxConcurrentBeads != 5 {
		t.Errorf("MaxConcurrentBeads default = %d, want 5", cfg.Orchestrator.MaxConcurrentBeads)
	}
	if cfg.Orchestrator.WorktreeRoot == "" {
		t.Error("WorktreeRoot default must be set (~/.kernl/worktrees)")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/kernl.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kernl.yaml")
	content := []byte(`{invalid yaml [[[[`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadRejectsEmptyAgentsWithActionableError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kernl.yaml")
	content := []byte(`settings:
  agents: {}
  pools: {}
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for config with zero agents")
	}
	for _, want := range []string{"KERNL DISPATCH FAILURE", "settings.agents", "kernl.yaml.example"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}
}