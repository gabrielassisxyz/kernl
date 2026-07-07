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

func TestEnricherRejectsBadSourceSequences(t *testing.T) {
	llm := &batchTestLLM{content: `{
		"context_title": "ignored",
		"segments": [
			{"body": "Good", "source_sequences": [0]},
			{"body": "Bad ref", "source_sequences": [99]},
			{"body": "Empty"}
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
	if len(result.Segments) != 2 {
		t.Fatalf("Segments = %d, want 2", len(result.Segments))
	}
	if result.Segments[0].Body != "Good" {
		t.Fatalf("Body = %q", result.Segments[0].Body)
	}
	// Bad reference gets reassigned; empty body is dropped.
	if result.Segments[1].Body == "" {
		t.Fatalf("second segment should carry reassigned content")
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
