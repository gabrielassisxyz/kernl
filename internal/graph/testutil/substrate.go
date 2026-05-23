package testutil

import (
	"context"
	"os"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

// NewTestGraph opens a tempfile-backed Graph and registers cleanup.
func NewTestGraph(t *testing.T) *graph.Graph {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.db")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()
	g, err := graph.Open(context.Background(), graph.Config{Path: f.Name()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { g.Close() })
	return g
}

// NewInMemoryTestGraph opens a shared-cache in-memory Graph and registers cleanup.
func NewInMemoryTestGraph(tb testing.TB) *graph.Graph {
	tb.Helper()
	g, err := graph.Open(context.Background(), graph.Config{InMemory: true, Path: tb.Name()})
	if err != nil {
		tb.Fatalf("Open: %v", err)
	}
	tb.Cleanup(func() { g.Close() })
	return g
}
