package nodes

import (
	"context"
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
