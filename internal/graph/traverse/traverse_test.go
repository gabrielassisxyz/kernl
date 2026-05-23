package traverse

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func TestNeighborsAtDepthChain(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	// Chain: A — B — C — D
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`
			INSERT INTO nodes(id, type, title) VALUES ('A','test','A'),('B','test','B'),('C','test','C'),('D','test','D');
			INSERT INTO edges(id, src, dst, label) VALUES ('e1','A','B','related'),('e2','B','C','related'),('e3','C','D','related');
		`)
		return err
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	var got []string
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = NeighborsAtDepth(ctx, tx, "A", 2)
		return err
	})
	if err != nil {
		t.Fatalf("NeighborsAtDepth: %v", err)
	}
	if len(got) != 2 || got[0] != "B" || got[1] != "C" {
		t.Errorf("expected [B C], got %v", got)
	}
}

func TestNeighborsAtDepthDepth1(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`
			INSERT INTO nodes(id, type, title) VALUES ('A','test','A'),('B','test','B'),('C','test','C'),('D','test','D');
			INSERT INTO edges(id, src, dst, label) VALUES ('e1','A','B','related'),('e2','B','C','related'),('e3','C','D','related');
		`)
		return err
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		got, err := NeighborsAtDepth(ctx, tx, "A", 1)
		if err != nil {
			return err
		}
		if len(got) != 1 || got[0] != "B" {
			t.Errorf("expected [B], got %v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("NeighborsAtDepth: %v", err)
	}
}

func TestNeighborsAtDepthUndirected(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	// Edge stored B→A only, traverse from A should reach B
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
		got, err := NeighborsAtDepth(ctx, tx, "A", 1)
		if err != nil {
			return err
		}
		if len(got) != 1 || got[0] != "B" {
			t.Errorf("expected [B], got %v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("NeighborsAtDepth: %v", err)
	}
}

func TestNeighborsAtDepthLabelFilter(t *testing.T) {
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

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		got, err := NeighborsAtDepth(ctx, tx, "A", 1, "related")
		if err != nil {
			return err
		}
		if len(got) != 1 || got[0] != "B" {
			t.Errorf("expected [B], got %v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("NeighborsAtDepth: %v", err)
	}
}

func TestNeighborsAtDepthCycle(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	// Triangle A—B—C—A
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`
			INSERT INTO nodes(id, type, title) VALUES ('A','test','A'),('B','test','B'),('C','test','C');
			INSERT INTO edges(id, src, dst, label) VALUES ('e1','A','B','related'),('e2','B','C','related'),('e3','C','A','related');
		`)
		return err
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		got, err := NeighborsAtDepth(ctx, tx, "A", 3)
		if err != nil {
			return err
		}
		if len(got) != 2 || got[0] != "B" || got[1] != "C" {
			t.Errorf("expected [B C], got %v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("NeighborsAtDepth: %v", err)
	}
}

func TestNeighborsAtDepthGuard(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`INSERT INTO nodes(id, type, title) VALUES ('A','test','A'),('B','test','B');
		INSERT INTO edges(id, src, dst, label) VALUES ('e1','A','B','related');`)
		return err
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		_, err := NeighborsAtDepth(ctx, tx, "A", MaxDepth+1)
		if !IsDepthExceeded(err) {
			t.Errorf("expected ErrDepthExceeded, got %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("NeighborsAtDepth: %v", err)
	}
}

func TestNeighborsAtDepthZeroOrNegative(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`INSERT INTO nodes(id, type, title) VALUES ('A','test','A'),('B','test','B');
		INSERT INTO edges(id, src, dst, label) VALUES ('e1','A','B','related');`)
		return err
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		got, err := NeighborsAtDepth(ctx, tx, "A", 0)
		if err != nil {
			return err
		}
		if len(got) != 0 {
			t.Errorf("expected empty for depth 0, got %v", got)
		}
		got, err = NeighborsAtDepth(ctx, tx, "A", -1)
		if err != nil {
			return err
		}
		if len(got) != 0 {
			t.Errorf("expected empty for depth -1, got %v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("NeighborsAtDepth: %v", err)
	}
}

func TestNeighborsAtDepthIsolated(t *testing.T) {
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
		got, err := NeighborsAtDepth(ctx, tx, "A", 3)
		if err != nil {
			return err
		}
		if len(got) != 0 {
			t.Errorf("expected empty for isolated node, got %v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("NeighborsAtDepth: %v", err)
	}
}

// IsDepthExceeded is a defensive helper matching the exact error.
func IsDepthExceeded(err error) bool {
	return err != nil && err == graph.ErrDepthExceeded
}
