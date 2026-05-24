//go:build test

package chat

import "context"

// MockLLMClient returns a fixed response for testing.
type MockLLMClient struct {
	Content   string
	ToolCalls []ToolCall
}

func (m *MockLLMClient) Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	return &ChatResponse{
		Content:   m.Content,
		ToolCalls: m.ToolCalls,
	}, nil
}
