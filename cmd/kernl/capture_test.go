package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

func TestRunCapture(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "kernl.yaml")
	dbPath := filepath.Join(dir, ".kernl-graph.db")

	cfgContent := `
settings:
  agents:
    dummy:
      command: dummy
vault:
  root: "` + dir + `"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Make sure DB exists and schema is migrated
	gInit, err := graph.Open(context.Background(), graph.Config{Path: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	gInit.Close()

	err = runCapture(cfgPath, []string{"test capture message"})
	if err != nil {
		t.Fatalf("runCapture failed: %v", err)
	}

	g, err := graph.Open(context.Background(), graph.Config{Path: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	var captures []*nodes.Capture
	err = g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		caps, err := nodes.ListCaptures(context.Background(), tx, nodes.CaptureFilter{})
		captures = caps
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(captures) != 1 {
		t.Fatalf("expected 1 capture, got %d", len(captures))
	}
	if captures[0].Body != "test capture message" {
		t.Errorf("expected body 'test capture message', got %q", captures[0].Body)
	}
	if captures[0].CapturedFrom != "cli" {
		t.Errorf("expected captured_from 'cli', got %q", captures[0].CapturedFrom)
	}
	foundPending := false
	for _, tag := range captures[0].Tags {
		if tag == "pending" {
			foundPending = true
		}
	}
	if !foundPending {
		t.Errorf("expected tag 'pending', got %v", captures[0].Tags)
	}
}
