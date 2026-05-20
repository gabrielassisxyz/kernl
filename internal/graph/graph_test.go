package graph_test

import (
	"context"
	"os"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

func openTestGraph(t *testing.T) *graph.Graph {
	t.Helper()
	f, err := os.CreateTemp("", "kernl-graph-test-*.db")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	g, err := graph.Open(context.Background(), graph.Config{Path: f.Name()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return g
}

func TestOpenAndClose(t *testing.T) {
	f, err := os.CreateTemp("", "kernl-graph-test-*.db")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	g, err := graph.Open(context.Background(), graph.Config{Path: f.Name()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := g.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestDoReadDoWrite(t *testing.T) {
	g := openTestGraph(t)
	defer g.Close()

	// DoWrite should be able to insert
	err := g.DoWrite(context.Background(), func(wtx *graph.WriteTx) error {
		_, err := wtx.Exec(`INSERT INTO nodes(id, type, title) VALUES ('n1', 'test', 'Test Node')`)
		return err
	})
	if err != nil {
		t.Fatalf("DoWrite insert: %v", err)
	}

	// DoRead should be able to query
	err = g.DoRead(context.Background(), func(rtx *graph.ReadTx) error {
		var id string
		return rtx.QueryRow(`SELECT id FROM nodes WHERE id = 'n1'`).Scan(&id)
	})
	if err != nil {
		t.Fatalf("DoRead query: %v", err)
	}
}

func TestOpenIdempotence(t *testing.T) {
	f, err := os.CreateTemp("", "kernl-idempotent-test-*.db")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	ctx := context.Background()
	g1, err := graph.Open(ctx, graph.Config{Path: f.Name()})
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	// Close g1 before opening again (same pool file, different Graph instance)
	if err := g1.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	g2, err := graph.Open(ctx, graph.Config{Path: f.Name()})
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	if err := g2.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
