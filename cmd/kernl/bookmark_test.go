package main

import (
	"context"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
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

	err = runBookmarkAdd(a, []string{"http://localhost:8080/fake"})
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

func TestBookmarkUsageErrorsTeachAndExitTwo(t *testing.T) {
	// Usage validation must not require a loadable config.
	err := runBookmark("definitely-missing.yaml", nil)
	if err == nil || !strings.Contains(err.Error(), "valid: add, import") {
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
