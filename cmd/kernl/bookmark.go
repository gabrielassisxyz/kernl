package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/bookmarks"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/vault/companion"
	"github.com/gabrielassisxyz/kernl/internal/vault/layout"
)

func runBookmark(configPath string, args []string) error {
	// Usage validation comes first: a wrong invocation should never need a
	// loadable config to be diagnosed.
	if len(args) == 0 {
		return usagef("KERNL DISPATCH FAILURE: bookmark requires a subcommand - valid: add, import, retitle, rm. Run: kernl bookmark --help")
	}
	valid := []string{"add", "import", "retitle", "rm"}
	switch args[0] {
	case "add", "import", "retitle", "rm":
	default:
		return usagef("KERNL DISPATCH FAILURE: unknown bookmark subcommand %q%s - valid: add, import, retitle, rm. Run: kernl bookmark --help",
			args[0], didYouMean(args[0], valid))
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
