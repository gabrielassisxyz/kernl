package inbox_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/tags"
	"github.com/gabrielassisxyz/kernl/internal/inbox"
)

func TestRollups(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	// Three captures created "now" — all land on the same calendar day.
	for _, body := range []string{"one", "two", "three"} {
		if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			_, err := nodes.CreateCapture(ctx, tx, nodes.Capture{Body: body, Tags: []string{tags.Pending}}, nodes.Author{Name: "tester"})
			return err
		}); err != nil {
			t.Fatalf("CreateCapture: %v", err)
		}
	}

	rollups, err := inbox.Rollups(ctx, g)
	if err != nil {
		t.Fatalf("Rollups: %v", err)
	}
	if len(rollups) != 1 {
		t.Fatalf("expected 1 day rollup, got %d", len(rollups))
	}
	if rollups[0].Count != 3 || len(rollups[0].Captures) != 3 {
		t.Errorf("expected day count=3 with 3 captures, got count=%d items=%d", rollups[0].Count, len(rollups[0].Captures))
	}
	if rollups[0].Date == "" {
		t.Errorf("expected a non-empty date")
	}
}
