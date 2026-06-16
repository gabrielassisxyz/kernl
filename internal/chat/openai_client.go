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

// OpenAIClient calls the OpenAI /v1/chat/completions endpoint.
type OpenAIClient struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

// NewOpenAIClient creates a new OpenAI client.
func NewOpenAIClient(cfg LLMProviderConfig) (*OpenAIClient, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("OpenAI provider requires an API key")
	}
	baseURL := strings.TrimRight(cfg.Endpoint, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	model := cfg.Model
	if model == "" {
		model = "gpt-4o"
	}
	return &OpenAIClient{
		apiKey:     cfg.APIKey,
		model:      model,
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}, nil
}

// Chat sends a chat completion request to OpenAI.
func (c *OpenAIClient) Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	reqBody := openAIRequest{
		Model:    c.model,
		Messages: messages,
		Tools:    convertToOpenAITools(tools),
	}
	if len(reqBody.Tools) == 0 {
		reqBody.Tools = nil
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

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
		return nil, fmt.Errorf("OpenAI API error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var oaiResp openAIResponse
	if err := json.Unmarshal(respBytes, &oaiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if len(oaiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in OpenAI response")
	}

	choice := oaiResp.Choices[0]
	msg := choice.Message
	cr := &ChatResponse{
		Content: msg.Content,
	}
	for _, tc := range msg.ToolCalls {
		cr.ToolCalls = append(cr.ToolCalls, ToolCall{
			ID:       tc.ID,
			Type:     tc.Type,
			Function: ToolFunction{Name: tc.Function.Name, Arguments: tc.Function.Arguments},
		})
	}
	return cr, nil
}

type openAIRequest struct {
	Model    string       `json:"model"`
	Messages []Message    `json:"messages"`
	Tools    []openAITool `json:"tools,omitempty"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

func convertToOpenAITools(tools []Tool) []openAITool {
	out := make([]openAITool, len(tools))
	for i, t := range tools {
		out[i] = openAITool{
			Type:     "function",
			Function: openAIFunction(t),
		}
	}
	return out
}

type openAIResponse struct {
	Choices []openAIChoice `json:"choices"`
}

type openAIChoice struct {
	Message openAIMessage `json:"message"`
}

type openAIMessage struct {
	Content   string           `json:"content"`
	ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function openAIFuncCall `json:"function"`
}

type openAIFuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
