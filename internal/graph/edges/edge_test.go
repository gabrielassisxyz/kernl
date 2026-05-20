package edges_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// insertTestNode creates a minimal node row directly in the nodes table.
// Must be called inside a DoWrite closure.
func insertTestNode(tx *graph.WriteTx, id, nodeType, title string) error {
	_, err := tx.Exec(
		`INSERT INTO nodes(id, type, title, attrs) VALUES (?, ?, ?, ?)`,
		id, nodeType, title, `{}`,
	)
	return err
}

// mustCreateNode is a test helper that calls insertTestNode via DoWrite.
func mustCreateNode(t *testing.T, g *graph.Graph, id, nodeType, title string) {
	t.Helper()
	err := g.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		return insertTestNode(tx, id, nodeType, title)
	})
	if err != nil {
		t.Fatalf("mustCreateNode(%q): %v", id, err)
	}
}

// mustCreateEdge is a test helper that calls edges.Create via DoWrite.
func mustCreateEdge(t *testing.T, g *graph.Graph, e edges.Edge) string {
	t.Helper()
	var id string
	err := g.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		var cerr error
		id, cerr = edges.Create(context.Background(), tx, e, nodes.Author{Name: "tester"})
		return cerr
	})
	if err != nil {
		t.Fatalf("mustCreateEdge(%+v): %v", e, err)
	}
	return id
}

// authorTester is a reusable valid Author.
var authorTester = nodes.Author{Name: "tester"}

// ---------------------------------------------------------------------------
// TestCreateAndOutgoing — bead kernl-xbz0
// ---------------------------------------------------------------------------

func TestCreateAndOutgoing(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	// Fixture: two nodes
	mustCreateNode(t, g, "n-a", "test", "node-a")
	mustCreateNode(t, g, "n-b", "test", "node-b")

	// Create two edges from n-a → n-b
	e1 := edges.Edge{ID: "edge-1", Src: "n-a", Dst: "n-b", Type: edges.EdgeTypeRelated, Attrs: json.RawMessage(`{"weight":1}`)}
	e2 := edges.Edge{ID: "edge-2", Src: "n-a", Dst: "n-b", Type: edges.EdgeTypeDependsOn}

	id1 := mustCreateEdge(t, g, e1)
	if id1 != "edge-1" {
		t.Errorf("expected edge-1, got %q", id1)
	}
	id2 := mustCreateEdge(t, g, e2)
	if id2 != "edge-2" {
		t.Errorf("expected edge-2, got %q", id2)
	}

	// Read outgoing from n-a
	var outgoing []edges.Edge
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var rerr error
		outgoing, rerr = edges.Outgoing(ctx, tx, "n-a")
		return rerr
	})
	if err != nil {
		t.Fatalf("Outgoing: %v", err)
	}

	if len(outgoing) != 2 {
		t.Fatalf("expected 2 outgoing edges, got %d", len(outgoing))
	}

	if outgoing[0].ID != "edge-1" {
		t.Errorf("expected edge-1 first, got %s", outgoing[0].ID)
	}
	if outgoing[1].ID != "edge-2" {
		t.Errorf("expected edge-2 second, got %s", outgoing[1].ID)
	}
}

// ---------------------------------------------------------------------------
// TestOutgoingFiltersByType — bead kernl-wyoz
// ---------------------------------------------------------------------------

func TestOutgoingFiltersByType(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	mustCreateNode(t, g, "n-src", "test", "src")
	mustCreateNode(t, g, "n-dst", "test", "dst")

	mustCreateEdge(t, g, edges.Edge{ID: "e-rel", Src: "n-src", Dst: "n-dst", Type: edges.EdgeTypeRelated})
	mustCreateEdge(t, g, edges.Edge{ID: "e-dep", Src: "n-src", Dst: "n-dst", Type: edges.EdgeTypeDependsOn})
	mustCreateEdge(t, g, edges.Edge{ID: "e-blk", Src: "n-src", Dst: "n-dst", Type: edges.EdgeTypeBlocks})

	// Filter only DependsOn
	var filtered []edges.Edge
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var rerr error
		filtered, rerr = edges.Outgoing(ctx, tx, "n-src", edges.WithType(edges.EdgeTypeDependsOn))
		return rerr
	})
	if err != nil {
		t.Fatalf("Outgoing with type filter: %v", err)
	}

	if len(filtered) != 1 {
		t.Fatalf("expected 1 edge with type depends_on, got %d", len(filtered))
	}
	if filtered[0].ID != "e-dep" {
		t.Errorf("expected e-dep, got %s", filtered[0].ID)
	}
	if filtered[0].Type != edges.EdgeTypeDependsOn {
		t.Errorf("expected depends_on type, got %s", filtered[0].Type)
	}

	// Filter multiple types
	var multi []edges.Edge
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var rerr error
		multi, rerr = edges.Incoming(ctx, tx, "n-dst",
			edges.WithType(edges.EdgeTypeRelated),
			edges.WithType(edges.EdgeTypeBlocks),
		)
		return rerr
	})
	if err != nil {
		t.Fatalf("Incoming with multi-type filter: %v", err)
	}
	if len(multi) != 2 {
		t.Fatalf("expected 2 edges (related + blocks), got %d", len(multi))
	}
}

// ---------------------------------------------------------------------------
// TestEdgeToNonexistentDestFails — bead kernl-2kos
// ---------------------------------------------------------------------------

func TestEdgeToNonexistentDestFails(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	mustCreateNode(t, g, "n-x", "test", "node-x")

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, cerr := edges.Create(ctx, tx,
			edges.Edge{Src: "n-x", Dst: "nonexistent", Type: edges.EdgeTypeRelated},
			authorTester,
		)
		return cerr
	})
	if err == nil {
		t.Fatal("expected error when dst node does not exist")
	}

	// Also test nonexistent src
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, cerr := edges.Create(ctx, tx,
			edges.Edge{Src: "nonexistent", Dst: "n-x", Type: edges.EdgeTypeRelated},
			authorTester,
		)
		return cerr
	})
	if err == nil {
		t.Fatal("expected error when src node does not exist")
	}
}

// ---------------------------------------------------------------------------
// TestSelfEdgeAllowed — bead kernl-1ddt
// ---------------------------------------------------------------------------

func TestSelfEdgeAllowed(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	mustCreateNode(t, g, "self-node", "test", "self")

	e := edges.Edge{ID: "self-edge", Src: "self-node", Dst: "self-node", Type: edges.EdgeTypeRelated}

	id := mustCreateEdge(t, g, e)
	if id != "self-edge" {
		t.Errorf("expected self-edge, got %q", id)
	}

	// Verify it appears in both outgoing and incoming
	var out, in []edges.Edge
	g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var rerr error
		out, rerr = edges.Outgoing(ctx, tx, "self-node")
		if rerr != nil {
			return rerr
		}
		in, rerr = edges.Incoming(ctx, tx, "self-node")
		return rerr
	})
	if len(out) != 1 || out[0].ID != "self-edge" {
		t.Errorf("self-edge not in outgoing: got %v", out)
	}
	if len(in) != 1 || in[0].ID != "self-edge" {
		t.Errorf("self-edge not in incoming: got %v", in)
	}
}

// ---------------------------------------------------------------------------
// TestDeleteSourceCascadesEdge — bead kernl-12q7
// ---------------------------------------------------------------------------

func TestDeleteSourceCascadesEdge(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	mustCreateNode(t, g, "src-node", "test", "src")
	mustCreateNode(t, g, "dst-node", "test", "dst")

	mustCreateEdge(t, g, edges.Edge{ID: "edge-csc-src", Src: "src-node", Dst: "dst-node", Type: edges.EdgeTypeDependsOn})

	// Delete the source node — edge should cascade-delete
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`DELETE FROM nodes WHERE id = ?`, "src-node")
		return err
	})
	if err != nil {
		t.Fatalf("delete src node: %v", err)
	}

	// Edge should no longer exist
	var count int
	g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM edges WHERE id = ?`, "edge-csc-src").Scan(&count)
	})
	if count != 0 {
		t.Errorf("expected edge to be cascading-deleted, but count is %d", count)
	}
}

// ---------------------------------------------------------------------------
// TestDeleteDestCascadesEdge — bead kernl-bgpr
// ---------------------------------------------------------------------------

func TestDeleteDestCascadesEdge(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	mustCreateNode(t, g, "src-node2", "test", "src")
	mustCreateNode(t, g, "dst-node2", "test", "dst")

	mustCreateEdge(t, g, edges.Edge{ID: "edge-csc-dst", Src: "src-node2", Dst: "dst-node2", Type: edges.EdgeTypeDependsOn})

	// Delete the destination node — edge should cascade-delete
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`DELETE FROM nodes WHERE id = ?`, "dst-node2")
		return err
	})
	if err != nil {
		t.Fatalf("delete dst node: %v", err)
	}

	// Edge should no longer exist
	var count int
	g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM edges WHERE id = ?`, "edge-csc-dst").Scan(&count)
	})
	if count != 0 {
		t.Errorf("expected edge to be cascading-deleted, but count is %d", count)
	}
}

// ---------------------------------------------------------------------------
// TestCascadeOnNodeDelete — bead kernl-wpth
// ---------------------------------------------------------------------------

func TestCascadeOnNodeDelete(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	// Create three nodes: A (middle), B and C (edges from B→A, A→C)
	mustCreateNode(t, g, "node-a", "test", "node-a")
	mustCreateNode(t, g, "node-b", "test", "node-b")
	mustCreateNode(t, g, "node-c", "test", "node-c")

	mustCreateEdge(t, g, edges.Edge{ID: "edge-ba", Src: "node-b", Dst: "node-a", Type: edges.EdgeTypeRelated})
	mustCreateEdge(t, g, edges.Edge{ID: "edge-ac", Src: "node-a", Dst: "node-c", Type: edges.EdgeTypeRelated})
	mustCreateEdge(t, g, edges.Edge{ID: "edge-bc-not-via-a", Src: "node-b", Dst: "node-c", Type: edges.EdgeTypeDependsOn})

	// Delete the middle node A — edges from B→A and A→C should cascade, B→C should survive
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`DELETE FROM nodes WHERE id = ?`, "node-a")
		return err
	})
	if err != nil {
		t.Fatalf("delete node-a: %v", err)
	}

	// Verify B→A edge is gone
	var count int
	g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM edges WHERE id = ?`, "edge-ba").Scan(&count)
	})
	if count != 0 {
		t.Errorf("edge-ba should be cascade-deleted, got count %d", count)
	}

	// Verify A→C edge is gone
	g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM edges WHERE id = ?`, "edge-ac").Scan(&count)
	})
	if count != 0 {
		t.Errorf("edge-ac should be cascade-deleted, got count %d", count)
	}

	// Verify B→C edge SURVIVES
	var bcCount int
	g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM edges WHERE id = ?`, "edge-bc-not-via-a").Scan(&bcCount)
	})
	if bcCount != 1 {
		t.Errorf("edge-bc-not-via-a should survive, got count %d", bcCount)
	}
}
