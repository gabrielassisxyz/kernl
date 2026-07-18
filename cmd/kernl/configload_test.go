package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCLIConfigMissingFileTeachesRecovery(t *testing.T) {
	_, err := loadCLIConfig(filepath.Join(t.TempDir(), "kernl.yaml"))
	if err == nil {
		t.Fatal("missing config must error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Fix:") || !strings.Contains(msg, "--config") {
		t.Errorf("missing-config error must name the Fix and --config, got: %v", msg)
	}
	if strings.Count(msg, "KERNL DISPATCH FAILURE") != 1 {
		t.Errorf("marker must appear exactly once, got: %v", msg)
	}
}

func TestLoadCLIConfigPassesThroughValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kernl.yaml")
	yaml := "settings:\n  agents:\n    a1:\n      command: echo\n"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadCLIConfig(path)
	if err != nil || cfg == nil {
		t.Fatalf("valid config must load, got cfg=%v err=%v", cfg, err)
	}
}
