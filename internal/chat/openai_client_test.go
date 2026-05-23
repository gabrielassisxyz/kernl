package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIClient_Chat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected /v1/chat/completions, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Authorization header, got %q", r.Header.Get("Authorization"))
		}

		resp := openAIResponse{
			Choices: []openAIChoice{
				{
					Message: openAIMessage{
						Content: "Hello from OpenAI!",
						ToolCalls: []openAIToolCall{
							{
								ID:   "call_123",
								Type: "function",
								Function: openAIFuncCall{
									Name:      "read_node",
									Arguments: `{"node_id":"abc"}`,
								},
							},
						},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewOpenAIClient(LLMProviderConfig{
		APIKey:   "test-key",
		Model:    "gpt-4o",
		Endpoint: server.URL,
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient: %v", err)
	}

	resp, err := client.Chat(context.Background(), []Message{
		{Role: "user", Content: "Hello"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "Hello from OpenAI!" {
		t.Errorf("expected 'Hello from OpenAI!', got %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Function.Name != "read_node" {
		t.Errorf("expected tool 'read_node', got %q", resp.ToolCalls[0].Function.Name)
	}
}

func TestOpenAIClient_Chat_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid api key"}`))
	}))
	defer server.Close()

	client, err := NewOpenAIClient(LLMProviderConfig{
		APIKey:   "bad-key",
		Endpoint: server.URL,
	})
	if err != nil {
		t.Fatalf("NewOpenAIClient: %v", err)
	}

	_, err = client.Chat(context.Background(), []Message{
		{Role: "user", Content: "Hello"},
	}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestOpenAIClient_RequiresAPIKey(t *testing.T) {
	_, err := NewOpenAIClient(LLMProviderConfig{})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}
