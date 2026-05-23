package vault_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/vault"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

// --- Config unit tests ---

func TestApplyDefaults_ZeroValues(t *testing.T) {
	v := config.VaultConfig{}
	vault.ApplyDefaults(&v)
	if v.CoalesceWindowMs != 300 {
		t.Errorf("CoalesceWindowMs = %d, want 300", v.CoalesceWindowMs)
	}
	if v.MoveWindowMs != 1000 {
		t.Errorf("MoveWindowMs = %d, want 1000", v.MoveWindowMs)
	}
}

func TestApplyDefaults_ExplicitValues(t *testing.T) {
	v := config.VaultConfig{CoalesceWindowMs: 500, MoveWindowMs: 2000}
	vault.ApplyDefaults(&v)
	if v.CoalesceWindowMs != 500 {
		t.Errorf("CoalesceWindowMs = %d, want 500", v.CoalesceWindowMs)
	}
	if v.MoveWindowMs != 2000 {
		t.Errorf("MoveWindowMs = %d, want 2000", v.MoveWindowMs)
	}
}

func TestValidate_EmptyRoot_Disabled(t *testing.T) {
	// Empty root is disabled — not an error.
	v := config.VaultConfig{}
	if err := vault.Validate(v); err != nil {
		t.Errorf("Validate(empty root) = %v, want nil", err)
	}
	if v.Enabled() {
		t.Error("Enabled() = true with empty root, want false")
	}
}

func TestValidate_ValidDir(t *testing.T) {
	dir := t.TempDir()
	v := config.VaultConfig{Root: dir}
	if err := vault.Validate(v); err != nil {
		t.Errorf("Validate(valid dir) = %v, want nil", err)
	}
	if !v.Enabled() {
		t.Error("Enabled() = false with non-empty root, want true")
	}
}

func TestValidate_NonExistentPath(t *testing.T) {
	v := config.VaultConfig{Root: "/tmp/kernl-test-nonexistent-12345678"}
	err := vault.Validate(v)
	if err == nil {
		t.Fatal("Validate(nonexistent) = nil, want error")
	}
}

func TestValidate_FileNotDir(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.md")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	v := config.VaultConfig{Root: f.Name()}
	err = vault.Validate(v)
	if err == nil {
		t.Fatal("Validate(file) = nil, want error")
	}
}

// --- kernl.yaml.example still parses ---

func TestExampleConfigParses(t *testing.T) {
	// Find repo root relative to this test file's location.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("cannot determine test file path")
	}
	// thisFile is .../internal/vault/config_test.go
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	examplePath := filepath.Join(repoRoot, "kernl.yaml.example")

	data, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("read kernl.yaml.example: %v", err)
	}

	import_yaml(t, data)
}

// import_yaml verifies the file is parseable as YAML without invoking config.Load
// (which requires at least one agent). We use gopkg.in/yaml.v3 indirectly via
// config.VaultConfig to stay in the same package graph.
func import_yaml(t *testing.T, data []byte) {
	t.Helper()
	// The example config is valid YAML but Load() would fail (no agents path).
	// Just verify YAML syntax is intact by parsing into a generic map.
	// We can't import yaml directly without adding a dependency, so parse
	// via os.CreateTemp + config subset.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	// Load will fail with "zero agents" but not a YAML parse error.
	// A YAML parse error contains "parsing config" in the message.
	_, err := loadConfig(cfgPath)
	if err != nil && isYAMLError(err) {
		t.Fatalf("kernl.yaml.example YAML parse error: %v", err)
	}
}

func loadConfig(path string) (*config.Config, error) {
	return config.Load(path)
}

func isYAMLError(err error) bool {
	return err != nil && contains(err.Error(), "parsing config")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}

// --- Start/Stop lifecycle integration test ---

func TestServiceStartStop_NoLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	// Open a real graph (in-memory for speed).
	g, err := graph.Open(context.Background(), graph.Config{InMemory: true, Path: t.Name()})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	t.Cleanup(func() { g.Close() })

	cfg := config.VaultConfig{Root: dir}
	vault.ApplyDefaults(&cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	svc := vault.New(g, cfg)
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Give the watcher a moment to set up watches.
	time.Sleep(50 * time.Millisecond)

	// Stop must return cleanly.
	done := make(chan struct{})
	go func() {
		svc.Stop()
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return within 5s — possible goroutine leak")
	}
}

func TestServiceStartStop_CreateFileReconciled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	g, err := graph.Open(context.Background(), graph.Config{InMemory: true, Path: t.Name()})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	t.Cleanup(func() { g.Close() })

	cfg := config.VaultConfig{
		Root:             dir,
		CoalesceWindowMs: 50, // short for fast test
		MoveWindowMs:     200,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	svc := vault.New(g, cfg)
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer svc.Stop()

	// Give watcher time to set up directory watches.
	time.Sleep(100 * time.Millisecond)

	// Create a markdown file.
	notePath := filepath.Join(dir, "hello.md")
	content := "---\ntitle: Hello\n---\n\nHello world.\n"
	if err := os.WriteFile(notePath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Poll until the note appears in the path cache (reconciled), with a deadline.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, found, err := reconcile.Lookup(ctx, g, "hello.md")
		if err != nil {
			t.Fatalf("Lookup: %v", err)
		}
		if found {
			return // success
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("note not reconciled within 5s — event routing or watcher may be broken")
}
