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

	content, err := CompleteChat(ctx, llmCfg, p, 1024)
	if err != nil {
		return nil, err
	}

	lines := strings.SplitN(strings.TrimSpace(content), "\n", 2)
	res := &WorkflowInferenceResult{
		ShapeID: strings.TrimSpace(lines[0]),
	}
	if len(lines) > 1 {
		res.Rationale = strings.TrimSpace(lines[1])
	}
	return res, nil
}

// chatMessage is the one-role, one-content shape every provider this
// function speaks to accepts on the way in, and openai hands back inside its
// own response envelope on the way out - shared between the two so the
// second use is a re-read of the same type, not a second definition of an
// identical shape.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// CompleteChat sends prompt to the configured LLM and returns its raw text
// response. It is the only place in this repository that builds an LLM chat
// request, resolves a provider's default endpoint, sets provider auth
// headers, and unwraps the anthropic vs. openai response envelope -
// InferWorkflow above and internal/app's run-report composer both call this
// rather than each carrying its own copy, because provider envelope parsing
// is exactly the kind of logic that quietly drifts apart when duplicated
// across files.
func CompleteChat(ctx context.Context, llmCfg config.LLMConfig, prompt string, maxTokens int) (string, error) {
	if !llmCfg.IsSet() {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: llm.provider is not set - Fix: set llm.provider (and llm.model / llm.api_key as the provider requires) in kernl.yaml")
	}

	reqBody, _ := json.Marshal(map[string]any{
		"model": llmCfg.Model,
		"messages": []chatMessage{
			{Role: "user", Content: prompt},
		},
		"max_tokens": maxTokens,
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
		return "", err
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
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("LLM API returned %d: %s", resp.StatusCode, string(body))
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
				Message chatMessage `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(body, &openaiResp); err == nil && len(openaiResp.Choices) > 0 {
			content = openaiResp.Choices[0].Message.Content
		}
	}

	if content == "" {
		return "", fmt.Errorf("unexpected empty response from LLM")
	}
	return content, nil
}
