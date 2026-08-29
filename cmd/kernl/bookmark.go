package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/bookmarks"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/vault/companion"
	"github.com/gabrielassisxyz/kernl/internal/vault/layout"
)

var bookmarkSubcommands = []string{"add", "import", "retitle", "rm", "list", "tag", "archive", "unarchive"}

func runBookmark(configPath string, args []string) error {
	// Usage validation comes first: a wrong invocation should never need a
	// loadable config to be diagnosed.
	if len(args) == 0 {
		return usagef("KERNL DISPATCH FAILURE: bookmark requires a subcommand - valid: %s. Run: kernl bookmark --help",
			strings.Join(bookmarkSubcommands, ", "))
	}
	valid := false
	for _, s := range bookmarkSubcommands {
		if args[0] == s {
			valid = true
			break
		}
	}
	if !valid {
		return usagef("KERNL DISPATCH FAILURE: unknown bookmark subcommand %q%s - valid: %s. Run: kernl bookmark --help",
			args[0], didYouMean(args[0], bookmarkSubcommands), strings.Join(bookmarkSubcommands, ", "))
	}

	cfg, err := loadCLIConfig(configPath)
	if err != nil {
		return err
	}

	a, err := app.NewApp(cfg)
	if err != nil {
		return wrapLoud("creating app", err)
	}
	defer a.Close()

	switch args[0] {
	case "add":
		return runBookmarkAdd(a, args[1:])
	case "retitle":
		return runBookmarkRetitle(a, args[1:])
	case "rm":
		return runBookmarkRm(a, args[1:])
	case "list":
		return runBookmarkList(a, os.Stdout, args[1:])
	case "tag":
		return runBookmarkTag(a, args[1:])
	case "archive":
		return runBookmarkArchive(a, args[1:], true)
	case "unarchive":
		return runBookmarkArchive(a, args[1:], false)
	}
	return runBookmarkImport(a, args[1:])
}

func runBookmarkAdd(a *app.App, args []string) error {
	title, args, err := parseStringFlag(args, "--title", "")
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return usagef("KERNL DISPATCH FAILURE: bookmark add requires a URL - run: kernl bookmark add [--title <title>] <url>")
	}
	url := args[0]
	ctx := context.Background()
	var id, saved string
	var cf companion.File

	// An explicit --title outranks extraction and is never overwritten by it:
	// the pages worth bookmarking by hand are often the ones whose markup lies
	// (a paywall stub, a title padded with the site name, an SPA shell).
	// Without one the URL stands in until the archiver reads the page.
	if title == "" {
		title = url
	}

	err = a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		author := nodes.Author{Name: "cli"}
		b := nodes.Bookmark{URL: url, Title: title}

		var err error
		id, err = nodes.CreateBookmark(ctx, tx, b, author)
		if err != nil {
			return wrapLoud("create bookmark", err)
		}

		b.ID = id
		archiver := bookmarks.NewArchiver(nil, bookmarks.ArchiveDir(a.Config.Vault.Root))
		res, err := archiver.ArchiveBookmark(ctx, &b)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: archiver failed: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Archived HTML to %s\n", res.HTMLPath)
		}

		if err := nodes.UpdateBookmark(ctx, tx, b, author); err != nil {
			return wrapLoud("update bookmark", err)
		}

		// Named after the title, which by here is the one the archiver extracted:
		// this surface archives inside the transaction, so unlike the API it has
		// the real title before the companion is written. Same rule either way -
		// the companion takes the entity's title - and the readable name is what
		// that rule buys wherever the title is already known.
		cf, err = companion.Create(ctx, tx, a.Config.Vault.Root, id, layout.BookmarksFolder, b.Title, "", "bookmark")
		if err != nil {
			return err
		}

		saved = b.Title
		return nil
	})

	if err != nil {
		return err
	}
	if err := companion.WriteFile(a.Config.Vault.Root, cf); err != nil {
		return err
	}

	// Echo the stored title: extraction happened after the command line was
	// typed, so this is the only place the result is visible without a query.
	fmt.Printf("Added bookmark %s (%s)\n", id, saved)
	return nil
}

// runBookmarkRetitle renames a bookmark that carries a placeholder or a wrong
// title. It exists because the graph holds bookmarks created before extraction
// did, and re-fetching cannot repair them: the page may be gone, paywalled, or
// have been the reason the title was wrong in the first place.
func runBookmarkRetitle(a *app.App, args []string) error {
	if len(args) < 2 {
		return usagef("KERNL DISPATCH FAILURE: bookmark retitle requires an ID and a title - run: kernl bookmark retitle <id> <title>")
	}
	id, title := args[0], strings.TrimSpace(args[1])
	if title == "" {
		return usagef("KERNL DISPATCH FAILURE: bookmark retitle needs a non-empty title - run: kernl bookmark retitle %s <title>", id)
	}

	ctx := context.Background()
	var b *nodes.Bookmark

	if err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		b, err = nodes.GetBookmark(ctx, tx, id)
		return err
	}); err != nil {
		return wrapLoud(fmt.Sprintf("no bookmark with id %s", id), err)
	}

	previous := b.Title
	b.Title = title
	if err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.UpdateBookmark(ctx, tx, *b, nodes.Author{Name: "cli"})
	}); err != nil {
		return wrapLoud("update bookmark", err)
	}

	fmt.Printf("Retitled bookmark %s: %q -> %q\n", id, previous, title)
	return nil
}

// runBookmarkList prints id, title, tags and archive state for every
// bookmark matching the filter, or JSON when asked. --tags is match-any -
// the same semantics the API route documents, kept in sync deliberately
// rather than by accident (internal/graph/nodes/bookmark.go's BookmarkFilter
// doc comment is the one place this is decided).
func runBookmarkList(a *app.App, w io.Writer, args []string) error {
	tagsCSV, args, err := parseStringFlag(args, "--tags", "")
	if err != nil {
		return err
	}
	archivedRaw, args, err := parseStringFlag(args, "--archived", "")
	if err != nil {
		return err
	}
	asJSON, args := parseBoolFlag(args, "--json")
	if len(args) > 0 {
		return usagef("KERNL DISPATCH FAILURE: unknown bookmark list flag %q%s - valid: --tags <tags>, --archived <true|false>, --json",
			args[0], didYouMean(args[0], []string{"--tags", "--archived", "--json"}))
	}

	filter := nodes.BookmarkFilter{IncludeArchived: true}
	if tagsCSV != "" {
		filter.Tags = strings.Split(tagsCSV, ",")
	}
	switch archivedRaw {
	case "":
	case "true":
		filter.ArchivedOnly = true
	case "false":
		filter.IncludeArchived = false
	default:
		return usagef("KERNL DISPATCH FAILURE: --archived needs true or false, got %q", archivedRaw)
	}

	ctx := context.Background()
	var list []*nodes.Bookmark
	if err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		list, err = nodes.ListBookmarks(ctx, tx, filter)
		return err
	}); err != nil {
		return wrapLoud("list bookmarks", err)
	}

	if asJSON {
		return json.NewEncoder(w).Encode(newBookmarkListOutput(list))
	}

	if len(list) == 0 {
		fmt.Fprintln(w, "No bookmarks.")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTITLE\tTAGS\tARCHIVED")
	for _, b := range list {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%v\n", b.ID, b.Title, strings.Join(b.Tags, ","), b.ArchivedAt != nil)
	}
	return tw.Flush()
}

// bookmarkListOutput is the machine contract for `kernl bookmark list --json`.
type bookmarkListOutput struct {
	Bookmarks []bookmarkListRow `json:"bookmarks"`
}

type bookmarkListRow struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	URL      string   `json:"url"`
	Tags     []string `json:"tags"`
	Archived bool     `json:"archived"`
}

func newBookmarkListOutput(list []*nodes.Bookmark) bookmarkListOutput {
	rows := make([]bookmarkListRow, 0, len(list))
	for _, b := range list {
		rows = append(rows, bookmarkListRow{
			ID: b.ID, Title: b.Title, URL: b.URL, Tags: b.Tags, Archived: b.ArchivedAt != nil,
		})
	}
	return bookmarkListOutput{Bookmarks: rows}
}

// runBookmarkTag sets, adds or removes tags on an existing bookmark. The
// three modes compose in one call - set first (replaces the list), then add,
// then remove - which lets "replace, but also drop this one" be expressed
// without two invocations; a bare call with none of the three is a usage
// error rather than a silent no-op.
func runBookmarkTag(a *app.App, args []string) error {
	if len(args) == 0 {
		return usagef("KERNL DISPATCH FAILURE: bookmark tag requires an ID - run: kernl bookmark tag <id> [--set <tags>] [--add <tags>] [--remove <tags>]")
	}
	id := args[0]
	rest := args[1:]

	setCSV, rest, err := parseStringFlag(rest, "--set", "")
	if err != nil {
		return err
	}
	addCSV, rest, err := parseStringFlag(rest, "--add", "")
	if err != nil {
		return err
	}
	removeCSV, rest, err := parseStringFlag(rest, "--remove", "")
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return usagef("KERNL DISPATCH FAILURE: unknown bookmark tag flag %q%s - valid: --set <tags>, --add <tags>, --remove <tags>",
			rest[0], didYouMean(rest[0], []string{"--set", "--add", "--remove"}))
	}
	if setCSV == "" && addCSV == "" && removeCSV == "" {
		return usagef("KERNL DISPATCH FAILURE: bookmark tag needs at least one of --set, --add, --remove - run: kernl bookmark tag %s --add work", id)
	}

	ctx := context.Background()
	var b *nodes.Bookmark
	if err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		b, err = nodes.GetBookmark(ctx, tx, id)
		return err
	}); err != nil {
		return wrapLoud(fmt.Sprintf("no bookmark with id %s", id), err)
	}

	tags := b.Tags
	if setCSV != "" {
		tags = splitBookmarkTags(setCSV)
	}
	if addCSV != "" {
		tags = append(tags, splitBookmarkTags(addCSV)...)
	}
	if removeCSV != "" {
		drop := make(map[string]bool)
		for _, t := range splitBookmarkTags(removeCSV) {
			drop[t] = true
		}
		kept := make([]string, 0, len(tags))
		for _, t := range tags {
			if !drop[t] {
				kept = append(kept, t)
			}
		}
		tags = kept
	}
	b.Tags = tags

	if err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.UpdateBookmark(ctx, tx, *b, nodes.Author{Name: "cli"})
	}); err != nil {
		return wrapLoud("update bookmark tags", err)
	}

	fmt.Printf("Tags for bookmark %s: %s\n", id, strings.Join(b.Tags, ", "))
	return nil
}

// splitBookmarkTags turns a comma-separated flag value into a trimmed,
// non-empty tag list - the CLI accepts "foo, bar" where the API's raw
// strings.Split(tags, ",") does not, because a human is typing this one.
func splitBookmarkTags(csv string) []string {
	var out []string
	for _, t := range strings.Split(csv, ",") {
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// runBookmarkArchive sets or clears ArchivedAt for one bookmark. It is
// idempotent: archiving an already-archived bookmark (or unarchiving one
// that is not archived) reports the current state and writes nothing, rather
// than bumping UpdatedAt for a state that did not change.
func runBookmarkArchive(a *app.App, args []string, archive bool) error {
	verb := "archive"
	if !archive {
		verb = "unarchive"
	}
	if len(args) != 1 {
		return usagef("KERNL DISPATCH FAILURE: bookmark %s requires one ID - run: kernl bookmark %s <id>", verb, verb)
	}
	id := args[0]
	ctx := context.Background()

	var b *nodes.Bookmark
	if err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		b, err = nodes.GetBookmark(ctx, tx, id)
		return err
	}); err != nil {
		return wrapLoud(fmt.Sprintf("no bookmark with id %s", id), err)
	}

	if archive == (b.ArchivedAt != nil) {
		fmt.Printf("Bookmark %s is already %sd\n", id, verb)
		return nil
	}

	if archive {
		now := time.Now()
		b.ArchivedAt = &now
	} else {
		b.ArchivedAt = nil
	}

	if err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.UpdateBookmark(ctx, tx, *b, nodes.Author{Name: "cli"})
	}); err != nil {
		return wrapLoud(fmt.Sprintf("%s bookmark", verb), err)
	}

	fmt.Printf("%sd bookmark %s\n", strings.ToUpper(verb[:1])+verb[1:], id)
	return nil
}

type bookmarkCompanion struct {
	id      string
	relPath string
}

func runBookmarkRm(a *app.App, args []string) error {
	if len(args) != 1 {
		return usagef("KERNL DISPATCH FAILURE: bookmark rm requires one ID - run: kernl bookmark rm <id>")
	}
	id := args[0]
	ctx := context.Background()

	var b *nodes.Bookmark
	if err := a.Graph.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		b, err = nodes.GetBookmark(ctx, tx, id)
		return err
	}); err != nil {
		return wrapLoud(fmt.Sprintf("no bookmark with id %s", id), err)
	}

	var companions []bookmarkCompanion
	if err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		companions, err = bookmarkCompanions(ctx, tx, id)
		if err != nil {
			return err
		}
		for _, c := range companions {
			if _, err := tx.Exec(`DELETE FROM note_paths WHERE uuid = ?`, c.id); err != nil {
				return fmt.Errorf("delete companion note path %s: %w", c.id, err)
			}
			if err := nodes.DeleteNote(ctx, tx, c.id, nodes.Author{Name: "cli"}); err != nil {
				return fmt.Errorf("delete companion note %s: %w", c.id, err)
			}
		}
		return nodes.DeleteBookmark(ctx, tx, id, nodes.Author{Name: "cli"})
	}); err != nil {
		return wrapLoud("delete bookmark", err)
	}

	for _, c := range companions {
		if c.relPath == "" || a.Config.Vault.Root == "" {
			continue
		}
		if err := os.Remove(filepath.Join(a.Config.Vault.Root, filepath.FromSlash(c.relPath))); err != nil && !os.IsNotExist(err) {
			return wrapLoud(fmt.Sprintf("remove companion note file %s", c.relPath), err)
		}
	}

	fmt.Printf("Deleted bookmark %s: %q <%s>\n", id, b.Title, b.URL)
	if len(companions) > 0 {
		fmt.Printf("Deleted %d companion note(s)\n", len(companions))
	}
	return nil
}

func bookmarkCompanions(ctx context.Context, tx *graph.WriteTx, bookmarkID string) ([]bookmarkCompanion, error) {
	rows, err := tx.Query(
		`SELECT n.id, COALESCE(np.path, '')
		   FROM edges e
		   JOIN nodes n ON n.id = e.src AND n.type = 'note' AND n.deleted_at IS NULL
		   LEFT JOIN note_paths np ON np.uuid = n.id
		  WHERE e.dst = ? AND e.label = 'describes'
		  ORDER BY n.id`,
		bookmarkID,
	)
	if err != nil {
		return nil, fmt.Errorf("lookup bookmark companions: %w", err)
	}
	defer rows.Close()

	var out []bookmarkCompanion
	for rows.Next() {
		var c bookmarkCompanion
		if err := rows.Scan(&c.id, &c.relPath); err != nil {
			return nil, fmt.Errorf("scan bookmark companion: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func runBookmarkImport(a *app.App, args []string) error {
	if len(args) < 2 {
		return usagef("KERNL DISPATCH FAILURE: bookmark import requires a format and a file - run: kernl bookmark import <pocket|pinboard> <file>")
	}
	format := args[0]
	filePath := args[1]

	if format != "pocket" && format != "pinboard" {
		return usagef("KERNL DISPATCH FAILURE: unknown import format %q%s - valid: pocket, pinboard",
			format, didYouMean(format, []string{"pocket", "pinboard"}))
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: cannot open import file %s: %w", filePath, err)
	}
	defer f.Close()

	ctx := context.Background()
	var count int

	var companions []companion.File
	err = a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		author := nodes.Author{Name: "cli"}
		var innerErr error
		switch format {
		case "pocket":
			count, companions, innerErr = bookmarks.ImportPocket(ctx, tx, a.Config.Vault.Root, f, author)
		case "pinboard":
			count, companions, innerErr = bookmarks.ImportPinboard(ctx, tx, a.Config.Vault.Root, f, author)
		default:
			return fmt.Errorf("KERNL DISPATCH FAILURE: unknown format %q", format)
		}
		if innerErr != nil {
			return wrapLoud("import failed", innerErr)
		}
		return nil
	})

	if err != nil {
		return err
	}
	if err := companion.WriteFiles(a.Config.Vault.Root, companions); err != nil {
		return err
	}

	fmt.Printf("Imported %d bookmarks\n", count)
	return nil
}
