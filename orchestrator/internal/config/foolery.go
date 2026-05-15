package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type ActionsConfig struct {
	Take             string `yaml:"take,omitempty"`
	Scene            string `yaml:"scene,omitempty"`
	ScopeRefinement  string `yaml:"scopeRefinement,omitempty"`
	StaleGrooming    string `yaml:"staleGrooming,omitempty"`
}

type Settings struct {
	Agents   map[string]AgentConfig `yaml:"agents"`
	Actions  ActionsConfig           `yaml:"actions,omitempty"`
	Pools    map[string]PoolConfig   `yaml:"pools"`
	Defaults DefaultsConfig          `yaml:"defaults"`
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
	Path           string `yaml:"path"`
	MemoryManager string `yaml:"memoryManager"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type Config struct {
	Settings Settings      `yaml:"settings"`
	Registry RegistryConfig `yaml:"registry"`
	Server    ServerConfig  `yaml:"server"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("FOOLERY DISPATCH FAILURE: reading config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("FOOLERY DISPATCH FAILURE: parsing config %s: %w", path, err)
	}

	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}

	if cfg.Settings.Defaults.InteractiveSessionTimeoutMinutes == 0 {
		cfg.Settings.Defaults.InteractiveSessionTimeoutMinutes = 10
	}

	return &cfg, nil
}