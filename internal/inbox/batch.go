package inbox

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

const (
	BatchSplitAuto      = "auto"
	BatchSplitWhatsApp  = "whatsapp"
	BatchSplitLines     = "lines"
	BatchSplitBlocks    = "blocks"
	BatchSplitDivider   = "divider"
	BatchSplitMarkdown  = "markdown"
	BatchSplitSemantic  = "semantic"
	BatchSourceWhatsApp = "whatsapp"
	BatchSourceText     = "text"
)

type BatchInput struct {
	RawText      string
	Source       string
	SplitMode    string
	ContextTitle string
	// RawSegments is the split the client reviewed. It matters only for the
	// semantic separator, whose cut points come from the LLM and so cannot be
	// re-derived on create; every other separator is a pure function of RawText
	// and is re-parsed server-side. Whatever arrives here is still checked
	// against RawText: a body that is not verbatim source text is rejected.
	RawSegments []BatchSegment
	// FinalSegments carries the capture candidates the client approved. Only
	// their SourceSequences are honored — which raw messages the user chose to
	// merge. The bodies are rebuilt from RawSegments, never taken from the
	// request, so no caller (and no model behind one) can rewrite the source.
	FinalSegments []FinalBatchSegment
}

type BatchSegment struct {
	Body            string `json:"body"`
	Sender          string `json:"sender,omitempty"`
	Timestamp       string `json:"timestamp,omitempty"`
	Sequence        int    `json:"sequence"`
	ParseConfidence string `json:"parseConfidence"`
}

type BatchAnalysis struct {
	Source                string              `json:"source"`
	Separator             string              `json:"separator"`
	SuggestedContextTitle string              `json:"suggestedContextTitle"`
	Segments              []BatchSegment      `json:"segments"`
	RawSegments           []BatchSegment      `json:"rawSegments,omitempty"`
	EnrichmentStatus      EnrichmentStatus    `json:"enrichmentStatus"`
	EnrichmentError       string              `json:"enrichmentError,omitempty"`
	FinalSegments         []FinalBatchSegment `json:"finalSegments,omitempty"`
	// MergeProposals are the merges the LLM suggests. They are offers the human
	// accepts or rejects; an analysis never returns them already applied, so the
	// capture count of a paste does not depend on what the model decided.
	MergeProposals []MergeProposal `json:"mergeProposals,omitempty"`
}

type BatchCreateResult struct {
	BatchID          string              `json:"batchId"`
	Segments         []BatchSegment      `json:"segments"`
	FinalSegments    []FinalBatchSegment `json:"finalSegments,omitempty"`
	IDs              []string            `json:"ids"`
	RawSegmentCount  int                 `json:"rawSegmentCount,omitempty"`
	EnrichmentStatus EnrichmentStatus    `json:"enrichmentStatus,omitempty"`
}

var (
	whatsAppBracketDateFirstHeader = regexp.MustCompile(`^\[(\d{1,2}/\d{1,2}/\d{2,4}),?\s+(\d{1,2}:\d{2}(?::\d{2})?)\]\s*([^:]+):\s?(.*)$`)
	whatsAppBracketTimeFirstHeader = regexp.MustCompile(`^\[(\d{1,2}:\d{2}(?::\d{2})?),?\s+(\d{1,2}/\d{1,2}/\d{2,4})\]\s*([^:]+):\s?(.*)$`)
	whatsAppDashDateFirstHeader    = regexp.MustCompile(`^(\d{1,2}/\d{1,2}/\d{2,4}),?\s+(\d{1,2}:\d{2}(?::\d{2})?)\s+-\s+([^:]+):\s?(.*)$`)
	whatsAppDashTimeFirstHeader    = regexp.MustCompile(`^(\d{1,2}:\d{2}(?::\d{2})?),?\s+(\d{1,2}/\d{1,2}/\d{2,4})\s+-\s+([^:]+):\s?(.*)$`)
	markdownHeading                = regexp.MustCompile(`(?m)^#{1,6}\s+\S`)
	dividerLine                    = regexp.MustCompile(`(?m)^\s*(---+|\*\*\*+|___+)\s*$`)
)

func AnalyzeBatch(input BatchInput) (*BatchAnalysis, error) {
	return AnalyzeBatchWithLLM(context.Background(), input, nil)
}

// AnalyzeBatchWithLLM returns the split the user is about to review. The
// captures it proposes are exactly the messages the deterministic parser found:
// the LLM contributes a context title and a list of merges it would suggest,
// and nothing else. Two runs of the same paste therefore always propose the
// same number of captures, whatever the model answered — the whole point, after
// one run of the same fixture produced 25 captures and the next produced 20.
func AnalyzeBatchWithLLM(ctx context.Context, input BatchInput, llm chat.LLMClient) (*BatchAnalysis, error) {
	rawSegments, err := previewBatchInternal(input)
	if err != nil {
		return nil, err
	}
	if len(rawSegments) == 0 {
		return nil, fmt.Errorf("no segments produced")
	}

	source, separator := resolveBatchSourceAndSeparator(input)
	contextTitle := strings.TrimSpace(input.ContextTitle)
	enricher := NewBatchEnricher(llm)

	// The semantic separator is the one case where the model decides the cut
	// points — there is nothing else to split on. It hands back slices of the
	// source, which SplitSemantic verifies are verbatim before we adopt them.
	status := EnrichmentNone
	if separator == BatchSplitSemantic {
		split, splitStatus := enricher.SplitSemantic(ctx, BatchEnrichmentInput{
			Source:      source,
			Separator:   separator,
			ContextHint: contextTitle,
			RawSegments: rawSegments,
		})
		status = splitStatus
		if len(split) > 0 {
			rawSegments = split
		}
	}

	enrichment := enricher.Enrich(ctx, BatchEnrichmentInput{
		Source:      source,
		Separator:   separator,
		ContextHint: contextTitle,
		RawSegments: rawSegments,
	})
	if separator != BatchSplitSemantic {
		status = enrichment.Status
	}

	finalSegments := buildFinalSegments(rawSegments, nil)
	viewSegments := viewSegmentsFromFinal(finalSegments)

	finalContextTitle := strings.TrimSpace(enrichment.ContextTitle)
	if finalContextTitle == "" {
		finalContextTitle = suggestedContextTitle(contextTitle, viewSegments)
	}

	return &BatchAnalysis{
		Source:                source,
		Separator:             separator,
		SuggestedContextTitle: finalContextTitle,
		Segments:              viewSegments,
		RawSegments:           rawSegments,
		FinalSegments:         finalSegments,
		MergeProposals:        enrichment.MergeProposals,
		EnrichmentStatus:      status,
	}, nil
}

// buildFinalSegments is the only constructor of capture candidates, and it can
// only assemble text that came out of the parser. mergeGroups are the merges the
// human accepted; every raw segment not named in one stays its own capture.
//
// A merged body is its source messages joined, in order, with a blank line
// between them — nothing is summarized away, and the merge stays legible as the
// several messages it was.
func buildFinalSegments(raw []BatchSegment, mergeGroups [][]int) []FinalBatchSegment {
	bySeq := make(map[int]BatchSegment, len(raw))
	for _, seg := range raw {
		bySeq[seg.Sequence] = seg
	}

	// Each raw message belongs to at most one group; a group is anchored at its
	// first member so the merged capture keeps that message's place in the paste.
	groupOf := map[int][]int{}
	for _, group := range mergeGroups {
		members := make([]int, 0, len(group))
		for _, seq := range group {
			if _, ok := bySeq[seq]; ok && groupOf[seq] == nil {
				members = append(members, seq)
			}
		}
		if len(members) < 2 {
			continue
		}
		sort.Ints(members)
		for _, seq := range members {
			groupOf[seq] = members
		}
	}

	out := make([]FinalBatchSegment, 0, len(raw))
	for _, seg := range raw {
		members := groupOf[seg.Sequence]
		if members == nil {
			members = []int{seg.Sequence}
		} else if members[0] != seg.Sequence {
			continue // already emitted at the group's anchor
		}

		bodies := make([]string, 0, len(members))
		confidence := ""
		for _, seq := range members {
			bodies = append(bodies, bySeq[seq].Body)
			if confidence == "" {
				confidence = bySeq[seq].ParseConfidence
			}
		}

		// Sender and timestamp come from the first message in the group: a merge
		// of 08:37 and 10:18 is still something you said at 08:37.
		anchor := bySeq[members[0]]
		out = append(out, FinalBatchSegment{
			Body:            strings.Join(bodies, "\n\n"),
			Sender:          anchor.Sender,
			Timestamp:       anchor.Timestamp,
			Sequence:        len(out),
			SourceSequences: members,
			Confidence:      confidence,
		})
	}
	return out
}

// mergeGroupsFrom reads the merge decisions out of the candidates the client
// approved. Only the grouping survives the trip; the bodies do not.
func mergeGroupsFrom(segments []FinalBatchSegment) [][]int {
	groups := make([][]int, 0, len(segments))
	for _, seg := range segments {
		if len(seg.SourceSequences) < 2 {
			continue
		}
		groups = append(groups, seg.SourceSequences)
	}
	return groups
}

// assertVerbatim fails the request when a segment body is not literally present
// in the pasted text. The parser cannot produce such a body; only a rewrite can,
// so this is the backstop that keeps a paraphrase from ever reaching the graph.
func assertVerbatim(rawText string, segments []BatchSegment) error {
	source := normalizeNewlines(rawText)
	for _, seg := range segments {
		if !strings.Contains(source, normalizeNewlines(seg.Body)) {
			return fmt.Errorf("segment %d is not verbatim source text: a capture body must be the message as it was written", seg.Sequence)
		}
	}
	return nil
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// resolveBatchSourceAndSeparator applies the auto-detection fallback shared by
// AnalyzeBatchWithLLM and by the client-approved-segments create path.
func resolveBatchSourceAndSeparator(input BatchInput) (source string, separator string) {
	source = strings.TrimSpace(input.Source)
	separator = normalizeBatchSplit(input.SplitMode)
	if separator == BatchSplitAuto {
		detectedSource, detectedSeparator := detectBatchShape(input.RawText)
		if source == "" {
			source = detectedSource
		}
		separator = detectedSeparator
	}
	if source == "" {
		source = BatchSourceText
	}
	return source, separator
}

// viewSegmentsFromFinal projects final capture candidates onto the
// UI-compatible BatchSegment shape used for previews and title suggestions.
func viewSegmentsFromFinal(finalSegments []FinalBatchSegment) []BatchSegment {
	out := make([]BatchSegment, 0, len(finalSegments))
	for _, fs := range finalSegments {
		out = append(out, BatchSegment{
			Body:            fs.Body,
			Sender:          fs.Sender,
			Timestamp:       fs.Timestamp,
			Sequence:        fs.Sequence,
			ParseConfidence: fs.Confidence,
		})
	}
	return out
}

// BatchPreview is the mechanical split — what the parser found, with no LLM in
// the path. The review modal opens on this, so the user is reading their own
// messages while enrichment is still thinking.
type BatchPreview struct {
	Source                string              `json:"source"`
	Separator             string              `json:"separator"`
	SuggestedContextTitle string              `json:"suggestedContextTitle"`
	Segments              []BatchSegment      `json:"segments"`
	FinalSegments         []FinalBatchSegment `json:"finalSegments"`
}

// PreviewBatchSplit parses the paste and proposes one capture per message. It
// never merges: a merge needs a human, and this call has not asked one yet.
func PreviewBatchSplit(input BatchInput) (*BatchPreview, error) {
	rawSegments, err := previewBatchInternal(input)
	if err != nil {
		return nil, err
	}
	source, separator := resolveBatchSourceAndSeparator(input)
	finalSegments := buildFinalSegments(rawSegments, nil)
	viewSegments := viewSegmentsFromFinal(finalSegments)
	return &BatchPreview{
		Source:                source,
		Separator:             separator,
		SuggestedContextTitle: suggestedContextTitle(strings.TrimSpace(input.ContextTitle), viewSegments),
		Segments:              viewSegments,
		FinalSegments:         finalSegments,
	}, nil
}

func previewBatchInternal(input BatchInput) ([]BatchSegment, error) {
	raw := strings.TrimSpace(normalizeNewlines(input.RawText))
	if raw == "" {
		return nil, fmt.Errorf("text is required")
	}
	mode := normalizeBatchSplit(input.SplitMode)
	switch mode {
	case BatchSplitAuto:
		_, detected := detectBatchShape(raw)
		return previewBatchInternal(BatchInput{RawText: raw, SplitMode: detected})
	case BatchSplitWhatsApp:
		return parseWhatsAppBatch(raw), nil
	case BatchSplitLines:
		return parseLineBatch(raw), nil
	case BatchSplitBlocks:
		return parseBlockBatch(raw), nil
	case BatchSplitDivider:
		return parseDividerBatch(raw), nil
	case BatchSplitMarkdown:
		return parseMarkdownHeadingBatch(raw), nil
	case BatchSplitSemantic:
		return parseSemanticFallbackBatch(raw), nil
	default:
		return nil, fmt.Errorf("unsupported split mode %q", input.SplitMode)
	}
}

func CreateBatch(ctx context.Context, g *graph.Graph, input BatchInput) (*BatchCreateResult, error) {
	return CreateBatchWithLLM(ctx, g, input, nil)
}

// resolveBatchAnalysisForCreate returns the BatchAnalysis to persist.
//
// When the client posts back candidates it reviewed, only its merge decisions
// are taken; the bodies are rebuilt from the source text. So the captures that
// land in the graph are the messages that were pasted, grouped the way the human
// asked — never a body a model (or a hand-written request) supplied.
//
// With nothing reviewed — the CLI, a test — the batch is created from the
// deterministic split, unmerged.
func resolveBatchAnalysisForCreate(ctx context.Context, input BatchInput, llm chat.LLMClient) (*BatchAnalysis, error) {
	if len(input.FinalSegments) == 0 {
		return AnalyzeBatchWithLLM(ctx, input, llm)
	}

	rawSegments := input.RawSegments
	if len(rawSegments) == 0 {
		parsed, err := previewBatchInternal(input)
		if err != nil {
			return nil, err
		}
		rawSegments = parsed
	}
	if err := assertVerbatim(input.RawText, rawSegments); err != nil {
		return nil, err
	}

	finalSegments := buildFinalSegments(rawSegments, mergeGroupsFrom(input.FinalSegments))
	source, separator := resolveBatchSourceAndSeparator(input)
	viewSegments := viewSegmentsFromFinal(finalSegments)
	return &BatchAnalysis{
		Source:                source,
		Separator:             separator,
		SuggestedContextTitle: suggestedContextTitle(strings.TrimSpace(input.ContextTitle), viewSegments),
		Segments:              viewSegments,
		RawSegments:           rawSegments,
		FinalSegments:         finalSegments,
	}, nil
}

func CreateBatchWithLLM(ctx context.Context, g *graph.Graph, input BatchInput, llm chat.LLMClient) (*BatchCreateResult, error) {
	analysis, err := resolveBatchAnalysisForCreate(ctx, input, llm)
	if err != nil {
		return nil, err
	}
	batchID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generate batch id: %w", err)
	}
	source := strings.TrimSpace(input.Source)
	if source == "" {
		source = analysis.Source
	}
	contextTitle := strings.TrimSpace(input.ContextTitle)
	if contextTitle == "" {
		contextTitle = analysis.SuggestedContextTitle
	}

	finalSegments := analysis.FinalSegments
	rawSegments := analysis.RawSegments

	rawSegmentsJSON, err := json.Marshal(rawSegments)
	if err != nil {
		return nil, fmt.Errorf("marshal raw segments: %w", err)
	}
	finalSegmentsJSON, err := json.Marshal(finalSegments)
	if err != nil {
		return nil, fmt.Errorf("marshal final segments: %w", err)
	}

	ids := make([]string, 0, len(finalSegments))
	logStore := NewBatchLogStore(g)
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		for _, segment := range finalSegments {
			id, err := nodes.CreateCapture(ctx, tx, nodes.Capture{
				Body:              segment.Body,
				CapturedFrom:      source,
				Tags:              []string{"pending"},
				BatchID:           batchID.String(),
				BatchSource:       source,
				BatchSequence:     segment.Sequence,
				BatchSender:       segment.Sender,
				BatchTimestamp:    segment.Timestamp,
				BatchContextTitle: contextTitle,
			}, nodes.Author{Name: "inbox-batch"})
			if err != nil {
				return fmt.Errorf("create batch capture: %w", err)
			}
			ids = append(ids, id)
			if len(ids) < 2 {
				continue
			}
			if _, err := edges.Create(ctx, tx, edges.Edge{
				Src:  ids[len(ids)-2],
				Dst:  id,
				Type: edges.EdgeTypeRelated,
			}, nodes.Author{Name: "inbox-batch"}); err != nil {
				return fmt.Errorf("link batch captures: %w", err)
			}
		}

		createdIDsJSON, err := json.Marshal(ids)
		if err != nil {
			return fmt.Errorf("marshal created ids: %w", err)
		}
		if err := logStore.Put(ctx, tx, BatchLogRecord{
			ID:                    batchID.String(),
			Source:                source,
			Separator:             analysis.Separator,
			ContextTitle:          contextTitle,
			RawText:               input.RawText,
			RawSegmentsJSON:       string(rawSegmentsJSON),
			FinalSegmentsJSON:     string(finalSegmentsJSON),
			CreatedCaptureIDsJSON: string(createdIDsJSON),
		}); err != nil {
			return fmt.Errorf("write batch log: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	viewSegments := viewSegmentsFromFinal(finalSegments)

	return &BatchCreateResult{
		BatchID:          batchID.String(),
		Segments:         viewSegments,
		FinalSegments:    finalSegments,
		IDs:              ids,
		RawSegmentCount:  len(rawSegments),
		EnrichmentStatus: analysis.EnrichmentStatus,
	}, nil
}

func normalizeBatchSplit(mode string) string {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return BatchSplitAuto
	}
	return mode
}

func detectBatchShape(raw string) (source string, separator string) {
	raw = strings.TrimSpace(raw)
	if hasWhatsAppHeader(raw) {
		return BatchSourceWhatsApp, BatchSplitWhatsApp
	}
	if markdownHeading.MatchString(raw) && len(markdownHeading.FindAllString(raw, -1)) > 1 {
		return BatchSourceText, BatchSplitMarkdown
	}
	if dividerLine.MatchString(raw) {
		return BatchSourceText, BatchSplitDivider
	}
	if len(parseBlockBatch(raw)) > 1 {
		return BatchSourceText, BatchSplitBlocks
	}
	if len(parseLineBatch(raw)) > 1 {
		return BatchSourceText, BatchSplitLines
	}
	return BatchSourceText, BatchSplitSemantic
}

func hasWhatsAppHeader(raw string) bool {
	for _, line := range strings.Split(raw, "\n") {
		if _, _, body, _, ok := parseWhatsAppHeader(line); ok && strings.TrimSpace(body) != "" {
			return true
		}
	}
	return false
}

// parseWhatsAppBatch splits an exported chat into one segment per message,
// folding continuation lines back into the message they belong to. The body is
// the message as it was written: interior blank lines are kept, because a
// multi-paragraph message is multi-paragraph on purpose. Only the outer edges
// are trimmed.
func parseWhatsAppBatch(raw string) []BatchSegment {
	var out []BatchSegment
	var current *BatchSegment
	var body []string

	flush := func() {
		if current == nil {
			return
		}
		current.Body = strings.TrimSpace(strings.Join(body, "\n"))
		out = append(out, *current)
		current, body = nil, nil
	}

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSuffix(line, "\r")
		date, sender, first, _, ok := parseWhatsAppHeader(line)
		if ok {
			flush()
			current = &BatchSegment{
				Sender:          strings.TrimSpace(sender),
				Timestamp:       strings.TrimSpace(date),
				Sequence:        len(out),
				ParseConfidence: "high",
			}
			body = []string{first}
			continue
		}
		if current == nil {
			if strings.TrimSpace(line) == "" {
				continue
			}
			current = &BatchSegment{Sequence: len(out), ParseConfidence: "low"}
			body = []string{line}
			continue
		}
		body = append(body, line)
	}
	flush()
	return cleanSegments(out)
}

func parseWhatsAppHeader(line string) (timestamp string, sender string, body string, rawPrefix string, ok bool) {
	if m := whatsAppBracketDateFirstHeader.FindStringSubmatch(line); len(m) == 5 {
		return m[1] + " " + m[2], m[3], m[4], line[:len(line)-len(m[4])], true
	}
	if m := whatsAppBracketTimeFirstHeader.FindStringSubmatch(line); len(m) == 5 {
		return m[2] + " " + m[1], m[3], m[4], line[:len(line)-len(m[4])], true
	}
	if m := whatsAppDashDateFirstHeader.FindStringSubmatch(line); len(m) == 5 {
		return m[1] + " " + m[2], m[3], m[4], line[:len(line)-len(m[4])], true
	}
	if m := whatsAppDashTimeFirstHeader.FindStringSubmatch(line); len(m) == 5 {
		return m[2] + " " + m[1], m[3], m[4], line[:len(line)-len(m[4])], true
	}
	return "", "", "", "", false
}

func parseBlockBatch(raw string) []BatchSegment {
	parts := regexp.MustCompile(`\n\s*\n+`).Split(raw, -1)
	out := make([]BatchSegment, 0, len(parts))
	for _, part := range parts {
		body := strings.TrimSpace(part)
		if body == "" {
			continue
		}
		out = append(out, BatchSegment{
			Body:            body,
			Sequence:        len(out),
			ParseConfidence: "medium",
		})
	}
	return cleanSegments(out)
}

func parseLineBatch(raw string) []BatchSegment {
	out := make([]BatchSegment, 0)
	for _, line := range strings.Split(raw, "\n") {
		body := strings.TrimSpace(line)
		if body == "" {
			continue
		}
		out = append(out, BatchSegment{
			Body:            body,
			Sequence:        len(out),
			ParseConfidence: "medium",
		})
	}
	return cleanSegments(out)
}

func parseDividerBatch(raw string) []BatchSegment {
	parts := dividerLine.Split(raw, -1)
	return segmentsFromParts(parts, "high")
}

func parseMarkdownHeadingBatch(raw string) []BatchSegment {
	var parts []string
	var current []string
	for _, line := range strings.Split(raw, "\n") {
		if markdownHeading.MatchString(line) && len(current) > 0 {
			parts = append(parts, strings.Join(current, "\n"))
			current = nil
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		parts = append(parts, strings.Join(current, "\n"))
	}
	return segmentsFromParts(parts, "high")
}

func parseSemanticFallbackBatch(raw string) []BatchSegment {
	return cleanSegments([]BatchSegment{{
		Body:            strings.TrimSpace(raw),
		Sequence:        0,
		ParseConfidence: "low",
	}})
}

func segmentsFromParts(parts []string, confidence string) []BatchSegment {
	out := make([]BatchSegment, 0, len(parts))
	for _, part := range parts {
		body := strings.TrimSpace(part)
		if body == "" {
			continue
		}
		out = append(out, BatchSegment{
			Body:            body,
			Sequence:        len(out),
			ParseConfidence: confidence,
		})
	}
	return cleanSegments(out)
}

func suggestedContextTitle(explicit string, segments []BatchSegment) string {
	if explicit != "" {
		return explicit
	}
	if len(segments) == 0 {
		return "Inbox batch"
	}
	title := strings.TrimSpace(segments[0].Body)
	if i := strings.IndexByte(title, '\n'); i >= 0 {
		title = title[:i]
	}
	title = strings.Trim(strings.TrimSpace(title), "#*- ")
	if title == "" {
		return "Inbox batch"
	}
	if utf8.RuneCountInString(title) > 56 {
		return truncateRunes(title, 56) + "…"
	}
	return title
}

// truncateRunes returns the first n runes of s. Slicing by byte offset (e.g.
// title[:56]) corrupts multi-byte UTF-8 text (accented characters, emoji),
// which this app must handle correctly for Portuguese content.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func cleanSegments(segments []BatchSegment) []BatchSegment {
	out := make([]BatchSegment, 0, len(segments))
	for _, segment := range segments {
		segment.Body = strings.TrimSpace(segment.Body)
		if segment.Body == "" {
			continue
		}
		segment.Sequence = len(out)
		out = append(out, segment)
	}
	return out
}
