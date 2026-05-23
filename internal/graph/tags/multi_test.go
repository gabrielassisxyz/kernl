package tags

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/internal/ids"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func TestNodesAllHappyPath(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)
	author := Author{Name: "test"}

	insertNode := func(nodeID string) {
		_ = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			_, err := tx.Exec(`INSERT INTO nodes(id, type, title) VALUES (?, 'test', ?)`, nodeID, nodeID)
			return err
		})
	}

	nodeAB := ids.New()
	nodeA := ids.New()
	nodeB := ids.New()
	insertNode(nodeAB)
	insertNode(nodeA)
	insertNode(nodeB)

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if err := Add(ctx, tx, nodeAB, "a", author); err != nil {
			return err
		}
		if err := Add(ctx, tx, nodeAB, "b", author); err != nil {
			return err
		}
		if err := Add(ctx, tx, nodeA, "a", author); err != nil {
			return err
		}
		if err := Add(ctx, tx, nodeB, "b", author); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	var got []string
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = NodesAll(ctx, tx, "a", "b")
		return err
	})
	if err != nil {
		t.Fatalf("NodesAll: %v", err)
	}
	if len(got) != 1 || got[0] != nodeAB {
		t.Errorf("expected [%s], got %v", nodeAB, got)
	}
}

func TestNodesAnyHappyPath(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)
	author := Author{Name: "test"}

	insertNode := func(nodeID string) {
		_ = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			_, err := tx.Exec(`INSERT INTO nodes(id, type, title) VALUES (?, 'test', ?)`, nodeID, nodeID)
			return err
		})
	}

	nodeAB := ids.New()
	nodeA := ids.New()
	nodeB := ids.New()
	insertNode(nodeAB)
	insertNode(nodeA)
	insertNode(nodeB)

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if err := Add(ctx, tx, nodeAB, "a", author); err != nil {
			return err
		}
		if err := Add(ctx, tx, nodeAB, "b", author); err != nil {
			return err
		}
		if err := Add(ctx, tx, nodeA, "a", author); err != nil {
			return err
		}
		if err := Add(ctx, tx, nodeB, "b", author); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	var got []string
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = NodesAny(ctx, tx, "a", "b")
		return err
	})
	if err != nil {
		t.Fatalf("NodesAny: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 results, got %d", len(got))
	}
	// Verify ordering and no duplicates
	if got[0] > got[1] || got[1] > got[2] {
		t.Errorf("expected sorted: %v", got)
	}
}

func TestNodesAnySingleTagParity(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)
	author := Author{Name: "test"}

	insertNode := func(nodeID string) {
		_ = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			_, err := tx.Exec(`INSERT INTO nodes(id, type, title) VALUES (?, 'test', ?)`, nodeID, nodeID)
			return err
		})
	}

	nodeA := ids.New()
	insertNode(nodeA)

	_ = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return Add(ctx, tx, nodeA, "shared", author)
	})

	var anyResult, allResult, singleResult []string
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		anyResult, err = NodesAny(ctx, tx, "shared")
		return err
	})
	if err != nil {
		t.Fatalf("NodesAny: %v", err)
	}
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		allResult, err = NodesAll(ctx, tx, "shared")
		return err
	})
	if err != nil {
		t.Fatalf("NodesAll: %v", err)
	}
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		singleResult, err = Nodes(ctx, tx, "shared")
		return err
	})
	if err != nil {
		t.Fatalf("Nodes: %v", err)
	}

	if len(anyResult) != 1 || anyResult[0] != nodeA {
		t.Errorf("NodesAny parity failed: %v", anyResult)
	}
	if len(allResult) != 1 || allResult[0] != nodeA {
		t.Errorf("NodesAll parity failed: %v", allResult)
	}
	if len(singleResult) != 1 || singleResult[0] != nodeA {
		t.Errorf("Nodes parity failed: %v", singleResult)
	}
}

func TestNodesAllNoMatch(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	var got []string
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = NodesAll(ctx, tx, "a", "missing")
		return err
	})
	if err != nil {
		t.Fatalf("NodesAll: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestNodesAllEmptyInput(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	var got []string
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = NodesAll(ctx, tx)
		return err
	})
	if err != nil {
		t.Fatalf("NodesAll empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestNodesAnyEmptyInput(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	var got []string
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = NodesAny(ctx, tx)
		return err
	})
	if err != nil {
		t.Fatalf("NodesAny empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}
