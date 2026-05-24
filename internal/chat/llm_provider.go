package chat

import (
	"fmt"
)

// ProviderRegistry maps provider names to factory functions.
type ProviderRegistry struct {
	factories map[string]func(cfg LLMProviderConfig) (LLMClient, error)
}

// LLMProviderConfig describes how to connect to an LLM provider.
type LLMProviderConfig struct {
	Provider string
	APIKey   string
	Model    string
	Endpoint string
}

// NewProviderRegistry creates a registry pre-populated with known providers.
func NewProviderRegistry() *ProviderRegistry {
	r := &ProviderRegistry{
		factories: make(map[string]func(LLMProviderConfig) (LLMClient, error)),
	}
	r.Register("openai", func(cfg LLMProviderConfig) (LLMClient, error) {
		return NewOpenAIClient(cfg)
	})
	r.Register("anthropic", func(cfg LLMProviderConfig) (LLMClient, error) {
		return NewAnthropicClient(cfg)
	})
	r.Register("ollama", func(cfg LLMProviderConfig) (LLMClient, error) {
		return NewOllamaClient(cfg)
	})
	r.Register("noop", func(cfg LLMProviderConfig) (LLMClient, error) {
		return &noopClient{}, nil
	})
	return r
}

// Register adds a custom provider factory.
func (r *ProviderRegistry) Register(name string, factory func(LLMProviderConfig) (LLMClient, error)) {
	r.factories[name] = factory
}

// Get returns an LLMClient for the named provider.
func (r *ProviderRegistry) Get(name string, cfg LLMProviderConfig) (LLMClient, error) {
	factory, ok := r.factories[name]
	if !ok {
		return nil, fmt.Errorf("unknown LLM provider %q (available: check your kernl.yaml llm.provider)", name)
	}
	return factory(cfg)
}

// NewProviderFromConfig creates an LLM client from a simple config struct.
// This is the primary entry point for the HTTP layer.
func NewProviderFromConfig(cfg LLMProviderConfig) (LLMClient, error) {
	registry := NewProviderRegistry()
	return registry.Get(cfg.Provider, cfg)
}
