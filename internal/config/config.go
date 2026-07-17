package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type ActionsConfig struct {
	Take            string `yaml:"take,omitempty"`
	Scene           string `yaml:"scene,omitempty"`
	ScopeRefinement string `yaml:"scopeRefinement,omitempty"`
	StaleGrooming   string `yaml:"staleGrooming,omitempty"`
}

type Settings struct {
	Agents   map[string]AgentConfig `yaml:"agents"`
	Actions  ActionsConfig          `yaml:"actions,omitempty"`
	Pools    map[string]PoolConfig  `yaml:"pools"`
	Defaults DefaultsConfig         `yaml:"defaults"`
}

type AgentConfig struct {
	Command      string            `yaml:"command"`
	Args         []string          `yaml:"args,omitempty"`
	Env          map[string]string `yaml:"env,omitempty"`
	Type         string            `yaml:"type,omitempty"`
	Vendor       string            `yaml:"vendor,omitempty"`
	Provider     string            `yaml:"provider,omitempty"`
	AgentName    string            `yaml:"agent_name,omitempty"`
	LeaseModel   string            `yaml:"lease_model,omitempty"`
	Model        string            `yaml:"model,omitempty"`
	Flavor       string            `yaml:"flavor,omitempty"`
	Version      string            `yaml:"version,omitempty"`
	ApprovalMode string            `yaml:"approvalMode,omitempty"`
	Label        string            `yaml:"label,omitempty"`
}

type PoolConfig struct {
	Agents []WeightedAgent `yaml:"agents"`
}

type WeightedAgent struct {
	AgentID string `yaml:"agentId"`
	Weight  int    `yaml:"weight"`
}

type DefaultsConfig struct {
	InteractiveSessionTimeoutMinutes int `yaml:"interactiveSessionTimeoutMinutes"`
}

type RegistryConfig struct {
	Repos []RepoEntry `yaml:"repos"`
}

type RepoEntry struct {
	Path          string `yaml:"path"`
	MemoryManager string `yaml:"memoryManager"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type OrchestratorConfig struct {
	WorktreeRoot       string `yaml:"worktreeRoot,omitempty"`
	MaxConcurrentBeads int    `yaml:"maxConcurrentBeads,omitempty"`
	RunStatePath       string `yaml:"runStatePath,omitempty"`
	StageRetryAttempts int    `yaml:"stageRetryAttempts,omitempty"`
}

type SweepConfig struct {
	AutoIntervalSeconds int   `yaml:"auto_interval_seconds"`
	PRStaleWarnDays     int   `yaml:"pr_stale_warn_days"`
	FailureThreshold    int   `yaml:"failure_threshold"`
	BackoffMinutes      []int `yaml:"backoff_minutes"`
}

// VaultConfig holds settings for the notes vault watcher.
type VaultConfig struct {
	// Root is the absolute path to the vault directory.
	// When empty, vault watching is disabled.
	Root string `yaml:"root"`
	// CoalesceWindowMs is the fsnotify quiet period before emitting an event (ms).
	// Default: 300.
	CoalesceWindowMs int `yaml:"coalesceWindowMs,omitempty"`
	// MoveWindowMs is the move/delete correlation window (ms).
	// Default: 1000.
	MoveWindowMs int `yaml:"moveWindowMs,omitempty"`
	// RescanIntervalSec, when >0, triggers a periodic cold-start diff as a
	// safety net for missed fsnotify events. Default: 0 (disabled).
	RescanIntervalSec int `yaml:"rescanIntervalSec,omitempty"`
}

// Enabled reports whether vault watching is configured (root is non-empty).
func (v VaultConfig) Enabled() bool { return v.Root != "" }

// LLMConfig holds settings for the LLM chat providers.
type LLMConfig struct {
	Provider string `yaml:"provider"` // "openai" | "anthropic" | "ollama"
	APIKey   string `yaml:"api_key"`
	Model    string `yaml:"model"`
	Endpoint string `yaml:"endpoint"` // custom base URL, optional
}

// IsSet reports whether the LLM is configured (provider is non-empty).
func (l LLMConfig) IsSet() bool { return l.Provider != "" }

// InboxConfig holds settings for the inbox DA pre-processing.
type InboxConfig struct {
	// AutoPrep lets the classifier proactively generate a primer for captures it
	// reads as questions. The manual prep trigger works regardless.
	AutoPrep bool `yaml:"auto_prep,omitempty"`
	// DASubdir is the folder under the vault root where DA-authored notes (preps)
	// are materialized as markdown. Default: "DA".
	DASubdir string `yaml:"da_subdir,omitempty"`
	// AutoClassify seeds the runtime auto-classify switch the background
	// classifier reads each tick. A pointer, not a plain bool, because the
	// default is ON: a plain bool cannot tell "user set false" from "absent",
	// so `auto_classify: false` could never be persisted. nil ⇒ default true.
	AutoClassify *bool `yaml:"auto_classify,omitempty"`
}

// AutoClassifyEnabled resolves the tri-state AutoClassify to its effective
// boolean: unset means the default-ON.
func (c InboxConfig) AutoClassifyEnabled() bool {
	return c.AutoClassify == nil || *c.AutoClassify
}

type Config struct {
	Settings     Settings           `yaml:"settings"`
	Registry     RegistryConfig     `yaml:"registry"`
	Server       ServerConfig       `yaml:"server"`
	Orchestrator OrchestratorConfig `yaml:"orchestrator"`
	Sweep        SweepConfig        `yaml:"sweep"`
	Vault        VaultConfig        `yaml:"vault,omitempty"`
	LLM          LLMConfig          `yaml:"llm,omitempty"`
	Inbox        InboxConfig        `yaml:"inbox,omitempty"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: reading config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: parsing config %s: %w", path, err)
	}

	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}

	if cfg.Inbox.DASubdir == "" {
		cfg.Inbox.DASubdir = "DA"
	}

	if cfg.Settings.Defaults.InteractiveSessionTimeoutMinutes == 0 {
		cfg.Settings.Defaults.InteractiveSessionTimeoutMinutes = 10
	}

	if cfg.Orchestrator.MaxConcurrentBeads == 0 {
		cfg.Orchestrator.MaxConcurrentBeads = 5
	}
	if cfg.Orchestrator.StageRetryAttempts == 0 {
		cfg.Orchestrator.StageRetryAttempts = 2
	}
	if cfg.Orchestrator.WorktreeRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("KERNL DISPATCH FAILURE: cannot resolve home dir for worktree root default: %w", err)
		}
		cfg.Orchestrator.WorktreeRoot = filepath.Join(home, ".kernl", "worktrees")
	}
	if cfg.Orchestrator.RunStatePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("KERNL DISPATCH FAILURE: cannot resolve home dir for run-state path default: %w", err)
		}
		cfg.Orchestrator.RunStatePath = filepath.Join(home, ".kernl", "runstate.db")
	}

	if cfg.Sweep.AutoIntervalSeconds == 0 {
		cfg.Sweep.AutoIntervalSeconds = 60
	}
	if cfg.Sweep.PRStaleWarnDays == 0 {
		cfg.Sweep.PRStaleWarnDays = 7
	}
	if cfg.Sweep.FailureThreshold == 0 {
		cfg.Sweep.FailureThreshold = 3
	}
	if len(cfg.Sweep.BackoffMinutes) == 0 {
		cfg.Sweep.BackoffMinutes = []int{5, 15, 60}
	}

	if len(cfg.Settings.Agents) == 0 {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: %s defines zero agents under settings.agents — the orchestrator cannot dispatch. Fix: copy kernl.yaml.example and fill in at least one agent. Next: kernl doctor", path)
	}

	return &cfg, nil
}
