package tags

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/internal/ids"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func TestSetPinnedAndPinnedNames(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)
	author := Author{Name: "test-agent"}

	nodeID := ids.New()
	insertNode(t, ctx, g, nodeID)
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		for _, name := range []string{"kernl", "learning", "sys/audit"} {
			if err := Add(ctx, tx, nodeID, name, author); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	var pinned []string
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		pinned, err = PinnedNames(ctx, tx)
		return err
	}); err != nil {
		t.Fatalf("PinnedNames: %v", err)
	}
	if len(pinned) != 0 {
		t.Fatalf("a fresh vault should have no pinned tags, got %v", pinned)
	}

	// A slashed name is the case a path parameter would have mangled, so it is
	// the one worth pinning in the test.
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if err := SetPinned(ctx, tx, "sys/audit", true, author); err != nil {
			return err
		}
		return SetPinned(ctx, tx, "kernl", true, author)
	})
	if err != nil {
		t.Fatalf("SetPinned: %v", err)
	}

	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		pinned, err = PinnedNames(ctx, tx)
		return err
	}); err != nil {
		t.Fatalf("PinnedNames: %v", err)
	}
	if len(pinned) != 2 || pinned[0] != "kernl" || pinned[1] != "sys/audit" {
		t.Errorf("PinnedNames = %v, want [kernl sys/audit] in name order", pinned)
	}

	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return SetPinned(ctx, tx, "kernl", false, author)
	}); err != nil {
		t.Fatalf("SetPinned(false): %v", err)
	}
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		pinned, err = PinnedNames(ctx, tx)
		return err
	}); err != nil {
		t.Fatalf("PinnedNames: %v", err)
	}
	if len(pinned) != 1 || pinned[0] != "sys/audit" {
		t.Errorf("PinnedNames after unpin = %v, want [sys/audit]", pinned)
	}
}

func TestSetPinnedUnknownTagIsNotFound(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return SetPinned(ctx, tx, "nothing-carries-this", true, Author{Name: "test-agent"})
	})
	if err != graph.ErrNotFound {
		t.Errorf("SetPinned on an unknown tag = %v, want graph.ErrNotFound", err)
	}
}

func TestSetPinnedRejectsEmptyNameAndAuthor(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return SetPinned(ctx, tx, "", true, Author{Name: "test-agent"})
	})
	if err != graph.ErrEmptyTag {
		t.Errorf("SetPinned with an empty name = %v, want graph.ErrEmptyTag", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return SetPinned(ctx, tx, "kernl", true, Author{})
	})
	if err != graph.ErrAuthorRequired {
		t.Errorf("SetPinned with no author = %v, want graph.ErrAuthorRequired", err)
	}
}

// TestPinDiesWithTheTag documents the consequence of Remove's orphan
// collection: untagging the last note holding a pinned tag deletes the tag row,
// and the pin goes with it rather than lying in wait for the name to come back.
func TestPinDiesWithTheTag(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)
	author := Author{Name: "test-agent"}

	nodeID := ids.New()
	insertNode(t, ctx, g, nodeID)
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if err := Add(ctx, tx, nodeID, "ephemeral", author); err != nil {
			return err
		}
		return SetPinned(ctx, tx, "ephemeral", true, author)
	}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return Remove(ctx, tx, nodeID, "ephemeral", author)
	}); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Re-adding the name creates a brand-new, unpinned tag.
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return Add(ctx, tx, nodeID, "ephemeral", author)
	}); err != nil {
		t.Fatalf("re-Add: %v", err)
	}

	var pinned []string
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		pinned, err = PinnedNames(ctx, tx)
		return err
	}); err != nil {
		t.Fatalf("PinnedNames: %v", err)
	}
	if len(pinned) != 0 {
		t.Errorf("a re-created tag inherited the old pin: %v", pinned)
	}
}
