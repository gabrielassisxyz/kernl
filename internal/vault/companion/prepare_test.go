package companion

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/google/uuid"
)

// TestPrepareNoteConcurrentDistinctPaths guards the collision the primitive
// exists to prevent: N writers racing within the same second, all slugifying to
// the same name, must each land on a distinct path and a distinct note_paths
// row. The old timestamp slug made every writer of one second share a name, and
// the last os.WriteFile silently won.
func TestPrepareNoteConcurrentDistinctPaths(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	vault := t.TempDir()

	const n = 20
	var wg sync.WaitGroup
	paths := make([]string, n)
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			noteID := uuid.Must(uuid.NewV7()).String()
			var cf File
			err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
				var err error
				cf, err = PrepareNote(tx, vault, "notes", NoteFrontmatter{
					ID:    noteID,
					Title: "Same Title",
					Tags:  []string{"test"},
				}, "body")
				return err
			})
			if err != nil {
				errs[idx] = err
				return
			}
			if err := WriteFile(vault, cf); err != nil {
				errs[idx] = err
				return
			}
			paths[idx] = cf.RelPath()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}

	seen := map[string]bool{}
	for i, p := range paths {
		if p == "" {
			t.Fatalf("goroutine %d produced an empty path", i)
		}
		if seen[p] {
			t.Fatalf("duplicate path %q", p)
		}
		seen[p] = true
	}

	var rows int
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM note_paths`).Scan(&rows)
	}); err != nil {
		t.Fatalf("count note_paths: %v", err)
	}
	if rows != n {
		t.Fatalf("expected %d note_paths rows, got %d", n, rows)
	}
}

// TestPrepareNoteRollsBackWithTransaction proves the note_paths insert lives in
// the caller's transaction: a failure after the insert must leave neither the
// note node nor its note_paths row behind. A node committed without its row is
// exactly the orphan this primitive removes.
func TestPrepareNoteRollsBackWithTransaction(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	vault := t.TempDir()

	noteID := uuid.Must(uuid.NewV7()).String()
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if _, err := nodes.CreateNote(ctx, tx, nodes.Note{
			ID:    noteID,
			Title: "Rollback",
			Body:  "body",
		}, nodes.Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := PrepareNote(tx, vault, "notes", NoteFrontmatter{
			ID:    noteID,
			Title: "Rollback",
			Tags:  []string{"test"},
		}, "body"); err != nil {
			return err
		}
		return fmt.Errorf("forced failure after the note_paths insert")
	})
	if err == nil {
		t.Fatal("expected the forced failure to surface")
	}

	var nodeRows, pathRows int
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		if err := tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id = ?`, noteID).Scan(&nodeRows); err != nil {
			return err
		}
		return tx.QueryRow(`SELECT COUNT(*) FROM note_paths WHERE uuid = ?`, noteID).Scan(&pathRows)
	}); err != nil {
		t.Fatalf("read after rollback: %v", err)
	}
	if nodeRows != 0 {
		t.Errorf("note node survived the rollback: %d", nodeRows)
	}
	if pathRows != 0 {
		t.Errorf("note_paths row survived the rollback: %d", pathRows)
	}
}

// TestPrepareNoteFrontmatterRoundTripsNewlineColonQuote covers the frontmatter
// bug the primitive fixes: prep and ingest used fmt.Sprintf with %q, which is Go
// string escaping, not YAML. A title carrying a newline, a colon and a quote
// must survive a yaml.Unmarshal round trip as the same string.
func TestPrepareNoteFrontmatterRoundTripsNewlineColonQuote(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	vault := t.TempDir()

	title := "First line\nSecond: line with a \"quote\""
	noteID := uuid.Must(uuid.NewV7()).String()

	var cf File
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		cf, err = PrepareNote(tx, vault, "notes", NoteFrontmatter{
			ID:    noteID,
			Title: title,
			Tags:  []string{"test"},
		}, "Body.\n")
		return err
	}); err != nil {
		t.Fatalf("PrepareNote: %v", err)
	}

	var parsed struct {
		Title string `yaml:"title"`
	}
	if err := yaml.Unmarshal(frontmatterBlock(t, cf.bytes), &parsed); err != nil {
		t.Fatalf("frontmatter does not parse: %v\n%s", err, cf.bytes)
	}
	if parsed.Title != title {
		t.Errorf("title did not round-trip:\n  wrote %q\n  read  %q", title, parsed.Title)
	}
}
