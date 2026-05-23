package reconcile_test

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

// TestUpsertAndLookup verifies basic insert and both lookup directions.
func TestUpsertAndLookup(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	const uuid = "note-uuid-1"
	const path = "notes/hello.md"
	const hash = "abc123"

	if err := reconcile.Upsert(ctx, g, uuid, path, hash); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	gotUUID, found, err := reconcile.Lookup(ctx, g, path)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !found {
		t.Fatal("Lookup: expected found=true")
	}
	if gotUUID != uuid {
		t.Errorf("Lookup: uuid = %q, want %q", gotUUID, uuid)
	}

	gotPath, found, err := reconcile.LookupByUUID(ctx, g, uuid)
	if err != nil {
		t.Fatalf("LookupByUUID: %v", err)
	}
	if !found {
		t.Fatal("LookupByUUID: expected found=true")
	}
	if gotPath != path {
		t.Errorf("LookupByUUID: path = %q, want %q", gotPath, path)
	}
}

// TestAE2_MovePreservesIdentity encodes AE2 at the cache layer:
// changing the path for the same UUID must update the path mapping,
// leave the old path unresolvable, and NOT write any nodes/revisions.
func TestAE2_MovePreservesIdentity(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	const uuid = "note-uuid-ae2"
	const oldPath = "folder-a/note.md"
	const newPath = "folder-b/note.md"
	const hash = "hash-ae2"

	// Insert at old path.
	if err := reconcile.Upsert(ctx, g, uuid, oldPath, hash); err != nil {
		t.Fatalf("initial Upsert: %v", err)
	}

	// Simulate a move: same UUID, new path.
	if err := reconcile.Upsert(ctx, g, uuid, newPath, hash); err != nil {
		t.Fatalf("move Upsert: %v", err)
	}

	// New path must resolve to the UUID.
	gotUUID, found, err := reconcile.Lookup(ctx, g, newPath)
	if err != nil {
		t.Fatalf("Lookup new path: %v", err)
	}
	if !found || gotUUID != uuid {
		t.Errorf("Lookup new path: got (%q, %v), want (%q, true)", gotUUID, found, uuid)
	}

	// Old path must no longer resolve.
	_, foundOld, err := reconcile.Lookup(ctx, g, oldPath)
	if err != nil {
		t.Fatalf("Lookup old path: %v", err)
	}
	if foundOld {
		t.Error("old path should not resolve after move")
	}

	// UUID resolves to the new path.
	gotPath, found, err := reconcile.LookupByUUID(ctx, g, uuid)
	if err != nil {
		t.Fatalf("LookupByUUID: %v", err)
	}
	if !found || gotPath != newPath {
		t.Errorf("LookupByUUID: got (%q, %v), want (%q, true)", gotPath, found, newPath)
	}

	// Cache must NOT have written any nodes or revisions.
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var nodeCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM nodes`).Scan(&nodeCount); err != nil {
			return err
		}
		if nodeCount != 0 {
			t.Errorf("AE2: expected 0 nodes after cache move, got %d", nodeCount)
		}
		var revCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM revisions`).Scan(&revCount); err != nil {
			return err
		}
		if revCount != 0 {
			t.Errorf("AE2: expected 0 revisions after cache move, got %d", revCount)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("DoRead: %v", err)
	}
}

// TestRenameWithinFolder verifies stem change (same UUID, same dir) updates cache.
func TestRenameWithinFolder(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	const uuid = "note-rename"
	const oldPath = "folder/old-stem.md"
	const newPath = "folder/new-stem.md"

	if err := reconcile.Upsert(ctx, g, uuid, oldPath, "h1"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := reconcile.Upsert(ctx, g, uuid, newPath, "h1"); err != nil {
		t.Fatalf("rename Upsert: %v", err)
	}

	gotPath, found, err := reconcile.LookupByUUID(ctx, g, uuid)
	if err != nil {
		t.Fatalf("LookupByUUID: %v", err)
	}
	if !found || gotPath != newPath {
		t.Errorf("LookupByUUID after rename: got (%q, %v), want (%q, true)", gotPath, found, newPath)
	}

	_, foundOld, err := reconcile.Lookup(ctx, g, oldPath)
	if err != nil {
		t.Fatalf("Lookup old stem: %v", err)
	}
	if foundOld {
		t.Error("old stem path should not resolve after rename")
	}
}

// TestMoveAcrossFolders verifies a cross-folder move updates only the path.
func TestMoveAcrossFolders(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	const uuid = "note-cross-move"
	const oldPath = "alpha/note.md"
	const newPath = "beta/gamma/note.md"

	if err := reconcile.Upsert(ctx, g, uuid, oldPath, "h-cross"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := reconcile.Upsert(ctx, g, uuid, newPath, "h-cross"); err != nil {
		t.Fatalf("cross-folder Upsert: %v", err)
	}

	gotPath, found, err := reconcile.LookupByUUID(ctx, g, uuid)
	if err != nil {
		t.Fatalf("LookupByUUID: %v", err)
	}
	if !found || gotPath != newPath {
		t.Errorf("LookupByUUID: got (%q, %v), want (%q, true)", gotPath, found, newPath)
	}
}

// TestUpsertIdempotent verifies re-upserting identical rows leaves a single row.
func TestUpsertIdempotent(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	const uuid = "note-idem"
	const path = "idem/note.md"
	const hash = "idem-hash"

	for i := 0; i < 3; i++ {
		if err := reconcile.Upsert(ctx, g, uuid, path, hash); err != nil {
			t.Fatalf("Upsert #%d: %v", i, err)
		}
	}

	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM note_paths WHERE uuid = ?`, uuid).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("expected exactly 1 row, got %d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("DoRead: %v", err)
	}
}

// TestForgetRemovesOnlyTarget verifies Forget removes exactly the target path.
func TestForgetRemovesOnlyTarget(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	// Insert two independent notes.
	if err := reconcile.Upsert(ctx, g, "uuid-keep", "keep/note.md", "hk"); err != nil {
		t.Fatalf("Upsert keep: %v", err)
	}
	if err := reconcile.Upsert(ctx, g, "uuid-drop", "drop/note.md", "hd"); err != nil {
		t.Fatalf("Upsert drop: %v", err)
	}

	// Forget only the drop path.
	if err := reconcile.Forget(ctx, g, "drop/note.md"); err != nil {
		t.Fatalf("Forget: %v", err)
	}

	// drop path must be gone.
	_, foundDrop, err := reconcile.Lookup(ctx, g, "drop/note.md")
	if err != nil {
		t.Fatalf("Lookup drop: %v", err)
	}
	if foundDrop {
		t.Error("drop/note.md should be gone after Forget")
	}

	// keep path must still be present.
	gotUUID, foundKeep, err := reconcile.Lookup(ctx, g, "keep/note.md")
	if err != nil {
		t.Fatalf("Lookup keep: %v", err)
	}
	if !foundKeep || gotUUID != "uuid-keep" {
		t.Errorf("keep/note.md should still exist, got (%q, %v)", gotUUID, foundKeep)
	}
}

// TestForgetNoopOnMissing verifies Forget on a non-existent path is a no-op.
func TestForgetNoopOnMissing(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	if err := reconcile.Forget(ctx, g, "never/existed.md"); err != nil {
		t.Fatalf("Forget non-existent: %v", err)
	}
}

// TestLookupNotFound verifies Lookup returns found=false for an absent path.
func TestLookupNotFound(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	_, found, err := reconcile.Lookup(ctx, g, "missing.md")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if found {
		t.Error("expected found=false for missing path")
	}
}

// TestFindByContentHash verifies the hash tiebreak helper returns the UUID when
// a matching row exists, and not-found otherwise.
func TestFindByContentHash(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	const uuid = "hash-note"
	const path = "hash/note.md"
	const hash = "deadbeef"

	if err := reconcile.Upsert(ctx, g, uuid, path, hash); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	gotUUID, found, err := reconcile.FindByContentHash(ctx, g, hash)
	if err != nil {
		t.Fatalf("FindByContentHash: %v", err)
	}
	if !found || gotUUID != uuid {
		t.Errorf("FindByContentHash: got (%q, %v), want (%q, true)", gotUUID, found, uuid)
	}

	// A hash that does not exist.
	_, foundMissing, err := reconcile.FindByContentHash(ctx, g, "unknown-hash")
	if err != nil {
		t.Fatalf("FindByContentHash missing: %v", err)
	}
	if foundMissing {
		t.Error("FindByContentHash: expected not-found for unknown hash")
	}
}

// TestHashBytes verifies the content-hash helper produces a stable hex digest.
func TestHashBytes(t *testing.T) {
	// SHA-256("") is well-known.
	const emptyHex = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got := reconcile.HashBytes([]byte{}); got != emptyHex {
		t.Errorf("HashBytes empty: got %q, want %q", got, emptyHex)
	}

	h1 := reconcile.HashBytes([]byte("hello"))
	h2 := reconcile.HashBytes([]byte("hello"))
	if h1 != h2 {
		t.Error("HashBytes not deterministic")
	}
	if h1 == reconcile.HashBytes([]byte("world")) {
		t.Error("different content produced same hash")
	}
}

// TestUUIDIsTheKey verifies that when two paths collide (same path, different
// UUIDs), the UUID keying means the later write wins on the path UNIQUE
// constraint — the UUID-keyed design resolves such races deterministically.
func TestUUIDIsTheKey(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	const sharedPath = "shared/note.md"

	// First writer claims the path.
	if err := reconcile.Upsert(ctx, g, "uuid-first", sharedPath, "h1"); err != nil {
		t.Fatalf("Upsert first: %v", err)
	}

	// Second writer attempts the same path with a different UUID. The UNIQUE
	// constraint on path means this MUST fail — the path is already owned by
	// uuid-first. The caller (U7/U11) is responsible for conflict resolution;
	// the cache layer surfaces the error.
	err := reconcile.Upsert(ctx, g, "uuid-second", sharedPath, "h2")
	if err == nil {
		t.Error("expected error when a different UUID claims an already-owned path; got nil")
	}

	// The original UUID must still own the path.
	gotUUID, found, err2 := reconcile.Lookup(ctx, g, sharedPath)
	if err2 != nil {
		t.Fatalf("Lookup: %v", err2)
	}
	if !found || gotUUID != "uuid-first" {
		t.Errorf("after conflict: path owned by %q (found=%v), want uuid-first", gotUUID, found)
	}
}
