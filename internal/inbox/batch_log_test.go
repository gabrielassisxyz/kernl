package inbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

func TestBatchLogStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	store := NewBatchLogStore(g)
	raw, _ := json.Marshal([]BatchSegment{{Body: "raw", Sequence: 0}})
	final, _ := json.Marshal([]FinalBatchSegment{{Body: "final", Sequence: 0, SourceSequences: []int{0}}})
	ids, _ := json.Marshal([]string{"n-1"})

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return store.Put(ctx, tx, BatchLogRecord{
			ID:                    "batch-1",
			Source:                "whatsapp",
			Separator:             "whatsapp",
			ContextTitle:          "planning",
			RawText:               "[...]",
			RawSegmentsJSON:       string(raw),
			FinalSegmentsJSON:     string(final),
			CreatedCaptureIDsJSON: string(ids),
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := store.Get(ctx, "batch-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "batch-1" || got.Source != "whatsapp" || got.ContextTitle != "planning" {
		t.Fatalf("unexpected record: %+v", got)
	}
}

func TestBatchLogStoreMissing(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	store := NewBatchLogStore(g)
	_, err = store.Get(ctx, "missing")
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}
