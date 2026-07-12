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

func twoRawSegments() BatchEnrichmentInput {
	return BatchEnrichmentInput{
		Source:      "whatsapp",
		Separator:   "whatsapp",
		ContextHint: "planning",
		RawSegments: []BatchSegment{
			{Body: "Project idea", Sequence: 0, ParseConfidence: "high"},
			{Body: "Task idea", Sequence: 1, ParseConfidence: "high"},
		},
	}
}

func TestEnricherUnavailableWithoutLLM(t *testing.T) {
	result := NewBatchEnricher(nil).Enrich(context.Background(), twoRawSegments())
	if result.Status != EnrichmentUnavailable {
		t.Fatalf("Status = %q, want unavailable", result.Status)
	}
	if len(result.MergeProposals) != 0 {
		t.Fatalf("MergeProposals = %v, want none without an LLM", result.MergeProposals)
	}
	if result.ContextTitle != "planning" {
		t.Fatalf("ContextTitle = %q", result.ContextTitle)
	}
}

func TestEnricherProposesMerges(t *testing.T) {
	llm := &batchTestLLM{content: `{
		"context_title": "ai-memory explainer",
		"merges": [
			{"source_sequences": [1, 0], "reason": "same request, restated"}
		]
	}`}
	result := NewBatchEnricher(llm).Enrich(context.Background(), twoRawSegments())
	if result.Status != EnrichmentApplied {
		t.Fatalf("Status = %q, want applied", result.Status)
	}
	if result.ContextTitle != "ai-memory explainer" {
		t.Fatalf("ContextTitle = %q", result.ContextTitle)
	}
	if len(result.MergeProposals) != 1 {
		t.Fatalf("MergeProposals = %d, want 1", len(result.MergeProposals))
	}
	got := result.MergeProposals[0]
	if len(got.SourceSequences) != 2 || got.SourceSequences[0] != 0 || got.SourceSequences[1] != 1 {
		t.Fatalf("SourceSequences = %v, want [0 1] in source order", got.SourceSequences)
	}
	if got.Reason == "" {
		t.Fatal("a merge proposal must say why, so it can be judged")
	}
}

// The enrichment prompt has no field for message text, but a model can always
// answer with whatever it likes. Whatever it invents, it cannot reach a body.
func TestEnricherIgnoresBodiesFromTheModel(t *testing.T) {
	llm := &batchTestLLM{content: `{
		"context_title": "tidied up",
		"segments": [{"body": "A tidy paraphrase of everything", "source_sequences": [0, 1]}],
		"merges": []
	}`}
	result := NewBatchEnricher(llm).Enrich(context.Background(), twoRawSegments())
	if len(result.MergeProposals) != 0 {
		t.Fatalf("MergeProposals = %v, want none", result.MergeProposals)
	}
}

func TestEnricherFailsOnInvalidJSON(t *testing.T) {
	llm := &batchTestLLM{content: "not json"}
	result := NewBatchEnricher(llm).Enrich(context.Background(), twoRawSegments())
	if result.Status != EnrichmentFailed {
		t.Fatalf("Status = %q, want failed", result.Status)
	}
	if len(result.MergeProposals) != 0 {
		t.Fatalf("a failed enrichment must propose nothing, got %v", result.MergeProposals)
	}
}

func TestEnricherDropsUnusableMergeProposals(t *testing.T) {
	llm := &batchTestLLM{content: `{
		"merges": [
			{"source_sequences": [0], "reason": "a merge of one is not a merge"},
			{"source_sequences": [0, 99], "reason": "99 does not exist, leaving one member"},
			{"source_sequences": [3, 3], "reason": "the same message twice"},
			{"source_sequences": [0, 1], "reason": "this one is real"}
		]
	}`}
	input := BatchEnrichmentInput{
		Source:    "text",
		Separator: "lines",
		RawSegments: []BatchSegment{
			{Body: "first", Sequence: 0, ParseConfidence: "medium"},
			{Body: "second", Sequence: 1, ParseConfidence: "medium"},
		},
	}
	result := NewBatchEnricher(llm).Enrich(context.Background(), input)
	if len(result.MergeProposals) != 1 {
		t.Fatalf("MergeProposals = %#v, want only the usable one", result.MergeProposals)
	}
	if got := result.MergeProposals[0].SourceSequences; len(got) != 2 {
		t.Fatalf("SourceSequences = %v, want [0 1]", got)
	}
}

// Overlapping proposals cannot be accepted or rejected independently — the
// message in both would be duplicated or dropped depending on click order — so
// the first claim on a message wins and the rest of that proposal is discarded.
func TestEnricherDropsOverlappingMergeProposals(t *testing.T) {
	llm := &batchTestLLM{content: `{
		"merges": [
			{"source_sequences": [0, 1], "reason": "first claim"},
			{"source_sequences": [1, 2], "reason": "overlaps on 1"}
		]
	}`}
	input := BatchEnrichmentInput{
		Source:    "text",
		Separator: "lines",
		RawSegments: []BatchSegment{
			{Body: "first", Sequence: 0, ParseConfidence: "medium"},
			{Body: "second", Sequence: 1, ParseConfidence: "medium"},
			{Body: "third", Sequence: 2, ParseConfidence: "medium"},
		},
	}
	result := NewBatchEnricher(llm).Enrich(context.Background(), input)
	if len(result.MergeProposals) != 1 {
		t.Fatalf("MergeProposals = %#v, want the overlapping one dropped", result.MergeProposals)
	}
}

func TestSemanticSplitAcceptsVerbatimSegments(t *testing.T) {
	llm := &batchTestLLM{content: `{
		"segments": [
			{"body": "First idea."},
			{"body": "Second idea."}
		]
	}`}
	segments, status := NewBatchEnricher(llm).SplitSemantic(context.Background(), BatchEnrichmentInput{
		Source:      "text",
		Separator:   "semantic",
		RawSegments: []BatchSegment{{Body: "First idea. Second idea.", Sequence: 0, ParseConfidence: "low"}},
	})
	if status != EnrichmentApplied {
		t.Fatalf("Status = %q, want applied", status)
	}
	if len(segments) != 2 || segments[0].Body != "First idea." {
		t.Fatalf("segments = %#v", segments)
	}
}

// The semantic split is the one place a model chooses where the text is cut, so
// it is the one place it could smuggle in a rewrite. A body that is not literally
// in the source discredits the whole split: better one honest capture than two
// tidied-up ones.
func TestSemanticSplitRejectsRewrittenSegments(t *testing.T) {
	llm := &batchTestLLM{content: `{
		"segments": [
			{"body": "First idea."},
			{"body": "Second idea, but polished by the model."}
		]
	}`}
	segments, status := NewBatchEnricher(llm).SplitSemantic(context.Background(), BatchEnrichmentInput{
		Source:      "text",
		Separator:   "semantic",
		RawSegments: []BatchSegment{{Body: "First idea. Second idea.", Sequence: 0, ParseConfidence: "low"}},
	})
	if status != EnrichmentFailed {
		t.Fatalf("Status = %q, want failed", status)
	}
	if segments != nil {
		t.Fatalf("segments = %#v, want none", segments)
	}
}

func TestSemanticSplitFailsOnInvalidJSON(t *testing.T) {
	llm := &batchTestLLM{content: "not json"}
	segments, status := NewBatchEnricher(llm).SplitSemantic(context.Background(), BatchEnrichmentInput{
		Source:      "text",
		Separator:   "semantic",
		RawSegments: []BatchSegment{{Body: "Dense paragraph.", Sequence: 0, ParseConfidence: "low"}},
	})
	if status != EnrichmentFailed {
		t.Fatalf("Status = %q, want failed", status)
	}
	if segments != nil {
		t.Fatalf("segments = %#v, want none", segments)
	}
}
