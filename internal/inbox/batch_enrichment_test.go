package inbox

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/chat"
)

type batchTestLLM struct {
	content string
}

func (m *batchTestLLM) Chat(ctx context.Context, messages []chat.Message, tools []chat.Tool) (*chat.ChatResponse, error) {
	return &chat.ChatResponse{Content: m.content}, nil
}

func TestEnricherUnavailableWithoutLLM(t *testing.T) {
	e := NewBatchEnricher(nil)
	input := BatchEnrichmentInput{
		Source:      "whatsapp",
		Separator:   "whatsapp",
		ContextHint: "planning",
		RawSegments: []BatchSegment{
			{Body: "Project idea", Sequence: 0, ParseConfidence: "high"},
			{Body: "Task idea", Sequence: 1, ParseConfidence: "high"},
		},
	}
	result := e.Enrich(context.Background(), input)
	if result.Status != EnrichmentUnavailable {
		t.Fatalf("Status = %q, want unavailable", result.Status)
	}
	if len(result.Segments) != 2 {
		t.Fatalf("Segments = %d, want 2", len(result.Segments))
	}
	if result.ContextTitle != "planning" {
		t.Fatalf("ContextTitle = %q", result.ContextTitle)
	}
}

func TestEnricherGroupsRelatedSegments(t *testing.T) {
	llm := &batchTestLLM{content: `{
		"context_title": "ai-memory explainer",
		"segments": [
			{"body": "Build an ai-memory explainer project with architecture map and usage examples", "source_sequences": [0, 1, 2], "kind_hint": "project", "confidence": "high"}
		]
	}`}
	e := NewBatchEnricher(llm)
	input := BatchEnrichmentInput{
		Source:    "whatsapp",
		Separator: "whatsapp",
		RawSegments: []BatchSegment{
			{Body: "Build an ai-memory explainer project", Sequence: 0, ParseConfidence: "high"},
			{Body: "Task: map the repo architecture", Sequence: 1, ParseConfidence: "high"},
			{Body: "Task: write usage examples", Sequence: 2, ParseConfidence: "high"},
		},
	}
	result := e.Enrich(context.Background(), input)
	if result.Status != EnrichmentApplied {
		t.Fatalf("Status = %q, want applied", result.Status)
	}
	if result.ContextTitle != "ai-memory explainer" {
		t.Fatalf("ContextTitle = %q", result.ContextTitle)
	}
	if len(result.Segments) != 1 {
		t.Fatalf("Segments = %d, want 1", len(result.Segments))
	}
	if len(result.Segments[0].SourceSequences) != 3 {
		t.Fatalf("SourceSequences = %v", result.Segments[0].SourceSequences)
	}
}

func TestEnricherFallsBackOnInvalidJSON(t *testing.T) {
	llm := &batchTestLLM{content: "not json"}
	e := NewBatchEnricher(llm)
	input := BatchEnrichmentInput{
		Source:    "whatsapp",
		Separator: "whatsapp",
		RawSegments: []BatchSegment{
			{Body: "Project idea", Sequence: 0, ParseConfidence: "high"},
			{Body: "Task idea", Sequence: 1, ParseConfidence: "high"},
		},
	}
	result := e.Enrich(context.Background(), input)
	if result.Status != EnrichmentFailed {
		t.Fatalf("Status = %q, want failed", result.Status)
	}
	if len(result.Segments) != 2 {
		t.Fatalf("fallback should preserve 2 segments, got %d", len(result.Segments))
	}
}

func TestEnricherReassignsBadOrMissingSourceSequences(t *testing.T) {
	llm := &batchTestLLM{content: `{
		"context_title": "ignored",
		"segments": [
			{"body": "Good", "source_sequences": [0]},
			{"body": "Bad ref", "source_sequences": [99]},
			{"body": "Missing refs"}
		]
	}`}
	e := NewBatchEnricher(llm)
	input := BatchEnrichmentInput{
		Source:    "text",
		Separator: "lines",
		RawSegments: []BatchSegment{
			{Body: "first", Sequence: 0, ParseConfidence: "medium"},
			{Body: "second", Sequence: 1, ParseConfidence: "medium"},
		},
	}
	result := e.Enrich(context.Background(), input)
	if result.Status != EnrichmentApplied {
		t.Fatalf("Status = %q, want applied", result.Status)
	}
	// Neither the bad-ref nor the no-ref candidate is dropped: content must
	// never be discarded just because the LLM's source_sequences were wrong
	// or missing.
	if len(result.Segments) != 3 {
		t.Fatalf("Segments = %d, want 3", len(result.Segments))
	}
	if result.Segments[0].Body != "Good" {
		t.Fatalf("Body = %q", result.Segments[0].Body)
	}
	// Reassigned candidates sort after the real-ref one, in their original order.
	if result.Segments[1].Body != "Bad ref" {
		t.Fatalf("Body = %q, want %q", result.Segments[1].Body, "Bad ref")
	}
	if result.Segments[2].Body != "Missing refs" {
		t.Fatalf("Body = %q, want %q", result.Segments[2].Body, "Missing refs")
	}
	// Fabricated refs must not borrow sender/timestamp from an unrelated raw segment.
	if result.Segments[1].Sender != "" || result.Segments[1].Timestamp != "" {
		t.Fatalf("reassigned segment should not carry attribution from an unrelated raw segment")
	}
}

func TestEnricherSemanticSplit(t *testing.T) {
	llm := &batchTestLLM{content: `{
		"segments": [
			{"body": "First idea"},
			{"body": "Second idea"}
		]
	}`}
	e := NewBatchEnricher(llm)
	input := BatchEnrichmentInput{
		Source:      "text",
		Separator:   "semantic",
		ContextHint: "notes",
		RawSegments: []BatchSegment{
			{Body: "First idea. Second idea.", Sequence: 0, ParseConfidence: "low"},
		},
	}
	result := e.EnrichSemantic(context.Background(), input)
	if result.Status != EnrichmentApplied {
		t.Fatalf("Status = %q, want applied", result.Status)
	}
	if len(result.Segments) != 2 {
		t.Fatalf("Segments = %d, want 2", len(result.Segments))
	}
	if result.Segments[0].Body != "First idea" {
		t.Fatalf("first body = %q", result.Segments[0].Body)
	}
}

func TestEnricherSemanticSplitInvalidFallsBack(t *testing.T) {
	llm := &batchTestLLM{content: "not json"}
	e := NewBatchEnricher(llm)
	input := BatchEnrichmentInput{
		Source:      "text",
		Separator:   "semantic",
		ContextHint: "notes",
		RawSegments: []BatchSegment{{Body: "Dense paragraph.", Sequence: 0, ParseConfidence: "low"}},
	}
	result := e.EnrichSemantic(context.Background(), input)
	if result.Status != EnrichmentFailed {
		t.Fatalf("Status = %q, want failed", result.Status)
	}
	if len(result.Segments) != 1 {
		t.Fatalf("fallback should preserve 1 segment, got %d", len(result.Segments))
	}
}

// A merged capture inherits the first source message's timestamp. Without it
// the inbox has no time to show and falls back to "#N", which tells you nothing
// about when you said it.
func TestMergedSegmentInheritsFirstSourceTimestamp(t *testing.T) {
	llm := &batchTestLLM{content: `{
		"context_title": "morning notes",
		"segments": [
			{"body": "The whole thought, merged", "source_sequences": [1, 0], "confidence": "high"}
		]
	}`}
	e := NewBatchEnricher(llm)
	result := e.Enrich(context.Background(), BatchEnrichmentInput{
		Source:    "whatsapp",
		Separator: "whatsapp",
		RawSegments: []BatchSegment{
			{Body: "First half", Sender: "Me", Timestamp: "4/1/26 08:37", Sequence: 0, ParseConfidence: "high"},
			{Body: "Second half", Sender: "Me", Timestamp: "4/1/26 10:18", Sequence: 1, ParseConfidence: "high"},
		},
	})
	if len(result.Segments) != 1 {
		t.Fatalf("Segments = %d, want 1 merged segment", len(result.Segments))
	}
	seg := result.Segments[0]
	if seg.Timestamp != "4/1/26 08:37" {
		t.Errorf("merged Timestamp = %q, want the first source message's 08:37", seg.Timestamp)
	}
	if seg.Sender != "Me" {
		t.Errorf("merged Sender = %q, want Me", seg.Sender)
	}
}

// A candidate the model invented (no valid source_sequences) is kept, but must
// not borrow a timestamp from an unrelated message.
func TestFabricatedSegmentBorrowsNoTimestamp(t *testing.T) {
	llm := &batchTestLLM{content: `{
		"segments": [
			{"body": "Something the model made up", "source_sequences": [42], "confidence": "low"}
		]
	}`}
	e := NewBatchEnricher(llm)
	result := e.Enrich(context.Background(), BatchEnrichmentInput{
		Source:    "whatsapp",
		Separator: "whatsapp",
		RawSegments: []BatchSegment{
			{Body: "Real message", Sender: "Me", Timestamp: "4/1/26 08:37", Sequence: 0, ParseConfidence: "high"},
		},
	})
	if len(result.Segments) != 1 {
		t.Fatalf("Segments = %d, want the candidate kept", len(result.Segments))
	}
	if result.Segments[0].Timestamp != "" {
		t.Errorf("fabricated segment Timestamp = %q, want empty", result.Segments[0].Timestamp)
	}
}
