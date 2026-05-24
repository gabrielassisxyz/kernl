package chat

import (
	"context"
	"encoding/json"
)

// LLMClient is the interface for LLM providers.
type LLMClient interface {
	Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error)
}

// Message represents a single message in a chat conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse is the LLM response — either content text or tool calls.
type ChatResponse struct {
	Content   string    `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall represents a tool invocation requested by the LLM.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction carries the function name and arguments.
type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool describes a tool available to the LLM.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}
