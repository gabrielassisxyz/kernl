package nodes

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// TestBeadReferenceRoundtrip verifies CreateBeadReference -> GetBeadReference
// returns identical fields, and that the ID is exactly the one supplied -
// never a generated one, since the whole point of this node is to exist at
// the bead's own tracker id.
func TestBeadReferenceRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	b := BeadReference{
		ID:          "kb-1",
		Title:       "Add CLI parity for epic run",
		TrackerKind: "br",
		Repository:  "/home/user/repositories/kernl",
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateBeadReference(ctx, tx, b, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateBeadReference: %v", err)
	}
	if id != "kb-1" {
		t.Fatalf("id = %q, want the supplied bead id %q, not a generated one", id, "kb-1")
	}

	var got *BeadReference
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = GetBeadReference(ctx, tx, id)
		return err
	})
	if err != nil {
		t.Fatalf("GetBeadReference: %v", err)
	}

	if got.ID != b.ID {
		t.Errorf("ID = %q, want %q", got.ID, b.ID)
	}
	if got.Title != b.Title {
		t.Errorf("Title = %q, want %q", got.Title, b.Title)
	}
	if got.TrackerKind != b.TrackerKind {
		t.Errorf("TrackerKind = %q, want %q", got.TrackerKind, b.TrackerKind)
	}
	if got.Repository != b.Repository {
		t.Errorf("Repository = %q, want %q", got.Repository, b.Repository)
	}
}

// TestBeadReferenceStoredAttrsAreExactlyTrackerKindAndRepository pins the
// stored attribute set (criterion 5 of the bead this type was built for): a
// reference node exists to dissolve the synchronization objection to
// mirroring bead state into the graph, and it only dissolves that objection
// because nothing mutable is stored. If someone later adds "status" or
// "labels" to NodeAttrs, this test fails and forces them to justify
// reintroducing exactly the drift this node type was designed to avoid.
func TestBeadReferenceStoredAttrsAreExactlyTrackerKindAndRepository(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	b := BeadReference{ID: "kb-attrs-1", Title: "t", TrackerKind: "bd", Repository: "/repo"}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := CreateBeadReference(ctx, tx, b, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateBeadReference: %v", err)
	}

	var attrsRaw string
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT attrs FROM nodes WHERE id = ?`, b.ID).Scan(&attrsRaw)
	})
	if err != nil {
		t.Fatalf("reading attrs: %v", err)
	}

	var attrs map[string]any
	if err := json.Unmarshal([]byte(attrsRaw), &attrs); err != nil {
		t.Fatalf("unmarshal attrs: %v", err)
	}

	wantKeys := map[string]bool{"tracker_kind": true, "repository": true}
	if len(attrs) != len(wantKeys) {
		t.Fatalf("stored attrs = %v, want exactly keys %v", attrs, wantKeys)
	}
	for k := range attrs {
		if !wantKeys[k] {
			t.Errorf("unexpected stored attribute %q - a bead reference node must carry nothing beyond tracker_kind and repository (title lives in the nodes.title column)", k)
		}
	}
}

// TestGetBeadReferenceMissingReturnsNotFound mirrors the Get* contract every
// other node type in this package follows.
func TestGetBeadReferenceMissingReturnsNotFound(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		_, err := GetBeadReference(ctx, tx, "does-not-exist")
		return err
	})
	if err != graph.ErrNotFound {
		t.Errorf("err = %v, want graph.ErrNotFound", err)
	}
}
