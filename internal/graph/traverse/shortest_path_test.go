package traverse

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func TestShortestPathDirectEdge(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`
			INSERT INTO nodes(id, type, title) VALUES ('A','test','A'),('B','test','B');
			INSERT INTO edges(id, src, dst, label) VALUES ('e1','A','B','related');
		`)
		return err
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		p, err := ShortestPath(ctx, tx, "A", "B")
		if err != nil {
			return err
		}
		if !p.Found {
			t.Error("expected path found")
		}
		if p.Length != 1 {
			t.Errorf("expected length 1, got %d", p.Length)
		}
		if len(p.Nodes) != 2 || p.Nodes[0] != "A" || p.Nodes[1] != "B" {
			t.Errorf("expected [A B], got %v", p.Nodes)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ShortestPath: %v", err)
	}
}

func TestShortestPathMultiHop(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`
			INSERT INTO nodes(id, type, title) VALUES ('A','test','A'),('B','test','B'),('C','test','C');
			INSERT INTO edges(id, src, dst, label) VALUES ('e1','A','B','related'),('e2','B','C','related');
		`)
		return err
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		p, err := ShortestPath(ctx, tx, "A", "C")
		if err != nil {
			return err
		}
		if !p.Found || p.Length != 2 {
			t.Errorf("expected length 2, got %v", p)
		}
		if len(p.Nodes) != 3 || p.Nodes[0] != "A" || p.Nodes[1] != "B" || p.Nodes[2] != "C" {
			t.Errorf("expected [A B C], got %v", p.Nodes)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ShortestPath: %v", err)
	}
}

func TestShortestPathPicksShortest(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	// A—B—D (len 2), A—C—D (len 2), A—X—Y—D (len 3)
	// Shortest should be length 2.
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`
			INSERT INTO nodes(id, type, title) VALUES ('A','test','A'),('B','test','B'),('C','test','C'),('D','test','D'),('X','test','X'),('Y','test','Y');
			INSERT INTO edges(id, src, dst, label) VALUES
				('e1','A','B','related'),('e2','B','D','related'),
				('e3','A','C','related'),('e4','C','D','related'),
				('e5','A','X','related'),('e6','X','Y','related'),('e7','Y','D','related');
		`)
		return err
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		p, err := ShortestPath(ctx, tx, "A", "D")
		if err != nil {
			return err
		}
		if !p.Found || p.Length != 2 {
			t.Errorf("expected length 2, got %v", p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ShortestPath: %v", err)
	}
}

func TestShortestPathNoPath(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`
			INSERT INTO nodes(id, type, title) VALUES ('A','test','A'),('B','test','B'),('C','test','C');
			INSERT INTO edges(id, src, dst, label) VALUES ('e1','A','B','related');
		`)
		return err
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		p, err := ShortestPath(ctx, tx, "A", "C")
		if err != nil {
			return err
		}
		if p.Found {
			t.Errorf("expected no path, got %v", p)
		}
		if p.Nodes != nil {
			t.Errorf("expected nil/nil Nodes for no path, got %v", p.Nodes)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ShortestPath: %v", err)
	}
}

func TestShortestPathLabelFilter(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`
			INSERT INTO nodes(id, type, title) VALUES ('A','test','A'),('B','test','B'),('C','test','C');
			INSERT INTO edges(id, src, dst, label) VALUES ('e1','A','B','related'),('e2','A','C','blocks');
		`)
		return err
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Filter to "related" only: A-B exists, but no path A→C via related.
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		p, err := ShortestPath(ctx, tx, "A", "C", "related")
		if err != nil {
			return err
		}
		if p.Found {
			t.Errorf("expected no path with label filter, got %v", p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ShortestPath: %v", err)
	}
}

func TestShortestPathSelf(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`INSERT INTO nodes(id, type, title) VALUES ('A','test','A');`)
		return err
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		p, err := ShortestPath(ctx, tx, "A", "A")
		if err != nil {
			return err
		}
		if !p.Found || p.Length != 0 || len(p.Nodes) != 1 || p.Nodes[0] != "A" {
			t.Errorf("expected trivial self-path, got %v", p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ShortestPath: %v", err)
	}
}

func TestShortestPathUndirected(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`
			INSERT INTO nodes(id, type, title) VALUES ('A','test','A'),('B','test','B');
			INSERT INTO edges(id, src, dst, label) VALUES ('e1','B','A','related');
		`)
		return err
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		p, err := ShortestPath(ctx, tx, "A", "B")
		if err != nil {
			return err
		}
		if !p.Found || p.Length != 1 {
			t.Errorf("expected length 1 for reverse edge, got %v", p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ShortestPath: %v", err)
	}
}
