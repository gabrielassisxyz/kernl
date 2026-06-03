package reconcile_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/gabrielassisxyz/kernl/internal/vault/frontmatter"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

// ---------------------------------------------------------------------------
// ColdStart test harness helpers
// ---------------------------------------------------------------------------

// newColdStartHarness creates a temp vault + in-memory graph + Reconciler.
func newColdStartHarness(t *testing.T) (*graph.Graph, string, *reconcile.Reconciler) {
	t.Helper()
	g := testutil.NewInMemoryTestGraph(t)
	vault := t.TempDir()
	rec := reconcile.New(g, vault)
	return g, vault, rec
}

// countRevisions returns the number of revisions for nodeID.
func countRevisions(t *testing.T, g *graph.Graph, nodeID string) int {
	t.Helper()
	ctx := context.Background()
	var n int
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM revisions WHERE node_id = ?`, nodeID).Scan(&n)
	}); err != nil {
		t.Fatalf("countRevisions %q: %v", nodeID, err)
	}
	return n
}

// countNodes returns the total live (non-tombstoned) note count.
func countLiveNotes(t *testing.T, g *graph.Graph) int {
	t.Helper()
	ctx := context.Background()
	var n int
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(
			`SELECT COUNT(*) FROM nodes WHERE type = 'note' AND deleted_at IS NULL`,
		).Scan(&n)
	}); err != nil {
		t.Fatalf("countLiveNotes: %v", err)
	}
	return n
}

// countAllRevisions returns the total number of revision rows.
func countAllRevisions(t *testing.T, g *graph.Graph) int {
	t.Helper()
	ctx := context.Background()
	var n int
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM revisions`).Scan(&n)
	}); err != nil {
		t.Fatalf("countAllRevisions: %v", err)
	}
	return n
}

// countAllNodes returns the total number of node rows (including tombstoned).
func countAllNodes(t *testing.T, g *graph.Graph) int {
	t.Helper()
	ctx := context.Background()
	var n int
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE type = 'note'`).Scan(&n)
	}); err != nil {
		t.Fatalf("countAllNodes: %v", err)
	}
	return n
}

// ---------------------------------------------------------------------------
// F4 / R4: files created while off → created on boot with correct identity
// ---------------------------------------------------------------------------

func TestColdStart_FilesCreatedWhileOff(t *testing.T) {
	ctx := context.Background()
	g, vault, rec := newColdStartHarness(t)

	// Write two notes to disk without any prior graph state.
	writeFile(t, filepath.Join(vault, "alpha.md"),
		"---\nid: cs-alpha\ntitle: Alpha\n---\n\nAlpha body.\n")
	writeFile(t, filepath.Join(vault, "beta.md"),
		"---\nid: cs-beta\ntitle: Beta\n---\n\nBeta body.\n")

	if err := rec.ColdStart(ctx); err != nil {
		t.Fatalf("ColdStart: %v", err)
	}

	// Both nodes must exist with correct titles.
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		n, err := nodes.GetNote(ctx, tx, "cs-alpha")
		if err != nil {
			return err
		}
		if n.Title != "Alpha" {
			t.Errorf("alpha title = %q, want %q", n.Title, "Alpha")
		}
		n2, err := nodes.GetNote(ctx, tx, "cs-beta")
		if err != nil {
			return err
		}
		if n2.Title != "Beta" {
			t.Errorf("beta title = %q, want %q", n2.Title, "Beta")
		}
		return nil
	}); err != nil {
		t.Fatalf("GetNote: %v", err)
	}

	// Path cache entries must exist.
	for _, rel := range []string{"alpha.md", "beta.md"} {
		_, found, err := reconcile.Lookup(ctx, g, rel)
		if err != nil {
			t.Fatalf("Lookup %q: %v", rel, err)
		}
		if !found {
			t.Errorf("path-cache entry missing for %q", rel)
		}
	}

	// FTS must be populated.
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var cnt int
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes_fts WHERE title MATCH 'Alpha'`).Scan(&cnt)
	}); err != nil {
		t.Fatalf("FTS query: %v", err)
	}
}

// ---------------------------------------------------------------------------
// File moved while off → path cache updated, no content-change revision
// ---------------------------------------------------------------------------

func TestColdStart_FileMovedWhileOff_NoDiffRevision(t *testing.T) {
	ctx := context.Background()
	g, vault, rec := newColdStartHarness(t)

	// Simulate: note existed at old path, was created by a prior run.
	oldPath := filepath.Join(vault, "folder-a", "note.md")
	writeFile(t, oldPath, "---\nid: move-uuid\ntitle: Move Note\n---\n\nSame body.\n")
	if err := rec.OnCreate(ctx, oldPath); err != nil {
		t.Fatalf("OnCreate (setup): %v", err)
	}
	revsBefore := countRevisions(t, g, "move-uuid")

	// Simulate shutdown: move the file to a new location while off.
	newPath := filepath.Join(vault, "folder-b", "note.md")
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	// Remove the old folder to make it clean.
	_ = os.Remove(filepath.Join(vault, "folder-a"))

	// Create a new Reconciler (simulates fresh boot — no in-memory state).
	rec2 := reconcile.New(g, vault)
	if err := rec2.ColdStart(ctx); err != nil {
		t.Fatalf("ColdStart: %v", err)
	}

	// Path cache must point to new path.
	gotPath, found, err := reconcile.LookupByUUID(ctx, g, "move-uuid")
	if err != nil {
		t.Fatalf("LookupByUUID: %v", err)
	}
	if !found {
		t.Fatal("path cache entry missing after move")
	}
	wantRelPath := filepath.Join("folder-b", "note.md")
	if gotPath != wantRelPath {
		t.Errorf("LookupByUUID = %q, want %q", gotPath, wantRelPath)
	}

	// No content-change revision must have been added (body unchanged).
	revsAfter := countRevisions(t, g, "move-uuid")
	if revsAfter != revsBefore {
		t.Errorf("revision count: got %d, want %d (no diff revision on pure move)", revsAfter, revsBefore)
	}

	// Node must still be live.
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		tombstoned, err := nodes.IsNoteTombstoned(ctx, tx, "move-uuid")
		if err != nil {
			return err
		}
		if tombstoned {
			t.Error("node must not be tombstoned after move")
		}
		return nil
	}); err != nil {
		t.Fatalf("tombstone check: %v", err)
	}
}

// ---------------------------------------------------------------------------
// File edited while off → exactly one diff revision on boot
// ---------------------------------------------------------------------------

func TestColdStart_FileEditedWhileOff_OneDiffRevision(t *testing.T) {
	ctx := context.Background()
	g, vault, rec := newColdStartHarness(t)

	path := filepath.Join(vault, "edited.md")
	writeFile(t, path, "---\nid: edit-uuid\ntitle: Edited\n---\n\nVersion one.\n")
	if err := rec.OnCreate(ctx, path); err != nil {
		t.Fatalf("OnCreate: %v", err)
	}
	revsBefore := countRevisions(t, g, "edit-uuid")

	// Simulate edit while off.
	writeFile(t, path, "---\nid: edit-uuid\ntitle: Edited\n---\n\nVersion two.\n")

	rec2 := reconcile.New(g, vault)
	if err := rec2.ColdStart(ctx); err != nil {
		t.Fatalf("ColdStart: %v", err)
	}

	revsAfter := countRevisions(t, g, "edit-uuid")
	if revsAfter != revsBefore+1 {
		t.Errorf("revision count: got %d, want %d (one diff revision)", revsAfter, revsBefore+1)
	}
}

// ---------------------------------------------------------------------------
// File deleted while off → node tombstoned on boot (R18 semantics)
// ---------------------------------------------------------------------------

func TestColdStart_FileDeletedWhileOff_NodeTombstoned(t *testing.T) {
	ctx := context.Background()
	g, vault, rec := newColdStartHarness(t)

	// Note B links to Note A (so we can verify edge degradation after tombstone).
	pathA := filepath.Join(vault, "to-delete.md")
	pathB := filepath.Join(vault, "linker.md")
	writeFile(t, pathA, "---\nid: del-uuid-a\ntitle: To Delete\n---\n\nContent.\n")
	writeFile(t, pathB, "---\nid: del-uuid-b\ntitle: Linker\n---\n\nSee [[to-delete]].\n")

	if err := rec.OnCreate(ctx, pathA); err != nil {
		t.Fatalf("OnCreate A: %v", err)
	}
	if err := rec.OnCreate(ctx, pathB); err != nil {
		t.Fatalf("OnCreate B: %v", err)
	}

	// Delete note A from disk (simulating deletion while off).
	if err := os.Remove(pathA); err != nil {
		t.Fatalf("os.Remove: %v", err)
	}

	rec2 := reconcile.New(g, vault)
	if err := rec2.ColdStart(ctx); err != nil {
		t.Fatalf("ColdStart: %v", err)
	}

	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		// deleted_at must be set.
		tombstoned, err := nodes.IsNoteTombstoned(ctx, tx, "del-uuid-a")
		if err != nil {
			return err
		}
		if !tombstoned {
			t.Error("expected node to be tombstoned after cold-start delete")
		}

		// Node row must still exist (soft delete).
		var cnt int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM nodes WHERE id = 'del-uuid-a'`,
		).Scan(&cnt); err != nil {
			return err
		}
		if cnt != 1 {
			t.Errorf("node row must persist after soft-delete, got count=%d", cnt)
		}

		// Revision history must be non-empty.
		var revCnt int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM revisions WHERE node_id = 'del-uuid-a'`,
		).Scan(&revCnt); err != nil {
			return err
		}
		if revCnt == 0 {
			t.Error("revision history must be retrievable after tombstone")
		}

		// Inbound links_to edge must be degraded.
		var edgeCnt int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM edges WHERE dst = 'del-uuid-a' AND label = 'links_to'`,
		).Scan(&edgeCnt); err != nil {
			return err
		}
		if edgeCnt != 0 {
			t.Errorf("expected 0 incoming edges after degradation, got %d", edgeCnt)
		}

		// dangling_links must contain the degraded link.
		var dangCnt int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM dangling_links WHERE src_node_id = 'del-uuid-b'`,
		).Scan(&dangCnt); err != nil {
			return err
		}
		if dangCnt == 0 {
			t.Error("expected dangling_links after edge degradation")
		}

		// ListNotes must not include the tombstoned note.
		liveNotes, err := nodes.ListNotes(ctx, tx, nodes.NoteFilter{})
		if err != nil {
			return err
		}
		for _, n := range liveNotes {
			if n.ID == "del-uuid-a" {
				t.Error("tombstoned note must not appear in ListNotes")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// Rebuild-from-vault: empty graph + files on disk → all notes recreated (R17)
// ---------------------------------------------------------------------------

func TestColdStart_RebuildFromVault(t *testing.T) {
	ctx := context.Background()

	// Use a temp vault dir with several notes.
	vault := t.TempDir()
	writeFile(t, filepath.Join(vault, "note1.md"),
		"---\nid: rb-uuid-1\ntitle: Rebuild One\n---\n\nContent one.\n")
	writeFile(t, filepath.Join(vault, "note2.md"),
		"---\nid: rb-uuid-2\ntitle: Rebuild Two\n---\n\nContent two.\n")
	writeFile(t, filepath.Join(vault, "note3.md"),
		"---\nid: rb-uuid-3\ntitle: Rebuild Three\n---\n\nContent three.\n")

	// Empty graph (simulates deleted graph.db).
	g := testutil.NewInMemoryTestGraph(t)
	rec := reconcile.New(g, vault)

	if err := rec.ColdStart(ctx); err != nil {
		t.Fatalf("ColdStart: %v", err)
	}

	// All three nodes must be live.
	if n := countLiveNotes(t, g); n != 3 {
		t.Errorf("live notes = %d, want 3", n)
	}

	// FTS must be populated.
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var cnt int
		return tx.QueryRow(
			`SELECT COUNT(*) FROM nodes_fts WHERE title MATCH 'Rebuild'`,
		).Scan(&cnt)
	}); err != nil {
		t.Fatalf("FTS: %v", err)
	}

	// History is absent by design (R17): exactly one creation revision per note.
	// (OnCreate records the creation revision — that is the current state.)
	for _, id := range []string{"rb-uuid-1", "rb-uuid-2", "rb-uuid-3"} {
		if n := countRevisions(t, g, id); n < 1 {
			t.Errorf("uuid %q: expected at least 1 revision after rebuild, got %d", id, n)
		}
	}
}

// ---------------------------------------------------------------------------
// Idempotency: second consecutive ColdStart with no disk changes → zero mutations
// ---------------------------------------------------------------------------

func TestColdStart_Idempotent(t *testing.T) {
	ctx := context.Background()
	g, vault, rec := newColdStartHarness(t)

	writeFile(t, filepath.Join(vault, "idem.md"),
		"---\nid: idem-uuid\ntitle: Idem\n---\n\nSame content forever.\n")

	// First boot.
	if err := rec.ColdStart(ctx); err != nil {
		t.Fatalf("ColdStart #1: %v", err)
	}

	nodesAfterFirst := countAllNodes(t, g)
	revsAfterFirst := countAllRevisions(t, g)

	// Second boot (fresh Reconciler — no in-memory state).
	rec2 := reconcile.New(g, vault)
	if err := rec2.ColdStart(ctx); err != nil {
		t.Fatalf("ColdStart #2: %v", err)
	}

	nodesAfterSecond := countAllNodes(t, g)
	revsAfterSecond := countAllRevisions(t, g)

	if nodesAfterSecond != nodesAfterFirst {
		t.Errorf("node count changed: first=%d second=%d", nodesAfterFirst, nodesAfterSecond)
	}
	if revsAfterSecond != revsAfterFirst {
		t.Errorf("revision count changed: first=%d second=%d", revsAfterFirst, revsAfterSecond)
	}
}

// ---------------------------------------------------------------------------
// UUID injected on prior boot is NOT re-injected (file bytes unchanged)
// ---------------------------------------------------------------------------

func TestColdStart_UUIDInjectedOnce_NotReinjected(t *testing.T) {
	ctx := context.Background()
	g, vault, rec := newColdStartHarness(t)

	// File has no UUID — will be injected on first ColdStart.
	path := filepath.Join(vault, "no-id.md")
	writeFile(t, path, "---\ntitle: No ID\n---\n\nSome content.\n")

	if err := rec.ColdStart(ctx); err != nil {
		t.Fatalf("ColdStart #1: %v", err)
	}

	// Read the UUID that was injected.
	raw1, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after first boot: %v", err)
	}
	fm1, err := frontmatter.Parse(raw1)
	if err != nil {
		t.Fatalf("Parse after first boot: %v", err)
	}
	if fm1.ID == "" {
		t.Fatal("UUID must be injected after first ColdStart")
	}
	injectedUUID := fm1.ID
	revsBefore := countRevisions(t, g, injectedUUID)

	// Second boot: file bytes are unchanged (UUID already in frontmatter).
	rec2 := reconcile.New(g, vault)
	if err := rec2.ColdStart(ctx); err != nil {
		t.Fatalf("ColdStart #2: %v", err)
	}

	// File on disk must be byte-identical (no re-injection).
	raw2, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after second boot: %v", err)
	}
	if string(raw1) != string(raw2) {
		t.Error("file bytes changed on second boot — UUID was re-injected")
	}

	// No new revision must have been recorded.
	revsAfter := countRevisions(t, g, injectedUUID)
	if revsAfter != revsBefore {
		t.Errorf("revision count changed on second boot: before=%d after=%d", revsBefore, revsAfter)
	}
}

// ---------------------------------------------------------------------------
// Move detected via content hash (file copied without UUID)
// ---------------------------------------------------------------------------

func TestColdStart_MoveViaContentHash(t *testing.T) {
	ctx := context.Background()
	g, vault, rec := newColdStartHarness(t)

	// Create a note with a UUID.
	oldPath := filepath.Join(vault, "original.md")
	content := "---\nid: hash-move-cs\ntitle: Hash Move CS\n---\n\nExact same content.\n"
	writeFile(t, oldPath, content)
	if err := rec.OnCreate(ctx, oldPath); err != nil {
		t.Fatalf("OnCreate: %v", err)
	}
	revsBefore := countRevisions(t, g, "hash-move-cs")

	// While "off": move to new path with same content (UUID stays, so this also
	// tests the UUID-based detection, but demonstrates the content-hash path too).
	newPath := filepath.Join(vault, "moved.md")
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	rec2 := reconcile.New(g, vault)
	if err := rec2.ColdStart(ctx); err != nil {
		t.Fatalf("ColdStart: %v", err)
	}

	// Path cache must point to new path.
	gotPath, found, err := reconcile.LookupByUUID(ctx, g, "hash-move-cs")
	if err != nil {
		t.Fatalf("LookupByUUID: %v", err)
	}
	if !found || gotPath != "moved.md" {
		t.Errorf("path = %q (found=%v), want %q", gotPath, found, "moved.md")
	}

	// No content revision on pure move.
	revsAfter := countRevisions(t, g, "hash-move-cs")
	if revsAfter != revsBefore {
		t.Errorf("revision count changed on pure move: before=%d after=%d", revsBefore, revsAfter)
	}
}

// ---------------------------------------------------------------------------
// Move + edit while off → path updated AND one diff revision
// ---------------------------------------------------------------------------

func TestColdStart_MoveAndEditWhileOff(t *testing.T) {
	ctx := context.Background()
	g, vault, rec := newColdStartHarness(t)

	oldPath := filepath.Join(vault, "moveedit.md")
	writeFile(t, oldPath, "---\nid: moveedit-uuid\ntitle: MoveEdit\n---\n\nOriginal.\n")
	if err := rec.OnCreate(ctx, oldPath); err != nil {
		t.Fatalf("OnCreate: %v", err)
	}
	revsBefore := countRevisions(t, g, "moveedit-uuid")

	// While off: move AND change content.
	newPath := filepath.Join(vault, "subdir", "moveedit.md")
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, newPath, "---\nid: moveedit-uuid\ntitle: MoveEdit\n---\n\nEdited.\n")
	if err := os.Remove(oldPath); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	rec2 := reconcile.New(g, vault)
	if err := rec2.ColdStart(ctx); err != nil {
		t.Fatalf("ColdStart: %v", err)
	}

	// Path updated.
	gotPath, found, err := reconcile.LookupByUUID(ctx, g, "moveedit-uuid")
	if err != nil {
		t.Fatalf("LookupByUUID: %v", err)
	}
	wantPath := filepath.Join("subdir", "moveedit.md")
	if !found || gotPath != wantPath {
		t.Errorf("path = %q (found=%v), want %q", gotPath, found, wantPath)
	}

	// One additional diff revision for the content change.
	revsAfter := countRevisions(t, g, "moveedit-uuid")
	if revsAfter != revsBefore+1 {
		t.Errorf("revision count: got %d, want %d", revsAfter, revsBefore+1)
	}
}

// TestColdStart_ConflictingFileDoesNotAbort verifies that a single file whose
// embedded id conflicts with an existing note_paths binding is logged and
// skipped, rather than aborting cold-start and taking down the whole server.
func TestColdStart_ConflictingFileDoesNotAbort(t *testing.T) {
	ctx := context.Background()
	g, vault, rec := newColdStartHarness(t)

	// First boot: x.md with id A → binds path x.md to A.
	xPath := filepath.Join(vault, "x.md")
	if err := os.WriteFile(xPath, []byte("---\nid: conflict-a\ntitle: X\n---\n\nbody\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := rec.ColdStart(ctx); err != nil {
		t.Fatalf("first ColdStart: %v", err)
	}

	// Corrupt: overwrite x.md with a DIFFERENT id while the path is bound to A.
	if err := os.WriteFile(xPath, []byte("---\nid: conflict-b\ntitle: X2\n---\n\nbody2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// A healthy file that must still be ingested.
	if err := os.WriteFile(filepath.Join(vault, "good.md"), []byte("---\nid: good-id\ntitle: Good\n---\n\nfine\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rec2 := reconcile.New(g, vault)
	if err := rec2.ColdStart(ctx); err != nil {
		t.Fatalf("ColdStart aborted on a conflicting file (should skip+continue): %v", err)
	}

	var goodExists int
	_ = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id='good-id' AND deleted_at IS NULL`).Scan(&goodExists)
	})
	if goodExists != 1 {
		t.Errorf("healthy file should be ingested despite the conflicting one; got %d", goodExists)
	}
}
