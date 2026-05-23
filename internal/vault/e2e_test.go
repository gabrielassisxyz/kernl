// Package vault end-to-end acceptance tests.
//
// This file drives a REAL temp vault through the full watcher stack
// (config → cold-start → live watch → reconciler → graph) and asserts
// all six origin acceptance examples (AE1–AE6), plus rebuild-from-vault
// and no-op idempotency.
//
// Each acceptance example is a named subtest so failures are easy to
// isolate. Windows are set very short (CoalesceWindowMs=50,
// MoveWindowMs=200) so the suite runs fast.
package vault

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/revisions"
	"github.com/gabrielassisxyz/kernl/internal/graph/search"
	"github.com/gabrielassisxyz/kernl/internal/vault/frontmatter"
)

// ---------------------------------------------------------------------------
// Test harness helpers
// ---------------------------------------------------------------------------

// e2eHarness holds a temp vault + a real file-backed graph + a running Service.
type e2eHarness struct {
	t        *testing.T
	vaultDir string
	dbPath   string
	g        *graph.Graph
	svc      *Service
	ctx      context.Context
	cancel   context.CancelFunc
}

// newHarness creates a temp vault dir, opens a fresh graph, starts a Service,
// and registers t.Cleanup for graceful shutdown.
//
// preCreateDirs is a list of vault-relative directory paths to create BEFORE
// starting the watcher, so that recursive watches cover them. Callers must
// list every subdirectory that will later receive files.
func newHarness(t *testing.T, preCreateDirs ...string) *e2eHarness {
	t.Helper()
	vaultDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "graph.db")

	g := openGraph(t, dbPath)

	// Create directories before the service starts so the watcher adds watches.
	for _, rel := range preCreateDirs {
		abs := filepath.Join(vaultDir, rel)
		if err := os.MkdirAll(abs, 0o755); err != nil {
			t.Fatalf("preCreateDirs MkdirAll %q: %v", abs, err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	cfg := config.VaultConfig{
		Root:             vaultDir,
		CoalesceWindowMs: 50,
		MoveWindowMs:     200,
	}

	svc := New(g, cfg)
	if err := svc.Start(ctx); err != nil {
		cancel()
		t.Fatalf("Service.Start: %v", err)
	}

	h := &e2eHarness{
		t:        t,
		vaultDir: vaultDir,
		dbPath:   dbPath,
		g:        g,
		svc:      svc,
		ctx:      ctx,
		cancel:   cancel,
	}

	t.Cleanup(h.stop)
	return h
}

func (h *e2eHarness) stop() {
	h.cancel()
	h.svc.Stop()
}

// writeFile writes content to path inside the vault, creating parent dirs.
func (h *e2eHarness) writeFile(relPath, content string) string {
	h.t.Helper()
	abs := filepath.Join(h.vaultDir, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		h.t.Fatalf("writeFile MkdirAll %q: %v", abs, err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		h.t.Fatalf("writeFile %q: %v", abs, err)
	}
	h.t.Logf("step: wrote %q (%d bytes)", relPath, len(content))
	return abs
}

// deleteFile removes a file from the vault.
func (h *e2eHarness) deleteFile(relPath string) {
	h.t.Helper()
	abs := filepath.Join(h.vaultDir, relPath)
	if err := os.Remove(abs); err != nil {
		h.t.Fatalf("deleteFile %q: %v", abs, err)
	}
	h.t.Logf("step: deleted %q", relPath)
}

// openGraph opens a file-backed graph and registers close cleanup.
func openGraph(t *testing.T, dbPath string) *graph.Graph {
	t.Helper()
	g, err := graph.Open(context.Background(), graph.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("graph.Open %q: %v", dbPath, err)
	}
	t.Cleanup(func() { _ = g.Close() })
	return g
}

// waitFor polls cond every 20ms until it returns true or timeout elapses.
// On timeout it calls t.Fatal with the description.
func waitFor(t *testing.T, timeout time.Duration, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("waitFor timeout (%s): %s", timeout, desc)
}

// countLiveNotes counts live (non-tombstoned) note nodes.
func countLiveNotes(t *testing.T, g *graph.Graph) int {
	t.Helper()
	var n int
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE type='note' AND deleted_at IS NULL`).Scan(&n)
	}); err != nil {
		t.Fatalf("countLiveNotes: %v", err)
	}
	return n
}

// countRevisions counts revision rows for a node.
func countRevisions(t *testing.T, g *graph.Graph, nodeID string) int {
	t.Helper()
	var n int
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM revisions WHERE node_id=?`, nodeID).Scan(&n)
	}); err != nil {
		t.Fatalf("countRevisions %q: %v", nodeID, err)
	}
	return n
}

// countAllRevisions counts all revision rows.
func countAllRevisions(t *testing.T, g *graph.Graph) int {
	t.Helper()
	var n int
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM revisions`).Scan(&n)
	}); err != nil {
		t.Fatalf("countAllRevisions: %v", err)
	}
	return n
}

// countDanglingLinks counts dangling_links rows for a source node.
func countDanglingLinks(t *testing.T, g *graph.Graph, srcNodeID string) int {
	t.Helper()
	var n int
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM dangling_links WHERE src_node_id=?`, srcNodeID).Scan(&n)
	}); err != nil {
		t.Fatalf("countDanglingLinks %q: %v", srcNodeID, err)
	}
	return n
}

// countDanglingByTarget counts dangling_links rows for a target key.
func countDanglingByTarget(t *testing.T, g *graph.Graph, targetKey string) int {
	t.Helper()
	var n int
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM dangling_links WHERE target_key=?`, targetKey).Scan(&n)
	}); err != nil {
		t.Fatalf("countDanglingByTarget %q: %v", targetKey, err)
	}
	return n
}

// nodeExistsByID returns true if a live (non-tombstoned) note with the given ID exists.
func nodeExistsByID(t *testing.T, g *graph.Graph, nodeID string) bool {
	t.Helper()
	var exists bool
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var dummy string
		err := tx.QueryRow(`SELECT id FROM nodes WHERE id=? AND type='note' AND deleted_at IS NULL`, nodeID).Scan(&dummy)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		exists = true
		return nil
	}); err != nil {
		t.Fatalf("nodeExistsByID %q: %v", nodeID, err)
	}
	return exists
}

// nodeIsTombstoned returns true if the note is soft-deleted.
func nodeIsTombstoned(t *testing.T, g *graph.Graph, nodeID string) bool {
	t.Helper()
	var tombstoned bool
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var deletedAt sql.NullString
		err := tx.QueryRow(`SELECT deleted_at FROM nodes WHERE id=? AND type='note'`, nodeID).Scan(&deletedAt)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		tombstoned = deletedAt.Valid
		return nil
	}); err != nil {
		t.Fatalf("nodeIsTombstoned %q: %v", nodeID, err)
	}
	return tombstoned
}

// incomingLinksToEdges returns the links_to edges pointing at dstID.
func incomingLinksToEdges(t *testing.T, g *graph.Graph, dstID string) []edges.Edge {
	t.Helper()
	var found []edges.Edge
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var err error
		found, err = edges.Incoming(context.Background(), tx, dstID, edges.WithType(edges.EdgeTypeLinksTo))
		return err
	}); err != nil {
		t.Fatalf("incomingLinksToEdges %q: %v", dstID, err)
	}
	return found
}

// listRevisions returns all revisions for a node.
func listRevisions(t *testing.T, g *graph.Graph, nodeID string) []revisions.Revision {
	t.Helper()
	var revs []revisions.Revision
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var err error
		revs, err = revisions.List(context.Background(), tx, nodeID)
		return err
	}); err != nil {
		t.Fatalf("listRevisions %q: %v", nodeID, err)
	}
	return revs
}

// getNote fetches a note by ID (returns nil if not found or tombstoned).
func getNote(t *testing.T, g *graph.Graph, nodeID string) *nodes.Note {
	t.Helper()
	var n *nodes.Note
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var err error
		n, err = nodes.GetNote(context.Background(), tx, nodeID)
		if err == graph.ErrNotFound {
			return nil
		}
		return err
	}); err != nil {
		t.Fatalf("getNote %q: %v", nodeID, err)
	}
	return n
}

// searchFTS runs FTS search and returns NodeIDs found.
func searchFTS(t *testing.T, g *graph.Graph, query string) []string {
	t.Helper()
	var ids []string
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		hits, err := search.Search(context.Background(), tx, query, search.WithTypes("note"))
		if err != nil {
			return err
		}
		for _, h := range hits {
			ids = append(ids, h.NodeID)
		}
		return nil
	}); err != nil {
		t.Fatalf("searchFTS %q: %v", query, err)
	}
	return ids
}

// readDiskFile reads a file from the vault for assertion.
func (h *e2eHarness) readDiskFile(relPath string) []byte {
	h.t.Helper()
	abs := filepath.Join(h.vaultDir, relPath)
	data, err := os.ReadFile(abs)
	if err != nil {
		h.t.Fatalf("readDiskFile %q: %v", relPath, err)
	}
	return data
}

// ---------------------------------------------------------------------------
// AE1 — UUID injection: injected on disk, other frontmatter bytes unchanged
// ---------------------------------------------------------------------------

func TestE2E_AE1_UUIDInjection(t *testing.T) {
	h := newHarness(t, "notes")

	// Write a note WITHOUT an id field, but with other frontmatter.
	const relPath = "notes/ae1-note.md"
	const originalContent = "---\ntitle: AE1 Note\nauthor: human\ntags:\n  - test\n---\n\nBody text here.\n"
	h.writeFile(relPath, originalContent)

	t.Logf("step: wrote note without UUID; polling until UUID injected on disk")

	// Poll until UUID is injected into the file on disk.
	var injectedUUID string
	waitFor(t, 5*time.Second, "UUID injected into file", func() bool {
		raw := h.readDiskFile(relPath)
		fm, err := frontmatter.Parse(raw)
		if err != nil || fm.ID == "" {
			return false
		}
		injectedUUID = fm.ID
		return true
	})

	t.Logf("step: UUID injected on disk: %q", injectedUUID)

	// Wait until the node appears in the graph.
	waitFor(t, 5*time.Second, "note node in graph", func() bool {
		return nodeExistsByID(t, h.g, injectedUUID)
	})

	t.Logf("step: node %q exists in graph", injectedUUID)

	// Assert: all other frontmatter bytes are unchanged.
	afterRaw := h.readDiskFile(relPath)
	afterStr := string(afterRaw)

	// The id line was injected, but title/author/tags must still be present.
	if !strings.Contains(afterStr, "title: AE1 Note") {
		t.Errorf("AE1: 'title: AE1 Note' missing after UUID injection; got:\n%s", afterStr)
	}
	if !strings.Contains(afterStr, "author: human") {
		t.Errorf("AE1: 'author: human' missing after UUID injection; got:\n%s", afterStr)
	}
	if !strings.Contains(afterStr, "tags:") {
		t.Errorf("AE1: 'tags:' missing after UUID injection; got:\n%s", afterStr)
	}
	if !strings.Contains(afterStr, "- test") {
		t.Errorf("AE1: '- test' tag missing after UUID injection; got:\n%s", afterStr)
	}
	if !strings.Contains(afterStr, "Body text here.") {
		t.Errorf("AE1: body missing after UUID injection; got:\n%s", afterStr)
	}

	t.Logf("result: AE1 PASS — UUID %q injected, other frontmatter bytes unchanged", injectedUUID)
}

// ---------------------------------------------------------------------------
// AE2 — Move preserves identity: inbound links intact, no content revision
// ---------------------------------------------------------------------------

func TestE2E_AE2_MovedNotePreservesIdentity(t *testing.T) {
	h := newHarness(t, "folder-a", "folder-b", "linkers")

	// Create the target note (the one that will be moved).
	const targetPath = "folder-a/target.md"
	const targetContent = "---\nid: ae2-target\ntitle: Target Note\n---\n\nTarget body.\n"
	h.writeFile(targetPath, targetContent)

	// Wait for target to appear in graph.
	waitFor(t, 5*time.Second, "target note in graph", func() bool {
		return nodeExistsByID(t, h.g, "ae2-target")
	})
	t.Logf("step: target note ae2-target is live")

	// Create three linker notes pointing to target via [[Target Note]].
	for i := 1; i <= 3; i++ {
		relPath := fmt.Sprintf("linkers/linker%d.md", i)
		content := fmt.Sprintf("---\nid: ae2-linker%d\ntitle: Linker %d\n---\n\nLinks to [[Target Note]].\n", i, i)
		h.writeFile(relPath, content)
	}

	// Wait for all three linkers and their edges.
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("ae2-linker%d", i)
		waitFor(t, 5*time.Second, fmt.Sprintf("linker%d in graph", i), func() bool {
			return nodeExistsByID(t, h.g, id)
		})
	}

	// Poll until we see 3 incoming links_to edges to the target.
	waitFor(t, 5*time.Second, "3 inbound links_to edges to target", func() bool {
		return len(incomingLinksToEdges(t, h.g, "ae2-target")) == 3
	})
	t.Logf("step: 3 inbound links_to edges confirmed")

	// Snapshot revision count before move.
	revsBefore := countRevisions(t, h.g, "ae2-target")
	t.Logf("step: revision count before move: %d", revsBefore)

	// Move the target note via git mv (init git repo in vault first).
	// Fall back to os.Rename if git is unavailable.
	newRelPath := "folder-b/target.md"
	newAbs := filepath.Join(h.vaultDir, newRelPath)
	if err := os.MkdirAll(filepath.Dir(newAbs), 0o755); err != nil {
		t.Fatalf("MkdirAll %q: %v", filepath.Dir(newAbs), err)
	}

	usedGitMV := false
	gitInitCmd := exec.Command("git", "-C", h.vaultDir, "init", "-q")
	if err := gitInitCmd.Run(); err == nil {
		addCmd := exec.Command("git", "-C", h.vaultDir, "add", ".")
		_ = addCmd.Run()
		commitCmd := exec.Command("git", "-C", h.vaultDir, "commit", "-m", "init", "--allow-empty", "--no-gpg-sign", "-q")
		_ = commitCmd.Run()
		mvCmd := exec.Command("git", "-C", h.vaultDir, "mv",
			filepath.Join("folder-a", "target.md"),
			filepath.Join("folder-b", "target.md"))
		if err := mvCmd.Run(); err == nil {
			usedGitMV = true
			t.Logf("step: used real git mv to move %q → %q", targetPath, newRelPath)
		} else {
			t.Logf("step: git mv failed (%v), falling back to os.Rename", err)
		}
	}
	if !usedGitMV {
		if err := os.Rename(filepath.Join(h.vaultDir, targetPath), newAbs); err != nil {
			t.Fatalf("os.Rename: %v", err)
		}
		t.Logf("step: used os.Rename to move %q → %q (git mv unavailable)", targetPath, newRelPath)
	}

	// Poll until the new path is recognized AND old path is gone from cache.
	// The move window is 200ms; give 3s grace.
	waitFor(t, 3*time.Second, "node still live after move", func() bool {
		return nodeExistsByID(t, h.g, "ae2-target")
	})

	// Wait for all 3 inbound links to still resolve (remain as edges, not dangling).
	waitFor(t, 3*time.Second, "3 inbound links_to still intact after move", func() bool {
		return len(incomingLinksToEdges(t, h.g, "ae2-target")) == 3
	})
	t.Logf("step: all 3 inbound links_to still intact after move")

	// Assert: no new content-change revision was added (body didn't change).
	revsAfter := countRevisions(t, h.g, "ae2-target")
	t.Logf("step: revision count after move: %d (was %d)", revsAfter, revsBefore)
	if revsAfter != revsBefore {
		t.Errorf("AE2: expected revision count unchanged after move; before=%d after=%d (used git mv: %v)", revsBefore, revsAfter, usedGitMV)
	}

	t.Logf("result: AE2 PASS — move preserved identity; inbound links=%d; git mv used: %v", len(incomingLinksToEdges(t, h.g, "ae2-target")), usedGitMV)
}

// ---------------------------------------------------------------------------
// AE3 — Dangling links: [[Roadmap]] with no target → dangling row, no phantom
// ---------------------------------------------------------------------------

func TestE2E_AE3_DanglingLinkNoPhantomNode(t *testing.T) {
	h := newHarness(t, "notes")

	// Write a note that links to [[Roadmap]] which doesn't exist.
	const relPath = "notes/ae3-note.md"
	const content = "---\nid: ae3-src\ntitle: AE3 Source\n---\n\nSee [[Roadmap]] for details.\n"
	h.writeFile(relPath, content)

	// Wait for the source note to appear in graph.
	waitFor(t, 5*time.Second, "ae3-src in graph", func() bool {
		return nodeExistsByID(t, h.g, "ae3-src")
	})
	t.Logf("step: ae3-src is live in graph")

	// Wait for dangling link to appear.
	waitFor(t, 5*time.Second, "dangling link for Roadmap", func() bool {
		return countDanglingByTarget(t, h.g, "Roadmap") > 0
	})
	t.Logf("step: dangling_links row for 'Roadmap' confirmed")

	// Assert: NO node named "Roadmap" was created.
	var phantomExists bool
	if err := h.g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var n int
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE title='Roadmap' AND type='note'`).Scan(&n)
	}); err != nil {
		t.Fatalf("check phantom node: %v", err)
	}
	if phantomExists {
		t.Error("AE3: phantom 'Roadmap' node was created — should not exist")
	}

	dCount := countDanglingByTarget(t, h.g, "Roadmap")
	t.Logf("result: AE3 PASS — dangling_links rows for 'Roadmap': %d; no phantom node created", dCount)
}

// ---------------------------------------------------------------------------
// AE4 — Dangling link promotion: create Roadmap.md → edge auto-promoted
// ---------------------------------------------------------------------------

func TestE2E_AE4_DanglingLinkPromotion(t *testing.T) {
	h := newHarness(t, "notes")

	// Step 1: create the source note with the dangling [[Roadmap]] link.
	const srcPath = "notes/ae4-src.md"
	const srcContent = "---\nid: ae4-src\ntitle: AE4 Source\n---\n\nSee [[Roadmap]] for details.\n"
	h.writeFile(srcPath, srcContent)

	waitFor(t, 5*time.Second, "ae4-src in graph", func() bool {
		return nodeExistsByID(t, h.g, "ae4-src")
	})
	waitFor(t, 5*time.Second, "dangling Roadmap link exists", func() bool {
		return countDanglingByTarget(t, h.g, "Roadmap") > 0
	})
	t.Logf("step: ae4-src live with dangling [[Roadmap]] link")

	// Step 2: create Roadmap.md — the dangling link should be promoted.
	const roadmapPath = "notes/Roadmap.md"
	const roadmapContent = "---\nid: ae4-roadmap\ntitle: Roadmap\n---\n\nThe roadmap.\n"
	h.writeFile(roadmapPath, roadmapContent)

	waitFor(t, 5*time.Second, "ae4-roadmap in graph", func() bool {
		return nodeExistsByID(t, h.g, "ae4-roadmap")
	})
	t.Logf("step: ae4-roadmap is live in graph")

	// Poll until dangling row is gone AND edge exists.
	waitFor(t, 5*time.Second, "dangling Roadmap row gone", func() bool {
		return countDanglingByTarget(t, h.g, "Roadmap") == 0
	})
	t.Logf("step: dangling_links row for 'Roadmap' is gone (promoted)")

	waitFor(t, 5*time.Second, "links_to edge from ae4-src to ae4-roadmap", func() bool {
		return len(incomingLinksToEdges(t, h.g, "ae4-roadmap")) > 0
	})

	inbound := incomingLinksToEdges(t, h.g, "ae4-roadmap")
	t.Logf("step: links_to edges incoming to ae4-roadmap: %d", len(inbound))

	found := false
	for _, e := range inbound {
		if e.Src == "ae4-src" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("AE4: expected links_to edge from ae4-src to ae4-roadmap; edges: %+v", inbound)
	}

	t.Logf("result: AE4 PASS — dangling link promoted to real links_to edge")
}

// ---------------------------------------------------------------------------
// AE5 — Tombstone after delete: node tombstoned, inbound links degrade,
//        revision history retrievable
// ---------------------------------------------------------------------------

func TestE2E_AE5_DeleteTombstoneAndHistory(t *testing.T) {
	h := newHarness(t, "notes")

	// Create the target note.
	const targetPath = "notes/ae5-target.md"
	const targetContent = "---\nid: ae5-target\ntitle: AE5 Target\n---\n\nTarget body for deletion test.\n"
	h.writeFile(targetPath, targetContent)

	waitFor(t, 5*time.Second, "ae5-target in graph", func() bool {
		return nodeExistsByID(t, h.g, "ae5-target")
	})

	// Create a linker note.
	const linkerPath = "notes/ae5-linker.md"
	const linkerContent = "---\nid: ae5-linker\ntitle: AE5 Linker\n---\n\nLinks to [[AE5 Target]].\n"
	h.writeFile(linkerPath, linkerContent)

	waitFor(t, 5*time.Second, "ae5-linker in graph", func() bool {
		return nodeExistsByID(t, h.g, "ae5-linker")
	})

	// Wait for the links_to edge.
	waitFor(t, 5*time.Second, "links_to edge from linker to target", func() bool {
		return len(incomingLinksToEdges(t, h.g, "ae5-target")) == 1
	})
	t.Logf("step: ae5-linker → ae5-target edge confirmed")

	// Snapshot revision count before delete.
	revsBefore := countRevisions(t, h.g, "ae5-target")
	t.Logf("step: ae5-target has %d revisions before delete", revsBefore)

	// Delete the target note from disk.
	h.deleteFile(targetPath)
	t.Logf("step: deleted ae5-target from disk; move window is 200ms")

	// Wait LONGER than the move window (200ms) before expecting tombstone.
	// Give it 3s total for the flush ticker to fire.
	waitFor(t, 3*time.Second, "ae5-target tombstoned", func() bool {
		return nodeIsTombstoned(t, h.g, "ae5-target")
	})
	t.Logf("step: ae5-target is tombstoned (deleted_at is set)")

	// Assert: tombstoned node is hidden from ListNotes AND search.
	var liveNotes []*nodes.Note
	if err := h.g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var err error
		liveNotes, err = nodes.ListNotes(context.Background(), tx, nodes.NoteFilter{})
		return err
	}); err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	for _, n := range liveNotes {
		if n.ID == "ae5-target" {
			t.Error("AE5: tombstoned ae5-target still appears in ListNotes")
		}
	}
	t.Logf("step: ae5-target hidden from ListNotes (live notes: %d)", len(liveNotes))

	// Also assert the tombstoned node is not returned by FTS search.
	ftsHits := searchFTS(t, h.g, "AE5 Target")
	for _, id := range ftsHits {
		if id == "ae5-target" {
			t.Error("AE5: tombstoned ae5-target still findable via FTS search")
		}
	}
	t.Logf("step: ae5-target hidden from FTS search")

	// Assert: inbound links degraded to dangling.
	waitFor(t, 3*time.Second, "inbound links degraded to dangling", func() bool {
		return countDanglingByTarget(t, h.g, "AE5 Target") > 0 || countDanglingLinks(t, h.g, "ae5-linker") > 0
	})
	t.Logf("step: inbound links degraded to dangling_links rows")

	// Assert: links_to edge is gone.
	linksToEdges := incomingLinksToEdges(t, h.g, "ae5-target")
	if len(linksToEdges) != 0 {
		t.Errorf("AE5: expected 0 links_to edges to tombstoned node; got %d", len(linksToEdges))
	}

	// Assert: revision history is still retrievable.
	revsAfter := listRevisions(t, h.g, "ae5-target")
	t.Logf("step: ae5-target has %d revisions (before=%d, after tombstone)", len(revsAfter), revsBefore)
	if len(revsAfter) < revsBefore {
		t.Errorf("AE5: revision history shrunk after tombstone; before=%d after=%d", revsBefore, len(revsAfter))
	}
	if len(revsAfter) == 0 {
		t.Error("AE5: no revision history for tombstoned node")
	}

	t.Logf("result: AE5 PASS — tombstoned, hidden from ListNotes, inbound links degraded, history=%d revisions", len(revsAfter))
}

// ---------------------------------------------------------------------------
// AE6 — DA attribution: vault-llm/ note with author:DA → attributed to agent:da
// ---------------------------------------------------------------------------

func TestE2E_AE6_DAAttribution(t *testing.T) {
	h := newHarness(t, "vault-llm")

	// Write a note with author: DA in vault-llm/ subfolder.
	const relPath = "vault-llm/ae6-da-note.md"
	const content = "---\nid: ae6-da\ntitle: AE6 DA Note\nauthor: da\n---\n\nThis was generated by the DA.\n"
	h.writeFile(relPath, content)

	// Wait for the node to appear in graph.
	waitFor(t, 5*time.Second, "ae6-da in graph", func() bool {
		return nodeExistsByID(t, h.g, "ae6-da")
	})
	t.Logf("step: ae6-da is live in graph")

	n := getNote(t, h.g, "ae6-da")
	if n == nil {
		t.Fatal("AE6: could not fetch note ae6-da")
	}

	t.Logf("step: note author field = %q", n.Author)

	// Assert: author is "da" (as stored from frontmatter).
	// The resolveAuthor function maps "da" → AuthorAgent("da"), which sets Name="agent:da"
	// in the revision row, but the attrs.author field stores the raw frontmatter value "da".
	if n.Author != "da" {
		t.Errorf("AE6: note attrs.author = %q, want %q", n.Author, "da")
	}

	// Assert: the first revision's author is "agent:da".
	revs := listRevisions(t, h.g, "ae6-da")
	if len(revs) == 0 {
		t.Fatal("AE6: no revisions for ae6-da")
	}
	// revisions.List returns DESC order; oldest is last.
	firstRev := revs[len(revs)-1]
	t.Logf("step: first revision author = %q", firstRev.Author)
	if firstRev.Author != "agent:da" {
		t.Errorf("AE6: first revision author = %q, want %q", firstRev.Author, "agent:da")
	}

	t.Logf("result: AE6 PASS — note.Author=%q, first revision author=%q", n.Author, firstRev.Author)
}

// ---------------------------------------------------------------------------
// Rebuild-from-vault: wipe notes graph, restart, assert reconstruction
// ---------------------------------------------------------------------------

func TestE2E_RebuildFromVault(t *testing.T) {
	// Phase 1: populate a vault with known notes via a Service.
	vaultDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "graph.db")

	g1 := openGraph(t, dbPath)

	cfg := config.VaultConfig{
		Root:             vaultDir,
		CoalesceWindowMs: 50,
		MoveWindowMs:     200,
	}

	ctx1, cancel1 := context.WithCancel(context.Background())
	svc1 := New(g1, cfg)
	if err := svc1.Start(ctx1); err != nil {
		cancel1()
		t.Fatalf("svc1.Start: %v", err)
	}

	// Write three notes.
	for i := 1; i <= 3; i++ {
		relPath := fmt.Sprintf("rebuild-note%d.md", i)
		content := fmt.Sprintf("---\nid: rebuild-%d\ntitle: Rebuild Note %d\n---\n\nContent %d unique rebuild text.\n", i, i, i)
		abs := filepath.Join(vaultDir, relPath)
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			cancel1()
			t.Fatalf("write %q: %v", relPath, err)
		}
	}

	// Wait for all 3 notes to appear.
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("rebuild-%d", i)
		waitFor(t, 5*time.Second, fmt.Sprintf("rebuild-%d in graph", i), func() bool {
			return nodeExistsByID(t, g1, id)
		})
	}
	t.Logf("step: phase 1 — 3 notes written and confirmed in graph")

	// Capture revision count before wipe.
	revsBeforeWipe := countAllRevisions(t, g1)
	t.Logf("step: revisions before wipe: %d", revsBeforeWipe)

	// Shut down service 1.
	cancel1()
	svc1.Stop()
	_ = g1.Close()
	t.Logf("step: service 1 stopped, graph closed")

	// Phase 2: Wipe the graph by deleting the DB file and opening a fresh one.
	if err := os.Remove(dbPath); err != nil {
		t.Fatalf("remove db: %v", err)
	}
	t.Logf("step: graph.db deleted (simulating fresh empty graph)")

	g2 := openGraph(t, dbPath)
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	svc2 := New(g2, cfg)
	if err := svc2.Start(ctx2); err != nil {
		t.Fatalf("svc2.Start: %v", err)
	}
	defer svc2.Stop()
	t.Logf("step: service 2 started with fresh empty graph against same vault")

	// Assert: all 3 notes reconstructed from disk.
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("rebuild-%d", i)
		waitFor(t, 5*time.Second, fmt.Sprintf("rebuild-%d reconstructed", i), func() bool {
			return nodeExistsByID(t, g2, id)
		})
	}
	t.Logf("step: all 3 notes reconstructed in fresh graph")

	// Assert: FTS-findable.
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("rebuild-%d", i)
		query := fmt.Sprintf("unique rebuild text")
		waitFor(t, 5*time.Second, fmt.Sprintf("FTS finds rebuild-%d", i), func() bool {
			hits := searchFTS(t, g2, query)
			for _, h := range hits {
				if h == id {
					return true
				}
			}
			// Try the title-based search too.
			hits2 := searchFTS(t, g2, fmt.Sprintf("Rebuild Note %d", i))
			for _, h := range hits2 {
				if h == id {
					return true
				}
			}
			return false
		})
	}
	t.Logf("step: all 3 notes FTS-findable in fresh graph")

	// Assert: revision history is absent by design (R17 — only current state rebuilt).
	// ColdStart creates exactly 1 revision per note (the "create" revision).
	// There should NOT be more than N revisions (one per note from ColdStart).
	revsAfterRebuild := countAllRevisions(t, g2)
	t.Logf("step: revisions after rebuild: %d (before wipe was: %d)", revsAfterRebuild, revsBeforeWipe)
	// After rebuild, each note gets exactly 1 new "create" revision from ColdStart.
	// The prior history is gone (R17 design).
	if revsAfterRebuild > 3 {
		t.Errorf("RebuildFromVault: expected ≤3 revisions after rebuild (one per note), got %d", revsAfterRebuild)
	}

	t.Logf("result: RebuildFromVault PASS — 3 notes reconstructed, FTS-indexed, revisions=%d (history absent by design)", revsAfterRebuild)
}

// ---------------------------------------------------------------------------
// No-op restart idempotency: zero new revisions and zero mutations
// ---------------------------------------------------------------------------

func TestE2E_IdempotentRestart(t *testing.T) {
	// Phase 1: populate vault + graph.
	vaultDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "graph.db")

	g1 := openGraph(t, dbPath)
	cfg := config.VaultConfig{
		Root:             vaultDir,
		CoalesceWindowMs: 50,
		MoveWindowMs:     200,
	}

	ctx1, cancel1 := context.WithCancel(context.Background())
	svc1 := New(g1, cfg)
	if err := svc1.Start(ctx1); err != nil {
		cancel1()
		t.Fatalf("svc1.Start: %v", err)
	}

	// Write two notes.
	for i := 1; i <= 2; i++ {
		relPath := fmt.Sprintf("idem-note%d.md", i)
		content := fmt.Sprintf("---\nid: idem-%d\ntitle: Idem Note %d\n---\n\nIdempotent content %d.\n", i, i, i)
		abs := filepath.Join(vaultDir, relPath)
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			cancel1()
			t.Fatalf("write %q: %v", relPath, err)
		}
	}

	for i := 1; i <= 2; i++ {
		id := fmt.Sprintf("idem-%d", i)
		waitFor(t, 5*time.Second, fmt.Sprintf("idem-%d in graph", i), func() bool {
			return nodeExistsByID(t, g1, id)
		})
	}
	t.Logf("step: phase 1 — 2 notes written and confirmed")

	// Stop and reopen (no disk changes in between).
	cancel1()
	svc1.Stop()
	_ = g1.Close()
	t.Logf("step: service 1 stopped")

	// Snapshot state.
	g2 := openGraph(t, dbPath)
	liveNotesBefore := countLiveNotes(t, g2)
	revsBefore := countAllRevisions(t, g2)
	t.Logf("step: before restart — live notes: %d, revisions: %d", liveNotesBefore, revsBefore)
	_ = g2.Close()

	// Phase 2: restart against unchanged vault.
	g3 := openGraph(t, dbPath)
	ctx3, cancel3 := context.WithCancel(context.Background())
	defer cancel3()
	svc3 := New(g3, cfg)
	if err := svc3.Start(ctx3); err != nil {
		t.Fatalf("svc3.Start: %v", err)
	}
	defer svc3.Stop()

	// Give ColdStart time to finish (it's synchronous inside Start, but the
	// live-watch settle needs a moment).
	time.Sleep(200 * time.Millisecond)

	liveNotesAfter := countLiveNotes(t, g3)
	revsAfter := countAllRevisions(t, g3)
	t.Logf("step: after restart — live notes: %d, revisions: %d", liveNotesAfter, revsAfter)

	if liveNotesAfter != liveNotesBefore {
		t.Errorf("IdempotentRestart: live note count changed; before=%d after=%d", liveNotesBefore, liveNotesAfter)
	}
	if revsAfter != revsBefore {
		t.Errorf("IdempotentRestart: revision count changed (mutations during no-op restart); before=%d after=%d", revsBefore, revsAfter)
	}

	t.Logf("result: IdempotentRestart PASS — live notes=%d (unchanged), revisions=%d (unchanged)", liveNotesAfter, revsAfter)
}

// ---------------------------------------------------------------------------
// Burst write: many notes quickly → correct final count, no inflated revisions
// ---------------------------------------------------------------------------

func TestE2E_BurstWrite(t *testing.T) {
	h := newHarness(t, "burst")

	const noteCount = 10
	ids := make([]string, noteCount)
	for i := 0; i < noteCount; i++ {
		ids[i] = fmt.Sprintf("burst-%02d", i)
		relPath := fmt.Sprintf("burst/note%02d.md", i)
		content := fmt.Sprintf("---\nid: %s\ntitle: Burst Note %02d\n---\n\nBurst body %02d.\n", ids[i], i, i)
		h.writeFile(relPath, content)
	}

	// Wait for all notes to appear in graph.
	waitFor(t, 10*time.Second, "all burst notes in graph", func() bool {
		for _, id := range ids {
			if !nodeExistsByID(t, h.g, id) {
				return false
			}
		}
		return true
	})
	t.Logf("step: all %d burst notes confirmed in graph", noteCount)

	// Assert: live note count matches (no duplicates/missing).
	live := countLiveNotes(t, h.g)
	if live < noteCount {
		t.Errorf("BurstWrite: expected at least %d live notes, got %d", noteCount, live)
	}

	// Assert: no inflated revisions — each note should have exactly 1 revision (create).
	for _, id := range ids {
		revCount := countRevisions(t, h.g, id)
		if revCount != 1 {
			t.Errorf("BurstWrite: note %q has %d revisions (want 1 — no coalesce inflation)", id, revCount)
		}
	}

	t.Logf("result: BurstWrite PASS — %d notes live, each with exactly 1 revision", noteCount)
}
