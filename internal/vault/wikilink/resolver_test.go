package wikilink

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// author is the test author used for node creation.
var testAuthor = nodes.Author{Name: "test"}

// createTestNote inserts a note node and returns its ID.
func createTestNote(t *testing.T, g *graph.Graph, ctx context.Context, title, body string) string {
	t.Helper()
	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateNote(ctx, tx, nodes.Note{Title: title, Body: body}, testAuthor)
		return err
	})
	if err != nil {
		t.Fatalf("createTestNote(%q): %v", title, err)
	}
	return id
}

// insertNotePath inserts a row into note_paths linking a node UUID to a path.
func insertNotePath(t *testing.T, g *graph.Graph, ctx context.Context, nodeID, path string) {
	t.Helper()
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(
			`INSERT INTO note_paths (uuid, path) VALUES (?, ?)`,
			nodeID, path,
		)
		return err
	})
	if err != nil {
		t.Fatalf("insertNotePath(%q, %q): %v", nodeID, path, err)
	}
}

// countDanglingForSrc returns how many dangling_links rows exist for a given source.
func countDanglingForSrc(t *testing.T, g *graph.Graph, ctx context.Context, srcID string) int {
	t.Helper()
	var count int
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM dangling_links WHERE src_node_id = ?`, srcID).Scan(&count)
	}); err != nil {
		t.Fatalf("countDanglingForSrc: %v", err)
	}
	return count
}

// countEdgesWithLabel returns how many edges with a given label exist between src and dst.
func countEdgesWithLabel(t *testing.T, g *graph.Graph, ctx context.Context, srcID, dstID, label string) int {
	t.Helper()
	var count int
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(
			`SELECT COUNT(*) FROM edges WHERE src = ? AND dst = ? AND label = ?`,
			srcID, dstID, label,
		).Scan(&count)
	}); err != nil {
		t.Fatalf("countEdgesWithLabel: %v", err)
	}
	return count
}

// ---------------------------------------------------------------------------
// Resolve tests
// ---------------------------------------------------------------------------

// TestResolveUUIDTarget verifies that a UUID wikilink target resolves directly to an edge.
func TestResolveUUIDTarget(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	srcID := createTestNote(t, g, ctx, "Source", "body")
	dstID := createTestNote(t, g, ctx, "Destination", "body")

	r := &Resolver{}
	outcomes, err := r.Resolve(ctx, g, srcID, "See [["+dstID+"|Dest]].")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if !outcomes[0].Resolved {
		t.Error("expected Resolved=true for UUID target")
	}
	if outcomes[0].ResolvedBy != "uuid" {
		t.Errorf("expected ResolvedBy='uuid', got %q", outcomes[0].ResolvedBy)
	}
	if outcomes[0].EdgeID == "" {
		t.Error("expected non-empty EdgeID")
	}
	if n := countEdgesWithLabel(t, g, ctx, srcID, dstID, "links_to"); n != 1 {
		t.Errorf("expected 1 links_to edge, got %d", n)
	}
}

// TestResolveStemViaNotePaths verifies stem resolution via the note_paths table.
func TestResolveStemViaNotePaths(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	srcID := createTestNote(t, g, ctx, "Source", "body")
	dstID := createTestNote(t, g, ctx, "Roadmap", "body")
	insertNotePath(t, g, ctx, dstID, "Roadmap.md")

	r := &Resolver{}
	outcomes, err := r.Resolve(ctx, g, srcID, "See [[Roadmap]].")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if !outcomes[0].Resolved {
		t.Error("expected Resolved=true")
	}
	if outcomes[0].ResolvedBy != "stem" {
		t.Errorf("expected ResolvedBy='stem', got %q", outcomes[0].ResolvedBy)
	}
	if n := countEdgesWithLabel(t, g, ctx, srcID, dstID, "links_to"); n != 1 {
		t.Errorf("expected 1 links_to edge, got %d", n)
	}
}

// TestResolveTitleFallback verifies stem→title fallback when note_paths has no match.
func TestResolveTitleFallback(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	srcID := createTestNote(t, g, ctx, "Source", "body")
	// Note with title "Roadmap" but NOT in note_paths.
	dstID := createTestNote(t, g, ctx, "Roadmap", "body")

	r := &Resolver{}
	outcomes, err := r.Resolve(ctx, g, srcID, "See [[Roadmap]].")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if !outcomes[0].Resolved {
		t.Error("expected Resolved=true via title fallback")
	}
	if outcomes[0].ResolvedBy != "title" {
		t.Errorf("expected ResolvedBy='title', got %q", outcomes[0].ResolvedBy)
	}
	if n := countEdgesWithLabel(t, g, ctx, srcID, dstID, "links_to"); n != 1 {
		t.Errorf("expected 1 links_to edge, got %d", n)
	}
}

// TestResolveUnresolved_AE3 verifies that an unresolved link creates exactly one
// dangling_links row and no node or edge (AE3).
func TestResolveUnresolved_AE3(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	srcID := createTestNote(t, g, ctx, "Source", "body")

	r := &Resolver{}
	outcomes, err := r.Resolve(ctx, g, srcID, "See [[Roadmap]].")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].Resolved {
		t.Error("expected Resolved=false for unresolved target")
	}

	// Exactly one dangling row, no edges, no phantom node.
	if n := countDanglingForSrc(t, g, ctx, srcID); n != 1 {
		t.Errorf("expected 1 dangling row, got %d", n)
	}

	// No edge created.
	var edgeCount int
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM edges WHERE src = ?`, srcID).Scan(&edgeCount)
	}); err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if edgeCount != 0 {
		t.Errorf("expected 0 edges, got %d", edgeCount)
	}

	// No phantom node named "Roadmap" (besides source).
	var nodeCount int
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(
			`SELECT COUNT(*) FROM nodes WHERE title = 'Roadmap'`,
		).Scan(&nodeCount)
	}); err != nil {
		t.Fatalf("count nodes: %v", err)
	}
	if nodeCount != 0 {
		t.Errorf("expected 0 phantom 'Roadmap' nodes, got %d", nodeCount)
	}
}

// TestResolveSelfLink verifies that a self-referencing link produces no edge.
func TestResolveSelfLink(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	// Create a note and an entry in note_paths so the UUID-less self-link resolves.
	srcID := createTestNote(t, g, ctx, "Source", "body")
	insertNotePath(t, g, ctx, srcID, "Source.md")

	r := &Resolver{}
	outcomes, err := r.Resolve(ctx, g, srcID, "Self-ref [[Source]].")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	// Self-link is "resolved" (target found) but no edge created.
	if !outcomes[0].Resolved {
		t.Error("expected Resolved=true for self-link (target exists)")
	}
	if outcomes[0].EdgeID != "" {
		t.Errorf("expected no EdgeID for self-link, got %q", outcomes[0].EdgeID)
	}

	var edgeCount int
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM edges WHERE src = ?`, srcID).Scan(&edgeCount)
	}); err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if edgeCount != 0 {
		t.Errorf("expected 0 edges for self-link, got %d", edgeCount)
	}
}

// TestResolveEdgeHasProvenance verifies that a resolved edge carries provenance attrs.
func TestResolveEdgeHasProvenance(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	srcID := createTestNote(t, g, ctx, "Source", "body")
	dstID := createTestNote(t, g, ctx, "Roadmap", "body")
	insertNotePath(t, g, ctx, dstID, "Roadmap.md")

	r := &Resolver{}
	outcomes, err := r.Resolve(ctx, g, srcID, "[[Roadmap]]")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(outcomes) != 1 || !outcomes[0].Resolved {
		t.Fatalf("expected resolved outcome, got %+v", outcomes)
	}

	// Check edge attrs contain target_text and resolved_by.
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var attrsStr string
		if err := tx.QueryRow(`SELECT attrs FROM edges WHERE id = ?`, outcomes[0].EdgeID).Scan(&attrsStr); err != nil {
			return err
		}
		var attrs map[string]interface{}
		if err := json.Unmarshal([]byte(attrsStr), &attrs); err != nil {
			return err
		}
		if attrs["target_text"] != "Roadmap" {
			t.Errorf("expected target_text='Roadmap', got %v", attrs["target_text"])
		}
		if attrs["resolved_by"] != "stem" {
			t.Errorf("expected resolved_by='stem', got %v", attrs["resolved_by"])
		}
		return nil
	}); err != nil {
		t.Fatalf("read edge attrs: %v", err)
	}
}

// TestResolveReindexClearsDangling verifies that calling Resolve twice for the same
// source results in exactly one dangling row (re-indexing clears old rows first).
func TestResolveReindexClearsDangling(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	srcID := createTestNote(t, g, ctx, "Source", "body")

	r := &Resolver{}
	// First call — creates one dangling row.
	if _, err := r.Resolve(ctx, g, srcID, "[[Unresolved]]"); err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	// Second call — should delete the old row and insert a fresh one.
	if _, err := r.Resolve(ctx, g, srcID, "[[Unresolved]]"); err != nil {
		t.Fatalf("second Resolve: %v", err)
	}

	if n := countDanglingForSrc(t, g, ctx, srcID); n != 1 {
		t.Errorf("expected exactly 1 dangling row after re-index, got %d", n)
	}
}

// TestResolveMultipleUnresolved verifies multiple unresolved links each get a row.
func TestResolveMultipleUnresolved(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	srcID := createTestNote(t, g, ctx, "Source", "body")

	r := &Resolver{}
	outcomes, err := r.Resolve(ctx, g, srcID, "[[Alpha]] and [[Beta]] and [[Gamma]].")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(outcomes) != 3 {
		t.Fatalf("expected 3 outcomes, got %d", len(outcomes))
	}
	for _, o := range outcomes {
		if o.Resolved {
			t.Errorf("expected Resolved=false for %q", o.Link.Target)
		}
	}
	if n := countDanglingForSrc(t, g, ctx, srcID); n != 3 {
		t.Errorf("expected 3 dangling rows, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// PromoteDangling tests
// ---------------------------------------------------------------------------

// TestPromoteDangling_AE4 verifies that a dangling link is promoted to a real edge
// when the target note appears (AE4).
func TestPromoteDangling_AE4(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	srcID := createTestNote(t, g, ctx, "Source", "body")

	r := &Resolver{}
	// Insert dangling row for "Roadmap".
	if _, err := r.Resolve(ctx, g, srcID, "[[Roadmap]]"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if n := countDanglingForSrc(t, g, ctx, srcID); n != 1 {
		t.Fatalf("expected 1 dangling row before promote, got %d", n)
	}

	// Now "create" the Roadmap note.
	dstID := createTestNote(t, g, ctx, "Roadmap", "body")
	insertNotePath(t, g, ctx, dstID, "Roadmap.md")

	// Promote — stem match.
	promoted, err := PromoteDangling(ctx, g, dstID, PromoteKey{Key: "Roadmap", Kind: "stem"})
	if err != nil {
		t.Fatalf("PromoteDangling: %v", err)
	}
	if promoted != 1 {
		t.Errorf("expected promoted=1, got %d", promoted)
	}

	// Dangling row gone.
	if n := countDanglingForSrc(t, g, ctx, srcID); n != 0 {
		t.Errorf("expected 0 dangling rows after promote, got %d", n)
	}
	// Real edge exists.
	if n := countEdgesWithLabel(t, g, ctx, srcID, dstID, "links_to"); n != 1 {
		t.Errorf("expected 1 links_to edge after promote, got %d", n)
	}
}

// TestPromoteDangling_EdgeLabel verifies promoted edge has label "links_to".
func TestPromoteDangling_EdgeLabel(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	srcID := createTestNote(t, g, ctx, "Source", "body")
	r := &Resolver{}
	if _, err := r.Resolve(ctx, g, srcID, "[[Target]]"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	dstID := createTestNote(t, g, ctx, "Target", "body")
	if _, err := PromoteDangling(ctx, g, dstID, PromoteKey{Key: "Target", Kind: "stem"}); err != nil {
		t.Fatalf("PromoteDangling: %v", err)
	}

	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		outgoing, err := edges.Outgoing(ctx, tx, srcID, edges.WithType(edges.EdgeTypeLinksTo))
		if err != nil {
			return err
		}
		if len(outgoing) != 1 {
			t.Errorf("expected 1 links_to edge, got %d", len(outgoing))
			return nil
		}
		if outgoing[0].Label != string(edges.EdgeTypeLinksTo) {
			t.Errorf("expected label 'links_to', got %q", outgoing[0].Label)
		}
		return nil
	}); err != nil {
		t.Fatalf("read edges: %v", err)
	}
}

// TestPromoteDangling_ProvenanceAttrs verifies promoted edge carries provenance attrs.
func TestPromoteDangling_ProvenanceAttrs(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	srcID := createTestNote(t, g, ctx, "Source", "body")
	r := &Resolver{}
	if _, err := r.Resolve(ctx, g, srcID, "[[Target]]"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	dstID := createTestNote(t, g, ctx, "Target", "body")
	if _, err := PromoteDangling(ctx, g, dstID, PromoteKey{Key: "Target", Kind: "stem"}); err != nil {
		t.Fatalf("PromoteDangling: %v", err)
	}

	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var attrsStr string
		return tx.QueryRow(`SELECT attrs FROM edges WHERE src = ? AND dst = ? AND label = 'links_to'`,
			srcID, dstID).Scan(&attrsStr)
	}); err != nil {
		t.Fatalf("read edge attrs: %v", err)
	}
}

// TestPromoteDangling_Idempotent verifies that calling PromoteDangling twice
// does not create duplicate edges (idempotency).
func TestPromoteDangling_Idempotent(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	srcID := createTestNote(t, g, ctx, "Source", "body")
	r := &Resolver{}
	if _, err := r.Resolve(ctx, g, srcID, "[[Target]]"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	dstID := createTestNote(t, g, ctx, "Target", "body")
	if _, err := PromoteDangling(ctx, g, dstID, PromoteKey{Key: "Target", Kind: "stem"}); err != nil {
		t.Fatalf("first PromoteDangling: %v", err)
	}

	// Second call — dangling row already deleted, so nothing to promote.
	n, err := PromoteDangling(ctx, g, dstID, PromoteKey{Key: "Target", Kind: "stem"})
	if err != nil {
		t.Fatalf("second PromoteDangling: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 promoted on second call, got %d", n)
	}

	// Still exactly one edge.
	if c := countEdgesWithLabel(t, g, ctx, srcID, dstID, "links_to"); c != 1 {
		t.Errorf("expected 1 edge after idempotent promote, got %d", c)
	}
}

// TestPromoteDangling_SelfLinkCleanedUp verifies that a self-link dangling row
// is removed without creating an edge.
func TestPromoteDangling_SelfLinkCleanedUp(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	// Create note. The note links to itself by title but no note_paths entry
	// exists yet, so the link is stored as dangling.
	noteID := createTestNote(t, g, ctx, "Self", "body")

	// Manually insert a dangling row that points back to the same note.
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(
			`INSERT INTO dangling_links (id, src_node_id, target_key, target_kind) VALUES (?, ?, ?, ?)`,
			"dl-self-test", noteID, "Self", "stem",
		)
		return err
	})
	if err != nil {
		t.Fatalf("insert self-dangling: %v", err)
	}

	// PromoteDangling is called for the note itself — self-link should be cleared.
	promoted, err := PromoteDangling(ctx, g, noteID, PromoteKey{Key: "Self", Kind: "stem"})
	if err != nil {
		t.Fatalf("PromoteDangling: %v", err)
	}
	if promoted != 0 {
		t.Errorf("expected 0 promoted edges for self-link, got %d", promoted)
	}

	// Dangling row should be gone.
	var count int
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM dangling_links WHERE id = 'dl-self-test'`).Scan(&count)
	}); err != nil {
		t.Fatalf("count dangling: %v", err)
	}
	if count != 0 {
		t.Errorf("expected self-dangling row to be deleted, got %d rows", count)
	}

	// No self-edge created.
	if n := countEdgesWithLabel(t, g, ctx, noteID, noteID, "links_to"); n != 0 {
		t.Errorf("expected no self-edge, got %d", n)
	}
}

// TestResolveIDCollisionStress verifies that inserting many dangling links in one
// transaction does not produce PRIMARY KEY collisions (guards against the
// time.Now().UnixNano() ID generator colliding in tight loops).
func TestResolveIDCollisionStress(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	srcID := createTestNote(t, g, ctx, "StressSource", "body")

	// 50 distinct unresolved targets in one body — all go into a single transaction.
	var linkParts []string
	for i := 0; i < 50; i++ {
		linkParts = append(linkParts, fmt.Sprintf("[[StressTarget%02d]]", i))
	}
	body := strings.Join(linkParts, " ")

	r := &Resolver{}
	for iter := 0; iter < 5; iter++ {
		outcomes, err := r.Resolve(ctx, g, srcID, body)
		if err != nil {
			t.Fatalf("iteration %d: Resolve failed: %v", iter, err)
		}
		if len(outcomes) != 50 {
			t.Fatalf("iteration %d: expected 50 outcomes, got %d", iter, len(outcomes))
		}
	}

	// After 5 re-indexes each producing 50 rows, only the last 50 should remain.
	if n := countDanglingForSrc(t, g, ctx, srcID); n != 50 {
		t.Errorf("expected 50 dangling rows, got %d", n)
	}
}

// TestStemCollisionTiebreak verifies deterministic tiebreak when two notes share the same stem.
// Per spec: oldest node id (lexicographic ascending) wins.
func TestStemCollisionTiebreak(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	srcID := createTestNote(t, g, ctx, "Source", "body")

	// Create two notes with the same title "Roadmap" but different IDs.
	// The resolver uses ORDER BY id ASC so the note with the smallest ID wins.
	dstID1 := createTestNote(t, g, ctx, "Roadmap", "first")
	dstID2 := createTestNote(t, g, ctx, "Roadmap", "second")

	// Neither has a note_paths entry — fallback to title lookup, which uses ORDER BY id ASC.
	r := &Resolver{}
	outcomes, err := r.Resolve(ctx, g, srcID, "[[Roadmap]]")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if !outcomes[0].Resolved {
		t.Fatal("expected Resolved=true (title match)")
	}

	// Determine which ID is lexicographically smaller — that's the expected winner.
	expectedDst := dstID1
	if dstID2 < dstID1 {
		expectedDst = dstID2
	}

	if n := countEdgesWithLabel(t, g, ctx, srcID, expectedDst, "links_to"); n != 1 {
		t.Errorf("expected edge to lexicographically-first dst (%s), got none; outcomes: %+v", expectedDst, outcomes)
	}
}
