package graph_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func TestBulkInsertLatency(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	const n = 1000
	start := time.Now()
	for i := 0; i < n; i++ {
		err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			_, err := nodes.CreateCapture(ctx, tx, nodes.Capture{
				Title: fmt.Sprintf("bulk-%d", i),
				Body:  fmt.Sprintf("body-%d", i),
			}, nodes.Author{Name: "bulk"})
			return err
		})
		if err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	totalInsert := time.Since(start)

	// Query all nodes back
	start = time.Now()
	var count int
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow("SELECT COUNT(*) FROM nodes WHERE type = 'capture'").Scan(&count)
	})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	totalQuery := time.Since(start)

	perInsert := totalInsert / n
	perQuery := totalQuery // only one query

	if count != n {
		t.Fatalf("expected %d nodes, got %d", n, count)
	}
	t.Logf("bulk insert: %d nodes in %v (per insert: %v)", n, totalInsert, perInsert)
	t.Logf("bulk query: count=%d in %v (per query: %v)", count, totalQuery, perQuery)

	if perInsert > time.Millisecond && !raceEnabled() {
		t.Errorf("per-insert latency %v exceeds 1ms threshold", perInsert)
	}
	if totalInsert > 50*time.Millisecond {
		t.Logf("WARNING: total insert %v exceeds 50ms soft threshold", totalInsert)
	}
}
