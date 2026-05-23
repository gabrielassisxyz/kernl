package chat

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func newTestApp(t *testing.T) *app.App {
	g := testutil.NewInMemoryTestGraph(t)
	return &app.App{
		Graph: g,
		Config: &config.Config{
			Vault: config.VaultConfig{Root: t.TempDir()},
		},
	}
}

func TestGraphPermissionCheckerPublicNode(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	var id string
	_ = a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateNote(ctx, tx, nodes.Note{Title: "Public", Body: "b", Tags: []string{}}, nodes.Author{Name: "test"})
		return err
	})
	checker := NewGraphPermissionChecker(a)
	ok, reason, err := checker.CanRead(ctx, id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected allowed, got denied: %s", reason)
	}
}

func TestGraphPermissionCheckerConfidentialTag(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	var id string
	_ = a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateNote(ctx, tx, nodes.Note{Title: "Secret", Body: "b", Tags: []string{"confidencial"}}, nodes.Author{Name: "test"})
		return err
	})
	checker := NewGraphPermissionChecker(a)
	ok, reason, err := checker.CanRead(ctx, id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected denied")
	}
	if reason != "node marked private" {
		t.Errorf("reason = %q, want 'node marked private'", reason)
	}
}

func TestGraphPermissionCheckerNotFound(t *testing.T) {
	a := newTestApp(t)
	ctx := context.Background()
	checker := NewGraphPermissionChecker(a)
	ok, reason, err := checker.CanRead(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected denied")
	}
	if reason != "node not found" {
		t.Errorf("reason = %q, want 'node not found'", reason)
	}
}
