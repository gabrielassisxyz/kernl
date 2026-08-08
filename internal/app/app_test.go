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

func TestGraphDBPathFallsBackToTheHomeDirectory(t *testing.T) {
	// The case a hand-built "cfg.Vault.Root + \"/.kernl-graph.db\"" got wrong:
	// with no vault configured it produced /.kernl-graph.db while the server
	// read ~/.kernl/.kernl-graph.db, so a capture written by the CLI was
	// invisible to the UI and looked like it had never been saved.
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := GraphDBPath(&config.Config{})
	if err != nil {
		t.Fatalf("GraphDBPath with no vault configured: %v", err)
	}
	want := filepath.Join(home, ".kernl", graphDBFileName)
	if path != want {
		t.Errorf("GraphDBPath = %q, want %q", path, want)
	}
}

func TestGraphDBPathUsesTheVaultRootWhenConfigured(t *testing.T) {
	cfg := &config.Config{}
	cfg.Vault.Root = t.TempDir()

	path, err := GraphDBPath(cfg)
	if err != nil {
		t.Fatalf("GraphDBPath: %v", err)
	}
	if want := filepath.Join(cfg.Vault.Root, graphDBFileName); path != want {
		t.Errorf("GraphDBPath = %q, want %q", path, want)
	}
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
