// Package llmendpoint is the single place that turns llm.provider and
// llm.endpoint into the URL an HTTP client actually posts to. Before this
// package existed, internal/chat's provider clients and
// internal/dispatch.CompleteChat each read llm.endpoint under a different
// assumption - one treated it as a base URL to extend, the other as a
// complete URL to use unchanged - and no single value satisfied both.
package llmendpoint

import (
	"fmt"
	"strings"
)

// providerSpec pairs a provider's default base URL (used when llm.endpoint
// is empty) with the request path this codebase's clients always append to
// whatever base is in effect, configured or defaulted alike. model is
// consulted only by gemini, whose request path carries the model name;
// every other provider's path func ignores it.
type providerSpec struct {
	defaultBase string
	path        func(model string) string
}

var providers = map[string]providerSpec{
	"openai": {
		defaultBase: "https://api.openai.com",
		path:        func(string) string { return "/v1/chat/completions" },
	},
	"anthropic": {
		defaultBase: "https://api.anthropic.com",
		path:        func(string) string { return "/v1/messages" },
	},
	"ollama": {
		defaultBase: "http://localhost:11434",
		path:        func(string) string { return "/api/generate" },
	},
	// gemini's request path embeds the model name instead of carrying it in
	// the request body like every other provider here. A custom endpoint
	// still gets this path appended: a gemini-compatible proxy is expected
	// to accept the same path shape Gemini's own API does, exactly as an
	// OpenAI-compatible proxy is expected to accept /v1/chat/completions
	// rather than the bare base URL.
	"gemini": {
		defaultBase: "https://generativelanguage.googleapis.com",
		path:        func(model string) string { return "/v1beta/models/" + model + ":generateContent" },
	},
}

// Base resolves llm.endpoint to the base URL a provider client dials:
// endpoint itself, trimmed of a trailing slash, or the provider's own
// default when endpoint is empty. llm.endpoint is always a base URL - a
// scheme and host a request path gets appended to - never a complete
// request URL; see Resolve for that.
func Base(provider, endpoint string) (string, error) {
	spec, ok := providers[provider]
	if !ok {
		return "", unknownProviderError(provider)
	}
	return resolveBase(spec, endpoint), nil
}

// Resolve builds the complete request URL for provider: the base from
// Base(), plus the fixed request path that provider's API expects. This is
// the one place both internal/chat's provider clients and
// internal/dispatch.CompleteChat call, so llm.endpoint gets exactly one
// interpretation across the codebase.
func Resolve(provider, endpoint, model string) (string, error) {
	spec, ok := providers[provider]
	if !ok {
		return "", unknownProviderError(provider)
	}
	return resolveBase(spec, endpoint) + spec.path(model), nil
}

func resolveBase(spec providerSpec, endpoint string) string {
	base := strings.TrimRight(endpoint, "/")
	if base == "" {
		base = spec.defaultBase
	}
	return base
}

func unknownProviderError(provider string) error {
	return fmt.Errorf("KERNL DISPATCH FAILURE: unknown LLM provider %q - fix: set llm.provider to one of openai, anthropic, ollama, gemini in kernl.yaml", provider)
}
