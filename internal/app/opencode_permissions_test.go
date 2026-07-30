package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
			Edit: "allow",
			Bash: "allow",
			Read: map[string]string{"/tmp/**": "allow"},
		},
	}
	data, _ := json.MarshalIndent(staticCfg, "", "  ")
	_ = os.WriteFile(staticCfgPath, data, 0644)

	worktree := filepath.Join(dir, "worktree")
	_ = os.MkdirAll(worktree, 0755)

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
	_ = os.WriteFile(staticCfgPath, data, 0644)

	outDir := filepath.Join(dir, "run", "kb-1")

	cfgPath, err := writeStageOpencodeConfig(staticCfgPath, outDir, "kb-1", "implementation", nil)
	if err != nil {
		t.Fatalf("writeStageOpencodeConfig: %v", err)
	}

	raw, _ := os.ReadFile(cfgPath)
	var cfg opencodeConfig
	_ = json.Unmarshal(raw, &cfg)

	editMap, _ := cfg.Permission.Edit.(map[string]any)
	if len(editMap) != 1 {
		t.Errorf("expected only the wildcard entry, got %d entries", len(editMap))
	}
	if editMap["*"] != "allow" {
		t.Errorf("expected * allow, got %v", editMap["*"])
	}
}

// A stage specialization adds deny rules; it must never hand back a policy
// wider than the one it started from. It used to rebuild the edit map from
// `{"*": "allow"}`, so an operator who denied a path in their own allowlist got
// it silently allowed again for every stage that had a contract.
func TestStageSpecializationKeepsTheConfiguredEditPolicy(t *testing.T) {
	dir := t.TempDir()
	staticCfgPath := filepath.Join(dir, "opencode-config.json")
	base := opencodeConfig{
		Permission: opencodePermission{
			Edit: map[string]string{"*": "allow", ".env": "deny", "secrets/**": "deny"},
			Bash: "allow",
		},
	}
	data, _ := json.MarshalIndent(base, "", "  ")
	if err := os.WriteFile(staticCfgPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	stages := map[string]backend.StageContract{
		"implementation": {ForbiddenPaths: []string{".kernl/**/plan.md"}},
	}

	cfgPath, err := writeStageOpencodeConfig(staticCfgPath, filepath.Join(dir, "run"), "kb-2", "implementation", stages)
	if err != nil {
		t.Fatalf("writeStageOpencodeConfig: %v", err)
	}

	raw, _ := os.ReadFile(cfgPath)
	var cfg opencodeConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse stage config: %v", err)
	}
	editMap, ok := cfg.Permission.Edit.(map[string]any)
	if !ok {
		t.Fatalf("expected edit permission to be a map, got %T", cfg.Permission.Edit)
	}
	for pattern, want := range map[string]string{
		".env":              "deny",
		"secrets/**":        "deny",
		".kernl/**/plan.md": "deny",
		"*":                 "allow",
	} {
		if editMap[pattern] != want {
			t.Errorf("edit[%q] = %v, want %q", pattern, editMap[pattern], want)
		}
	}
}

func TestStageSpecializationRefusesAnEditPolicyItCannotRead(t *testing.T) {
	dir := t.TempDir()
	staticCfgPath := filepath.Join(dir, "opencode-config.json")
	if err := os.WriteFile(staticCfgPath, []byte(`{"permission":{"edit":["allow"]}}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := writeStageOpencodeConfig(staticCfgPath, filepath.Join(dir, "run"), "kb-3", "implementation",
		map[string]backend.StageContract{"implementation": {ForbiddenPaths: []string{"**/*.go"}}})
	if err == nil {
		t.Fatal("expected a loud failure rather than dropping the configured edit policy")
	}
	if !strings.Contains(err.Error(), "permission.edit") {
		t.Fatalf("error must name the field it could not read, got: %v", err)
	}
}

// The allowlist used to be looked up at <target-repo>/orchestrator/…, a path
// that only ever existed inside kernl. It is kernl's policy, so kernl owns the
// file, and an operator who does name a path gets told when it is wrong.
func TestOpencodeAllowlistIsKernlOwned(t *testing.T) {
	t.Run("writes its own on first use", func(t *testing.T) {
		kernlDir := filepath.Join(t.TempDir(), ".kernl")

		path, err := resolveOpencodeBaseConfig("", kernlDir)
		if err != nil {
			t.Fatalf("resolveOpencodeBaseConfig: %v", err)
		}
		if !strings.HasPrefix(path, kernlDir) {
			t.Fatalf("allowlist at %q, want it under kernl's own dir %q", path, kernlDir)
		}
		var cfg opencodeConfig
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read allowlist: %v", err)
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("parse allowlist: %v", err)
		}
		// Without an external_directory rule opencode auto-rejects, which is
		// the failure this file exists to prevent.
		if cfg.Permission.ExternalDirectory == nil {
			t.Error("the built-in allowlist must say something about external_directory")
		}
	})

	t.Run("keeps an existing file", func(t *testing.T) {
		kernlDir := t.TempDir()
		path := filepath.Join(kernlDir, "opencode-config.json")
		if err := os.WriteFile(path, []byte(`{"permission":{"bash":"deny"}}`), 0644); err != nil {
			t.Fatal(err)
		}

		if _, err := resolveOpencodeBaseConfig("", kernlDir); err != nil {
			t.Fatalf("resolveOpencodeBaseConfig: %v", err)
		}
		raw, _ := os.ReadFile(path)
		if !strings.Contains(string(raw), `"deny"`) {
			t.Error("an allowlist already on disk must not be overwritten")
		}
	})

	t.Run("a configured path that is missing fails loud", func(t *testing.T) {
		_, err := resolveOpencodeBaseConfig(filepath.Join(t.TempDir(), "nope.json"), t.TempDir())
		if err == nil {
			t.Fatal("expected a loud failure rather than a silent fallback")
		}
		if !strings.Contains(err.Error(), "opencodeConfigPath") {
			t.Fatalf("error must name the config key that fixes it, got: %v", err)
		}
	})
}
