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
		IsFixup:     true,
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
	if got.IsFixup != b.IsFixup {
		t.Errorf("IsFixup = %v, want %v", got.IsFixup, b.IsFixup)
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

	// is_fixup was added deliberately, per this test's own doc comment: it
	// is fixed at creation and never updated, exactly like tracker_kind and
	// repository, so it does not reintroduce the drift this pin test exists
	// to catch (a mutable field such as "status" or "labels" would).
	wantKeys := map[string]bool{"tracker_kind": true, "repository": true, "is_fixup": true}
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

// TestCreateBeadReference_EmptyIDFailsRatherThanGenerating pins the
// constructor's own invariant, not just app.ensureBeadReferenceNode's: an
// empty ID must error, not silently fall back to createNode's usual
// generated-UUID behavior. app's caller checks this today with a more
// actionable message, but this is the check a future caller cannot bypass
// by calling CreateBeadReference directly.
func TestCreateBeadReference_EmptyIDFailsRatherThanGenerating(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := CreateBeadReference(ctx, tx, BeadReference{Title: "t", TrackerKind: "br", Repository: "/repo"}, Author{Name: "test"})
		return err
	})
	if err == nil {
		t.Fatal("expected an error: ID is empty")
	}

	var count int
	if readErr := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE type = 'bead_reference'`).Scan(&count)
	}); readErr != nil {
		t.Fatalf("counting nodes: %v", readErr)
	}
	if count != 0 {
		t.Errorf("bead_reference node count = %d, want 0 - an empty ID must never mint a generated one", count)
	}
}

// TestCreateBeadReference_MissingFieldsFailRatherThanStoringBlank is the
// constructor-level counterpart to
// app.TestEnsureBeadReferenceNode_MissingFactsFailsLoud: called directly,
// bypassing that caller's own check, a BeadReference missing Title,
// TrackerKind, or Repository must still error rather than write a node with
// a blank standing in for the missing fact.
func TestCreateBeadReference_MissingFieldsFailRatherThanStoringBlank(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := CreateBeadReference(ctx, tx, BeadReference{ID: "kb-blank-1"}, Author{Name: "test"})
		return err
	})
	if err == nil {
		t.Fatal("expected an error: Title, TrackerKind and Repository are all empty")
	}

	if readErr := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		_, err := GetBeadReference(ctx, tx, "kb-blank-1")
		return err
	}); readErr != graph.ErrNotFound {
		t.Errorf("GetBeadReference after a rejected create = %v, want graph.ErrNotFound", readErr)
	}
}

// TestCreateBeadReference_SecondCallForSameIDIsANoOp pins the idempotent
// insert itself (createNodeIfAbsent), independent of app's own
// ensureBeadReferenceNode wrapper: a second CreateBeadReference call for an
// id that already exists returns success and leaves exactly one row, even
// though the second call's data has nothing to do with what was already
// there - there is nothing on this node a later call could have a more
// current answer for (see CreateBeadReference's own doc comment).
func TestCreateBeadReference_SecondCallForSameIDIsANoOp(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	first := BeadReference{ID: "kb-idempotent-1", Title: "first title", TrackerKind: "br", Repository: "/repo"}
	second := BeadReference{ID: "kb-idempotent-1", Title: "second title", TrackerKind: "bd", Repository: "/other-repo"}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if _, err := CreateBeadReference(ctx, tx, first, Author{Name: "test"}); err != nil {
			return err
		}
		_, err := CreateBeadReference(ctx, tx, second, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateBeadReference: %v", err)
	}

	var count int
	if readErr := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id = ?`, first.ID).Scan(&count)
	}); readErr != nil {
		t.Fatalf("counting nodes: %v", readErr)
	}
	if count != 1 {
		t.Fatalf("node count for %s = %d, want 1", first.ID, count)
	}

	var got *BeadReference
	if readErr := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = GetBeadReference(ctx, tx, first.ID)
		return err
	}); readErr != nil {
		t.Fatalf("GetBeadReference: %v", readErr)
	}
	if got.Title != first.Title {
		t.Errorf("Title = %q, want the first call's %q - the second call must not have overwritten it", got.Title, first.Title)
	}
}
