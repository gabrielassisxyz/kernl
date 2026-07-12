package inbox_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/inbox"
	"github.com/gabrielassisxyz/kernl/internal/ingest"
)

// openInboxGraph opens a throwaway graph for inbox tests.
func openInboxGraph(t *testing.T) *graph.Graph {
	t.Helper()
	g, err := graph.Open(context.Background(), graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	t.Cleanup(func() { g.Close() })
	return g
}

// TestProcessCaptureUpdate merges a capture into an existing note via accepted
// hunks, links it with merged_into (not derived_from), and triages the capture.
func TestProcessCaptureUpdate(t *testing.T) {
	ctx := context.Background()
	g := openInboxGraph(t)
	vault := t.TempDir()

	var noteID, captureID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		noteID, err = nodes.CreateNote(ctx, tx, nodes.Note{Title: "Sourdough", Body: "Sourdough uses wild yeast."}, nodes.Author{Name: "test"})
		if err != nil {
			return err
		}
		captureID, err = nodes.CreateCapture(ctx, tx, nodes.Capture{
			Body: "Sourdough also benefits from a long cold ferment.",
			Tags: []string{"pending"},
		}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	err := inbox.ProcessCapture(ctx, g, vault, nil, captureID, inbox.ProcessRequest{
		Actions:       []inbox.Action{{Target: "update"}},
		TargetNoteID:  noteID,
		AcceptedHunks: []ingest.MergeHunk{{ID: "0", Content: "Benefits from a long cold ferment."}},
	})
	if err != nil {
		t.Fatalf("ProcessCapture update: %v", err)
	}

	// Note body merged, capture triaged, merged_into edge present.
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		n, err := nodes.GetNote(ctx, tx, noteID)
		if err != nil {
			t.Fatalf("GetNote: %v", err)
		}
		if !strings.Contains(n.Body, "long cold ferment") {
			t.Errorf("expected merged hunk in note body, got %q", n.Body)
		}
		c, err := nodes.GetCapture(ctx, tx, captureID)
		if err != nil {
			t.Fatalf("GetCapture: %v", err)
		}
		if !hasTagT(c.Tags, "triaged") || hasTagT(c.Tags, "pending") {
			t.Errorf("expected capture triaged, got tags %v", c.Tags)
		}
		in, _ := edges.Incoming(ctx, tx, captureID)
		var merged bool
		for _, e := range in {
			if e.Label == "merged_into" && e.Src == noteID {
				merged = true
			}
			if e.Label == "derived_from" {
				t.Errorf("update must not create a derived_from edge")
			}
		}
		if !merged {
			t.Errorf("expected merged_into edge note→capture")
		}
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
	}

	// Undo must NOT delete the pre-existing note — only re-pend the capture.
	if err := inbox.Reopen(ctx, g, vault, captureID); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		if _, err := nodes.GetNote(ctx, tx, noteID); err != nil {
			t.Errorf("note must survive undo of an update, got %v", err)
		}
		c, err := nodes.GetCapture(ctx, tx, captureID)
		if err != nil {
			t.Fatalf("GetCapture: %v", err)
		}
		if !hasTagT(c.Tags, "pending") {
			t.Errorf("expected capture re-pended after undo, got tags %v", c.Tags)
		}
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
	}
}

// TestProcessCaptureUpdateNoTargetFallsBack creates a note when no confident
// target exists, so an update suggestion never loses the capture.
func TestProcessCaptureUpdateNoTargetFallsBack(t *testing.T) {
	ctx := context.Background()
	g := openInboxGraph(t)
	vault := t.TempDir()

	var captureID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		captureID, err = nodes.CreateCapture(ctx, tx, nodes.Capture{
			Body: "An entirely novel unmatched thought.",
			Tags: []string{"pending"},
		}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	err := inbox.ProcessCapture(ctx, g, vault, nil, captureID, inbox.ProcessRequest{
		Actions: []inbox.Action{{Target: "update"}},
	})
	if err != nil {
		t.Fatalf("ProcessCapture update: %v", err)
	}

	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var noteCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE type='note'`).Scan(&noteCount); err != nil {
			return err
		}
		if noteCount != 1 {
			t.Errorf("expected fallback to create 1 note, got %d", noteCount)
		}
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
	}
}

func hasTagT(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}
