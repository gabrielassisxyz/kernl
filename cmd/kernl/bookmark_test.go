package main

import (
	"context"
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
