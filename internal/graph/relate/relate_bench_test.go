package relate

import (
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// Provisional per-op latency budgets (ratchet-able baseline).
// Measured on AMD Ryzen 7 5700X. These will tighten once real usage data arrives in P2.1.
const (
	budgetRelatedTo1k   = 25 * time.Millisecond
	budgetRelatedTo10k  = 100 * time.Millisecond
	budgetRelatedTo100k = 3 * time.Second
)

func assertBudget(b *testing.B, budget time.Duration) {
	avg := b.Elapsed() / time.Duration(b.N)
	if avg > budget {
		b.Fatalf("latency budget exceeded: avg %v > budget %v (tier=%d)", avg, budget, b.N)
	}
}

// BenchmarkRelatedTo_1k measures RelatedTo latency on a 1k-node graph.
func BenchmarkRelatedTo_1k(b *testing.B) {
	doBenchRelatedTo(b, 1000, budgetRelatedTo1k)
}

// BenchmarkRelatedTo_10k measures RelatedTo latency on a 10k-node graph.
func BenchmarkRelatedTo_10k(b *testing.B) {
	doBenchRelatedTo(b, 10000, budgetRelatedTo10k)
}

// BenchmarkRelatedTo_100k measures RelatedTo latency on a 100k-node graph.
// This is the explicit gate for Risk R-1 (CTE shortest-path at 100k).
func BenchmarkRelatedTo_100k(b *testing.B) {
	doBenchRelatedTo(b, 100000, budgetRelatedTo100k)
}

func doBenchRelatedTo(b *testing.B, nodeCount int, budget time.Duration) {
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
	assertBudget(b, budget)
}
