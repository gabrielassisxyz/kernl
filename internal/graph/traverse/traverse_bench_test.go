package traverse

import (
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func BenchmarkNeighborsAtDepth_1k(b *testing.B) {
	doBenchNeighborsAtDepth(b, 1000)
}

func BenchmarkNeighborsAtDepth_10k(b *testing.B) {
	doBenchNeighborsAtDepth(b, 10000)
}

func BenchmarkNeighborsAtDepth_100k(b *testing.B) {
	doBenchNeighborsAtDepth(b, 100000)
}

func doBenchNeighborsAtDepth(b *testing.B, nodeCount int) {
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
}

func BenchmarkShortestPath_1k(b *testing.B) {
	doBenchShortestPath(b, 1000)
}

func BenchmarkShortestPath_10k(b *testing.B) {
	doBenchShortestPath(b, 10000)
}

func BenchmarkShortestPath_100k(b *testing.B) {
	doBenchShortestPath(b, 100000)
}

func doBenchShortestPath(b *testing.B, nodeCount int) {
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
}

// BenchmarkSyntheticDeterminism confirms the generator produces stable output.
func BenchmarkSyntheticDeterminism(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if !testutil.DeterminismCheck(b, 42, 100) {
			b.Fatal("synthetic graph not deterministic")
		}
	}
}
