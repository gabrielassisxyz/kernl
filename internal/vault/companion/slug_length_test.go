package companion

import (
	"context"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/google/uuid"
)

// TestPrepareNoteLongTitleStaysWritable guards the one input that is guaranteed to
// be long: a DA briefing takes its title from the whole capture body, and the real
// vault already holds titles of 900 to 1700 characters. A slug built from those
// overruns the 255-byte limit every Linux filesystem enforces on a path component,
// and the write happens after the transaction commits - so the node and its
// note_paths row survive while the file never appears, which is precisely the
// orphan this primitive exists to prevent.
func TestPrepareNoteLongTitleStaysWritable(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	vault := t.TempDir()

	title := "Briefing: " + strings.Repeat("palavra ", 60)
	noteID := uuid.Must(uuid.NewV7()).String()

	var f File
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		f, err = PrepareNote(tx, vault, "DA", NoteFrontmatter{ID: noteID, Title: title}, "body")
		return err
	}); err != nil {
		t.Fatalf("PrepareNote: %v", err)
	}

	name := f.relPath[strings.LastIndex(f.relPath, "/")+1:]
	if len(name) > 255 {
		t.Errorf("file name is %d bytes, over the 255-byte filesystem limit: %q", len(name), name)
	}
	if err := WriteFile(vault, f); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
