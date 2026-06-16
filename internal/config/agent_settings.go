package config

type AgentSettings struct {
	Agents  map[string]AgentConfig
	Actions ActionsConfig
	Pools   map[string]PoolConfig
}

func NewAgentSettings(cfg *Config) *AgentSettings {
	return &AgentSettings{
		Agents:  cfg.Settings.Agents,
		Actions: cfg.Settings.Actions,
		Pools:   cfg.Settings.Pools,
	}
}
