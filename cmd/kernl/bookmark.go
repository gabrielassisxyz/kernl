package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/bookmarks"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

func runBookmark(configPath string, args []string) error {
	// Usage validation comes first: a wrong invocation should never need a
	// loadable config to be diagnosed.
	if len(args) == 0 {
		return usagef("KERNL DISPATCH FAILURE: bookmark requires a subcommand — valid: add, import. Run: kernl bookmark --help")
	}
	if args[0] != "add" && args[0] != "import" {
		return usagef("KERNL DISPATCH FAILURE: unknown bookmark subcommand %q%s — valid: add, import. Run: kernl bookmark --help",
			args[0], didYouMean(args[0], []string{"add", "import"}))
	}

	cfg, err := loadCLIConfig(configPath)
	if err != nil {
		return err
	}

	a, err := app.NewApp(cfg)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: creating app: %w", err)
	}
	defer a.Close()

	if args[0] == "add" {
		return runBookmarkAdd(a, args[1:])
	}
	return runBookmarkImport(a, args[1:])
}

func runBookmarkAdd(a *app.App, args []string) error {
	if len(args) == 0 {
		return usagef("KERNL DISPATCH FAILURE: bookmark add requires a URL — run: kernl bookmark add <url>")
	}
	url := args[0]
	ctx := context.Background()
	var id string

	err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		author := nodes.Author{Name: "cli"}
		b := nodes.Bookmark{URL: url, Title: "Imported via CLI"}

		var err error
		id, err = nodes.CreateBookmark(ctx, tx, b, author)
		if err != nil {
			return fmt.Errorf("create bookmark: %w", err)
		}

		b.ID = id
		dataDir := filepath.Join(a.Config.Vault.Root, ".kernl", "archives")
		archiver := bookmarks.NewArchiver(nil, dataDir)
		res, err := archiver.ArchiveBookmark(ctx, &b)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: archiver failed: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Archived HTML to %s\n", res.HTMLPath)
		}

		if err := nodes.UpdateBookmark(ctx, tx, b, author); err != nil {
			return fmt.Errorf("update bookmark: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	fmt.Printf("Added bookmark %s\n", id)
	return nil
}

func runBookmarkImport(a *app.App, args []string) error {
	if len(args) < 2 {
		return usagef("KERNL DISPATCH FAILURE: bookmark import requires a format and a file — run: kernl bookmark import <pocket|pinboard> <file>")
	}
	format := args[0]
	filePath := args[1]

	if format != "pocket" && format != "pinboard" {
		return usagef("KERNL DISPATCH FAILURE: unknown import format %q%s — valid: pocket, pinboard",
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
			return fmt.Errorf("import failed: %w", innerErr)
		}
		return nil
	})

	if err != nil {
		return err
	}

	fmt.Printf("Imported %d bookmarks\n", count)
	return nil
}
