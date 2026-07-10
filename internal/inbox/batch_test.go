package inbox

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

func TestPreviewBatchParsesWhatsAppMessages(t *testing.T) {
	segments, err := PreviewBatch(BatchInput{
		RawText: "[06/07/2026, 14:32] Gabriel: Project idea\ncontinued context\n[06/07/2026, 14:33] Gabriel: Task: write parser",
		Source:  "whatsapp",
	})
	if err != nil {
		t.Fatal(err)
	}
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
	segments, err := PreviewBatch(BatchInput{
		RawText: "[13:54, 7/4/2026] Gabriel Assis: substitutos para opencode:\n\n- mariozechner/pi-coding-agent\n[14:12, 7/4/2026] Gabriel Assis: meu objetivo principal é ter dados confiáveis",
		Source:  "whatsapp",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %d: %#v", len(segments), segments)
	}
	if segments[0].Sender != "Gabriel Assis" {
		t.Fatalf("Sender = %q, want Gabriel Assis", segments[0].Sender)
	}
	if segments[0].Timestamp != "7/4/2026 13:54" {
		t.Fatalf("Timestamp = %q, want 7/4/2026 13:54", segments[0].Timestamp)
	}
	if segments[0].Body != "substitutos para opencode:\n- mariozechner/pi-coding-agent" {
		t.Fatalf("first body = %q", segments[0].Body)
	}
	if segments[1].Body != "meu objetivo principal é ter dados confiáveis" {
		t.Fatalf("second body = %q", segments[1].Body)
	}
}

func TestPreviewBatchFallsBackToBlocks(t *testing.T) {
	segments, err := PreviewBatch(BatchInput{RawText: "first idea\n\nsecond idea"})
	if err != nil {
		t.Fatal(err)
	}
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
