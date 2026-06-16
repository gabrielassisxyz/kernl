package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// fakeSpec is a minimal NodeSpec for testing.
type fakeSpec struct {
	meta  Meta
	attrs json.RawMessage
	tags  []string
	fts   FTSFields
}

func (f fakeSpec) Meta() *Meta          { return &f.meta }
func (f fakeSpec) NodeAttrs() []byte    { return f.attrs }
func (f fakeSpec) NodeTags() []string   { return f.tags }
func (f fakeSpec) FTSFields() FTSFields { return f.fts }

// diffableFakeSpec extends fakeSpec with DiffBody.
type diffableFakeSpec struct {
	fakeSpec
}

func (d diffableFakeSpec) DiffBody(prev NodeSpec) []byte {
	var oldTitle string
	switch p := prev.(type) {
	case diffableFakeSpec:
		oldTitle = p.fts.Title
	default:
		// prev may be a concrete Note from the chokepoint; fall back
		oldTitle = ""
		if fts := prev.FTSFields(); fts != (FTSFields{}) {
			oldTitle = fts.Title
		}
	}
	return []byte(fmt.Sprintf(`{"old_title":"%s","new_title":"%s"}`, oldTitle, d.fts.Title))
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestCreateNodeWritesOneRevision verifies that a newly created node
// writes exactly one revision row with a non-empty snapshot diff.
// Label: kernl-omr
func TestCreateNodeWritesOneRevision(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	spec := fakeSpec{
		meta:  Meta{ID: "node-omr"},
		attrs: json.RawMessage(`{"key":"value"}`),
		tags:  []string{"tag-a", "tag-b"},
		fts:   FTSFields{Title: "My Node", Body: "body text", Tags: "tag-a,tag-b"},
	}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := createNode(ctx, tx, "test", spec, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("createNode: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		if err := tx.QueryRow("SELECT COUNT(*) FROM revisions WHERE node_id = ?", "node-omr").Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("expected 1 revision, got %d", count)
		}

		var diff string
		if err := tx.QueryRow("SELECT diff FROM revisions WHERE node_id = ?", "node-omr").Scan(&diff); err != nil {
			return err
		}
		if diff == "" {
			return fmt.Errorf("expected non-empty diff (snapshot JSON)")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestEmptyAuthorRejected verifies that createNode with an empty author
// returns graph.ErrAuthorRequired.
// Label: kernl-srrp
func TestEmptyAuthorRejected(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	spec := fakeSpec{
		meta: Meta{ID: "node-srrp"},
		fts:  FTSFields{Title: "no author"},
	}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := createNode(ctx, tx, "test", spec, Author{})
		return err
	})
	if err == nil {
		t.Fatal("expected error for empty author, got nil")
	}
	if !errors.Is(err, graph.ErrAuthorRequired) {
		t.Fatalf("expected graph.ErrAuthorRequired, got %v", err)
	}
}

// TestEmptyTitleAllowed verifies that creating a node with an empty title
// succeeds (no validation rejects empty titles).
// Label: kernl-foa5
func TestEmptyTitleAllowed(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	spec := fakeSpec{
		meta: Meta{ID: "node-foa5"},
		fts:  FTSFields{Title: "", Body: "has body"},
	}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := createNode(ctx, tx, "test", spec, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("createNode with empty title should succeed: %v", err)
	}
}

// TestUpdateNodeReplacesFTSContent verifies that updating a node replaces
// the FTS index entries — old title is gone, new title is indexed.
// Label: kernl-drf
func TestUpdateNodeReplacesFTSContent(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	oldSpec := fakeSpec{
		meta:  Meta{ID: "node-drf"},
		attrs: json.RawMessage(`{"v":1}`),
		fts:   FTSFields{Title: "old", Body: "old body"},
	}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := createNode(ctx, tx, "test", oldSpec, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("createNode: %v", err)
	}

	newSpec := fakeSpec{
		meta:  Meta{ID: "node-drf"},
		attrs: json.RawMessage(`{"v":2}`),
		fts:   FTSFields{Title: "new", Body: "new body"},
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return updateNode(ctx, tx, newSpec, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("updateNode: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var oldCount int
		if err := tx.QueryRow("SELECT COUNT(*) FROM nodes_fts WHERE title MATCH 'old'").Scan(&oldCount); err != nil {
			return err
		}
		if oldCount != 0 {
			return fmt.Errorf("expected 0 rows matching 'old', got %d", oldCount)
		}

		var newCount int
		if err := tx.QueryRow("SELECT COUNT(*) FROM nodes_fts WHERE title MATCH 'new'").Scan(&newCount); err != nil {
			return err
		}
		if newCount != 1 {
			return fmt.Errorf("expected 1 row matching 'new', got %d", newCount)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestDiffableNodeStoresDiff verifies that updating a DiffableNode
// stores a computed diff in the revision row.
// When DiffableNode support is active, the diff contains the DiffBody output
// (not a full snapshot).
// Label: kernl-kos1
func TestDiffableNodeStoresDiff(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	oldSpec := diffableFakeSpec{
		fakeSpec: fakeSpec{
			meta:  Meta{ID: "node-kos1"},
			fts:   FTSFields{Title: "old title"},
			attrs: json.RawMessage(`{"body":"old body"}`),
		},
	}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := createNode(ctx, tx, "test", oldSpec, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("createNode: %v", err)
	}

	newSpec := diffableFakeSpec{
		fakeSpec: fakeSpec{
			meta:  Meta{ID: "node-kos1"},
			fts:   FTSFields{Title: "new title"},
			attrs: json.RawMessage(`{"body":"new body"}`),
		},
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return updateNode(ctx, tx, newSpec, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("updateNode: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var diff string
		if err := tx.QueryRow(
			"SELECT diff FROM revisions WHERE node_id = ? ORDER BY created_at DESC, id DESC LIMIT 1",
			"node-kos1",
		).Scan(&diff); err != nil {
			return err
		}
		if diff == "" {
			return fmt.Errorf("expected non-NULL diff for DiffableNode update")
		}
		if !json.Valid([]byte(diff)) {
			return fmt.Errorf("expected valid JSON diff, got: %s", diff)
		}
		// DiffableNode is wired up — verify it contains DiffBody output.
		if !strings.Contains(diff, `"old_title"`) || !strings.Contains(diff, `"new_title"`) {
			return fmt.Errorf("expected DiffBody output in diff, got: %s", diff)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestNonDiffableStillStoresSnapshot verifies that non-DiffableNode types
// (Capture, Decision, etc.) still store full snapshot JSON after the
// DiffableNode branch is added to updateNode.
// Label: kernl-char-snapshot
func TestNonDiffableStillStoresSnapshot(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	oldSpec := fakeSpec{
		meta:  Meta{ID: "node-char1"},
		fts:   FTSFields{Title: "original title", Body: "original body"},
		attrs: json.RawMessage(`{"body":"original body"}`),
		tags:  []string{"alpha"},
	}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := createNode(ctx, tx, "test", oldSpec, Author{Name: "creator"})
		return err
	})
	if err != nil {
		t.Fatalf("createNode: %v", err)
	}

	newSpec := fakeSpec{
		meta:  Meta{ID: "node-char1"},
		fts:   FTSFields{Title: "updated title", Body: "updated body"},
		attrs: json.RawMessage(`{"body":"updated body"}`),
		tags:  []string{"beta"},
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return updateNode(ctx, tx, newSpec, Author{Name: "updater"})
	})
	if err != nil {
		t.Fatalf("updateNode: %v", err)
	}

	// Verify the latest revision stores snapshot JSON (title/attrs/tags keys).
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var diff string
		if err := tx.QueryRow(
			"SELECT diff FROM revisions WHERE node_id = ? ORDER BY created_at DESC, id DESC LIMIT 1",
			"node-char1",
		).Scan(&diff); err != nil {
			return err
		}

		var snap struct {
			Title string   `json:"title"`
			Attrs string   `json:"attrs"`
			Tags  []string `json:"tags"`
		}
		if err := json.Unmarshal([]byte(diff), &snap); err != nil {
			return fmt.Errorf("expected snapshot JSON, got: %s (%v)", diff, err)
		}
		if snap.Title != "updated title" {
			t.Errorf("snapshot title = %q, want %q", snap.Title, "updated title")
		}
		if !strings.Contains(snap.Attrs, "updated body") {
			t.Errorf("snapshot attrs missing body: %s", snap.Attrs)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestNoteUpdateStoresLineDiff verifies that updating a Note stores a
// line-diff payload instead of a full snapshot.
// Label: kernl-note-line-diff
func TestNoteUpdateStoresLineDiff(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateNote(ctx, tx, Note{
			Title: "Diff Test",
			Body:  "line one\nline two\nline three",
			Tags:  []string{"draft"},
		}, Author{Name: "creator"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateNote(ctx, tx, Note{
			ID:    id,
			Title: "Diff Test",
			Body:  "line one\nline two modified\nline three",
			Tags:  []string{"draft"},
		}, Author{Name: "updater"})
	})
	if err != nil {
		t.Fatalf("UpdateNote: %v", err)
	}

	// Verify the update revision contains a line-diff payload.
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var diff string
		if err := tx.QueryRow(
			"SELECT diff FROM revisions WHERE node_id = ? ORDER BY created_at DESC, id DESC LIMIT 1",
			id,
		).Scan(&diff); err != nil {
			return err
		}

		// Line-diff payload has "ops" key; snapshots do not.
		var payload struct {
			Ops []json.RawMessage `json:"ops"`
		}
		if err := json.Unmarshal([]byte(diff), &payload); err != nil {
			return fmt.Errorf("unmarshal diff: %v", err)
		}
		if len(payload.Ops) == 0 {
			return fmt.Errorf("expected line-diff ops in payload, got: %s", diff)
		}
		// The diff should be smaller than a full snapshot.
		fullSnap, _ := snapshotJSON("Diff Test",
			`{"body":"line one\nline two modified\nline three","origin":"","author":""}`,
			[]string{"draft"})
		if len(diff) >= len(fullSnap) {
			return fmt.Errorf("line-diff (%d bytes) should be smaller than snapshot (%d bytes)", len(diff), len(fullSnap))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestNoteEmptyBodyDiff verifies empty→nonempty and nonempty→empty body edits.
func TestNoteEmptyBodyDiff(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateNote(ctx, tx, Note{
			Title: "Empty Test",
			Body:  "",
		}, Author{Name: "creator"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	// Empty → nonempty
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateNote(ctx, tx, Note{
			ID:    id,
			Title: "Empty Test",
			Body:  "now has content",
		}, Author{Name: "updater"})
	})
	if err != nil {
		t.Fatalf("UpdateNote (empty→nonempty): %v", err)
	}

	// Nonempty → empty
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateNote(ctx, tx, Note{
			ID:    id,
			Title: "Empty Test",
			Body:  "",
		}, Author{Name: "updater"})
	})
	if err != nil {
		t.Fatalf("UpdateNote (nonempty→empty): %v", err)
	}

	// Both updates should have line-diff payloads.
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		rows, err := tx.Query(
			"SELECT diff FROM revisions WHERE node_id = ? ORDER BY created_at ASC",
			id,
		)
		if err != nil {
			return err
		}
		defer rows.Close()

		var diffs []string
		for rows.Next() {
			var d string
			if err := rows.Scan(&d); err != nil {
				return err
			}
			diffs = append(diffs, d)
		}
		if len(diffs) != 3 {
			return fmt.Errorf("expected 3 revisions, got %d", len(diffs))
		}

		// Second and third diffs should be line-diffs.
		for i, d := range diffs[1:] {
			var payload struct {
				Ops []json.RawMessage `json:"ops"`
			}
			if err := json.Unmarshal([]byte(d), &payload); err != nil {
				return fmt.Errorf("revision %d: %v", i+2, err)
			}
			if len(payload.Ops) == 0 {
				return fmt.Errorf("revision %d: expected line-diff ops", i+2)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestNoteAuthorAttributionOnDiff verifies author is preserved on revisions
// that use line-diff storage.
func TestNoteAuthorAttributionOnDiff(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateNote(ctx, tx, Note{Title: "Auth Test", Body: "v1"}, Author{Name: "creator"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateNote(ctx, tx, Note{ID: id, Title: "Auth Test", Body: "v2"}, Author{Name: "updater"})
	})
	if err != nil {
		t.Fatalf("UpdateNote: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var author string
		if err := tx.QueryRow(
			"SELECT author FROM revisions WHERE node_id = ? ORDER BY created_at DESC, id DESC LIMIT 1",
			id,
		).Scan(&author); err != nil {
			return err
		}
		if author != "updater" {
			t.Errorf("expected author 'updater', got %q", author)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestDeleteNodePreservesRevisionHistory verifies that deleting a node
// preserves all revision history (create + update + tombstone) and cascades
// to edges and node_tags.
// Label: kernl-gxp
func TestDeleteNodePreservesRevisionHistory(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	spec := fakeSpec{
		meta: Meta{ID: "node-gxp"},
		tags: []string{"tag-gxp"},
		fts:  FTSFields{Title: "delete me"},
	}

	// 1. Create — writes 1 revision
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := createNode(ctx, tx, "test", spec, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("createNode: %v", err)
	}

	// 2. Update — writes 1 more revision (total 2)
	updatedSpec := fakeSpec{
		meta: Meta{ID: "node-gxp"},
		fts:  FTSFields{Title: "updated title"},
	}
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return updateNode(ctx, tx, updatedSpec, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("updateNode: %v", err)
	}

	// 3. Insert a fake edge referencing this node for cascade verification
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		// Need a real target node for FK constraint
		_, execErr := tx.Exec(
			`INSERT INTO nodes(id, type, title, attrs) VALUES (?, ?, ?, ?)`,
			"node-gxp-target", "test", "target", "{}",
		)
		if execErr != nil {
			return execErr
		}
		_, execErr = tx.Exec(
			"INSERT INTO edges (id, src, dst, label) VALUES (?, ?, ?, ?)",
			"edge-gxp", "node-gxp", "node-gxp-target", "tests",
		)
		return execErr
	})
	if err != nil {
		t.Fatalf("insert edge: %v", err)
	}

	// 4. Delete — writes tombstone revision (total 3)
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return deleteNode(ctx, tx, "node-gxp", Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("deleteNode: %v", err)
	}

	// 5. Verify: 3 revisions preserved (survive as SET NULL tombstones)
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var revCount int
		if err := tx.QueryRow("SELECT COUNT(*) FROM revisions").Scan(&revCount); err != nil {
			return err
		}
		if revCount != 3 {
			return fmt.Errorf("expected 3 revisions (create+update+tombstone), got %d", revCount)
		}

		// Node no longer in nodes table
		var nodeCount int
		if err := tx.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = ?", "node-gxp").Scan(&nodeCount); err != nil {
			return err
		}
		if nodeCount != 0 {
			return fmt.Errorf("expected node to be deleted from nodes table, got %d rows", nodeCount)
		}

		// Edge cascaded
		var edgeCount int
		if err := tx.QueryRow("SELECT COUNT(*) FROM edges WHERE id = ?", "edge-gxp").Scan(&edgeCount); err != nil {
			return err
		}
		if edgeCount != 0 {
			return fmt.Errorf("expected edge to be cascade-deleted, got %d rows", edgeCount)
		}

		// node_tags cascaded
		var tagCount int
		if err := tx.QueryRow("SELECT COUNT(*) FROM node_tags WHERE node_id = ?", "node-gxp").Scan(&tagCount); err != nil {
			return err
		}
		if tagCount != 0 {
			return fmt.Errorf("expected node_tags to be cascade-deleted, got %d rows", tagCount)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestDeleteTombstonePreservesAuthor verifies that the tombstone revision
// records the deleting agent's author string.
// Label: kernl-wy60
func TestDeleteTombstonePreservesAuthor(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	spec := fakeSpec{
		meta: Meta{ID: "node-wy60"},
		fts:  FTSFields{Title: "tombstone author test"},
	}

	// Create with human author
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := createNode(ctx, tx, "test", spec, Author{Name: "human:me"})
		return err
	})
	if err != nil {
		t.Fatalf("createNode: %v", err)
	}

	// Delete with agent author
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return deleteNode(ctx, tx, "node-wy60", AuthorAgent("kimi"))
	})
	if err != nil {
		t.Fatalf("deleteNode: %v", err)
	}

	// Verify tombstone author
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var author string
		if err := tx.QueryRow(
			"SELECT author FROM revisions ORDER BY created_at DESC, id DESC LIMIT 1",
		).Scan(&author); err != nil {
			return err
		}
		if author != "agent:kimi" {
			return fmt.Errorf("expected tombstone author 'agent:kimi', got %q", author)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
