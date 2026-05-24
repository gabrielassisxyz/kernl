package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaClient_Chat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/generate" {
			t.Errorf("expected /api/generate, got %s", r.URL.Path)
		}

		var req ollamaRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "llama3" {
			t.Errorf("expected model 'llama3', got %q", req.Model)
		}
		if req.Stream {
			t.Error("expected stream=false")
		}

		resp := ollamaResponse{Response: "Hello from Ollama!"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewOllamaClient(LLMProviderConfig{
		Model:    "llama3",
		Endpoint: server.URL,
	})
	if err != nil {
		t.Fatalf("NewOllamaClient: %v", err)
	}

	resp, err := client.Chat(context.Background(), []Message{
		{Role: "user", Content: "Hello"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "Hello from Ollama!" {
		t.Errorf("expected 'Hello from Ollama!', got %q", resp.Content)
	}
}

func TestOllamaClient_Chat_WithTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ollamaRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		// Verify tools are embedded in the prompt.
		if req.Prompt == "" {
			t.Error("expected non-empty prompt")
		}

		resp := ollamaResponse{Response: "I'll read that node."}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewOllamaClient(LLMProviderConfig{
		Endpoint: server.URL,
	})
	if err != nil {
		t.Fatalf("NewOllamaClient: %v", err)
	}

	resp, err := client.Chat(context.Background(), []Message{
		{Role: "user", Content: "Read node abc"},
	}, []Tool{{
		Name:        "read_node",
		Description: "Read a graph node",
	}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "I'll read that node." {
		t.Errorf("expected 'I'll read that node.', got %q", resp.Content)
	}
}

func TestOllamaClient_DefaultEndpoint(t *testing.T) {
	client, err := NewOllamaClient(LLMProviderConfig{})
	if err != nil {
		t.Fatalf("NewOllamaClient: %v", err)
	}
	if client.baseURL != "http://localhost:11434" {
		t.Errorf("expected default endpoint, got %q", client.baseURL)
	}
	if client.model != "llama3" {
		t.Errorf("expected default model 'llama3', got %q", client.model)
	}
}
