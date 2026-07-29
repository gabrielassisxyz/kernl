package bookmarks_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/bookmarks"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func TestImportPocket(t *testing.T) {
	db := testutil.NewInMemoryTestGraph(t)
	vault := t.TempDir()

	html := `<!DOCTYPE html>
<html><body>
<ul>
<li><a href="https://example.com" time_added="123" tags="a,b">Example Title</a></li>
<li><a href="https://example.org">Org Title</a></li>
</ul>
</body></html>`

	err := db.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		author := nodes.Author{Name: "tester"}
		count, companions, err := bookmarks.ImportPocket(context.Background(), tx, vault, strings.NewReader(html), author)
		if err != nil {
			return err
		}
		if count != 2 {
			t.Errorf("expected 2 bookmarks, got %d", count)
		}
		// One companion note per imported bookmark: an import that skipped them
		// would put a thousand links in the graph db and none in the backed-up
		// markdown.
		if len(companions) != count {
			t.Errorf("got %d companion files for %d bookmarks", len(companions), count)
		}
		return nil
	})

	if err != nil {
		t.Fatal(err)
	}
}

func TestImportPinboard(t *testing.T) {
	db := testutil.NewInMemoryTestGraph(t)
	vault := t.TempDir()

	jsonStr := `[
		{"href":"https://example.com","description":"Example","extended":"desc","tags":"t1 t2"},
		{"href":"https://example.org","description":"Org","extended":"","tags":""}
	]`

	err := db.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		author := nodes.Author{Name: "tester"}
		count, companions, err := bookmarks.ImportPinboard(context.Background(), tx, vault, strings.NewReader(jsonStr), author)
		if err != nil {
			return err
		}
		if count != 2 {
			t.Errorf("expected 2 bookmarks, got %d", count)
		}
		if len(companions) != count {
			t.Errorf("got %d companion files for %d bookmarks", len(companions), count)
		}
		return nil
	})

	if err != nil {
		t.Fatal(err)
	}
}
