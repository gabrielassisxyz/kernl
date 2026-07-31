package llmendpoint

import (
	"strings"
	"testing"
)

func TestResolve_PerProviderDefaultsAndCustomEndpoint(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		endpoint string
		model    string
		want     string
	}{
		{"openai default", "openai", "", "gpt-4o", "https://api.openai.com/v1/chat/completions"},
		{"openai custom proxy", "openai", "http://localhost:4000", "gpt-4o", "http://localhost:4000/v1/chat/completions"},
		{"openai custom proxy trailing slash", "openai", "http://localhost:4000/", "gpt-4o", "http://localhost:4000/v1/chat/completions"},

		{"anthropic default", "anthropic", "", "claude-3-5-sonnet-20241022", "https://api.anthropic.com/v1/messages"},
		{"anthropic custom proxy", "anthropic", "http://localhost:4000", "claude-3-5-sonnet-20241022", "http://localhost:4000/v1/messages"},

		{"ollama default", "ollama", "", "llama3", "http://localhost:11434/api/generate"},
		{"ollama custom", "ollama", "http://localhost:4000", "llama3", "http://localhost:4000/api/generate"},

		{"gemini default embeds model", "gemini", "", "gemini-1.5-pro", "https://generativelanguage.googleapis.com/v1beta/models/gemini-1.5-pro:generateContent"},
		{"gemini custom proxy still embeds model", "gemini", "http://localhost:4000", "gemini-1.5-pro", "http://localhost:4000/v1beta/models/gemini-1.5-pro:generateContent"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(tc.provider, tc.endpoint, tc.model)
			if err != nil {
				t.Fatalf("Resolve(%q, %q, %q): unexpected error: %v", tc.provider, tc.endpoint, tc.model, err)
			}
			if got != tc.want {
				t.Errorf("Resolve(%q, %q, %q) = %q, want %q", tc.provider, tc.endpoint, tc.model, got, tc.want)
			}
		})
	}
}

func TestResolve_UnknownProviderFailsLoud(t *testing.T) {
	_, err := Resolve("does-not-exist", "http://localhost:4000", "some-model")
	if err == nil {
		t.Fatal("expected an error for an unknown provider, got nil")
	}
	got := err.Error()
	for _, want := range []string{"KERNL DISPATCH FAILURE", "does-not-exist", "llm.provider"} {
		if !strings.Contains(got, want) {
			t.Errorf("error %q does not contain %q (marker, offending provider, and config key to fix are all required)", got, want)
		}
	}
}

func TestBase_MatchesResolveForKnownProviders(t *testing.T) {
	base, err := Base("openai", "")
	if err != nil {
		t.Fatalf("Base: unexpected error: %v", err)
	}
	if base != "https://api.openai.com" {
		t.Errorf("Base(openai, \"\") = %q, want %q", base, "https://api.openai.com")
	}

	base, err = Base("openai", "http://localhost:4000/")
	if err != nil {
		t.Fatalf("Base: unexpected error: %v", err)
	}
	if base != "http://localhost:4000" {
		t.Errorf("Base(openai, custom) = %q, want trailing slash trimmed", base)
	}
}
