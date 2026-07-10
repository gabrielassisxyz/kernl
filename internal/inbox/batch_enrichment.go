package inbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/chat"
)

// EnrichmentStatus reports how LLM enrichment concluded for a batch analysis.
type EnrichmentStatus string

const (
	EnrichmentNone        EnrichmentStatus = "none"
	EnrichmentApplied     EnrichmentStatus = "applied"
	EnrichmentUnavailable EnrichmentStatus = "unavailable"
	EnrichmentFailed      EnrichmentStatus = "failed"
)

// BatchEnrichmentInput is the deterministic parse output submitted to the LLM.
type BatchEnrichmentInput struct {
	Source      string         `json:"source"`
	Separator   string         `json:"separator"`
	ContextHint string         `json:"contextHint"`
	RawSegments []BatchSegment `json:"rawSegments"`
}

// FinalBatchSegment is one capture candidate produced by enrichment.
type FinalBatchSegment struct {
	Body            string `json:"body"`
	Sender          string `json:"sender,omitempty"`
	Timestamp       string `json:"timestamp,omitempty"`
	Sequence        int    `json:"sequence"`
	SourceSequences []int  `json:"sourceSequences"`
	KindHint        string `json:"kindHint,omitempty"`
	Confidence      string `json:"confidence,omitempty"`
}

// BatchEnrichmentResult is the LLM output normalized to final candidates.
type BatchEnrichmentResult struct {
	ContextTitle string              `json:"contextTitle"`
	Segments     []FinalBatchSegment `json:"segments"`
	Status       EnrichmentStatus    `json:"status"`
	Err          error               `json:"-"`
}

// BatchEnricher performs optional LLM-based enrichment on top of a
// deterministic batch parse. It never mutates the raw segments.
type BatchEnricher struct {
	llm chat.LLMClient
}

// NewBatchEnricher creates an enricher. A nil llm means enrichment is
// unavailable; calls degrade to the deterministic input unchanged.
func NewBatchEnricher(llm chat.LLMClient) *BatchEnricher {
	return &BatchEnricher{llm: llm}
}

// Enrich asks the LLM to group related raw segments and suggest a context
// title. When the LLM is nil, missing, or returns invalid output, the result
// mirrors the deterministic input with a clear status.
func (e *BatchEnricher) Enrich(ctx context.Context, input BatchEnrichmentInput) *BatchEnrichmentResult {
	if e.llm == nil {
		return fallbackEnrichment(input, EnrichmentUnavailable)
	}
	if len(input.RawSegments) == 0 {
		return fallbackEnrichment(input, EnrichmentNone)
	}

	prompt := buildBatchEnrichmentPrompt(input)
	resp, err := e.llm.Chat(ctx, []chat.Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		slog.Warn("batch enrichment llm call failed", "error", err)
		return fallbackEnrichment(input, EnrichmentFailed)
	}

	result, ok := parseBatchEnrichmentResponse(resp.Content, input)
	if !ok {
		return fallbackEnrichment(input, EnrichmentFailed)
	}
	result.Status = EnrichmentApplied
	return result
}

// EnrichSemantic asks the LLM to split a dense text into raw-like segments
// and then group/title them. This is the LLM-backed path for the
// "semantic" separator; without an LLM it returns a single low-confidence
// segment.
func (e *BatchEnricher) EnrichSemantic(ctx context.Context, input BatchEnrichmentInput) *BatchEnrichmentResult {
	if e.llm == nil {
		return fallbackEnrichment(input, EnrichmentUnavailable)
	}

	prompt := buildSemanticSplitPrompt(input)
	resp, err := e.llm.Chat(ctx, []chat.Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		slog.Warn("semantic split llm call failed", "error", err)
		return fallbackEnrichment(input, EnrichmentFailed)
	}

	segments, ok := parseSemanticSegments(resp.Content)
	if !ok || len(segments) == 0 {
		return fallbackEnrichment(input, EnrichmentFailed)
	}
	for i := range segments {
		segments[i].Sequence = i
	}
	return e.Enrich(ctx, BatchEnrichmentInput{
		Source:      input.Source,
		Separator:   input.Separator,
		ContextHint: input.ContextHint,
		RawSegments: toBatchSegmentsForEnrichment(segments),
	})
}

func toBatchSegmentsForEnrichment(segments []BatchSegment) []BatchSegment {
	out := make([]BatchSegment, len(segments))
	copy(out, segments)
	return out
}

func buildBatchEnrichmentPrompt(input BatchEnrichmentInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are helping organize a pasted batch of notes/messages into clean Inbox captures.\n\n")
	fmt.Fprintf(&b, "Source: %s\n", input.Source)
	fmt.Fprintf(&b, "Separator: %s\n", input.Separator)
	if input.ContextHint != "" {
		fmt.Fprintf(&b, "User context title hint: %s\n", input.ContextHint)
	}
	fmt.Fprintf(&b, "\nRaw segments (one per line, but adjacent or related items may belong together):\n")
	for _, seg := range input.RawSegments {
		fmt.Fprintf(&b, "[%d]", seg.Sequence)
		if seg.Sender != "" {
			fmt.Fprintf(&b, " %s:", seg.Sender)
		}
		fmt.Fprintf(&b, " %s\n", seg.Body)
	}
	fmt.Fprintf(&b, "\nInstructions:\n")
	fmt.Fprintf(&b, "- Group related raw segments into one final capture candidate when they describe the same project/idea/task set.\n")
	fmt.Fprintf(&b, "- Preserve unrelated raw segments as separate candidates.\n")
	fmt.Fprintf(&b, "- Do not delete valuable content. Support-only messages may be summarized inside a grouped candidate or listed as their own candidate.\n")
	fmt.Fprintf(&b, "- Suggest one concise context title for the whole paste.\n")
	fmt.Fprintf(&b, "- Respond with ONLY a JSON object, no prose.\n")
	fmt.Fprintf(&b, "{\n")
	fmt.Fprintf(&b, "  \"context_title\": \"string\",\n")
	fmt.Fprintf(&b, "  \"segments\": [\n")
	fmt.Fprintf(&b, "    {\n")
	fmt.Fprintf(&b, "      \"body\": \"string\",\n")
	fmt.Fprintf(&b, "      \"source_sequences\": [0],\n")
	fmt.Fprintf(&b, "      \"kind_hint\": \"project|task|note|bookmark|update|discard\",\n")
	fmt.Fprintf(&b, "      \"confidence\": \"high|medium|low\"\n")
	fmt.Fprintf(&b, "    }\n")
	fmt.Fprintf(&b, "  ]\n")
	fmt.Fprintf(&b, "}\n")
	return b.String()
}

func buildSemanticSplitPrompt(input BatchEnrichmentInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Split the following pasted text into distinct capture candidates.\n\n")
	fmt.Fprintf(&b, "Source: %s\n", input.Source)
	if input.ContextHint != "" {
		fmt.Fprintf(&b, "User context title hint: %s\n", input.ContextHint)
	}
	fmt.Fprintf(&b, "Text:\n%s\n\n", input.RawSegments[0].Body)
	fmt.Fprintf(&b, "Instructions:\n")
	fmt.Fprintf(&b, "- Divide the text into logical segments (one idea, task, or note per segment).\n")
	fmt.Fprintf(&b, "- Preserve ordering.\n")
	fmt.Fprintf(&b, "- Respond with ONLY a JSON object: {\"segments\":[{\"body\":\"...\"}]}\n")
	return b.String()
}

func parseBatchEnrichmentResponse(raw string, input BatchEnrichmentInput) (*BatchEnrichmentResult, bool) {
	obj := extractJSONObject(raw)
	if obj == "" {
		return nil, false
	}
	var parsed struct {
		ContextTitle string `json:"context_title"`
		Segments     []struct {
			Body            string `json:"body"`
			SourceSequences []int  `json:"source_sequences"`
			KindHint        string `json:"kind_hint"`
			Confidence      string `json:"confidence"`
		} `json:"segments"`
	}
	if err := json.Unmarshal([]byte(obj), &parsed); err != nil {
		return nil, false
	}

	rawBySeq := map[int]BatchSegment{}
	maxRawSeq := -1
	for _, seg := range input.RawSegments {
		rawBySeq[seg.Sequence] = seg
		if seg.Sequence > maxRawSeq {
			maxRawSeq = seg.Sequence
		}
	}

	out := make([]FinalBatchSegment, 0, len(parsed.Segments))
	fallbackIdx := 0
	for _, item := range parsed.Segments {
		body := strings.TrimSpace(item.Body)
		if body == "" {
			continue
		}
		refs := make([]int, 0, len(item.SourceSequences))
		for _, seq := range item.SourceSequences {
			if _, ok := rawBySeq[seq]; !ok {
				continue
			}
			refs = append(refs, seq)
		}
		if len(refs) == 0 && len(input.RawSegments) > 0 {
			// LLM omitted source sequences, or gave only invalid ones; keep
			// the candidate (never drop valuable content) but key it above
			// maxRawSeq so it sorts after every candidate that maps to a
			// real raw sequence, and so the sender/timestamp copy below
			// cannot mistake it for a real rawBySeq entry.
			refs = []int{maxRawSeq + 1 + fallbackIdx}
			fallbackIdx++
		}
		if len(refs) == 0 {
			continue
		}

		seg := FinalBatchSegment{
			Body:            body,
			SourceSequences: refs,
			KindHint:        normalizeKindHint(item.KindHint),
			Confidence:      normalizeConfidence(item.Confidence),
		}
		// Carry sender/timestamp from the referenced raw segment only when
		// it maps to exactly one *real* raw sequence; a fabricated fallback
		// ref must not borrow attribution from an unrelated raw segment.
		if len(refs) == 1 {
			if raw, ok := rawBySeq[refs[0]]; ok {
				seg.Sender = raw.Sender
				seg.Timestamp = raw.Timestamp
			}
		}
		out = append(out, seg)
	}

	if len(out) == 0 {
		return nil, false
	}

	// Preserve deterministic ordering by first referenced raw sequence.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].SourceSequences[0] < out[j].SourceSequences[0]
	})
	// Renumber final sequences after sorting.
	for i := range out {
		out[i].Sequence = i
	}

	contextTitle := strings.TrimSpace(parsed.ContextTitle)
	if contextTitle == "" {
		contextTitle = input.ContextHint
	}

	return &BatchEnrichmentResult{
		ContextTitle: contextTitle,
		Segments:     out,
	}, true
}

func parseSemanticSegments(raw string) ([]BatchSegment, bool) {
	obj := extractJSONObject(raw)
	if obj == "" {
		return nil, false
	}
	var parsed struct {
		Segments []struct {
			Body string `json:"body"`
		} `json:"segments"`
	}
	if err := json.Unmarshal([]byte(obj), &parsed); err != nil {
		return nil, false
	}
	out := make([]BatchSegment, 0, len(parsed.Segments))
	for i, item := range parsed.Segments {
		body := strings.TrimSpace(item.Body)
		if body == "" {
			continue
		}
		out = append(out, BatchSegment{
			Body:            body,
			Sequence:        i,
			ParseConfidence: "medium",
		})
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func fallbackEnrichment(input BatchEnrichmentInput, status EnrichmentStatus) *BatchEnrichmentResult {
	segments := make([]FinalBatchSegment, 0, len(input.RawSegments))
	for _, seg := range input.RawSegments {
		segments = append(segments, FinalBatchSegment{
			Body:            seg.Body,
			Sender:          seg.Sender,
			Timestamp:       seg.Timestamp,
			Sequence:        seg.Sequence,
			SourceSequences: []int{seg.Sequence},
			KindHint:        "",
			Confidence:      seg.ParseConfidence,
		})
	}
	return &BatchEnrichmentResult{
		ContextTitle: input.ContextHint,
		Segments:     segments,
		Status:       status,
	}
}

func normalizeKindHint(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "project", "task", "note", "bookmark", "update", "discard":
		return s
	}
	return ""
}

func normalizeConfidence(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "high", "medium", "low":
		return s
	}
	return "medium"
}
