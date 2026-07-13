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
//
// ToolCalls and ToolCallID carry the tool-use half of the protocol. Without them
// a tool result was appended as a bare `role: "tool"` message with no assistant
// turn in front of it claiming the call — so the model never saw that IT had
// already run the tool, and called it again on the next pass. That is what a
// "the DA thinks, finishes, and says nothing" loop is made of.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// ToolCalls is set on an assistant turn that invoked tools.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// ToolCallID is set on a tool turn, naming the call it answers.
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ChatResponse is the LLM response — either content text or tool calls.
type ChatResponse struct {
	Content   string     `json:"content"`
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
