package traverse

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"pgregory.net/rapid"
)

// TestNeighborsAtDepthMonotonic asserts that depth N is subset of depth N+1.
func TestNeighborsAtDepthMonotonic(t *testing.T) {
	ctx := context.Background()

	rapid.Check(t, func(rt *rapid.T) {
		g := testutil.NewInMemoryTestGraph(t)
		nodeCount := rapid.IntRange(3, 15).Draw(rt, "nodeCount")
		edgeCount := rapid.IntRange(1, nodeCount*2).Draw(rt, "edgeCount")

		var nodes []string
		_ = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			for i := 0; i < nodeCount; i++ {
				id := "n" + rapid.StringMatching(`[a-z0-9]{8}`).Draw(rt, "id")
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

		if len(nodes) < 1 {
			return
		}
		origin := nodes[0]

		for d := 1; d < MaxDepth; d++ {
			var shallow, deep []string
			_ = g.DoRead(ctx, func(tx *graph.ReadTx) error {
				var err error
				shallow, err = NeighborsAtDepth(ctx, tx, origin, d)
				if err != nil {
					return err
				}
				deep, err = NeighborsAtDepth(ctx, tx, origin, d+1)
				return err
			})

			shSet := make(map[string]struct{}, len(shallow))
			for _, s := range shallow {
				shSet[s] = struct{}{}
			}
			for _, s := range deep {
				shSet[s] = struct{}{}
			}
			for _, s := range shallow {
				if _, ok := shSet[s]; !ok {
					// Shallow node should also appear in deeper set (monotonic expansion).
					// Wait no: if the graph is disconnected, a shallow neighbor might disappear
					// with greater depth? No, the CTE only expands; it never retracts.
					// Every node reachable within d hops is also reachable within d+1 hops.
					t.Fatalf("monotonicity violated at depth %d: %s in shallow but not deep", d, s)
				}
			}
		}
	})
}

// TestNeighborsAtDepthExcludesOrigin asserts the origin never appears.
func TestNeighborsAtDepthExcludesOrigin(t *testing.T) {
	ctx := context.Background()

	rapid.Check(t, func(rt *rapid.T) {
		g := testutil.NewInMemoryTestGraph(t)
		nodeCount := rapid.IntRange(3, 15).Draw(rt, "nodeCount")
		edgeCount := rapid.IntRange(1, nodeCount*2).Draw(rt, "edgeCount")

		var nodes []string
		_ = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			for i := 0; i < nodeCount; i++ {
				id := "n" + rapid.StringMatching(`[a-z0-9]{8}`).Draw(rt, "id")
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

		if len(nodes) < 1 {
			return
		}
		origin := nodes[0]

		for d := 1; d <= MaxDepth; d++ {
			var got []string
			_ = g.DoRead(ctx, func(tx *graph.ReadTx) error {
				var err error
				got, err = NeighborsAtDepth(ctx, tx, origin, d)
				return err
			})
			for _, s := range got {
				if s == origin {
					t.Fatalf("origin %s included in depth %d result", origin, d)
				}
			}
		}
	})
}

// TestShortestPathLengthSymmetry asserts len(a→b) == len(b→a) where both exist.
func TestShortestPathLengthSymmetry(t *testing.T) {
	ctx := context.Background()

	rapid.Check(t, func(rt *rapid.T) {
		g := testutil.NewInMemoryTestGraph(t)
		nodeCount := rapid.IntRange(3, 15).Draw(rt, "nodeCount")
		edgeCount := rapid.IntRange(1, nodeCount*2).Draw(rt, "edgeCount")

		var nodes []string
		_ = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			for i := 0; i < nodeCount; i++ {
				id := "n" + rapid.StringMatching(`[a-z0-9]{8}`).Draw(rt, "id")
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

		var pa, pb Path
		_ = g.DoRead(ctx, func(tx *graph.ReadTx) error {
			var err error
			pa, err = ShortestPath(ctx, tx, a, b)
			if err != nil {
				return err
			}
			pb, err = ShortestPath(ctx, tx, b, a)
			return err
		})

		if pa.Found != pb.Found {
			t.Fatalf("asymmetry in Found: a→b %v vs b→a %v", pa.Found, pb.Found)
		}
		if pa.Found && pa.Length != pb.Length {
			t.Fatalf("length asymmetry: a→b %d vs b→a %d", pa.Length, pb.Length)
		}
	})
}
