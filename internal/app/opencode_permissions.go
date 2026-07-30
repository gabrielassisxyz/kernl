package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
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
	Schema     string             `json:"$schema,omitempty"`
	Provider   any                `json:"provider,omitempty"`
	Permission opencodePermission `json:"permission,omitempty"`
}

// writeStageOpencodeConfig creates a per-stage opencode config that adds the
// stage contract's forbidden_paths as edit deny entries. The base config
// (provider, schema, the rest of the permissions) is copied from the allowlist
// at staticConfigPath. Returns the path to the generated file.
//
// outDir is kernl's own directory, never the worktree: this is kernl's control
// file, and a file kernl drops inside the agent's tree gets swept up by the
// stage's own `git add -A` and travels into the target repository's pull
// request.
//
// artifactDir, when non-empty, is granted external_directory access scoped
// to this bead's own artifact directory - nowhere else. Exit-gate artifacts
// (plan.md, review verdicts, ...) now live there instead of inside the
// worktree, and opencode auto-rejects any external_directory path that is
// not explicitly allowed; without this rule every write the stage contract
// requires would be rejected, and renderOperatingRules tells the agent to
// treat that rejection as fatal.
func writeStageOpencodeConfig(staticConfigPath, outDir, beadID, stage, artifactDir string, stages map[string]backend.StageContract) (string, error) {
	baseCfg, err := loadOpencodeBase(staticConfigPath)
	if err != nil {
		return "", err
	}

	editRules, err := normalizeEditRules(baseCfg.Permission.Edit, staticConfigPath)
	if err != nil {
		return "", err
	}
	contract, hasContract := stages[stage]
	if hasContract {
		for _, fp := range contract.ForbiddenPaths {
			editRules[fp] = "deny"
		}
	}
	baseCfg.Permission.Edit = editRules

	if artifactDir != "" {
		extRules, err := normalizeExternalDirRules(baseCfg.Permission.ExternalDirectory, staticConfigPath)
		if err != nil {
			return "", err
		}
		extRules[filepath.Join(artifactDir, "**")] = "allow"
		baseCfg.Permission.ExternalDirectory = extRules
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: creating stage config dir %s: %w", outDir, err)
	}

	configPath := filepath.Join(outDir, fmt.Sprintf("opencode-%s-%s.json", beadID, stage))
	data, err := json.MarshalIndent(baseCfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: marshaling stage config: %w", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: writing stage config %s: %w", configPath, err)
	}
	return configPath, nil
}

// normalizeEditRules turns whatever the base allowlist says about edits into
// the per-pattern map a stage specialization can add deny entries to.
//
// Specialization used to discard this and start from `{"*": "allow"}`, so an
// operator who denied a path in their own allowlist got it allowed back the
// moment a stage contract existed - a specialization that widens the policy it
// specializes. opencode accepts either a bare verdict or a pattern map here, so
// both are carried over; a shape that is neither fails rather than being
// dropped, because the alternative is dispatching under a policy nobody wrote.
func normalizeEditRules(edit any, sourcePath string) (map[string]string, error) {
	switch v := edit.(type) {
	case nil:
		return map[string]string{"*": "allow"}, nil
	case string:
		return map[string]string{"*": v}, nil
	case map[string]string:
		return maps.Clone(v), nil
	case map[string]any:
		rules := make(map[string]string, len(v))
		for pattern, verdict := range v {
			s, ok := verdict.(string)
			if !ok {
				return nil, fmt.Errorf("KERNL DISPATCH FAILURE: opencode allowlist %s has permission.edit[%q] = %v, which is not a verdict string - Fix: make it \"allow\", \"ask\" or \"deny\"", sourcePath, pattern, verdict)
			}
			rules[pattern] = s
		}
		return rules, nil
	default:
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: opencode allowlist %s has a permission.edit of type %T, which is neither a verdict nor a pattern map - Fix: make it a string or an object of pattern -> verdict", sourcePath, edit)
	}
}

// normalizeExternalDirRules mirrors normalizeEditRules for the
// external_directory permission field, so a stage specialization can add its
// own artifact-directory allow rule without discarding whatever an operator
// already granted there.
//
// Unlike normalizeEditRules, a nil field normalizes to an empty map rather
// than "*": "allow" - external_directory is closed by default (opencode's
// own behavior, and this file's builtinOpencodeAllowlist only opens /tmp),
// and a stage specialization must not widen that just because the operator's
// own config left the field unset.
func normalizeExternalDirRules(externalDirectory any, sourcePath string) (map[string]string, error) {
	switch v := externalDirectory.(type) {
	case nil:
		return map[string]string{}, nil
	case string:
		return map[string]string{"*": v}, nil
	case map[string]string:
		return maps.Clone(v), nil
	case map[string]any:
		rules := make(map[string]string, len(v))
		for pattern, verdict := range v {
			s, ok := verdict.(string)
			if !ok {
				return nil, fmt.Errorf("KERNL DISPATCH FAILURE: opencode allowlist %s has permission.external_directory[%q] = %v, which is not a verdict string - Fix: make it \"allow\", \"ask\" or \"deny\"", sourcePath, pattern, verdict)
			}
			rules[pattern] = s
		}
		return rules, nil
	default:
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: opencode allowlist %s has a permission.external_directory of type %T, which is neither a verdict nor a pattern map - Fix: make it a string or an object of pattern -> verdict", sourcePath, externalDirectory)
	}
}

// builtinOpencodeAllowlist is the permission policy kernl hands to a dispatched
// opencode agent when the operator has not supplied one of their own. Edits are
// re-derived per stage from the contract's forbidden paths, so what matters
// here is the rest: bash and reads are open because a coding agent runs tests
// and reads its dependencies, while writes outside the worktree are not - only
// /tmp is reachable, and only to read. Without this file opencode falls back to
// its own defaults, which auto-reject external_directory and produce rejections
// that look nothing like a missing allowlist.
func builtinOpencodeAllowlist() opencodeConfig {
	return opencodeConfig{
		Schema: "https://opencode.ai/config.json",
		Permission: opencodePermission{
			Edit:              "allow",
			Bash:              "allow",
			Read:              map[string]string{"*": "allow"},
			ExternalDirectory: map[string]string{"/tmp/**": "allow"},
		},
	}
}

// ensureBuiltinOpencodeConfig writes kernl's own allowlist into dir on first
// run and returns its path. An existing file is left alone: it is kernl's
// default, but once written it is the operator's to edit.
//
// The create is O_EXCL rather than stat-then-write. Two beads of the same epic
// dispatch concurrently, and a check followed by an unconditional write lets
// the second one truncate a file the first had already customized - "written
// once, never overwritten" has to be one operation to mean anything.
func ensureBuiltinOpencodeConfig(dir string) (string, error) {
	path := filepath.Join(dir, "opencode-config.json")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: creating %s for kernl's opencode allowlist: %w", dir, err)
	}
	data, err := json.MarshalIndent(builtinOpencodeAllowlist(), "", "  ")
	if err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: marshaling kernl's opencode allowlist: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return path, nil
		}
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: writing kernl's opencode allowlist %s: %w", path, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: writing kernl's opencode allowlist %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: writing kernl's opencode allowlist %s: %w", path, err)
	}
	return path, nil
}

// resolveOpencodeBaseConfig answers where the permission allowlist comes from.
//
// It used to be <target-repo>/orchestrator/opencode-config.json: a path that
// only ever existed inside kernl, and not even there since the module moved to
// the repository root. So every dispatch failed to find it, warned, and fell
// through to opencode's defaults - which is how a run in someone else's
// repository ended up with a policy kernl never chose.
//
// An unset path uses kernl's own file, written on first run and with no
// warning. A path the operator did configure is a promise, so a missing one
// fails loud instead of silently becoming the default.
func resolveOpencodeBaseConfig(configured, kernlDir string) (string, error) {
	if configured == "" {
		return ensureBuiltinOpencodeConfig(kernlDir)
	}
	if _, err := os.Stat(configured); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: orchestrator.opencodeConfigPath points at %s, which cannot be read - %w - Fix: correct the path in kernl.yaml, or remove the key to use kernl's own allowlist", configured, err)
	}
	return configured, nil
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
