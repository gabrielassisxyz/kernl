package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

func TestRunBookmarkAdd(t *testing.T) {
	cfg := &config.Config{}
	cfg.Vault.Root = t.TempDir()

	a := &app.App{
		Config:  cfg,
		Backend: backend.NewBdCliBackend("/tmp/test"),
	}
	a.Graph = testutil.NewInMemoryTestGraph(t)

	err := runBookmarkAdd(a, []string{})
	if err == nil {
		t.Error("expected error for missing url")
	}

	// Port 1 rather than 8080: the unit suite is meant to be hermetic, and 8080
	// is where the developer's own kernl serves. Now that the title comes from
	// whatever that fetch returns, a running server would have been feeding
	// this test its answer.
	err = runBookmarkAdd(a, []string{"http://localhost:1/unreachable"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	err = a.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		list, err := nodes.ListBookmarks(context.Background(), tx, nodes.BookmarkFilter{})
		if err != nil {
			return err
		}
		if len(list) != 1 {
			t.Errorf("expected 1 bookmark, got %d", len(list))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The archiver cannot reach the network here, so the title the bookmark keeps
// is the one the command line gave it. That is the point: --title has to
// survive a page that never loads, which is half of why it exists.
func TestRunBookmarkAddKeepsExplicitTitle(t *testing.T) {
	a := newBookmarkTestApp(t)

	if err := runBookmarkAdd(a, []string{"--title", "Books That Make You Dangerous", "http://localhost:1/unreachable"}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got := onlyBookmark(t, a).Title; got != "Books That Make You Dangerous" {
		t.Errorf("title = %q, want the explicit one", got)
	}
}

// Without --title an unreachable page leaves the URL as the title. It is a bad
// title, but it is true and it is replaceable - which "Pending" never was.
func TestRunBookmarkAddFallsBackToURLNotAPlaceholder(t *testing.T) {
	a := newBookmarkTestApp(t)

	if err := runBookmarkAdd(a, []string{"http://localhost:1/unreachable"}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got := onlyBookmark(t, a).Title; got != "http://localhost:1/unreachable" {
		t.Errorf("title = %q, want the URL", got)
	}
}

func TestRunBookmarkRetitle(t *testing.T) {
	a := newBookmarkTestApp(t)
	if err := runBookmarkAdd(a, []string{"http://localhost:1/unreachable"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	id := onlyBookmark(t, a).ID

	if err := runBookmarkRetitle(a, []string{id, "A Name I Chose"}); err != nil {
		t.Fatalf("retitle: %v", err)
	}
	if got := onlyBookmark(t, a).Title; got != "A Name I Chose" {
		t.Errorf("title = %q, want the new one", got)
	}
}

func TestRunBookmarkRetitleUsageErrors(t *testing.T) {
	a := newBookmarkTestApp(t)

	err := runBookmarkRetitle(a, []string{"bkm-1"})
	if err == nil || exitCode(err) != 2 {
		t.Errorf("missing title is a usage error, got: %v", err)
	}

	err = runBookmarkRetitle(a, []string{"bkm-1", "   "})
	if err == nil || exitCode(err) != 2 {
		t.Errorf("blank title is a usage error, got: %v", err)
	}

	// An unknown ID is a real failure, not a usage mistake, and must be loud.
	err = runBookmarkRetitle(a, []string{"no-such-id", "Whatever"})
	if err == nil || !strings.Contains(err.Error(), "no bookmark with id no-such-id") {
		t.Errorf("unknown id must name the id, got: %v", err)
	}
}

func TestRunBookmarkRmDeletesBookmarkAndCompanion(t *testing.T) {
	a := newBookmarkTestApp(t)
	ctx := context.Background()
	relPath := filepath.ToSlash(filepath.Join("kernl", "bookmarks", "example.md"))
	raw := []byte("---\nid: companion-1\ntitle: Example\n---\n\nNotes for [[bookmark-1|Example]].\n")
	fullPath := filepath.Join(a.Config.Vault.Root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("mkdir companion dir: %v", err)
	}
	if err := os.WriteFile(fullPath, raw, 0644); err != nil {
		t.Fatalf("write companion file: %v", err)
	}

	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := nodes.CreateBookmark(ctx, tx, nodes.Bookmark{
			ID:    "bookmark-1",
			URL:   "https://example.com",
			Title: "Example",
		}, nodes.Author{Name: "test"})
		if err != nil {
			return err
		}
		if _, err := nodes.CreateNote(ctx, tx, nodes.Note{
			ID:    "companion-1",
			Title: "Example",
			Body:  "Notes for [[bookmark-1|Example]].\n",
		}, nodes.Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := edges.Create(ctx, tx, edges.Edge{
			Src:   "companion-1",
			Dst:   "bookmark-1",
			Label: "describes",
		}, nodes.Author{Name: "test"}); err != nil {
			return err
		}
		_, err = tx.Exec(
			`INSERT INTO note_paths (uuid, path, content_hash, updated_at)
			 VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))`,
			"companion-1", relPath, reconcile.HashBytes(raw),
		)
		return err
	})
	if err != nil {
		t.Fatalf("seed bookmark with companion: %v", err)
	}

	if err := runBookmarkRm(a, []string{"bookmark-1"}); err != nil {
		t.Fatalf("rm: %v", err)
	}

	err = a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		if _, err := nodes.GetBookmark(ctx, tx, "bookmark-1"); !errors.Is(err, graph.ErrNotFound) {
			t.Errorf("bookmark lookup err = %v, want ErrNotFound", err)
		}
		if _, err := nodes.GetNote(ctx, tx, "companion-1"); !errors.Is(err, graph.ErrNotFound) {
			t.Errorf("companion lookup err = %v, want ErrNotFound", err)
		}
		var pathRows int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM note_paths WHERE uuid = ?`, "companion-1").Scan(&pathRows); err != nil {
			return err
		}
		if pathRows != 0 {
			t.Errorf("note_paths rows = %d, want 0", pathRows)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
		t.Fatalf("companion file still exists or stat failed: %v", err)
	}
}

func TestRunBookmarkRmUsageErrors(t *testing.T) {
	a := newBookmarkTestApp(t)

	err := runBookmarkRm(a, nil)
	if err == nil || exitCode(err) != 2 {
		t.Errorf("missing id is a usage error, got: %v", err)
	}

	err = runBookmarkRm(a, []string{"missing"})
	if err == nil || !strings.Contains(err.Error(), "no bookmark with id missing") {
		t.Errorf("unknown id must name the id, got: %v", err)
	}
}

func newBookmarkTestApp(t *testing.T) *app.App {
	t.Helper()
	cfg := &config.Config{}
	cfg.Vault.Root = t.TempDir()

	a := &app.App{
		Config:  cfg,
		Backend: backend.NewBdCliBackend("/tmp/test"),
	}
	a.Graph = testutil.NewInMemoryTestGraph(t)
	return a
}

func onlyBookmark(t *testing.T, a *app.App) *nodes.Bookmark {
	t.Helper()
	var list []*nodes.Bookmark
	err := a.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var err error
		list, err = nodes.ListBookmarks(context.Background(), tx, nodes.BookmarkFilter{IncludeArchived: true})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected exactly 1 bookmark, got %d", len(list))
	}
	return list[0]
}

func TestBookmarkUsageErrorsTeachAndExitTwo(t *testing.T) {
	// Usage validation must not require a loadable config.
	err := runBookmark("definitely-missing.yaml", nil)
	if err == nil || !strings.Contains(err.Error(), "valid: add, import, retitle") {
		t.Fatalf("missing subcommand must list valid ones, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("want usage error, got exit %d", exitCode(err))
	}

	err = runBookmark("definitely-missing.yaml", []string{"ad"})
	if err == nil || !strings.Contains(err.Error(), `did you mean "add"?`) {
		t.Fatalf("typo'd subcommand must hint, got: %v", err)
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("bookmark errors must carry the marker, got: %v", err)
	}
}

func TestBookmarkImportUnknownFormatHints(t *testing.T) {
	a := &app.App{Config: &config.Config{}}
	err := runBookmarkImport(a, []string{"pockt", "/tmp/x"})
	if err == nil || !strings.Contains(err.Error(), `did you mean "pocket"?`) {
		t.Fatalf("unknown format must hint, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("unknown format is usage error, got exit %d", exitCode(err))
	}
}
