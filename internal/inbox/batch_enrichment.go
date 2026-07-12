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

// FinalBatchSegment is one capture candidate: the text of one raw message, or
// of several raw messages the user chose to merge.
//
// Body is never LLM output. It is built from the raw segments named by
// SourceSequences (see buildFinalSegments) — the model may propose how the
// messages group, never what they say.
type FinalBatchSegment struct {
	Body            string `json:"body"`
	Sender          string `json:"sender,omitempty"`
	Timestamp       string `json:"timestamp,omitempty"`
	Sequence        int    `json:"sequence"`
	SourceSequences []int  `json:"sourceSequences"`
	Confidence      string `json:"confidence,omitempty"`
}

// MergeProposal is the LLM saying "these raw messages are one thought". It is
// an offer, not a decision: nothing merges until the human accepts it. An
// unreviewed merge that collapsed two messages on its own would delete one of
// them from the record.
type MergeProposal struct {
	SourceSequences []int  `json:"sourceSequences"`
	Reason          string `json:"reason,omitempty"`
}

// BatchEnrichmentResult is the LLM's layer on top of the deterministic parse:
// a title for the paste, and merges it suggests. It carries no bodies.
type BatchEnrichmentResult struct {
	ContextTitle   string           `json:"contextTitle"`
	MergeProposals []MergeProposal  `json:"mergeProposals"`
	Status         EnrichmentStatus `json:"status"`
	Err            error            `json:"-"`
}

// BatchEnricher performs optional LLM-based enrichment on top of a
// deterministic batch parse. It never mutates the raw segments.
type BatchEnricher struct {
	llm chat.LLMClient
}

// NewBatchEnricher creates an enricher. A nil llm means enrichment is
// unavailable; calls degrade to the deterministic parse unchanged.
func NewBatchEnricher(llm chat.LLMClient) *BatchEnricher {
	return &BatchEnricher{llm: llm}
}

// Enrich asks the LLM to suggest a context title and which raw segments belong
// together. When the LLM is nil, missing, or returns invalid output, the batch
// simply keeps its deterministic split and no merge is proposed.
func (e *BatchEnricher) Enrich(ctx context.Context, input BatchEnrichmentInput) *BatchEnrichmentResult {
	if e.llm == nil {
		return &BatchEnrichmentResult{ContextTitle: input.ContextHint, Status: EnrichmentUnavailable}
	}
	if len(input.RawSegments) == 0 {
		return &BatchEnrichmentResult{ContextTitle: input.ContextHint, Status: EnrichmentNone}
	}

	prompt := buildBatchEnrichmentPrompt(input)
	resp, err := e.llm.Chat(ctx, []chat.Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		slog.Warn("batch enrichment llm call failed", "error", err)
		return &BatchEnrichmentResult{ContextTitle: input.ContextHint, Status: EnrichmentFailed, Err: err}
	}

	result, ok := parseBatchEnrichmentResponse(resp.Content, input)
	if !ok {
		return &BatchEnrichmentResult{ContextTitle: input.ContextHint, Status: EnrichmentFailed}
	}
	result.Status = EnrichmentApplied
	return result
}

// SplitSemantic asks the LLM where to cut a dense, separator-less paste. It
// returns segments, not rewrites: every returned body must appear verbatim in
// the source text, or the whole split is rejected and the caller keeps the
// deterministic single segment. This is the only path where the model decides
// segment boundaries, so it is the only one that has to be policed this way.
func (e *BatchEnricher) SplitSemantic(ctx context.Context, input BatchEnrichmentInput) ([]BatchSegment, EnrichmentStatus) {
	if e.llm == nil {
		return nil, EnrichmentUnavailable
	}
	if len(input.RawSegments) == 0 {
		return nil, EnrichmentNone
	}

	source := input.RawSegments[0].Body
	prompt := buildSemanticSplitPrompt(input)
	resp, err := e.llm.Chat(ctx, []chat.Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		slog.Warn("semantic split llm call failed", "error", err)
		return nil, EnrichmentFailed
	}

	segments, ok := parseSemanticSegments(resp.Content, source)
	if !ok {
		return nil, EnrichmentFailed
	}
	return segments, EnrichmentApplied
}

func buildBatchEnrichmentPrompt(input BatchEnrichmentInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are helping organize a pasted batch of notes/messages into Inbox captures.\n\n")
	fmt.Fprintf(&b, "Source: %s\n", input.Source)
	fmt.Fprintf(&b, "Separator: %s\n", input.Separator)
	if input.ContextHint != "" {
		fmt.Fprintf(&b, "User context title hint: %s\n", input.ContextHint)
	}
	fmt.Fprintf(&b, "\nMessages, one per numbered block:\n")
	for _, seg := range input.RawSegments {
		fmt.Fprintf(&b, "[%d]", seg.Sequence)
		if seg.Sender != "" {
			fmt.Fprintf(&b, " %s:", seg.Sender)
		}
		fmt.Fprintf(&b, " %s\n", seg.Body)
	}
	fmt.Fprintf(&b, "\nYour job:\n")
	fmt.Fprintf(&b, "- Each message is already its own capture. Leave it that way unless two or more messages are literally the SAME thought, restated or continued (the user typed it twice, or finished a sentence in the next message).\n")
	fmt.Fprintf(&b, "- Merging is destructive and the default is NOT to merge. Same topic is NOT enough. Related is NOT enough. Two ideas about one project stay two captures.\n")
	fmt.Fprintf(&b, "- A merge you propose is only a suggestion; a human accepts or rejects it. Say why in one short clause.\n")
	fmt.Fprintf(&b, "- Never rewrite, summarize, translate or retitle a message. You do not output message text at all.\n")
	fmt.Fprintf(&b, "- Suggest one concise context title for the whole paste.\n")
	fmt.Fprintf(&b, "- Respond with ONLY a JSON object, no prose.\n")
	fmt.Fprintf(&b, "{\n")
	fmt.Fprintf(&b, "  \"context_title\": \"string\",\n")
	fmt.Fprintf(&b, "  \"merges\": [\n")
	fmt.Fprintf(&b, "    { \"source_sequences\": [0, 4], \"reason\": \"same request, restated\" }\n")
	fmt.Fprintf(&b, "  ]\n")
	fmt.Fprintf(&b, "}\n")
	fmt.Fprintf(&b, "An empty \"merges\" list is a perfectly good answer.\n")
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
	fmt.Fprintf(&b, "- COPY each segment out of the text character for character. Do not rewrite, summarize, fix typos, or translate — an edited segment is rejected and the split is thrown away.\n")
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
		Merges       []struct {
			SourceSequences []int  `json:"source_sequences"`
			Reason          string `json:"reason"`
		} `json:"merges"`
	}
	if err := json.Unmarshal([]byte(obj), &parsed); err != nil {
		return nil, false
	}

	known := map[int]bool{}
	for _, seg := range input.RawSegments {
		known[seg.Sequence] = true
	}

	// A raw message belongs to at most one proposal: overlapping merges cannot
	// be accepted independently, and a message in two groups would be duplicated
	// or lost depending on the order the user clicked.
	claimed := map[int]bool{}
	proposals := make([]MergeProposal, 0, len(parsed.Merges))
	for _, m := range parsed.Merges {
		refs := make([]int, 0, len(m.SourceSequences))
		seen := map[int]bool{}
		for _, seq := range m.SourceSequences {
			if !known[seq] || seen[seq] || claimed[seq] {
				continue
			}
			seen[seq] = true
			refs = append(refs, seq)
		}
		if len(refs) < 2 {
			continue
		}
		sort.Ints(refs)
		for _, seq := range refs {
			claimed[seq] = true
		}
		proposals = append(proposals, MergeProposal{
			SourceSequences: refs,
			Reason:          strings.TrimSpace(m.Reason),
		})
	}
	sort.SliceStable(proposals, func(i, j int) bool {
		return proposals[i].SourceSequences[0] < proposals[j].SourceSequences[0]
	})

	contextTitle := strings.TrimSpace(parsed.ContextTitle)
	if contextTitle == "" {
		contextTitle = input.ContextHint
	}

	return &BatchEnrichmentResult{
		ContextTitle:   contextTitle,
		MergeProposals: proposals,
	}, true
}

// parseSemanticSegments accepts only segments the model copied out of source.
// A body that is not a substring of the source is a rewrite, and one rewrite is
// enough to distrust the whole response.
func parseSemanticSegments(raw string, source string) ([]BatchSegment, bool) {
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
	for _, item := range parsed.Segments {
		body := strings.TrimSpace(item.Body)
		if body == "" {
			continue
		}
		if !strings.Contains(source, body) {
			slog.Warn("semantic split rejected: segment is not verbatim source text")
			return nil, false
		}
		out = append(out, BatchSegment{
			Body:            body,
			Sequence:        len(out),
			ParseConfidence: "medium",
		})
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
