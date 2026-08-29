package companion

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

type syncFixture struct {
	graph    *graph.Graph
	vault    string
	entityID string
	noteID   string
	file     File
}

func newSyncFixture(t *testing.T) syncFixture {
	t.Helper()
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)
	vault := t.TempDir()
	var entityID string
	var file File
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		entityID, err = nodes.CreateTask(ctx, tx, nodes.Task{Title: "Old title"}, nodes.Author{Name: "test"})
		if err != nil {
			return err
		}
		file, err = CreateTask(ctx, tx, vault, entityID, "tasks", "Old title", "Description", "task", "old")
		return err
	}); err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	if err := WriteFile(vault, file); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	var noteID string
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT uuid FROM note_paths WHERE path = ?`, file.relPath).Scan(&noteID)
	}); err != nil {
		t.Fatalf("read fixture note id: %v", err)
	}
	return syncFixture{graph: g, vault: vault, entityID: entityID, noteID: noteID, file: file}
}

func ptr[T any](v T) *T {
	return &v
}

func (f syncFixture) fullPath() string {
	return filepath.Join(f.vault, filepath.FromSlash(f.file.relPath))
}

func (f syncFixture) hash(t *testing.T) string {
	t.Helper()
	var hash string
	if err := f.graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT content_hash FROM note_paths WHERE uuid = ?`, f.noteID).Scan(&hash)
	}); err != nil {
		t.Fatalf("read content hash: %v", err)
	}
	return hash
}

func TestSyncTitleAndTagsRenderCanonicalFrontmatter(t *testing.T) {
	f := newSyncFixture(t)
	body := "Body survives byte-for-byte.\nSecond line.\n"
	manual := []byte("---\n# managed block\nid: " + f.noteID + "\ntitle: Hand edit\ndescription: Description\norigin: discarded\nauthor: discarded\ncustom: discarded\ntags:\n  - task\n  - manual\n---\n" + body)
	if err := os.WriteFile(f.fullPath(), manual, 0o644); err != nil {
		t.Fatal(err)
	}

	var titleFile File
	if err := f.graph.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		if err := nodes.SetTaskTitle(context.Background(), tx, f.entityID, "New title", nodes.Author{Name: "test"}); err != nil {
			return err
		}
		var err error
		titleFile, err = SyncTaskFields(context.Background(), tx, f.vault, f.entityID, nil, nil)
		return err
	}); err != nil {
		t.Fatalf("SyncTitle: %v", err)
	}
	if err := WriteFile(f.vault, titleFile); err != nil {
		t.Fatal(err)
	}

	var tagsFile File
	if err := f.graph.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		var err error
		tagsFile, err = SyncTaskFields(context.Background(), tx, f.vault, f.entityID, nil, ptr([]string{"task", "blocked"}))
		return err
	}); err != nil {
		t.Fatalf("SyncTags: %v", err)
	}
	if err := WriteFile(f.vault, tagsFile); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(f.fullPath())
	if err != nil {
		t.Fatal(err)
	}
	want := renderMarkdown(NoteFrontmatter{
		ID:          f.noteID,
		Title:       "New title",
		Description: "Description",
		Tags:        []string{"task", "blocked"},
	}, body)
	if !bytes.Equal(got, want) {
		t.Fatalf("sync output differs from Create writer:\ngot:\n%s\nwant:\n%s", got, want)
	}
	if f.hash(t) != reconcile.HashBytes(got) {
		t.Fatal("note_paths hash does not match rewritten bytes")
	}
}

func TestSyncTagsRestoresTitleFromTaskNode(t *testing.T) {
	f := newSyncFixture(t)
	manual := renderMarkdown(NoteFrontmatter{
		ID:          f.noteID,
		Title:       "Hand-edited title",
		Description: "Description",
		Tags:        []string{"task", "manual"},
	}, "Body.\n")
	if err := os.WriteFile(f.fullPath(), manual, 0o644); err != nil {
		t.Fatal(err)
	}
	var file File
	if err := f.graph.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		var err error
		file, err = SyncTaskFields(context.Background(), tx, f.vault, f.entityID, nil, ptr([]string{"task", "old"}))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(file.bytes, []byte("title: Old title\n")) {
		t.Fatalf("tag sync kept file title instead of task title:\n%s", file.bytes)
	}
}

func TestSyncMetadataRefusalsLeaveHashUntouched(t *testing.T) {
	t.Run("no vault", func(t *testing.T) {
		f := newSyncFixture(t)
		before := f.hash(t)
		// Standing in the vault, an empty root joins to a relative path that
		// resolves to the real file. Without this the subtest passes on the
		// file-unreadable refusal and never exercises the one it names.
		t.Chdir(f.vault)
		if err := f.graph.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
			titleFile, err := SyncTaskFields(context.Background(), tx, "", f.entityID, nil, nil)
			if err != nil {
				return err
			}
			tagsFile, err := SyncTaskFields(context.Background(), tx, "", f.entityID, nil, ptr([]string{"task"}))
			if titleFile.relPath != "" || tagsFile.relPath != "" {
				t.Fatal("no-vault refusal returned a file")
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if f.hash(t) != before {
			t.Fatal("no-vault refusal refreshed hash")
		}
	})

	t.Run("no companion", func(t *testing.T) {
		g := testutil.NewInMemoryTestGraph(t)
		var entityID string
		if err := g.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
			var err error
			entityID, err = nodes.CreateTask(context.Background(), tx, nodes.Task{Title: "Bare"}, nodes.Author{Name: "test"})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if err := g.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
			vault := t.TempDir()
			titleFile, err := SyncTaskFields(context.Background(), tx, vault, entityID, nil, nil)
			if err != nil {
				return err
			}
			tagsFile, err := SyncTaskFields(context.Background(), tx, vault, entityID, nil, ptr([]string{"task"}))
			if titleFile.relPath != "" || tagsFile.relPath != "" {
				t.Fatal("no-companion refusal returned a file")
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("entity is not a live task", func(t *testing.T) {
		f := newSyncFixture(t)
		before := f.hash(t)
		if err := f.graph.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
			if _, err := tx.Exec(`UPDATE nodes SET deleted_at = 'now' WHERE id = ?`, f.entityID); err != nil {
				return err
			}
			file, err := SyncTaskFields(context.Background(), tx, f.vault, f.entityID, nil, ptr([]string{"task"}))
			if err != nil {
				return err
			}
			if file.relPath != "" {
				t.Fatal("deleted-task refusal returned a file")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if f.hash(t) != before {
			t.Fatal("deleted-task refusal refreshed hash")
		}
	})

	for _, tc := range []struct {
		name    string
		content func(syncFixture) []byte
	}{
		{"file gone", nil},
		{"frontmatter unreadable", func(f syncFixture) []byte { return []byte("---\ntitle: [\n---\nBody\n") }},
		{"wrong note id", func(f syncFixture) []byte { return []byte("---\nid: another-note\ntitle: Old\n---\nBody\n") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSyncFixture(t)
			before := f.hash(t)
			var expected []byte
			if tc.content == nil {
				if err := os.Remove(f.fullPath()); err != nil {
					t.Fatal(err)
				}
			} else {
				expected = tc.content(f)
				if err := os.WriteFile(f.fullPath(), expected, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.graph.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
				titleFile, err := SyncTaskFields(context.Background(), tx, f.vault, f.entityID, nil, nil)
				if err != nil {
					return err
				}
				tagsFile, err := SyncTaskFields(context.Background(), tx, f.vault, f.entityID, nil, ptr([]string{"task"}))
				if titleFile.relPath != "" || tagsFile.relPath != "" {
					t.Fatal("refusal returned a file")
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if f.hash(t) != before {
				t.Fatal("refusal refreshed hash")
			}
			if expected != nil {
				got, err := os.ReadFile(f.fullPath())
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, expected) {
					t.Fatal("refusal changed file bytes")
				}
			}
		})
	}
}

func TestSyncMetadataMatchingValueIsNoOp(t *testing.T) {
	f := newSyncFixture(t)
	before := f.hash(t)
	if err := f.graph.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		titleFile, err := SyncTaskFields(context.Background(), tx, f.vault, f.entityID, nil, nil)
		if err != nil {
			return err
		}
		tagsFile, err := SyncTaskFields(context.Background(), tx, f.vault, f.entityID, nil, ptr([]string{"task", "old"}))
		if titleFile.relPath != "" || tagsFile.relPath != "" {
			t.Fatal("matching value returned a file")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if f.hash(t) != before {
		t.Fatal("matching value refreshed hash")
	}
}

func TestSyncMetadataHashRollsBackWithTransaction(t *testing.T) {
	f := newSyncFixture(t)
	before := f.hash(t)
	err := f.graph.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		if err := nodes.SetTaskTitle(context.Background(), tx, f.entityID, "Rolled back", nodes.Author{Name: "test"}); err != nil {
			return err
		}
		file, err := SyncTaskFields(context.Background(), tx, f.vault, f.entityID, nil, nil)
		if err != nil {
			return err
		}
		if file.relPath == "" {
			t.Fatal("sync returned no file, so the rollback below proves nothing")
		}
		var pending string
		if err := tx.QueryRow(`SELECT content_hash FROM note_paths WHERE uuid = ?`, f.noteID).Scan(&pending); err != nil {
			return err
		}
		if pending == before {
			t.Fatal("hash was never written inside the transaction")
		}
		return os.ErrInvalid
	})
	if err == nil {
		t.Fatal("expected forced rollback error")
	}
	if f.hash(t) != before {
		t.Fatal("hash survived rolled-back transaction")
	}
}

func TestSyncMetadataRestoresMissingFrontmatterID(t *testing.T) {
	f := newSyncFixture(t)
	// Every mirrored field already agrees, so only the absent id can trigger the
	// rewrite. Without it reconcile has nothing to match the node by and builds a
	// duplicate on the next cold start.
	if err := os.WriteFile(f.fullPath(), []byte("---\ntitle: Old title\ndescription: Description\ntags:\n    - task\n    - old\n---\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var file File
	if err := f.graph.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		var err error
		file, err = SyncTaskFields(context.Background(), tx, f.vault, f.entityID, nil, ptr([]string{"task", "old"}))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(file.bytes, []byte("id: "+f.noteID+"\n")) {
		t.Fatalf("sync left the file without its note id:\n%s", file.bytes)
	}
}
