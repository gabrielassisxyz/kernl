package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kernl.yaml")
	content := []byte(`
settings:
  agents:
    opencode:
      command: opencode
      type: claude
  pools:
    implementation:
      agents:
        - agentId: opencode
          weight: 1
  defaults:
    interactiveSessionTimeoutMinutes: 10
registry:
  repos:
    - path: /tmp/test-repo
      memoryManager: beads
server:
  port: 8080
orchestrator:
  maxConcurrentBeads: 3
`)
	if err := os.WriteFile(cfgPath, content, 0644); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load test config: %v", err)
	}
	// Use the temp dir as the vault root so graph.db is created in isolation.
	cfg.Vault.Root = dir
	return cfg
}

func TestNewAppWiresEngineFromConfig(t *testing.T) {
	cfg := testConfig(t)
	a, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	defer a.Close()
	if a.Backend == nil {
		t.Error("NewApp must wire backend")
	}
	if a.Terminal == nil {
		t.Error("NewApp must wire terminal manager")
	}
	if a.SCM == nil {
		t.Error("NewApp must wire SCM")
	}
	if a.Driver == nil {
		t.Error("NewApp must wire driver")
	}
	if a.Graph == nil {
		t.Error("NewApp must wire graph")
	}
	if !a.Backend.Capabilities().CanCreate {
		t.Error("expected bd backend capabilities")
	}
}
