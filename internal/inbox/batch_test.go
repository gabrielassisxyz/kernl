package inbox

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

func TestPreviewBatchParsesWhatsAppMessages(t *testing.T) {
	preview, err := PreviewBatchSplit(BatchInput{
		RawText: "[06/07/2026, 14:32] Gabriel: Project idea\ncontinued context\n[06/07/2026, 14:33] Gabriel: Task: write parser",
		Source:  "whatsapp",
	})
	if err != nil {
		t.Fatal(err)
	}
	segments := preview.Segments
	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %d: %#v", len(segments), segments)
	}
	if segments[0].Sender != "Gabriel" {
		t.Fatalf("Sender = %q, want Gabriel", segments[0].Sender)
	}
	if segments[0].Body != "Project idea\ncontinued context" {
		t.Fatalf("first body = %q", segments[0].Body)
	}
	if segments[1].Body != "Task: write parser" {
		t.Fatalf("second body = %q", segments[1].Body)
	}
}

func TestPreviewBatchParsesWhatsAppTimeFirstMessages(t *testing.T) {
	preview, err := PreviewBatchSplit(BatchInput{
		RawText: "[13:54, 7/4/2026] Gabriel Assis: substitutos para opencode:\n\n- mariozechner/pi-coding-agent\n[14:12, 7/4/2026] Gabriel Assis: meu objetivo principal é ter dados confiáveis",
		Source:  "whatsapp",
	})
	if err != nil {
		t.Fatal(err)
	}
	segments := preview.Segments
	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %d: %#v", len(segments), segments)
	}
	if segments[0].Sender != "Gabriel Assis" {
		t.Fatalf("Sender = %q, want Gabriel Assis", segments[0].Sender)
	}
	if segments[0].Timestamp != "7/4/2026 13:54" {
		t.Fatalf("Timestamp = %q, want 7/4/2026 13:54", segments[0].Timestamp)
	}
	// The blank line the author typed is part of the message: a body is kept as
	// written, not reflowed.
	if segments[0].Body != "substitutos para opencode:\n\n- mariozechner/pi-coding-agent" {
		t.Fatalf("first body = %q", segments[0].Body)
	}
	if segments[1].Body != "meu objetivo principal é ter dados confiáveis" {
		t.Fatalf("second body = %q", segments[1].Body)
	}
}

// The capture body is the primary source: whatever the author typed, byte for
// byte. A multi-paragraph message must survive the parser with its paragraph
// breaks — the reflowed one-paragraph version is a different document.
func TestPreviewBatchKeepsMessageParagraphsVerbatim(t *testing.T) {
	message := "falar \"trablhar 1h por dia\" é facil, mas tenho dificuldade de internalizar.\n\nou me falta entender o que estou construindo, sei la\n\npreciso de uma forma de visualizar esse progresso"
	raw := "4/1/26, 16:34 - Gabriel Assis: " + message + "\n4/1/26, 19:46 - Gabriel Assis: next message"

	preview, err := PreviewBatchSplit(BatchInput{RawText: raw})
	if err != nil {
		t.Fatal(err)
	}
	segments := preview.Segments
	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %d: %#v", len(segments), segments)
	}
	if segments[0].Body != message {
		t.Fatalf("body was rewritten by the parser:\n got: %q\nwant: %q", segments[0].Body, message)
	}
}

func TestPreviewBatchFallsBackToBlocks(t *testing.T) {
	preview, err := PreviewBatchSplit(BatchInput{RawText: "first idea\n\nsecond idea"})
	if err != nil {
		t.Fatal(err)
	}
	segments := preview.Segments
	if len(segments) != 2 {
		t.Fatalf("expected 2 block segments, got %d", len(segments))
	}
	if segments[0].ParseConfidence != "medium" {
		t.Fatalf("ParseConfidence = %q, want medium", segments[0].ParseConfidence)
	}
}

func TestAnalyzeBatchDetectsShape(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		source    string
		separator string
		count     int
	}{
		{
			name:      "whatsapp date first",
			raw:       "[06/07/2026, 14:32] Gabriel: Project idea\n[06/07/2026, 14:33] Gabriel: Task idea",
			source:    BatchSourceWhatsApp,
			separator: BatchSplitWhatsApp,
			count:     2,
		},
		{
			name:      "whatsapp time first",
			raw:       "[13:54, 7/4/2026] Gabriel Assis: Project idea\n[14:12, 7/4/2026] Gabriel Assis: Task idea",
			source:    BatchSourceWhatsApp,
			separator: BatchSplitWhatsApp,
			count:     2,
		},
		{
			name:      "markdown headings",
			raw:       "# Project idea\nSome context\n# Next task\nDo the thing",
			source:    BatchSourceText,
			separator: BatchSplitMarkdown,
			count:     2,
		},
		{
			name:      "divider",
			raw:       "first idea\n---\nsecond idea",
			source:    BatchSourceText,
			separator: BatchSplitDivider,
			count:     2,
		},
		{
			name:      "one per line",
			raw:       "first idea\nsecond idea",
			source:    BatchSourceText,
			separator: BatchSplitLines,
			count:     2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis, err := AnalyzeBatch(BatchInput{RawText: tt.raw})
			if err != nil {
				t.Fatal(err)
			}
			if analysis.Source != tt.source {
				t.Fatalf("Source = %q, want %q", analysis.Source, tt.source)
			}
			if analysis.Separator != tt.separator {
				t.Fatalf("Separator = %q, want %q", analysis.Separator, tt.separator)
			}
			if len(analysis.Segments) != tt.count {
				t.Fatalf("segments = %d, want %d", len(analysis.Segments), tt.count)
			}
			if analysis.SuggestedContextTitle == "" {
				t.Fatal("SuggestedContextTitle is empty")
			}
		})
	}
}

func TestAnalyzeBatchRespectsOverrides(t *testing.T) {
	analysis, err := AnalyzeBatch(BatchInput{
		RawText:   "first idea\nsecond idea",
		Source:    "manual",
		SplitMode: BatchSplitBlocks,
	})
	if err != nil {
		t.Fatal(err)
	}
	if analysis.Source != "manual" {
		t.Fatalf("Source = %q, want manual", analysis.Source)
	}
	if analysis.Separator != BatchSplitBlocks {
		t.Fatalf("Separator = %q, want blocks", analysis.Separator)
	}
	if len(analysis.Segments) != 1 {
		t.Fatalf("segments = %d, want 1 block", len(analysis.Segments))
	}
}

// flakyLLM answers differently every call — the real failure mode: the same
// fixture through the same merge prompt gave 25 captures on one run and 20 on
// the next, and five messages were merged away by nobody's decision.
type flakyLLM struct {
	replies []string
	calls   int
}

func (m *flakyLLM) Chat(ctx context.Context, messages []chat.Message, tools []chat.Tool) (*chat.ChatResponse, error) {
	reply := m.replies[m.calls%len(m.replies)]
	m.calls++
	return &chat.ChatResponse{Content: reply}, nil
}

const threeMessages = "4/1/26, 08:37 - Me: first message\n4/1/26, 10:18 - Me: second message\n4/1/26, 12:10 - Me: third message"

// The number of captures a paste produces is decided by the parser, never by the
// model: pasting the same text twice must offer the same captures both times,
// however enthusiastically the model wants to merge them.
func TestAnalyzeBatchProposesTheSameCaptureCountEveryRun(t *testing.T) {
	llm := &flakyLLM{replies: []string{
		`{"context_title":"run one","merges":[]}`,
		`{"context_title":"run two","merges":[{"source_sequences":[0,1,2],"reason":"feeling merge-y today"}]}`,
	}}
	ctx := context.Background()

	first, err := AnalyzeBatchWithLLM(ctx, BatchInput{RawText: threeMessages}, llm)
	if err != nil {
		t.Fatal(err)
	}
	second, err := AnalyzeBatchWithLLM(ctx, BatchInput{RawText: threeMessages}, llm)
	if err != nil {
		t.Fatal(err)
	}

	if len(first.Segments) != 3 || len(second.Segments) != 3 {
		t.Fatalf("capture counts drifted with the model: %d then %d, want 3 both times",
			len(first.Segments), len(second.Segments))
	}
	// The merge the model wanted is offered, not taken.
	if len(second.MergeProposals) != 1 {
		t.Fatalf("MergeProposals = %#v, want the merge offered as a proposal", second.MergeProposals)
	}
	for i, seg := range second.FinalSegments {
		if len(seg.SourceSequences) != 1 {
			t.Fatalf("segment %d was merged without anyone accepting it: %v", i, seg.SourceSequences)
		}
	}
}

// The capture body is the message as it was written. Not even a caller posting
// its own bodies can change that — only the merge grouping survives the trip.
func TestCreateBatchIgnoresBodiesSuppliedByTheClient(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	result, err := CreateBatch(ctx, g, BatchInput{
		RawText: threeMessages,
		Source:  "whatsapp",
		FinalSegments: []FinalBatchSegment{
			{Body: "A tidy paraphrase of the first message", Sequence: 0, SourceSequences: []int{0}},
			{Body: "Another paraphrase", Sequence: 1, SourceSequences: []int{1}},
			{Body: "And a third", Sequence: 2, SourceSequences: []int{2}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	bodies := captureBodies(ctx, t, g, result.BatchID)
	want := []string{"first message", "second message", "third message"}
	for i, w := range want {
		if bodies[i] != w {
			t.Fatalf("capture %d body = %q, want the source message %q", i, bodies[i], w)
		}
	}
}

func TestCreateBatchAppliesOnlyAcceptedMerges(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	// The human accepted a merge of messages 0 and 1, and left 2 alone.
	result, err := CreateBatch(ctx, g, BatchInput{
		RawText: threeMessages,
		Source:  "whatsapp",
		FinalSegments: []FinalBatchSegment{
			{Sequence: 0, SourceSequences: []int{0, 1}},
			{Sequence: 1, SourceSequences: []int{2}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.IDs) != 2 {
		t.Fatalf("captures = %d, want 2", len(result.IDs))
	}

	merged := result.FinalSegments[0]
	if merged.Body != "first message\n\nsecond message" {
		t.Fatalf("merged body = %q, want both messages kept whole", merged.Body)
	}
	if merged.Timestamp != "4/1/26 08:37" {
		t.Fatalf("merged Timestamp = %q, want the first source message's 08:37", merged.Timestamp)
	}
}

// A body that is not literally in the pasted text cannot have come from the
// parser, so it is a rewrite — and the request fails rather than writing it.
func TestCreateBatchRejectsSegmentsThatAreNotSourceText(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	_, err = CreateBatch(ctx, g, BatchInput{
		RawText:       threeMessages,
		Source:        "whatsapp",
		RawSegments:   []BatchSegment{{Body: "a message nobody ever sent", Sequence: 0, ParseConfidence: "high"}},
		FinalSegments: []FinalBatchSegment{{Sequence: 0, SourceSequences: []int{0}}},
	})
	if err == nil {
		t.Fatal("expected a rewritten segment to be rejected")
	}
	if !strings.Contains(err.Error(), "verbatim") {
		t.Fatalf("error = %v, want it to name the verbatim rule", err)
	}
}

func captureBodies(ctx context.Context, t *testing.T, g *graph.Graph, batchID string) []string {
	t.Helper()
	var captures []*nodes.Capture
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		captures, err = nodes.ListCaptures(ctx, tx, nodes.CaptureFilter{BatchID: batchID})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	sort.Slice(captures, func(i, j int) bool { return captures[i].BatchSequence < captures[j].BatchSequence })
	bodies := make([]string, 0, len(captures))
	for _, c := range captures {
		bodies = append(bodies, c.Body)
	}
	return bodies
}

func TestCreateBatchPersistsRelatedCaptures(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	result, err := CreateBatch(ctx, g, BatchInput{
		RawText:      "[06/07/2026, 14:32] Me: Project idea\n[06/07/2026, 14:33] Me: Task idea",
		Source:       "whatsapp",
		ContextTitle: "Planning dump",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.BatchID == "" || len(result.IDs) != 2 {
		t.Fatalf("unexpected result: %#v", result)
	}
	var captures []*nodes.Capture
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		captures, err = nodes.ListCaptures(ctx, tx, nodes.CaptureFilter{BatchID: result.BatchID})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(captures) != 2 {
		t.Fatalf("expected 2 captures, got %d", len(captures))
	}
	for _, cap := range captures {
		if cap.BatchID != result.BatchID {
			t.Fatalf("BatchID = %q, want %q", cap.BatchID, result.BatchID)
		}
		if cap.BatchContextTitle != "Planning dump" {
			t.Fatalf("BatchContextTitle = %q", cap.BatchContextTitle)
		}
	}
}
