package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

func TestForbiddenPathsRejectedAtSandbox(t *testing.T) {
	// Create a static opencode config template so writeStageOpencodeConfig
	// has something to base the per-stage config on.
	dir := t.TempDir()
	staticCfgPath := filepath.Join(dir, "opencode-config.json")
	staticCfg := opencodeConfig{
		Permission: opencodePermission{
			Edit:     "allow",
			Bash:     "allow",
			Read:     map[string]string{"/tmp/**": "allow"},
		},
	}
	data, _ := json.MarshalIndent(staticCfg, "", "  ")
	os.WriteFile(staticCfgPath, data, 0644)

	worktree := filepath.Join(dir, "worktree")
	os.MkdirAll(worktree, 0755)

	stages := map[string]backend.StageContract{
		"planning": {
			Role: "Plan the work.",
			ForbiddenPaths: []string{
				"**/*.go",
				"**/*.ts",
			},
		},
	}

	cfgPath, err := writeStageOpencodeConfig(staticCfgPath, worktree, "kb-1", "planning", stages)
	if err != nil {
		t.Fatalf("writeStageOpencodeConfig: %v", err)
	}

	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read stage config: %v", err)
	}

	var cfg opencodeConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse stage config: %v", err)
	}

	editMap, ok := cfg.Permission.Edit.(map[string]any)
	if !ok {
		t.Fatal("expected edit permission to be a map")
	}
	if editMap["*"] != "allow" {
		t.Errorf("expected * allow, got %v", editMap["*"])
	}
	if editMap["**/*.go"] != "deny" {
		t.Errorf("expected **/*.go deny, got %v", editMap["**/*.go"])
	}
	if editMap["**/*.ts"] != "deny" {
		t.Errorf("expected **/*.ts deny, got %v", editMap["**/*.ts"])
	}
}

func TestForbiddenPathsEmptyWhenNoContract(t *testing.T) {
	dir := t.TempDir()
	staticCfgPath := filepath.Join(dir, "opencode-config.json")
	data, _ := json.MarshalIndent(opencodeConfig{Permission: opencodePermission{Edit: "allow"}}, "", "  ")
	os.WriteFile(staticCfgPath, data, 0644)

	worktree := filepath.Join(dir, "worktree")
	os.MkdirAll(worktree, 0755)

	cfgPath, err := writeStageOpencodeConfig(staticCfgPath, worktree, "kb-1", "implementation", nil)
	if err != nil {
		t.Fatalf("writeStageOpencodeConfig: %v", err)
	}

	raw, _ := os.ReadFile(cfgPath)
	var cfg opencodeConfig
	json.Unmarshal(raw, &cfg)

	editMap, _ := cfg.Permission.Edit.(map[string]any)
	if len(editMap) != 1 {
		t.Errorf("expected only the wildcard entry, got %d entries", len(editMap))
	}
	if editMap["*"] != "allow" {
		t.Errorf("expected * allow, got %v", editMap["*"])
	}
}
