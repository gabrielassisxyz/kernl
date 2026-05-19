package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

type opencodePermission struct {
	Edit              any `json:"edit,omitempty"`
	Bash              any `json:"bash,omitempty"`
	Webfetch          any `json:"webfetch,omitempty"`
	Read              any `json:"read,omitempty"`
	ExternalDirectory any `json:"external_directory,omitempty"`
}

type opencodeConfig struct {
	Schema     string            `json:"$schema,omitempty"`
	Provider   any               `json:"provider,omitempty"`
	Permission opencodePermission `json:"permission,omitempty"`
}

// writeStageOpencodeConfig creates a per-stage opencode config file in the
// worktree that includes the stage contract's forbidden_paths as edit deny
// entries. The base config (provider, schema) is copied from the static
// config. Returns the path to the generated file.
func writeStageOpencodeConfig(staticConfigPath, worktree, beadID, stage string, stages map[string]backend.StageContract) (string, error) {
	baseCfg, err := loadOpencodeBase(staticConfigPath)
	if err != nil {
		return "", err
	}

	editRules := map[string]string{"*": "allow"}
	contract, hasContract := stages[stage]
	if hasContract {
		for _, fp := range contract.ForbiddenPaths {
			editRules[fp] = "deny"
		}
	}
	baseCfg.Permission.Edit = editRules

	dir := filepath.Join(worktree, "_kernl")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: creating _kernl config dir: %w", err)
	}

	configPath := filepath.Join(dir, fmt.Sprintf("opencode-%s-%s.json", beadID, stage))
	data, err := json.MarshalIndent(baseCfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: marshaling stage config: %w", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: writing stage config %s: %w", configPath, err)
	}
	return configPath, nil
}

func loadOpencodeBase(path string) (opencodeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return opencodeConfig{}, fmt.Errorf("KERNL DISPATCH FAILURE: reading opencode base config %s: %w", path, err)
	}
	var cfg opencodeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return opencodeConfig{}, fmt.Errorf("KERNL DISPATCH FAILURE: parsing opencode base config %s: %w", path, err)
	}
	return cfg, nil
}
