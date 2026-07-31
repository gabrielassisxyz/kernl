package dispatch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

// TestCompleteChat_UsesResolvedEndpointForCustomProxy reproduces the
// reported defect: llm.endpoint pointed at a local OpenAI-compatible proxy
// (e.g. http://localhost:4000) used to be posted to unchanged, while
// internal/chat's clients correctly extended it with the provider's known
// request path. A server that only answers on that known path stands in
// for such a proxy - it 404s any request that arrives at the bare base URL,
// which is exactly what the old code sent.
func TestCompleteChat_UsesResolvedEndpointForCustomProxy(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "hello from proxy"}},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	llmCfg := config.LLMConfig{
		Provider: "openai",
		APIKey:   "test-key",
		Model:    "gpt-4o",
		Endpoint: server.URL, // a base URL, the shape a local proxy config takes
	}

	got, err := CompleteChat(context.Background(), llmCfg, "hi", 128)
	if err != nil {
		t.Fatalf("CompleteChat: %v", err)
	}
	if got != "hello from proxy" {
		t.Errorf("CompleteChat = %q, want %q", got, "hello from proxy")
	}
}

// TestCompleteChat_UnknownProviderFailsLoud pins the KERNL DISPATCH FAILURE
// contract: an llm.provider this codebase does not know how to reach must
// halt with a marker naming the offending value and the config key to fix,
// not build a malformed request from an empty URL.
func TestCompleteChat_UnknownProviderFailsLoud(t *testing.T) {
	llmCfg := config.LLMConfig{
		Provider: "does-not-exist",
		APIKey:   "test-key",
	}

	_, err := CompleteChat(context.Background(), llmCfg, "hi", 128)
	if err == nil {
		t.Fatal("expected an error for an unknown provider, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "KERNL DISPATCH FAILURE") {
		t.Errorf("error %q does not carry the KERNL DISPATCH FAILURE marker", got)
	}
}

// TestCompleteChat_UnsupportedProviderRefusedBeforeDispatch covers a gap
// between two provider lists: llmendpoint.Resolve knows how to build a URL
// for ollama and gemini (internal/chat's ollama client, and the historical
// gemini default, both need it to), but CompleteChat's auth-header and
// response-parsing switches only handle anthropic and openai. Without a
// guard, ollama/gemini would resolve to a plausible URL, dispatch a request
// with no auth header, and fail downstream as an unexplained empty
// response - or, for ollama, actually reach a live server and get back an
// answer this function cannot parse. The server below answers instantly if
// asked to; a passing test proves CompleteChat never asks it.
func TestCompleteChat_UnsupportedProviderRefusedBeforeDispatch(t *testing.T) {
	for _, provider := range []string{"ollama", "gemini"} {
		t.Run(provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("CompleteChat dispatched a request for provider %q; it should have refused before reaching the network", provider)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			llmCfg := config.LLMConfig{
				Provider: provider,
				APIKey:   "test-key",
				Model:    "some-model",
				Endpoint: server.URL,
			}

			_, err := CompleteChat(context.Background(), llmCfg, "hi", 128)
			if err == nil {
				t.Fatalf("expected an error for unsupported provider %q, got nil", provider)
			}
			got := err.Error()
			if !strings.Contains(got, "KERNL DISPATCH FAILURE") || !strings.Contains(got, provider) {
				t.Errorf("error %q does not name the KERNL DISPATCH FAILURE marker and the offending provider %q", got, provider)
			}
		})
	}
}
