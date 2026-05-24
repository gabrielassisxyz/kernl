package graph_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	_ "modernc.org/sqlite"
)

func TestSentinelsAreDistinct(t *testing.T) {
	// Each sentinel error must be a unique value.
	if errors.Is(graph.ErrNotFound, graph.ErrFTSQuerySyntax) {
		t.Error("ErrNotFound should be distinct from ErrFTSQuerySyntax")
	}
	if errors.Is(graph.ErrNotFound, graph.ErrSchemaLocked) {
		t.Error("ErrNotFound should be distinct from ErrSchemaLocked")
	}
	if errors.Is(graph.ErrNotFound, graph.ErrAuthorRequired) {
		t.Error("ErrNotFound should be distinct from ErrAuthorRequired")
	}
	if errors.Is(graph.ErrFTSQuerySyntax, graph.ErrSchemaLocked) {
		t.Error("ErrFTSQuerySyntax should be distinct from ErrSchemaLocked")
	}
	if errors.Is(graph.ErrFTSQuerySyntax, graph.ErrAuthorRequired) {
		t.Error("ErrFTSQuerySyntax should be distinct from ErrAuthorRequired")
	}
	if errors.Is(graph.ErrSchemaLocked, graph.ErrAuthorRequired) {
		t.Error("ErrSchemaLocked should be distinct from ErrAuthorRequired")
	}
}

func TestSchemaLockedSurfacesFromOpen(t *testing.T) {
	f, err := createTempDB(t)
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()
	t.Cleanup(func() { removeFile(f.Name()) })

	// First open — apply schema, then mark dirty.
	g1, err := graph.Open(context.Background(), graph.Config{Path: f.Name()})
	if err != nil {
		t.Fatalf("first open: %v", err)
	}

	// Manually flip the latest migration dirty via raw SQL.
	// Must target the row with the highest version so that
	// migrate.Current() detects dirty=true.
	if err := g1.DoWrite(context.Background(), func(wtx *graph.WriteTx) error {
		_, err := wtx.Exec(`UPDATE schema_migrations SET dirty=1 WHERE version=(SELECT MAX(version) FROM schema_migrations)`)
		return err
	}); err != nil {
		t.Fatalf("mark dirty: %v", err)
	}
	if err := g1.Close(); err != nil {
		t.Fatalf("close g1: %v", err)
	}

	// Re-open — should translate migrate.ErrDirty to graph.ErrSchemaLocked.
	_, err = graph.Open(context.Background(), graph.Config{Path: f.Name()})
	if err == nil {
		t.Fatal("expected error on second open, got nil")
	}
	if !errors.Is(err, graph.ErrSchemaLocked) {
		t.Fatalf("expected graph.ErrSchemaLocked, got %v", err)
	}
}

func createTempDB(t *testing.T) (*os.File, error) {
	return os.CreateTemp("", "kernl-graph-test-*.db")
}

func removeFile(name string) {
	os.Remove(name)
}
