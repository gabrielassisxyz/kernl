package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/gabrielassisxyz/kernl/internal/vault/layout"
)

// bookmarkCompanionApp is an app with a real graph and a temp vault, so a
// companion note's file can be asserted on disk.
func bookmarkCompanionApp(t *testing.T) (*app.App, string) {
	t.Helper()
	cfg := &config.Config{}
	vault := t.TempDir()
	cfg.Vault.Root = vault
	a := &app.App{Config: cfg, Backend: backend.NewBdCliBackend("/tmp/test")}
	a.Graph = testutil.NewInMemoryTestGraph(t)
	return a, vault
}

// companionFilesUnder lists the companion markdown the bookmarks folder holds.
func companionFilesUnder(t *testing.T, vault string) []string {
	t.Helper()
	dir := filepath.Join(vault, filepath.FromSlash(layout.BookmarksFolder))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("ReadDir %s: %v", dir, err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			out = append(out, e.Name())
		}
	}
	return out
}

// countBookmarkCompanions counts describes edges pointing at a bookmark.
func countBookmarkCompanions(t *testing.T, a *app.App) int {
	t.Helper()
	var n int
	if err := a.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		return tx.QueryRow(`
			SELECT COUNT(*) FROM edges e
			JOIN nodes note ON note.id = e.src AND note.type = 'note' AND note.deleted_at IS NULL
			JOIN nodes bm ON bm.id = e.dst AND bm.type = 'bookmark' AND bm.deleted_at IS NULL
			WHERE e.label = 'describes'`).Scan(&n)
	}); err != nil {
		t.Fatalf("count companions: %v", err)
	}
	return n
}

// `kernl bookmark add` writes the graph directly, bypassing the API handler that
// used to be the only place a bookmark companion was created. On the vault this
// bug read as bookmarks existing nowhere but the git-ignored database: 54 of 54
// in one real graph had no companion note.
func TestBookmarkAddWritesACompanion(t *testing.T) {
	a, vault := bookmarkCompanionApp(t)

	// Port 1 for the same reason as the neighbouring test: unreachable on purpose,
	// so the archiver cannot reach a real page and the title stays the URL.
	if err := runBookmarkAdd(a, []string{"http://localhost:1/unreachable"}); err != nil {
		t.Fatalf("runBookmarkAdd: %v", err)
	}

	if got := countBookmarkCompanions(t, a); got != 1 {
		t.Errorf("describes edges to bookmarks = %d, want 1", got)
	}
	if files := companionFilesUnder(t, vault); len(files) != 1 {
		t.Errorf("companion files in %s = %v, want exactly one", layout.BookmarksFolder, files)
	}
}

// The bulk importers create one node per link and owe one companion each: an
// import is precisely the case where the count is large enough that "the graph
// has it" and "the backup has it" diverge for good.
func TestBookmarkImportWritesACompanionPerLink(t *testing.T) {
	for _, tc := range []struct {
		name    string
		file    string
		content string
	}{
		{
			name: "pocket",
			file: "pocket.html",
			content: `<!DOCTYPE html><html><body><ul>
<li><a href="https://example.com" time_added="123">Example</a></li>
<li><a href="https://example.org">Org</a></li>
</ul></body></html>`,
		},
		{
			name: "pinboard",
			file: "pinboard.json",
			content: `[{"href":"https://example.com","description":"Example","extended":"","tags":""},
			{"href":"https://example.org","description":"Org","extended":"","tags":""}]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, vault := bookmarkCompanionApp(t)
			path := filepath.Join(t.TempDir(), tc.file)
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatalf("write export: %v", err)
			}

			if err := runBookmarkImport(a, []string{tc.name, path}); err != nil {
				t.Fatalf("runBookmarkImport: %v", err)
			}

			if got := countBookmarkCompanions(t, a); got != 2 {
				t.Errorf("describes edges to bookmarks = %d, want 2", got)
			}
			files := companionFilesUnder(t, vault)
			if len(files) != 2 {
				t.Fatalf("companion files = %v, want two", files)
			}
			// Named after the title the export carried, not the URL: one rule for
			// every companion, and the readable name is what it buys here.
			want := map[string]bool{"example.md": true, "org.md": true}
			for _, f := range files {
				if !want[f] {
					t.Errorf("companion %q is not named after its title (want one of %v)", f, want)
				}
			}
		})
	}
}
