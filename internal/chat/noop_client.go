package chat

import "context"

// noopClient is a minimal LLM client that returns a fixed test response.
// Registered as provider "noop" for test environments or CI where no real
// LLM key is available.
type noopClient struct{}

func (n *noopClient) Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	return &ChatResponse{Content: "test"}, nil
}
