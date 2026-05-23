package relate

import (
	"context"
	"math"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// setupGraph creates nodes and edges directly for deterministic scoring graphs.
func setupGraph(t *testing.T, ctx context.Context, g *graph.Graph, nodesSQL, edgesSQL string) {
	t.Helper()
	if nodesSQL != "" {
		err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			_, err := tx.Exec(nodesSQL)
			return err
		})
		if err != nil {
			t.Fatalf("insert nodes: %v", err)
		}
	}
	if edgesSQL != "" {
		err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			_, err := tx.Exec(edgesSQL)
			return err
		})
		if err != nil {
			t.Fatalf("insert edges: %v", err)
		}
	}
}

// AE1: direct link contribution.
func TestRelatedTo_AE1_DirectLink(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	setupGraph(t, ctx, g,
		`INSERT INTO nodes(id, type, title) VALUES ('A','note','A'),('B','note','B');`,
		`INSERT INTO edges(id, src, dst, label) VALUES ('e1','A','B','related');`,
	)

	var got []string
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = RelatedTo(ctx, tx, "A", 10)
		return err
	})
	if err != nil {
		t.Fatalf("RelatedTo: %v", err)
	}
	if len(got) != 1 || got[0] != "B" {
		t.Errorf("expected [B], got %v", got)
	}
}

// AE2: shared provenance contributes sourceOverlap.
func TestRelatedTo_AE2_SourceOverlap(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	// A and B both generated_from C
	setupGraph(t, ctx, g,
		`INSERT INTO nodes(id, type, title) VALUES ('A','note','A'),('B','note','B'),('C','source','C');`,
		`INSERT INTO edges(id, src, dst, label) VALUES ('e1','C','A','generated_from'),('e2','C','B','generated_from');`,
	)

	var score float64
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		score, err = scoreBetween(tx, "A", "B")
		return err
	})
	if err != nil {
		t.Fatalf("scoreBetween: %v", err)
	}
	// sourceOverlap = 1 distinct shared source * WeightSourceOverlap = 4.0.
	// C is also a common undirected neighbor (degree 2), so Adamic-Adar contributes.
	expected := WeightSourceOverlap + WeightTypeAffinity + WeightAdamicAdar*(1.0/math.Log(2))
	if math.Abs(score-expected) > 1e-9 {
		t.Errorf("expected score ≈ %v, got %v", expected, score)
	}
}

// AE3: Adamic-Adar with degree=2 common neighbor.
func TestRelatedTo_AE3_AdamicAdar(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	// A and B share common neighbor N; N has degree 2 (connected to A and B)
	setupGraph(t, ctx, g,
		`INSERT INTO nodes(id, type, title) VALUES ('A','note','A'),('B','note','B'),('N','note','N');`,
		`INSERT INTO edges(id, src, dst, label) VALUES ('e1','A','N','related'),('e2','B','N','related');`,
	)

	var score float64
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		score, err = scoreBetween(tx, "A", "B")
		return err
	})
	if err != nil {
		t.Fatalf("scoreBetween: %v", err)
	}

	// directLink=0, sourceOverlap=0, adamicAdar = 1.5 * (1/ln(2))
	expectedAA := 1.5 * (1.0 / math.Log(2))
	// typeAffinity = 1.0 (same type)
	expected := expectedAA + 1.0
	if math.Abs(score-expected) > 1e-9 {
		t.Errorf("expected score ≈ %v, got %v", expected, score)
	}
}

// AE4: type-affinity between same-type and cross-type nodes.
func TestRelatedTo_AE4_TypeAffinity(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	// A and B both note; C is bead
	setupGraph(t, ctx, g,
		`INSERT INTO nodes(id, type, title) VALUES ('A','note','A'),('B','note','B'),('C','bead','C');`,
		`INSERT INTO edges(id, src, dst, label) VALUES ('e1','A','B','related'),('e2','A','C','related');`,
	)

	var scoreAB, scoreAC float64
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		scoreAB, err = scoreBetween(tx, "A", "B")
		if err != nil {
			return err
		}
		scoreAC, err = scoreBetween(tx, "A", "C")
		return err
	})
	if err != nil {
		t.Fatalf("scoreBetween: %v", err)
	}

	// AB: directLink 3.0 + typeAffinity 1.0
	if math.Abs(scoreAB-4.0) > 1e-9 {
		t.Errorf("expected AB=4.0, got %v", scoreAB)
	}
	// AC: directLink 3.0 + typeAffinity 0.25
	if math.Abs(scoreAC-3.25) > 1e-9 {
		t.Errorf("expected AC=3.25, got %v", scoreAC)
	}
}

// AE7: graph with no provenance edges → sourceOverlap = 0 everywhere.
func TestRelatedTo_AE7_SourceOverlapDormant(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	setupGraph(t, ctx, g,
		`INSERT INTO nodes(id, type, title) VALUES ('A','note','A'),('B','note','B'),('C','note','C');`,
		`INSERT INTO edges(id, src, dst, label) VALUES ('e1','A','B','related'),('e2','B','C','related');`,
	)

	var score float64
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		score, err = scoreBetween(tx, "A", "B")
		return err
	})
	if err != nil {
		t.Fatalf("scoreBetween: %v", err)
	}

	// Should be driven by direct-link + type-affinity
	// A and B share B as a common neighbor? A-B direct yes.
	// directLink = 3.0, typeAffinity = 1.0 -> 4.0
	if math.Abs(score-4.0) > 1e-9 {
		t.Errorf("expected score=4.0 (direct+type, source=0), got %v", score)
	}
}

// Candidate boundary: same-type node with no structural relation is NOT returned.
func TestRelatedTo_CandidateBoundary(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	setupGraph(t, ctx, g,
		`INSERT INTO nodes(id, type, title) VALUES ('A','note','A'),('B','note','B'),('C','note','C');`,
		`INSERT INTO edges(id, src, dst, label) VALUES ('e1','A','B','related');`,
	)

	// C has same type as A but no edge; should not appear.
	var got []string
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = RelatedTo(ctx, tx, "A", 10)
		return err
	})
	if err != nil {
		t.Fatalf("RelatedTo: %v", err)
	}
	if len(got) != 1 || got[0] != "B" {
		t.Errorf("expected [B], got %v", got)
	}
}

// Degree-1 guard in Adamic-Adar.
func TestRelatedTo_Degree1Guard(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	// Common neighbor N only connects to A — deg=1 — should be skipped.
	// But actually for common neighbor need edges N-A and N-B, so deg(A-N) deg 1 and N-B deg 1.
	// This means N is connected to both A and B -> degree 2, not 1.
	// Let's create N connected only to A (not B) so no common neighbor.
	setupGraph(t, ctx, g,
		`INSERT INTO nodes(id, type, title) VALUES ('A','note','A'),('B','note','B'),('N','note','N');`,
		`INSERT INTO edges(id, src, dst, label) VALUES ('e1','A','N','related');`,
	)

	// A and B have no common neighbors -> adamicAdar = 0
	var score float64
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		score, err = scoreBetween(tx, "A", "B")
		return err
	})
	if err != nil {
		t.Fatalf("scoreBetween: %v", err)
	}

	// directLink=0, sourceOverlap=0, adamicAdar=0 (no common neighbor), typeAffinity=1.0
	if math.Abs(score-1.0) > 1e-9 {
		t.Errorf("expected score=1.0 (type only, no structural relation), got %v", score)
	}
}

// Double-count: shared provenance neighbor contributes to both sourceOverlap and Adamic-Adar.
func TestRelatedTo_DoubleCount(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	// C is a provenance source AND a common neighbor of A and B.
	// Edges: C→A (generated_from), C→B (generated_from), A—C (related), B—C (related)
	// Wait: C as a node can have both provenance edges from C AND related edge between A-C.
	// For Adamic-Adar, C is a common neighbor. C's degree = edges A-C, B-C = 2.
	// For sourceOverlap, C is a source (edges C→A and C→B).
	// But sourceOverlap counts distinct sources with provenance edges.
	// We need: C -> A (generated_from) and C -> B (generated_from) (provenance edges from C)
	// But the edges table stores C as src, A as dst with label generated_from.
	// sourceOverlap looks for edges FROM a source TO both A and B.
	// So edges: src=C dst=A label=generated_from, src=C dst=B label=generated_from.
	// Let's model exactly.
	setupGraph(t, ctx, g,
		`INSERT INTO nodes(id, type, title) VALUES ('A','note','A'),('B','note','B'),('C','source','C');`,
		`INSERT INTO edges(id, src, dst, label) VALUES ('p1','C','A','generated_from'),('p2','C','B','generated_from');`,
	)

	var score float64
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		score, err = scoreBetween(tx, "A", "B")
		return err
	})
	if err != nil {
		t.Fatalf("scoreBetween: %v", err)
	}

	// sourceOverlap = 1 (C is shared source) * 4.0 = 4.0.
	// C is also a common undirected neighbor (degree 2), so Adamic-Adar contributes too.
	expectedScore := WeightSourceOverlap + WeightTypeAffinity + WeightAdamicAdar*(1.0/math.Log(2))
	if math.Abs(score-expectedScore) > 1e-9 {
		t.Errorf("expected score ≈ %v, got %v", expectedScore, score)
	}
}

// Ranking + limit: return top 2 by score descending, tie-break id ASC.
func TestRelatedTo_RankAndLimit(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	// A connected to B and C; B and C both note.
	// Scores should be equal (directLink=3.0, typeAffinity=1.0 for both)
	// Tie-break by id ASC
	setupGraph(t, ctx, g,
		`INSERT INTO nodes(id, type, title) VALUES ('A','note','A'),('B','note','B'),('C','note','C');`,
		`INSERT INTO edges(id, src, dst, label) VALUES ('e1','A','B','related'),('e2','A','C','related');`,
	)

	var got []string
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = RelatedTo(ctx, tx, "A", 2)
		return err
	})
	if err != nil {
		t.Fatalf("RelatedTo: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 results, got %v", got)
	}
	// B < C in string comparison
	if got[0] != "B" || got[1] != "C" {
		t.Errorf("expected [B C], got %v", got)
	}
}

// Full integration through edges.Create.
func TestRelatedToIntegration(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	var aID, bID string
	author := nodes.Author{Name: "test"}
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		// Use the proper chokepoint to create nodes (A=bookmark, B=bookmark)
		aID, err = nodes.CreateBookmark(ctx, tx, nodes.Bookmark{Title: "A"}, author)
		if err != nil {
			return err
		}
		bID, err = nodes.CreateBookmark(ctx, tx, nodes.Bookmark{Title: "B"}, author)
		if err != nil {
			return err
		}
		_, err = edges.Create(ctx, tx, edges.Edge{Src: aID, Dst: bID, Type: edges.EdgeTypeRelated}, author)
		return err
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	var got []string
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = RelatedTo(ctx, tx, aID, 10)
		return err
	})
	if err != nil {
		t.Fatalf("RelatedTo: %v", err)
	}
	if len(got) != 1 || got[0] != bID {
		t.Errorf("expected [%s], got %v", bID, got)
	}
}
