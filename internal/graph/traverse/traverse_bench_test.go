package traverse

import (
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// Provisional per-op latency budgets (ratchet-able baseline).
// Measured on AMD Ryzen 7 5700X. These will tighten once real usage data arrives in P2.1.
const (
	budgetNeighborsAtDepth1k   = 2 * time.Millisecond
	budgetNeighborsAtDepth10k  = 5 * time.Millisecond
	budgetNeighborsAtDepth100k = 50 * time.Millisecond

	budgetShortestPath1k   = 10 * time.Millisecond
	budgetShortestPath10k  = 100 * time.Millisecond
	budgetShortestPath100k = 3 * time.Second
)

func assertBudget(b *testing.B, budget time.Duration) {
	avg := b.Elapsed() / time.Duration(b.N)
	if avg > budget {
		b.Fatalf("latency budget exceeded: avg %v > budget %v (tier=%d)", avg, budget, b.N)
	}
}

func BenchmarkNeighborsAtDepth_1k(b *testing.B) {
	doBenchNeighborsAtDepth(b, 1000, budgetNeighborsAtDepth1k)
}

func BenchmarkNeighborsAtDepth_10k(b *testing.B) {
	doBenchNeighborsAtDepth(b, 10000, budgetNeighborsAtDepth10k)
}

func BenchmarkNeighborsAtDepth_100k(b *testing.B) {
	doBenchNeighborsAtDepth(b, 100000, budgetNeighborsAtDepth100k)
}

func doBenchNeighborsAtDepth(b *testing.B, nodeCount int, budget time.Duration) {
	b.ReportAllocs()
	sg := testutil.GenerateSynthetic(b, 42, nodeCount)
	origin := sg.Nodes[0].ID

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := sg.Graph.DoRead(b.Context(), func(tx *graph.ReadTx) error {
			_, err := NeighborsAtDepth(b.Context(), tx, origin, 2)
			return err
		})
		if err != nil {
			b.Fatalf("NeighborsAtDepth: %v", err)
		}
	}
	assertBudget(b, budget)
}

func BenchmarkShortestPath_1k(b *testing.B) {
	doBenchShortestPath(b, 1000, budgetShortestPath1k)
}

func BenchmarkShortestPath_10k(b *testing.B) {
	doBenchShortestPath(b, 10000, budgetShortestPath10k)
}

func BenchmarkShortestPath_100k(b *testing.B) {
	doBenchShortestPath(b, 100000, budgetShortestPath100k)
}

func doBenchShortestPath(b *testing.B, nodeCount int, budget time.Duration) {
	b.ReportAllocs()
	sg := testutil.GenerateSynthetic(b, 42, nodeCount)
	origin := sg.Nodes[0].ID
	target := sg.Nodes[len(sg.Nodes)-1].ID

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := sg.Graph.DoRead(b.Context(), func(tx *graph.ReadTx) error {
			_, err := ShortestPath(b.Context(), tx, origin, target)
			return err
		})
		if err != nil {
			b.Fatalf("ShortestPath: %v", err)
		}
	}
	assertBudget(b, budget)
}

// BenchmarkSyntheticDeterminism confirms the generator produces stable output.
func BenchmarkSyntheticDeterminism(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if !testutil.DeterminismCheck(b, 42, 100) {
			b.Fatal("synthetic graph not deterministic")
		}
	}
}
