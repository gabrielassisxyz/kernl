package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicClient_Chat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected /v1/messages, got %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-ant-key" {
			t.Errorf("expected x-api-key header, got %q", r.Header.Get("x-api-key"))
		}

		resp := anthropicResponse{
			Content: []anthropicResponseBlock{
				{Type: "text", Text: "Hello from Anthropic!"},
				{Type: "tool_use", ID: "tool_001", Name: "read_node", Input: map[string]interface{}{"node_id": "xyz"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewAnthropicClient(LLMProviderConfig{
		APIKey:   "test-ant-key",
		Model:    "claude-3-5-sonnet-20241022",
		Endpoint: server.URL,
	})
	if err != nil {
		t.Fatalf("NewAnthropicClient: %v", err)
	}

	resp, err := client.Chat(context.Background(), []Message{
		{Role: "user", Content: "Hello"},
	}, []Tool{{
		Name:        "read_node",
		Description: "Read a node",
		Parameters:  json.RawMessage(`{"type":"object"}`),
	}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "Hello from Anthropic!" {
		t.Errorf("expected 'Hello from Anthropic!', got %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Function.Name != "read_node" {
		t.Errorf("expected tool 'read_node', got %q", resp.ToolCalls[0].Function.Name)
	}
}

func TestAnthropicClient_Chat_SystemMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := anthropicResponse{
			Content: []anthropicResponseBlock{
				{Type: "text", Text: "Understood."},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewAnthropicClient(LLMProviderConfig{
		APIKey:   "test-ant-key",
		Endpoint: server.URL,
	})
	if err != nil {
		t.Fatalf("NewAnthropicClient: %v", err)
	}

	resp, err := client.Chat(context.Background(), []Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Hi"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "Understood." {
		t.Errorf("expected 'Understood.', got %q", resp.Content)
	}
}

func TestAnthropicClient_RequiresAPIKey(t *testing.T) {
	_, err := NewAnthropicClient(LLMProviderConfig{})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}
