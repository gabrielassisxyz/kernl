package ingest

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/chat"
)

// fakeLLM is a minimal chat.LLMClient returning canned content for tests.
type fakeLLM struct {
	content string
}

func (f *fakeLLM) Chat(ctx context.Context, messages []chat.Message, tools []chat.Tool) (*chat.ChatResponse, error) {
	return &chat.ChatResponse{Content: f.content}, nil
}

func TestLLMExtractorParsesPlainJSON(t *testing.T) {
	content := `[
		{"type": "Create Page", "title": "New Concept", "payload": "snippet one"},
		{"type": "Update", "title": "Extend topic", "payload": "snippet two"}
	]`
	ex := NewLLMExtractor(&fakeLLM{content: content})

	actions, err := ex.ExtractActions(context.Background(), "doc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}
	if actions[0].Type != "Create Page" || actions[0].Title != "New Concept" || actions[0].Payload != "snippet one" {
		t.Errorf("unexpected first action: %+v", actions[0])
	}
	if actions[1].Type != "Update" {
		t.Errorf("unexpected second action type: %q", actions[1].Type)
	}
}

// The extractor is narrowed to implemented actions: Deep Research and Add
// Contradiction Callout must never reach the queue.
func TestLLMExtractorDropsRetiredActionTypes(t *testing.T) {
	content := `[
		{"type": "Deep Research", "title": "Verify claim", "payload": "x"},
		{"type": "Add Contradiction Callout", "title": "Conflict", "payload": "y"},
		{"type": "Create Page", "title": "Keep me", "payload": "z"}
	]`
	ex := NewLLMExtractor(&fakeLLM{content: content})

	actions, err := ex.ExtractActions(context.Background(), "doc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 action (retired types dropped), got %d", len(actions))
	}
	if actions[0].Type != "Create Page" {
		t.Errorf("expected Create Page, got %q", actions[0].Type)
	}
}

func TestLLMExtractorTolueratesFencesAndProse(t *testing.T) {
	content := "Sure, here are the actions:\n\n```json\n" +
		`[{"type": "Update", "title": "Extend topic", "payload": "more text"}]` +
		"\n```\n\nLet me know if you need more."
	ex := NewLLMExtractor(&fakeLLM{content: content})

	actions, err := ex.ExtractActions(context.Background(), "doc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Type != "Update" || actions[0].Payload != "more text" {
		t.Errorf("unexpected action: %+v", actions[0])
	}
}

func TestLLMExtractorDropsUnknownActionTypes(t *testing.T) {
	content := `[
		{"type": "Frobnicate", "title": "bogus", "payload": "x"},
		{"type": "Skip", "title": "Nothing actionable", "payload": "y"}
	]`
	ex := NewLLMExtractor(&fakeLLM{content: content})

	actions, err := ex.ExtractActions(context.Background(), "doc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 action (unknown type dropped), got %d", len(actions))
	}
	if actions[0].Type != "Skip" {
		t.Errorf("expected Skip, got %q", actions[0].Type)
	}
}

func TestLLMExtractorEmptyArrayReturnsNoActions(t *testing.T) {
	ex := NewLLMExtractor(&fakeLLM{content: "[]"})

	actions, err := ex.ExtractActions(context.Background(), "doc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("expected 0 actions, got %d", len(actions))
	}
}

func TestLLMExtractorGarbageReturnsNoActionsNoError(t *testing.T) {
	cases := []string{
		"",
		"I'm sorry, I can't help with that.",
		"```json\nnot json at all\n```",
		`{"type": "Create Page"}`, // object, not the expected array
	}
	for _, c := range cases {
		ex := NewLLMExtractor(&fakeLLM{content: c})
		actions, err := ex.ExtractActions(context.Background(), "doc")
		if err != nil {
			t.Errorf("content %q: unexpected error: %v", c, err)
		}
		if len(actions) != 0 {
			t.Errorf("content %q: expected 0 actions, got %d", c, len(actions))
		}
	}
}
