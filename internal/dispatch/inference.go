package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/prompt"
)

// WorkflowInferenceResult represents the outcome of U2 workflow inference.
type WorkflowInferenceResult struct {
	ShapeID   string
	Rationale string
}

// InferWorkflow asks the configured LLM to pick a workflow shape for an epic.
func InferWorkflow(ctx context.Context, llmCfg config.LLMConfig, epicBead *backend.Bead) (*WorkflowInferenceResult, error) {
	if epicBead == nil {
		return nil, fmt.Errorf("epicBead is nil")
	}

	if !llmCfg.IsSet() {
		return nil, fmt.Errorf("LLM config is not set, cannot infer workflow autonomously")
	}

	p := prompt.RenderWorkflowInfer(prompt.WorkflowInferInput{
		Title:       epicBead.Title,
		Description: epicBead.Description,
		Shapes:      []string{"vibe-coding-pipeline", "brainstorm-shape", "worker"},
	})

	// Perform raw HTTP POST to avoid importing internal/chat and causing import cycle
	type Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	reqBody, _ := json.Marshal(map[string]any{
		"model": llmCfg.Model,
		"messages": []Message{
			{Role: "user", Content: p},
		},
		"max_tokens": 1024,
	})

	endpoint := llmCfg.Endpoint
	if endpoint == "" && llmCfg.Provider == "anthropic" {
		endpoint = "https://api.anthropic.com/v1/messages"
	} else if endpoint == "" && llmCfg.Provider == "openai" {
		endpoint = "https://api.openai.com/v1/chat/completions"
	} else if endpoint == "" && llmCfg.Provider == "gemini" {
		endpoint = "https://generativelanguage.googleapis.com/v1beta/models/" + llmCfg.Model + ":generateContent"
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	switch llmCfg.Provider {
	case "anthropic":
		httpReq.Header.Set("x-api-key", llmCfg.APIKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")
	case "openai":
		httpReq.Header.Set("Authorization", "Bearer "+llmCfg.APIKey)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("LLM API returned %d: %s", resp.StatusCode, string(body))
	}

	var content string
	switch llmCfg.Provider {
	case "anthropic":
		var anthropicResp struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(body, &anthropicResp); err == nil && len(anthropicResp.Content) > 0 {
			content = anthropicResp.Content[0].Text
		}
	case "openai":
		var openaiResp struct {
			Choices []struct {
				Message Message `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(body, &openaiResp); err == nil && len(openaiResp.Choices) > 0 {
			content = openaiResp.Choices[0].Message.Content
		}
	}

	lines := strings.SplitN(strings.TrimSpace(content), "\n", 2)
	if len(lines) == 0 || content == "" {
		return nil, fmt.Errorf("unexpected empty response from LLM")
	}

	res := &WorkflowInferenceResult{
		ShapeID: strings.TrimSpace(lines[0]),
	}
	if len(lines) > 1 {
		res.Rationale = strings.TrimSpace(lines[1])
	}
	return res, nil
}
