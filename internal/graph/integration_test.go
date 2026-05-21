package graph_test

import (
	"context"
	"os"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/revisions"
	"github.com/gabrielassisxyz/kernl/internal/graph/search"
	"github.com/gabrielassisxyz/kernl/internal/graph/tags"
)

// TestSpec11_05_EdgeCreateAndCascade verifies edge creation between two captures
// and that deleting the source node cascades to remove the edge.
func TestSpec11_05_EdgeCreateAndCascade(t *testing.T) {
	g := openTempGraph(t)
	ctx := context.Background()

	author := nodes.Author{Name: "test"}

	// 1. Create two captures.
	var idA, idB string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		idA, err = nodes.CreateCapture(ctx, tx, nodes.Capture{Title: "A"}, author)
		if err != nil {
			return err
		}
		idB, err = nodes.CreateCapture(ctx, tx, nodes.Capture{Title: "B"}, author)
		return err
	})
	if err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}

	// 2. Create edge A → B.
	var edgeID string
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		edgeID, err = edges.Create(ctx, tx, edges.Edge{Src: idA, Dst: idB, Label: "relates_to"}, author)
		return err
	})
	if err != nil {
		t.Fatalf("edges.Create: %v", err)
	}
	if edgeID == "" {
		t.Fatal("expected non-empty edge ID")
	}

	// 3. Query outgoing from A: assert 1 edge, Dst == idB.
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		out, err := edges.Outgoing(ctx, tx, idA)
		if err != nil {
			return err
		}
		if len(out) != 1 {
			t.Errorf("Outgoing from A: got %d edges, want 1", len(out))
			return nil
		}
		if out[0].Dst != idB {
			t.Errorf("Outgoing edge Dst = %q, want %q", out[0].Dst, idB)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// 4. Delete capture A (cascade removes edge).
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.DeleteCapture(ctx, tx, idA, author)
	})
	if err != nil {
		t.Fatalf("DeleteCapture A: %v", err)
	}

	// 5. Query incoming to B: assert 0 edges (cascade).
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		in, err := edges.Incoming(ctx, tx, idB)
		if err != nil {
			return err
		}
		if len(in) != 0 {
			t.Errorf("Incoming to B after cascade: got %d edges, want 0", len(in))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// 6. Verify no panic on edge queries after deletion (implicit — reaching here passes).
}

func openTempGraph(t *testing.T) *graph.Graph {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "kernl-integration-*.db")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()
	g, err := graph.Open(context.Background(), graph.Config{Path: f.Name()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { g.Close() })
	return g
}

func TestSpec11_01_CreateGetBeadRoundtrip(t *testing.T) {
	g := openTempGraph(t)
	ctx := context.Background()

	var createdID string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		id, err := nodes.CreateCapture(ctx, tx, nodes.Capture{
			Title: "Roundtrip",
			Body:  "hello",
			Tags:  []string{"a", "b"},
		}, nodes.Author{Name: "test"})
		createdID = id
		return err
	})
	if err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}
	if createdID == "" {
		t.Fatal("expected non-empty ID from CreateCapture")
	}

	var got *nodes.Capture
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var fetchErr error
		got, fetchErr = nodes.GetCapture(ctx, tx, createdID)
		return fetchErr
	})
	if err != nil {
		t.Fatalf("GetCapture: %v", err)
	}

	if got.Title != "Roundtrip" {
		t.Errorf("Title = %q, want %q", got.Title, "Roundtrip")
	}
	if got.Body != "hello" {
		t.Errorf("Body = %q, want %q", got.Body, "hello")
	}
	if len(got.Tags) != 2 || got.Tags[0] != "a" || got.Tags[1] != "b" {
		t.Errorf("Tags = %v, want [a b]", got.Tags)
	}
}

func TestSpec11_02_UpdateBeadProducesOneRevision(t *testing.T) {
	g := openTempGraph(t)
	ctx := context.Background()
	author := nodes.Author{Name: "updater"}

	var nodeID string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		id, err := nodes.CreateCapture(ctx, tx, nodes.Capture{
			Title: "Before",
			Body:  "original",
		}, author)
		nodeID = id
		return err
	})
	if err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}
	if nodeID == "" {
		t.Fatal("expected non-empty ID from CreateCapture")
	}

	// Count revisions after create — expect exactly 1.
	var initialRevs []revisions.Revision
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var listErr error
		initialRevs, listErr = revisions.List(ctx, tx, nodeID)
		return listErr
	})
	if err != nil {
		t.Fatalf("revisions.List (initial): %v", err)
	}
	if len(initialRevs) != 1 {
		t.Fatalf("expected 1 revision after create, got %d", len(initialRevs))
	}

	// Update the capture.
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.UpdateCapture(ctx, tx, nodes.Capture{
			ID:    nodeID,
			Title: "After",
			Body:  "updated",
		}, author)
	})
	if err != nil {
		t.Fatalf("UpdateCapture: %v", err)
	}

	// Count revisions after update — expect exactly 2.
	var afterRevs []revisions.Revision
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var listErr error
		afterRevs, listErr = revisions.List(ctx, tx, nodeID)
		return listErr
	})
	if err != nil {
		t.Fatalf("revisions.List (after update): %v", err)
	}
	if len(afterRevs) != 2 {
		t.Fatalf("expected 2 revisions after update, got %d", len(afterRevs))
	}

	// Verify the latest (most recent) revision.
	// revisions.List orders by created_at DESC, so index 0 is newest.
	latest := afterRevs[0]
	if latest.Author != author.Name {
		t.Errorf("revision author = %q, want %q", latest.Author, author.Name)
	}
	if len(latest.Diff) == 0 {
		t.Error("expected non-empty Diff on latest revision")
	}
}

func TestSpec11_06_TagAddListNodesRemove(t *testing.T) {
	g := openTempGraph(t)
	ctx := context.Background()
	author := nodes.Author{Name: "test"}

	var nodeID string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		nodeID, err = nodes.CreateCapture(ctx, tx, nodes.Capture{
			Title: "tag-test",
			Body:  "body for tag test",
		}, author)
		return err
	})
	if err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}

	// Add tag "x"
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return tags.Add(ctx, tx, nodeID, "x", author)
	})
	if err != nil {
		t.Fatalf("tags.Add: %v", err)
	}

	// Verify tags.Nodes("x") returns nodeID
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		ids, err := tags.Nodes(ctx, tx, "x")
		if err != nil {
			return err
		}
		if len(ids) != 1 || ids[0] != nodeID {
			t.Errorf("tags.Nodes('x') = %v; want [%s]", ids, nodeID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tags.Nodes: %v", err)
	}

	// Verify tags.List(nodeID) contains "x"
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		got, err := tags.List(ctx, tx, nodeID)
		if err != nil {
			return err
		}
		found := false
		for _, tag := range got {
			if tag == "x" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tags.List(%s) = %v; want to contain 'x'", nodeID, got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tags.List: %v", err)
	}

	// Remove tag "x"
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return tags.Remove(ctx, tx, nodeID, "x", author)
	})
	if err != nil {
		t.Fatalf("tags.Remove: %v", err)
	}

	// Verify tags.Nodes("x") is empty
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		ids, err := tags.Nodes(ctx, tx, "x")
		if err != nil {
			return err
		}
		if len(ids) != 0 {
			t.Errorf("tags.Nodes('x') after remove = %v; want empty", ids)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tags.Nodes after remove: %v", err)
	}

	// Verify tags.List(nodeID) does NOT contain "x"
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		got, err := tags.List(ctx, tx, nodeID)
		if err != nil {
			return err
		}
		for _, tag := range got {
			if tag == "x" {
				t.Errorf("tags.List(%s) after remove = %v; want NOT to contain 'x'", nodeID, got)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tags.List after remove: %v", err)
	}
}

func TestSpec11_07_PersistenceAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/persist.db"

	const title = "Persist Test"
	const body = "unique-content-for-fts-test-xyz"

	// Create and populate graph
	g, err := graph.Open(ctx, graph.Config{Path: path})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	var nodeID string
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		nodeID, err = nodes.CreateCapture(ctx, tx, nodes.Capture{
			Title: title,
			Body:  body,
		}, nodes.Author{Name: "test"})
		return err
	})
	if err != nil {
		g.Close()
		t.Fatalf("CreateCapture: %v", err)
	}

	if err := g.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Re-open same file
	g, err = graph.Open(ctx, graph.Config{Path: path})
	if err != nil {
		t.Fatalf("Re-open: %v", err)
	}
	t.Cleanup(func() { g.Close() })

	// Verify capture data persists
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		got, err := nodes.GetCapture(ctx, tx, nodeID)
		if err != nil {
			return err
		}
		if got.ID != nodeID {
			t.Errorf("ID = %q, want %q", got.ID, nodeID)
		}
		if got.Title != title {
			t.Errorf("Title = %q, want %q", got.Title, title)
		}
		if got.Body != body {
			t.Errorf("Body = %q, want %q", got.Body, body)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("GetCapture after reopen: %v", err)
	}

	// Verify FTS state persisted
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		hits, err := search.Search(ctx, tx, body)
		if err != nil {
			return err
		}
		if len(hits) != 1 {
			t.Errorf("search hits = %d; want 1", len(hits))
		} else if hits[0].NodeID != nodeID {
			t.Errorf("search hit NodeID = %q; want %q", hits[0].NodeID, nodeID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Search after reopen: %v", err)
	}
}

func TestSpec11_03_NoteBodyFTSReturnsHit(t *testing.T) {
	g := openTempGraph(t)
	ctx := context.Background()
	author := nodes.Author{Name: "test"}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateCapture(ctx, tx, nodes.Capture{
			Title: "FTS Note",
			Body:  "The quick brown fox jumps.",
			Tags:  []string{"searchable"},
		}, author)
		return err
	})
	if err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		hits, err := search.Search(ctx, tx, "brown")
		if err != nil {
			return err
		}
		if len(hits) != 1 {
			t.Errorf("search hits = %d; want 1", len(hits))
		} else if hits[0].NodeID != id {
			t.Errorf("search hit NodeID = %q; want %q", hits[0].NodeID, id)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
}

func TestSpec11_04_NoteBodyFTSReplacedOnUpdate(t *testing.T) {
	g := openTempGraph(t)
	ctx := context.Background()
	author := nodes.Author{Name: "test"}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateCapture(ctx, tx, nodes.Capture{
			Title: "FTS Update Note",
			Body:  "old content here",
		}, author)
		return err
	})
	if err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}

	// Search for "old" — should find it
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		hits, err := search.Search(ctx, tx, "old")
		if err != nil {
			return err
		}
		if len(hits) != 1 {
			t.Errorf("search for 'old': hits = %d; want 1", len(hits))
		} else if hits[0].NodeID != id {
			t.Errorf("search for 'old': NodeID = %q; want %q", hits[0].NodeID, id)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Search for 'old': %v", err)
	}

	// Update capture body
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.UpdateCapture(ctx, tx, nodes.Capture{
			ID:    id,
			Title: "FTS Update Note",
			Body:  "new content here",
		}, author)
	})
	if err != nil {
		t.Fatalf("UpdateCapture: %v", err)
	}

	// Search for "old" — should yield 0 hits
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		hits, err := search.Search(ctx, tx, "old")
		if err != nil {
			return err
		}
		if len(hits) != 0 {
			t.Errorf("search for 'old' after update: hits = %d; want 0", len(hits))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Search for 'old' after update: %v", err)
	}

	// Search for "new" — should yield 1 hit with correct ID
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		hits, err := search.Search(ctx, tx, "new")
		if err != nil {
			return err
		}
		if len(hits) != 1 {
			t.Errorf("search for 'new': hits = %d; want 1", len(hits))
		} else if hits[0].NodeID != id {
			t.Errorf("search for 'new': NodeID = %q; want %q", hits[0].NodeID, id)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Search for 'new': %v", err)
	}
}

// TestSpec11_08_PropertyTestPasses is a thin wrapper proving that the existing
// rapid property test (TestSubstrateProperties in property_test.go) already
// exercises the invariants required by spec §11 #8.  The underlying test runs
// random sequences of Create/Update/Delete/Tag/Edge operations and verifies:
//
//   1. Every mutation produces exactly one revision row.
//   2. FTS5 reflects current state (no stale entries after Update).
//   3. ON DELETE CASCADE removes edges/tags/fts_map/revisions.
//   4. UUIDv7s are monotonically increasing.
//
// Re-exporting it here makes the spec↔test link mechanically auditable via grep.
func TestSpec11_08_PropertyTestPasses(t *testing.T) {
	TestSubstrateProperties(t)
}
