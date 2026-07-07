package inbox

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

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

func AnalyzeBatchWithLLM(ctx context.Context, input BatchInput, llm chat.LLMClient) (*BatchAnalysis, error) {
	rawSegments, err := previewBatchInternal(input)
	if err != nil {
		return nil, err
	}
	if len(rawSegments) == 0 {
		return nil, fmt.Errorf("no segments produced")
	}

	source := strings.TrimSpace(input.Source)
	separator := normalizeBatchSplit(input.SplitMode)
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

	contextTitle := strings.TrimSpace(input.ContextTitle)

	enricher := NewBatchEnricher(llm)
	var enrichment *BatchEnrichmentResult
	if separator == BatchSplitSemantic && llm != nil {
		enrichment = enricher.EnrichSemantic(ctx, BatchEnrichmentInput{
			Source:      source,
			Separator:   separator,
			ContextHint: contextTitle,
			RawSegments: rawSegments,
		})
	} else {
		enrichment = enricher.Enrich(ctx, BatchEnrichmentInput{
			Source:      source,
			Separator:   separator,
			ContextHint: contextTitle,
			RawSegments: rawSegments,
		})
	}

	var finalSegments []FinalBatchSegment
	if enrichment != nil {
		finalSegments = enrichment.Segments
	}

	// Build UI-compatible BatchSegment view from final segments.
	viewSegments := make([]BatchSegment, 0, len(finalSegments))
	for _, fs := range finalSegments {
		viewSegments = append(viewSegments, BatchSegment{
			Body:            fs.Body,
			Sender:          fs.Sender,
			Timestamp:       fs.Timestamp,
			Sequence:        fs.Sequence,
			ParseConfidence: fs.Confidence,
		})
	}

	var finalContextTitle string
	if enrichment != nil && strings.TrimSpace(enrichment.ContextTitle) != "" {
		finalContextTitle = enrichment.ContextTitle
	}
	if finalContextTitle == "" {
		finalContextTitle = suggestedContextTitle(contextTitle, viewSegments)
	}

	analysis := &BatchAnalysis{
		Source:                source,
		Separator:             separator,
		SuggestedContextTitle: finalContextTitle,
		Segments:              viewSegments,
		RawSegments:           rawSegments,
		FinalSegments:         finalSegments,
	}
	if enrichment != nil {
		analysis.EnrichmentStatus = enrichment.Status
	}
	return analysis, nil
}

func PreviewBatch(input BatchInput) ([]BatchSegment, error) {
	return previewBatchInternal(input)
}

func previewBatchInternal(input BatchInput) ([]BatchSegment, error) {
	raw := strings.TrimSpace(input.RawText)
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

func CreateBatchWithLLM(ctx context.Context, g *graph.Graph, input BatchInput, llm chat.LLMClient) (*BatchCreateResult, error) {
	analysis, err := AnalyzeBatchWithLLM(ctx, input, llm)
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
	if len(finalSegments) == 0 {
		finalSegments = make([]FinalBatchSegment, 0, len(analysis.Segments))
		for _, seg := range analysis.Segments {
			finalSegments = append(finalSegments, FinalBatchSegment{
				Body:            seg.Body,
				Sender:          seg.Sender,
				Timestamp:       seg.Timestamp,
				Sequence:        seg.Sequence,
				SourceSequences: []int{seg.Sequence},
				Confidence:      seg.ParseConfidence,
			})
		}
	}

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

	viewSegments := make([]BatchSegment, 0, len(finalSegments))
	for _, fs := range finalSegments {
		viewSegments = append(viewSegments, BatchSegment{
			Body:            fs.Body,
			Sender:          fs.Sender,
			Timestamp:       fs.Timestamp,
			Sequence:        fs.Sequence,
			ParseConfidence: fs.Confidence,
		})
	}

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

func parseWhatsAppBatch(raw string) []BatchSegment {
	var out []BatchSegment
	var current *BatchSegment
	for _, line := range strings.Split(raw, "\n") {
		date, sender, body, _, ok := parseWhatsAppHeader(line)
		if ok {
			if current != nil {
				out = append(out, *current)
			}
			current = &BatchSegment{
				Body:            strings.TrimSpace(body),
				Sender:          strings.TrimSpace(sender),
				Timestamp:       strings.TrimSpace(date),
				Sequence:        len(out),
				ParseConfidence: "high",
			}
			continue
		}
		if current == nil {
			text := strings.TrimSpace(line)
			if text == "" {
				continue
			}
			current = &BatchSegment{Body: text, Sequence: len(out), ParseConfidence: "low"}
			continue
		}
		current.Body = strings.TrimSpace(current.Body + "\n" + line)
	}
	if current != nil {
		out = append(out, *current)
	}
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
	if len(title) > 56 {
		return title[:56] + "…"
	}
	return title
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
