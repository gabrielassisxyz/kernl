package main

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestRunBookmarkListPlainTextAndJSON(t *testing.T) {
	a := newBookmarkTestApp(t)
	if err := runBookmarkAdd(a, []string{"http://localhost:1/one"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	id := onlyBookmark(t, a).ID
	if err := runBookmarkTag(a, []string{id, "--add", "reading,go"}); err != nil {
		t.Fatalf("tag: %v", err)
	}

	var out bytes.Buffer
	if err := runBookmarkList(a, &out, nil); err != nil {
		t.Fatalf("list: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, id) || !strings.Contains(text, "go") {
		t.Errorf("plain-text list = %q, want it to name the bookmark and its tag", text)
	}

	out.Reset()
	if err := runBookmarkList(a, &out, []string{"--json"}); err != nil {
		t.Fatalf("list --json: %v", err)
	}
	var parsed bookmarkListOutput
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("decode --json output: %v, body: %s", err, out.String())
	}
	if len(parsed.Bookmarks) != 1 {
		t.Fatalf("expected 1 bookmark in --json output, got %d", len(parsed.Bookmarks))
	}
	if parsed.Bookmarks[0].ID != id {
		t.Errorf("json id = %q, want %q", parsed.Bookmarks[0].ID, id)
	}
	if len(parsed.Bookmarks[0].Tags) != 2 {
		t.Errorf("json tags = %v, want 2 entries", parsed.Bookmarks[0].Tags)
	}
}

func TestRunBookmarkListFiltersByTagsAndArchived(t *testing.T) {
	a := newBookmarkTestApp(t)
	if err := runBookmarkAdd(a, []string{"http://localhost:1/one"}); err != nil {
		t.Fatalf("add one: %v", err)
	}
	oneID := onlyBookmarkMatching(t, a, "http://localhost:1/one").ID
	if err := runBookmarkTag(a, []string{oneID, "--set", "go"}); err != nil {
		t.Fatalf("tag one: %v", err)
	}

	if err := runBookmarkAdd(a, []string{"http://localhost:1/two"}); err != nil {
		t.Fatalf("add two: %v", err)
	}
	twoID := onlyBookmarkMatching(t, a, "http://localhost:1/two").ID
	if err := runBookmarkTag(a, []string{twoID, "--set", "python"}); err != nil {
		t.Fatalf("tag two: %v", err)
	}
	if err := runBookmarkArchive(a, []string{twoID}, true); err != nil {
		t.Fatalf("archive two: %v", err)
	}

	var out bytes.Buffer
	if err := runBookmarkList(a, &out, []string{"--tags", "go", "--json"}); err != nil {
		t.Fatalf("list --tags go: %v", err)
	}
	var byTag bookmarkListOutput
	if err := json.Unmarshal(out.Bytes(), &byTag); err != nil {
		t.Fatal(err)
	}
	if len(byTag.Bookmarks) != 1 || byTag.Bookmarks[0].ID != oneID {
		t.Errorf("--tags go = %+v, want only bookmark %s", byTag.Bookmarks, oneID)
	}

	out.Reset()
	if err := runBookmarkList(a, &out, []string{"--archived", "false", "--json"}); err != nil {
		t.Fatalf("list --archived false: %v", err)
	}
	var unarchived bookmarkListOutput
	if err := json.Unmarshal(out.Bytes(), &unarchived); err != nil {
		t.Fatal(err)
	}
	if len(unarchived.Bookmarks) != 1 || unarchived.Bookmarks[0].ID != oneID {
		t.Errorf("--archived false = %+v, want only bookmark %s", unarchived.Bookmarks, oneID)
	}

	out.Reset()
	if err := runBookmarkList(a, &out, []string{"--archived", "true", "--json"}); err != nil {
		t.Fatalf("list --archived true: %v", err)
	}
	var archived bookmarkListOutput
	if err := json.Unmarshal(out.Bytes(), &archived); err != nil {
		t.Fatal(err)
	}
	if len(archived.Bookmarks) != 1 || archived.Bookmarks[0].ID != twoID {
		t.Errorf("--archived true = %+v, want only bookmark %s", archived.Bookmarks, twoID)
	}

	out.Reset()
	if err := runBookmarkList(a, &out, nil); err != nil {
		t.Fatalf("list (default): %v", err)
	}
	// Default listing still contains the archived bookmark, matching the API's
	// "archiving is success, not removal" rule.
	if !strings.Contains(out.String(), twoID) {
		t.Errorf("default list = %q, want it to still include archived bookmark %s", out.String(), twoID)
	}
}

func TestRunBookmarkTagSetAddRemove(t *testing.T) {
	a := newBookmarkTestApp(t)
	if err := runBookmarkAdd(a, []string{"http://localhost:1/unreachable"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	id := onlyBookmark(t, a).ID

	if err := runBookmarkTag(a, []string{id, "--set", "go, reading"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := onlyBookmark(t, a).Tags; !hasExactly(got, "go", "reading") {
		t.Fatalf("after --set: tags = %v", got)
	}

	if err := runBookmarkTag(a, []string{id, "--add", "cli"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if got := onlyBookmark(t, a).Tags; !hasExactly(got, "go", "reading", "cli") {
		t.Fatalf("after --add: tags = %v", got)
	}

	if err := runBookmarkTag(a, []string{id, "--remove", "reading"}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := onlyBookmark(t, a).Tags; !hasExactly(got, "go", "cli") {
		t.Fatalf("after --remove: tags = %v", got)
	}
}

func TestRunBookmarkTagUsageErrors(t *testing.T) {
	a := newBookmarkTestApp(t)

	if err := runBookmarkTag(a, nil); err == nil || exitCode(err) != 2 {
		t.Errorf("missing id is a usage error, got: %v", err)
	}
	if err := runBookmarkTag(a, []string{"bkm-1"}); err == nil || exitCode(err) != 2 {
		t.Errorf("no --set/--add/--remove is a usage error, got: %v", err)
	}
}

func TestRunBookmarkArchiveAndUnarchiveAreIdempotent(t *testing.T) {
	a := newBookmarkTestApp(t)
	if err := runBookmarkAdd(a, []string{"http://localhost:1/unreachable"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	id := onlyBookmark(t, a).ID

	if err := runBookmarkArchive(a, []string{id}, true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if onlyBookmark(t, a).ArchivedAt == nil {
		t.Fatal("expected bookmark to be archived")
	}
	// Archiving again must not error and must not clear/reset the state.
	if err := runBookmarkArchive(a, []string{id}, true); err != nil {
		t.Fatalf("second archive: %v", err)
	}
	if onlyBookmark(t, a).ArchivedAt == nil {
		t.Fatal("second archive call cleared the archived state")
	}

	if err := runBookmarkArchive(a, []string{id}, false); err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	if onlyBookmark(t, a).ArchivedAt != nil {
		t.Fatal("expected bookmark to be unarchived")
	}
	// Unarchiving again must not error.
	if err := runBookmarkArchive(a, []string{id}, false); err != nil {
		t.Fatalf("second unarchive: %v", err)
	}
	if onlyBookmark(t, a).ArchivedAt != nil {
		t.Fatal("second unarchive call re-archived the bookmark")
	}
}

func TestRunBookmarkArchiveUsageErrorsAndUnknownID(t *testing.T) {
	a := newBookmarkTestApp(t)

	if err := runBookmarkArchive(a, nil, true); err == nil || exitCode(err) != 2 {
		t.Errorf("missing id is a usage error, got: %v", err)
	}
	if err := runBookmarkArchive(a, []string{"no-such-id"}, true); err == nil || !strings.Contains(err.Error(), "no bookmark with id no-such-id") {
		t.Errorf("unknown id must name the id, got: %v", err)
	}
}

// onlyBookmarkMatching finds the one bookmark in a with the given URL, among
// possibly several - onlyBookmark only works while exactly one exists.
func onlyBookmarkMatching(t *testing.T, a *app.App, url string) *nodes.Bookmark {
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
	for _, b := range list {
		if b.URL == url {
			return b
		}
	}
	t.Fatalf("no bookmark with url %s among %d", url, len(list))
	return nil
}

// hasExactly reports whether got holds exactly the given tags, in any order.
func hasExactly(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[string]bool, len(got))
	for _, t := range got {
		seen[t] = true
	}
	for _, w := range want {
		if !seen[w] {
			return false
		}
	}
	return true
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
