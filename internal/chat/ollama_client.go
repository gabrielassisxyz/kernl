package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OllamaClient calls a local Ollama /api/generate endpoint.
type OllamaClient struct {
	model      string
	baseURL    string
	httpClient *http.Client
}

// NewOllamaClient creates a new Ollama client.
func NewOllamaClient(cfg LLMProviderConfig) (*OllamaClient, error) {
	baseURL := strings.TrimRight(cfg.Endpoint, "/")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	model := cfg.Model
	if model == "" {
		model = "llama3"
	}
	return &OllamaClient{
		model:      model,
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}, nil
}

// Chat sends a generate request to Ollama. Tools are not yet supported by
// the Ollama generate API, so they are embedded in the prompt.
func (c *OllamaClient) Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	prompt := buildOllamaPrompt(messages, tools)

	reqBody := ollamaRequest{
		Model:  c.model,
		Prompt: prompt,
		Stream: false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error from Ollama API (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var oResp ollamaResponse
	if err := json.Unmarshal(respBytes, &oResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &ChatResponse{Content: oResp.Response}, nil
}

func buildOllamaPrompt(messages []Message, tools []Tool) string {
	var b strings.Builder
	for _, m := range messages {
		fmt.Fprintf(&b, "[%s] %s\n", m.Role, m.Content)
	}
	if len(tools) > 0 {
		b.WriteString("\nAvailable tools:\n")
		for _, t := range tools {
			fmt.Fprintf(&b, "- %s: %s\n", t.Name, t.Description)
		}
		b.WriteString("\nIf you need to use a tool, output JSON in this format: {\"tool\": \"name\", \"args\": {...}}\n")
	}
	return b.String()
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
}
