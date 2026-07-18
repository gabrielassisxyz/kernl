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
	cfg, err := loadCLIConfig(configPath)
	if err != nil {
		return err
	}

	a, err := app.NewApp(cfg)
	if err != nil {
		return fmt.Errorf("creating app: %w", err)
	}
	defer a.Close()

	if len(args) == 0 {
		return fmt.Errorf("bookmark requires a subcommand (add, import)")
	}

	switch args[0] {
	case "add":
		return runBookmarkAdd(a, args[1:])
	case "import":
		return runBookmarkImport(a, args[1:])
	default:
		return fmt.Errorf("unknown bookmark subcommand %q", args[0])
	}
}

func runBookmarkAdd(a *app.App, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("add requires a url")
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
			fmt.Printf("Warning: archiver failed: %v\n", err)
		} else {
			fmt.Printf("Archived HTML to %s\n", res.HTMLPath)
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
		return fmt.Errorf("import requires format (pocket, pinboard) and file path")
	}
	format := args[0]
	filePath := args[1]

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
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
			return fmt.Errorf("unknown format %q", format)
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
