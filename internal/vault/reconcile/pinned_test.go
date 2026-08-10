package reconcile_test

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

func readPinned(t *testing.T, g *graph.Graph, uuid string) bool {
	t.Helper()
	var pinned int
	err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT pinned FROM note_paths WHERE uuid = ?`, uuid).Scan(&pinned)
	})
	if err != nil {
		t.Fatalf("read pinned for %q: %v", uuid, err)
	}
	return pinned != 0
}

func setPinned(t *testing.T, g *graph.Graph, uuid string, pinned bool) error {
	t.Helper()
	return g.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		return reconcile.SetPinned(context.Background(), tx, uuid, pinned)
	})
}

func TestSetPinnedRoundTrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	const uuid = "note-uuid-pin"
	if err := reconcile.Upsert(ctx, g, uuid, "notes/pinned.md", "hash-1"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if readPinned(t, g, uuid) {
		t.Fatal("a freshly indexed note should not be pinned")
	}
	if err := setPinned(t, g, uuid, true); err != nil {
		t.Fatalf("SetPinned(true): %v", err)
	}
	if !readPinned(t, g, uuid) {
		t.Error("note should be pinned after SetPinned(true)")
	}
	if err := setPinned(t, g, uuid, false); err != nil {
		t.Fatalf("SetPinned(false): %v", err)
	}
	if readPinned(t, g, uuid) {
		t.Error("note should be unpinned after SetPinned(false)")
	}
}

// TestPinSurvivesReindex is the regression guard for why the pin lives on
// note_paths at all. The obvious home was nodes.attrs, where project pinning
// lives - but a note's attrs are rebuilt from the file on every reconcile, so
// a pin kept there would disappear the next time the note was edited, silently
// and with no error to notice. Editing a note re-upserts its note_paths row
// with a new content hash, and this asserts the pin comes through that.
func TestPinSurvivesReindex(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	const uuid = "note-uuid-reindex"
	if err := reconcile.Upsert(ctx, g, uuid, "notes/edited.md", "hash-before"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := setPinned(t, g, uuid, true); err != nil {
		t.Fatalf("SetPinned: %v", err)
	}

	// The note is edited: same uuid, same path, new content hash.
	if err := reconcile.Upsert(ctx, g, uuid, "notes/edited.md", "hash-after"); err != nil {
		t.Fatalf("Upsert after edit: %v", err)
	}
	if !readPinned(t, g, uuid) {
		t.Error("pin was lost when the note was re-indexed")
	}

	// And through a move, which rewrites the path column.
	if err := reconcile.Upsert(ctx, g, uuid, "archive/edited.md", "hash-after"); err != nil {
		t.Fatalf("Upsert after move: %v", err)
	}
	if !readPinned(t, g, uuid) {
		t.Error("pin was lost when the note was moved")
	}
}

func TestSetPinnedUnknownNoteIsNotFound(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)

	err := setPinned(t, g, "no-such-note", true)
	if err != graph.ErrNotFound {
		t.Errorf("SetPinned on an unindexed note = %v, want graph.ErrNotFound", err)
	}
}

// TestForgetDropsThePin: a note deleted from disk loses its note_paths row, and
// the pin has to go with it rather than be inherited by whatever next claims
// the uuid.
func TestForgetDropsThePin(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	const uuid = "note-uuid-forget"
	const path = "notes/doomed.md"
	if err := reconcile.Upsert(ctx, g, uuid, path, "hash-1"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := setPinned(t, g, uuid, true); err != nil {
		t.Fatalf("SetPinned: %v", err)
	}
	if err := reconcile.Forget(ctx, g, path); err != nil {
		t.Fatalf("Forget: %v", err)
	}

	if err := reconcile.Upsert(ctx, g, uuid, path, "hash-1"); err != nil {
		t.Fatalf("re-Upsert: %v", err)
	}
	if readPinned(t, g, uuid) {
		t.Error("a re-indexed note inherited the pin of the deleted one")
	}
}
