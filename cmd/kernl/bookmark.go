package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/bookmarks"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

func runBookmark(configPath string, args []string) error {
	// Usage validation comes first: a wrong invocation should never need a
	// loadable config to be diagnosed.
	if len(args) == 0 {
		return usagef("KERNL DISPATCH FAILURE: bookmark requires a subcommand - valid: add, import, retitle. Run: kernl bookmark --help")
	}
	switch args[0] {
	case "add", "import", "retitle":
	default:
		return usagef("KERNL DISPATCH FAILURE: unknown bookmark subcommand %q%s - valid: add, import, retitle. Run: kernl bookmark --help",
			args[0], didYouMean(args[0], []string{"add", "import", "retitle"}))
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

		saved = b.Title
		return nil
	})

	if err != nil {
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

	err = a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		author := nodes.Author{Name: "cli"}
		var innerErr error
		switch format {
		case "pocket":
			count, innerErr = bookmarks.ImportPocket(ctx, tx, f, author)
		case "pinboard":
			count, innerErr = bookmarks.ImportPinboard(ctx, tx, f, author)
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

	fmt.Printf("Imported %d bookmarks\n", count)
	return nil
}
