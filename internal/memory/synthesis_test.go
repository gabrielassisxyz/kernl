package memory_test

import (
	"context"
	"os"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/memory"
)

func TestSynthesizeTopic(t *testing.T) {
	ctx := context.Background()
	f, err := os.CreateTemp("", "kernl-graph-test-*.db")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()
	t.Cleanup(func() { _ = os.Remove(f.Name()) })

	g, err := graph.Open(ctx, graph.Config{Path: f.Name()})
	if err != nil {
		t.Fatalf("failed to open graph: %v", err)
	}
	defer g.Close()

	author := nodes.Author{
		Name: "test-user",
	}

	var c1, c2, c3 string

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		c1, err = nodes.CreateMemoryClaim(ctx, tx, nodes.MemoryClaim{
			Title:      "Go is typed",
			Subject:    "go-programming",
			Statement:  "Go is statically typed.",
			Confidence: 1.0,
		}, author)
		if err != nil {
			return err
		}

		c2, err = nodes.CreateMemoryClaim(ctx, tx, nodes.MemoryClaim{
			Title:      "Go has generics",
			Subject:    "go-programming",
			Statement:  "Go added generics in 1.18.",
			Confidence: 1.0,
		}, author)
		if err != nil {
			return err
		}

		c3, err = nodes.CreateMemoryClaim(ctx, tx, nodes.MemoryClaim{
			Title:      "Go has exceptions",
			Subject:    "go-programming",
			Statement:  "Go uses try-catch.",
			Confidence: 0.5,
		}, author)
		if err != nil {
			return err
		}

		// Create claim for another topic
		_, err = nodes.CreateMemoryClaim(ctx, tx, nodes.MemoryClaim{
			Title:      "Rust has lifetimes",
			Subject:    "rust-programming",
			Statement:  "Rust has a borrow checker.",
			Confidence: 1.0,
		}, author)
		if err != nil {
			return err
		}

		// Refute c3
		ref, err := nodes.CreateMemoryRefutation(ctx, tx, nodes.MemoryRefutation{
			Title:   "Refute exceptions",
			ClaimID: c3,
			Reason:  "Go uses values for errors, not try-catch.",
		}, author)
		if err != nil {
			return err
		}

		_, err = edges.Create(ctx, tx, edges.Edge{
			Src:   ref,
			Dst:   c3,
			Label: "refutes",
			Type:  edges.EdgeType("refutes"),
		}, author)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed to setup data: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		active, err := memory.SynthesizeTopic(ctx, tx, "go-programming")
		if err != nil {
			t.Fatalf("SynthesizeTopic failed: %v", err)
		}

		if len(active) != 2 {
			t.Fatalf("expected 2 active claims, got %d", len(active))
		}

		hasTyped := false
		hasGenerics := false
		for _, c := range active {
			if c.Statement == "Go is statically typed." {
				hasTyped = true
			}
			if c.Statement == "Go added generics in 1.18." {
				hasGenerics = true
			}
			if c.ID == c3 {
				t.Errorf("refuted claim c3 should not be in active list")
			}
		}

		if !hasTyped || !hasGenerics {
			t.Errorf("missing expected active claims. got: %+v", active)
		}

		// Check that both c1 and c2 are in the active slice.
		foundC1, foundC2 := false, false
		for _, a := range active {
			if a.ID == c1 {
				foundC1 = true
			}
			if a.ID == c2 {
				foundC2 = true
			}
		}
		if !foundC1 || !foundC2 {
			t.Errorf("expected both c1 and c2 to be active")
		}

		return nil
	})
	if err != nil {
		t.Fatalf("DoRead failed: %v", err)
	}
}
