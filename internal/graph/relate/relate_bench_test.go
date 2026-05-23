package relate

import (
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// BenchmarkRelatedTo_1k measures RelatedTo latency on a 1k-node graph.
func BenchmarkRelatedTo_1k(b *testing.B) {
	doBenchRelatedTo(b, 1000)
}

// BenchmarkRelatedTo_10k measures RelatedTo latency on a 10k-node graph.
func BenchmarkRelatedTo_10k(b *testing.B) {
	doBenchRelatedTo(b, 10000)
}

// BenchmarkRelatedTo_100k measures RelatedTo latency on a 100k-node graph.
// This is the explicit gate for Risk R-1 (CTE shortest-path at 100k).
func BenchmarkRelatedTo_100k(b *testing.B) {
	doBenchRelatedTo(b, 100000)
}

func doBenchRelatedTo(b *testing.B, nodeCount int) {
	b.ReportAllocs()
	// Generate fixed seed for stable benchmark comparisons.
	sg := testutil.GenerateSynthetic(b, 42, nodeCount)
	origin := sg.Nodes[0].ID

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := sg.Graph.DoRead(b.Context(), func(tx *graph.ReadTx) error {
			_, err := RelatedTo(b.Context(), tx, origin, 20)
			return err
		})
		if err != nil {
			b.Fatalf("RelatedTo: %v", err)
		}
	}
}
