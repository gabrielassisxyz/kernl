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

// AnthropicClient calls the Anthropic Messages API.
type AnthropicClient struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

// NewAnthropicClient creates a new Anthropic client.
func NewAnthropicClient(cfg LLMProviderConfig) (*AnthropicClient, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("missing API key for the Anthropic provider")
	}
	baseURL := strings.TrimRight(cfg.Endpoint, "/")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	model := cfg.Model
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}
	return &AnthropicClient{
		apiKey:     cfg.APIKey,
		model:      model,
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}, nil
}

// Chat sends a message request to Anthropic.
func (c *AnthropicClient) Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	systemPrompt, chatMessages := splitSystemMessages(messages)

	reqBody := anthropicRequest{
		Model:     c.model,
		Messages:  convertToAnthropicMessages(chatMessages),
		Tools:     convertToAnthropicTools(tools),
		System:    systemPrompt,
		MaxTokens: 4096,
	}
	if len(reqBody.Tools) == 0 {
		reqBody.Tools = nil
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

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
		return nil, fmt.Errorf("error from Anthropic API (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var antResp anthropicResponse
	if err := json.Unmarshal(respBytes, &antResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	cr := &ChatResponse{}
	for _, block := range antResp.Content {
		switch block.Type {
		case "text":
			cr.Content += block.Text
		case "tool_use":
			cr.ToolCalls = append(cr.ToolCalls, ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: ToolFunction{
					Name:      block.Name,
					Arguments: string(mustMarshalJSON(block.Input)),
				},
			})
		}
	}
	return cr, nil
}

func splitSystemMessages(messages []Message) (string, []Message) {
	var system []string
	var rest []Message
	for _, m := range messages {
		if m.Role == "system" {
			system = append(system, m.Content)
		} else {
			rest = append(rest, m)
		}
	}
	return strings.Join(system, "\n\n"), rest
}

func convertToAnthropicMessages(messages []Message) []anthropicMessage {
	out := make([]anthropicMessage, 0, len(messages))
	for _, m := range messages {
		role := m.Role
		// Anthropic requires "user" or "assistant" roles.
		if role == "tool" {
			role = "user"
		}
		out = append(out, anthropicMessage{
			Role:    role,
			Content: []anthropicContentBlock{{Type: "text", Text: m.Content}},
		})
	}
	return out
}

func convertToAnthropicTools(tools []Tool) []anthropicTool {
	out := make([]anthropicTool, 0, len(tools))
	for _, t := range tools {
		var schema map[string]interface{}
		if t.Parameters != nil {
			if err := json.Unmarshal(t.Parameters, &schema); err != nil {
				schema = nil
			}
		}
		out = append(out, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}
	return out
}

func mustMarshalJSON(v interface{}) []byte {
	data, _ := json.Marshal(v)
	return data
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	Messages  []anthropicMessage `json:"messages"`
	System    string             `json:"system,omitempty"`
	MaxTokens int                `json:"max_tokens"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type anthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type anthropicResponse struct {
	Content []anthropicResponseBlock `json:"content"`
}

type anthropicResponseBlock struct {
	Type  string                 `json:"type"`
	Text  string                 `json:"text,omitempty"`
	ID    string                 `json:"id,omitempty"`
	Name  string                 `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`
}
