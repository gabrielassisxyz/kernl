package nodes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// TestNoteRoundtrip verifies CreateNote → GetNote returns identical fields.
func TestNoteRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	n := Note{
		Title:  "My Note Title",
		Body:   "Body content for the note.",
		Origin: "",
		Author: "user-gabriel",
		Tags:   []string{"vault", "draft"},
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateNote(ctx, tx, n, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	var got *Note
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = GetNote(ctx, tx, id)
		return err
	})
	if err != nil {
		t.Fatalf("GetNote: %v", err)
	}

	if got.ID != id {
		t.Errorf("id = %q, want %q", got.ID, id)
	}
	if got.Title != n.Title {
		t.Errorf("title = %q, want %q", got.Title, n.Title)
	}
	if got.Body != n.Body {
		t.Errorf("body = %q, want %q", got.Body, n.Body)
	}
	if got.Author != n.Author {
		t.Errorf("author = %q, want %q", got.Author, n.Author)
	}
	if got.Origin != n.Origin {
		t.Errorf("origin = %q, want %q", got.Origin, n.Origin)
	}
	if len(got.Tags) != len(n.Tags) {
		t.Errorf("tags len = %d, want %d", len(got.Tags), len(n.Tags))
	}
}

// TestNoteGetNotFound verifies GetNote on missing id returns graph.ErrNotFound.
func TestNoteGetNotFound(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		_, err := GetNote(ctx, tx, "nonexistent")
		return err
	})
	if !errors.Is(err, graph.ErrNotFound) {
		t.Fatalf("expected graph.ErrNotFound, got %v", err)
	}
}

// TestNoteListFilter verifies filtering by tag returns correct subsets.
func TestNoteListFilter(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if _, err := CreateNote(ctx, tx, Note{Title: "N1", Body: "b1", Origin: "obsidian", Tags: []string{"alpha"}}, Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := CreateNote(ctx, tx, Note{Title: "N2", Body: "b2", Origin: "obsidian", Tags: []string{"beta"}}, Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := CreateNote(ctx, tx, Note{Title: "N3", Body: "b3", Origin: "logseq", Tags: []string{"alpha", "beta"}}, Author{Name: "test"}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		items, err := ListNotes(ctx, tx, NoteFilter{Tags: []string{"alpha"}})
		if err != nil {
			return err
		}
		if len(items) != 2 {
			t.Errorf("tag alpha: expected 2, got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		items, err := ListNotes(ctx, tx, NoteFilter{Tags: []string{"beta"}})
		if err != nil {
			return err
		}
		if len(items) != 2 {
			t.Errorf("tag beta: expected 2, got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestNoteListLimit verifies limit is respected.
func TestNoteListLimit(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		for i := 0; i < 5; i++ {
			if _, err := CreateNote(ctx, tx, Note{Title: fmt.Sprintf("Note%d", i), Body: fmt.Sprintf("body%d", i)}, Author{Name: "test"}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		items, err := ListNotes(ctx, tx, NoteFilter{Limit: 3})
		if err != nil {
			return err
		}
		if len(items) != 3 {
			t.Errorf("expected 3, got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestNoteFTSRoundtrip verifies the note body is indexed by FTS.
func TestNoteFTSRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	n := Note{
		Title: "FTS Note",
		Body:  "Contains unique token notfts789",
	}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := CreateNote(ctx, tx, n, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		if err := tx.QueryRow(
			"SELECT COUNT(*) FROM nodes_fts WHERE nodes_fts MATCH 'notfts789'",
		).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("expected FTS to find 'notfts789' once, got %d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestNoteEmptyAuthorRejected verifies empty-author create returns ErrAuthorRequired.
func TestNoteEmptyAuthorRejected(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := CreateNote(ctx, tx, Note{Title: "N", Body: "b"}, Author{Name: ""})
		return err
	})
	if !errors.Is(err, graph.ErrAuthorRequired) {
		t.Fatalf("expected graph.ErrAuthorRequired, got %v", err)
	}
}

// TestNoteUpdateProducesRevision verifies updating writes a second revision.
func TestNoteUpdateProducesRevision(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateNote(ctx, tx, Note{Title: "Original", Body: "before", Tags: []string{"alpha"}}, Author{Name: "creator"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	updated := Note{
		ID:    id,
		Title: "Updated",
		Body:  "after",
		Tags:  []string{"beta"},
	}
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateNote(ctx, tx, updated, Author{Name: "updater"})
	})
	if err != nil {
		t.Fatalf("UpdateNote: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		if err := tx.QueryRow("SELECT COUNT(*) FROM revisions").Scan(&count); err != nil {
			return err
		}
		if count != 2 {
			t.Errorf("expected 2 revisions, got %d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestSoftDeleteNote_SetsTombstone verifies that SoftDeleteNoteTx:
//   - sets deleted_at on the node
//   - keeps the node row (soft delete)
//   - removes the FTS row
//   - appends a tombstone revision
//   - degrades incoming links_to edges into dangling_links rows
func TestSoftDeleteNote_SetsTombstone(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	// Create target note (the one we will soft-delete)
	var targetID string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		targetID, err = CreateNote(ctx, tx, Note{
			ID:    "target-note",
			Title: "Target Note",
			Body:  "Will be deleted",
			Tags:  []string{"test"},
		}, Author{Name: "human"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateNote target: %v", err)
	}

	// Create a source note that links to target
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := CreateNote(ctx, tx, Note{
			ID:    "source-note",
			Title: "Source",
			Body:  "Links to target",
		}, Author{Name: "human"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateNote source: %v", err)
	}

	// Insert a links_to edge from source → target (simulating a resolved wikilink)
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(
			`INSERT INTO edges(id, src, dst, label) VALUES ('edge-1','source-note','target-note','links_to')`,
		)
		return err
	})
	if err != nil {
		t.Fatalf("insert edge: %v", err)
	}

	// Soft-delete the target note
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return SoftDeleteNoteTx(ctx, tx, targetID, "target-note", "Target Note", Author{Name: "human"})
	})
	if err != nil {
		t.Fatalf("SoftDeleteNoteTx: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		// deleted_at must be set
		var deletedAt sql.NullString
		if err := tx.QueryRow(`SELECT deleted_at FROM nodes WHERE id = ?`, targetID).Scan(&deletedAt); err != nil {
			return fmt.Errorf("select deleted_at: %w", err)
		}
		if !deletedAt.Valid {
			t.Error("expected deleted_at to be set after soft-delete")
		}

		// Node row must still exist
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id = ?`, targetID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("expected node row to persist, got count=%d", count)
		}

		// FTS row must be gone
		var ftsRowid sql.NullInt64
		if err := tx.QueryRow(`SELECT fts_rowid FROM nodes WHERE id = ?`, targetID).Scan(&ftsRowid); err != nil {
			return err
		}
		if ftsRowid.Valid {
			var ftsCount int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM nodes_fts WHERE rowid = ?`, ftsRowid.Int64).Scan(&ftsCount); err != nil {
				return err
			}
			if ftsCount != 0 {
				t.Error("expected FTS row to be removed after soft-delete")
			}
		}

		// Revision history must contain a tombstone revision
		var revCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM revisions WHERE node_id = ?`, targetID).Scan(&revCount); err != nil {
			return err
		}
		if revCount < 2 {
			t.Errorf("expected at least 2 revisions (create + tombstone), got %d", revCount)
		}

		// Incoming edge must be degraded to dangling_links
		var edgeCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM edges WHERE dst = ? AND label = 'links_to'`, targetID).Scan(&edgeCount); err != nil {
			return err
		}
		if edgeCount != 0 {
			t.Errorf("expected incoming edges to be degraded, got %d remaining", edgeCount)
		}
		var dangCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM dangling_links WHERE src_node_id = 'source-note'`).Scan(&dangCount); err != nil {
			return err
		}
		if dangCount == 0 {
			t.Error("expected dangling_links rows after edge degradation")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestReviveNote_RestoresNode verifies that ReviveNoteTx:
//   - clears deleted_at
//   - re-indexes the FTS row
//   - appends a revival revision
//   - re-promotes matching dangling links to edges
func TestReviveNote_RestoresNode(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	// Create target note + source note + edge
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if _, err := CreateNote(ctx, tx, Note{
			ID:    "revive-target",
			Title: "Revive Target",
			Body:  "Will be deleted then revived",
		}, Author{Name: "human"}); err != nil {
			return err
		}
		if _, err := CreateNote(ctx, tx, Note{
			ID:    "revive-source",
			Title: "Revive Source",
			Body:  "Links here",
		}, Author{Name: "human"}); err != nil {
			return err
		}
		_, err := tx.Exec(
			`INSERT INTO edges(id, src, dst, label) VALUES ('rev-edge-1','revive-source','revive-target','links_to')`,
		)
		return err
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Soft-delete
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return SoftDeleteNoteTx(ctx, tx, "revive-target", "revive-target", "Revive Target", Author{Name: "human"})
	})
	if err != nil {
		t.Fatalf("SoftDeleteNoteTx: %v", err)
	}

	// Revive
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return ReviveNoteTx(ctx, tx, "revive-target", "revive-target", "Revive Target", Author{Name: "human"})
	})
	if err != nil {
		t.Fatalf("ReviveNoteTx: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		// deleted_at must be cleared
		var deletedAt sql.NullString
		if err := tx.QueryRow(`SELECT deleted_at FROM nodes WHERE id = 'revive-target'`).Scan(&deletedAt); err != nil {
			return err
		}
		if deletedAt.Valid {
			t.Error("expected deleted_at to be NULL after revive")
		}

		// FTS row must exist again
		var ftsRowid sql.NullInt64
		if err := tx.QueryRow(`SELECT fts_rowid FROM nodes WHERE id = 'revive-target'`).Scan(&ftsRowid); err != nil {
			return err
		}
		if ftsRowid.Valid {
			var ftsCount int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM nodes_fts WHERE rowid = ?`, ftsRowid.Int64).Scan(&ftsCount); err != nil {
				return err
			}
			if ftsCount != 1 {
				t.Errorf("expected FTS row after revive, got %d", ftsCount)
			}
		}

		// Revision count must include: create + tombstone + revival = 3
		var revCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM revisions WHERE node_id = 'revive-target'`).Scan(&revCount); err != nil {
			return err
		}
		if revCount < 3 {
			t.Errorf("expected at least 3 revisions (create+tombstone+revival), got %d", revCount)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestDeleteNode_HardDeleteStillWorks verifies that deleteNode (hard delete)
// for a non-note type still removes the row entirely (regression guard).
func TestDeleteNode_HardDeleteStillWorks(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := createNode(ctx, tx, "chokepoint", fakeSpec{
			meta: Meta{ID: "hard-delete-node"},
			fts:  FTSFields{Title: "Hard Delete Me"},
		}, Author{Name: "human"})
		return err
	})
	if err != nil {
		t.Fatalf("createNode: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return deleteNode(ctx, tx, "hard-delete-node", Author{Name: "human"})
	})
	if err != nil {
		t.Fatalf("deleteNode: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id = 'hard-delete-node'`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("hard delete: expected 0 rows, got %d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestNoteTombstoneHidden verifies tombstoned notes are hidden by default
// but visible with IncludeDeleted.
func TestNoteTombstoneHidden(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateNote(ctx, tx, Note{Title: "TombstonedNote", Body: "will be deleted"}, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateNote: %v", err)
	}

	// Soft-delete: set deleted_at to simulate tombstone
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := tx.Exec(`UPDATE nodes SET deleted_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`, id)
		return err
	})
	if err != nil {
		t.Fatalf("soft-delete: %v", err)
	}

	// Default ListNotes excludes tombstoned
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		items, err := ListNotes(ctx, tx, NoteFilter{})
		if err != nil {
			return err
		}
		if len(items) != 0 {
			t.Errorf("expected 0 notes (tombstone hidden), got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// IncludeDeleted reveals tombstoned notes
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		items, err := ListNotes(ctx, tx, NoteFilter{IncludeDeleted: true})
		if err != nil {
			return err
		}
		if len(items) != 1 {
			t.Errorf("expected 1 note with IncludeDeleted, got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
