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
