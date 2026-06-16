package tags

import (
	"context"
	"errors"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/internal/ids"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// insertNode inserts a minimal node row directly. Must be called inside a
// DoWrite so the transaction scope matches the helper functions.
func insertNode(t *testing.T, ctx context.Context, g *graph.Graph, nodeID string) {
	t.Helper()
	_ = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`INSERT OR IGNORE INTO nodes(id, type, title) VALUES (?, 'test', ?)`, nodeID, nodeID)
		if err != nil {
			t.Fatalf("insertNode(%q): %v", nodeID, err)
		}
		return nil
	})
}

func TestAddAndList(t *testing.T) {
	// bead kernl-ntm8
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	nodeID := ids.New()
	insertNode(t, ctx, g, nodeID)

	author := Author{Name: "test-agent"}

	// Add tags in non-sorted order to verify List orders them ASC.
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if err := Add(ctx, tx, nodeID, "zulu", author); err != nil {
			return err
		}
		if err := Add(ctx, tx, nodeID, "alpha", author); err != nil {
			return err
		}
		return Add(ctx, tx, nodeID, "mike", author)
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// List should return alphabetically sorted tags.
	var tags []string
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var listErr error
		tags, listErr = List(ctx, tx, nodeID)
		return listErr
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(tags) != 3 {
		t.Fatalf("expected 3 tags, got %d: %v", len(tags), tags)
	}
	if tags[0] != "alpha" || tags[1] != "mike" || tags[2] != "zulu" {
		t.Fatalf("expected [alpha mike zulu], got %v", tags)
	}
}

func TestNodesByTag(t *testing.T) {
	// bead kernl-5wid
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	nodeA := ids.New()
	nodeB := ids.New()
	nodeC := ids.New()
	insertNode(t, ctx, g, nodeA)
	insertNode(t, ctx, g, nodeB)
	insertNode(t, ctx, g, nodeC)

	author := Author{Name: "test-agent"}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if err := Add(ctx, tx, nodeA, "shared", author); err != nil {
			return err
		}
		if err := Add(ctx, tx, nodeB, "shared", author); err != nil {
			return err
		}
		return Add(ctx, tx, nodeC, "shared", author)
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	var nodeIDs []string
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var nodesErr error
		nodeIDs, nodesErr = Nodes(ctx, tx, "shared")
		return nodesErr
	})
	if err != nil {
		t.Fatalf("Nodes: %v", err)
	}

	if len(nodeIDs) != 3 {
		t.Fatalf("expected 3 node IDs, got %d: %v", len(nodeIDs), nodeIDs)
	}
	// Should be ordered by node_id ASC.
	if nodeIDs[0] >= nodeIDs[1] || nodeIDs[1] >= nodeIDs[2] {
		t.Fatalf("expected sorted node IDs, got %v", nodeIDs)
	}
}

func TestAddEmptyTagRejected(t *testing.T) {
	// bead kernl-z6ht
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	nodeID := ids.New()
	insertNode(t, ctx, g, nodeID)

	author := Author{Name: "test-agent"}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return Add(ctx, tx, nodeID, "", author)
	})
	if !errors.Is(err, graph.ErrEmptyTag) {
		t.Fatalf("expected graph.ErrEmptyTag, got %v", err)
	}
}

func TestRemove(t *testing.T) {
	// bead kernl-j5br
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	nodeID := ids.New()
	insertNode(t, ctx, g, nodeID)

	author := Author{Name: "test-agent"}

	// Add a tag.
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return Add(ctx, tx, nodeID, "ephemeral", author)
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Verify it is listed.
	var before []string
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var listErr error
		before, listErr = List(ctx, tx, nodeID)
		return listErr
	})
	if err != nil {
		t.Fatalf("List before remove: %v", err)
	}
	if len(before) != 1 || before[0] != "ephemeral" {
		t.Fatalf("expected [ephemeral], got %v", before)
	}

	// Remove the tag.
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return Remove(ctx, tx, nodeID, "ephemeral", author)
	})
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Verify list is empty.
	var after []string
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var listErr error
		after, listErr = List(ctx, tx, nodeID)
		return listErr
	})
	if err != nil {
		t.Fatalf("List after remove: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("expected empty list after remove, got %v", after)
	}

	// Verify the tag row was orphaned and deleted from tags table.
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		err := tx.QueryRow(`SELECT COUNT(*) FROM tags WHERE name = 'ephemeral'`).Scan(&count)
		if err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("orphaned tag row still exists in tags table: count=%d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify orphaned tag deleted: %v", err)
	}
}
