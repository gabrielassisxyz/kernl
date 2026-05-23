package relate

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"pgregory.net/rapid"
)

// TestRelatedToSymmetry asserts signal symmetry (score(A,B) == score(B,A)).
func TestRelatedToSymmetry(t *testing.T) {
	ctx := context.Background()

	rapid.Check(t, func(rt *rapid.T) {
		g := testutil.NewInMemoryTestGraph(t)
		nodeCount := rapid.IntRange(3, 15).Draw(rt, "nodeCount")
		edgeCount := rapid.IntRange(1, nodeCount*2).Draw(rt, "edgeCount")

		// Generate random node IDs.
		var nodes []string
		_ = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			for i := 0; i < nodeCount; i++ {
				id := "n" + rapid.StringMatching(`[a-z0-9]{8}`).Draw(rt, "id")
				// Ensure unique.
				for _, existing := range nodes {
					if existing == id {
						continue
					}
				}
				nodes = append(nodes, id)
				_, err := tx.Exec(`INSERT INTO nodes(id, type, title) VALUES (?, 'note', 't')`, id)
				if err != nil {
					return err
				}
			}
			for i := 0; i < edgeCount; i++ {
				a := nodes[rapid.IntRange(0, len(nodes)-1).Draw(rt, "a")]
				b := nodes[rapid.IntRange(0, len(nodes)-1).Draw(rt, "b")]
				if a == b {
					continue
				}
				_, err := tx.Exec(`INSERT INTO edges(id, src, dst, label) VALUES (?, ?, ?, 'related')`, "e"+a+b, a, b)
				if err != nil {
					return err
				}
			}
			return nil
		})

		if len(nodes) < 2 {
			return
		}
		a := nodes[0]
		b := nodes[1]

		var sa, sb float64
		_ = g.DoRead(ctx, func(tx *graph.ReadTx) error {
			var err error
			sa, err = scoreBetween(tx, a, b)
			if err != nil {
				return err
			}
			sb, err = scoreBetween(tx, b, a)
			return err
		})

		if sa != sb {
			t.Fatalf("score(%s,%s)=%v score(%s,%s)=%v", a, b, sa, b, a, sb)
		}
	})
}
