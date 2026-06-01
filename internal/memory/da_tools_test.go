package memory

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func TestDATools(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)

	var sessionID string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		id, err := nodes.CreateChatSession(ctx, tx, &nodes.ChatSession{}, nodes.Author{Name: "test"})
		if err != nil {
			return err
		}
		sessionID = id
		return nil
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	var claimID string
	t.Run("AddMemoryClaim", func(t *testing.T) {
		err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			id, err := AddMemoryClaim(ctx, tx, sessionID, "preferences", "likes coffee")
			if err != nil {
				return err
			}
			claimID = id
			return nil
		})
		if err != nil {
			t.Fatalf("AddMemoryClaim: %v", err)
		}

		err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
			// Verify claim exists
			claim, err := nodes.GetMemoryClaim(ctx, tx, claimID)
			if err != nil {
				t.Fatalf("GetMemoryClaim: %v", err)
			}
			if claim.Subject != "preferences" || claim.Statement != "likes coffee" {
				t.Errorf("unexpected claim content: %+v", claim)
			}

			// Verify source edge
			out, err := edges.Outgoing(ctx, tx, claimID)
			if err != nil {
				t.Fatalf("Outgoing: %v", err)
			}
			if len(out) != 1 || out[0].Dst != sessionID || out[0].Label != "source" {
				t.Errorf("unexpected outgoing edges: %+v", out)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("Read verification failed: %v", err)
		}
	})

	t.Run("RefuteMemoryClaim", func(t *testing.T) {
		var refID string
		err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			id, err := RefuteMemoryClaim(ctx, tx, claimID, "prefers tea actually")
			if err != nil {
				return err
			}
			refID = id
			return nil
		})
		if err != nil {
			t.Fatalf("RefuteMemoryClaim: %v", err)
		}

		err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
			// Verify refutation exists
			ref, err := nodes.GetMemoryRefutation(ctx, tx, refID)
			if err != nil {
				t.Fatalf("GetMemoryRefutation: %v", err)
			}
			if ref.ClaimID != claimID || ref.Reason != "prefers tea actually" {
				t.Errorf("unexpected refutation content: %+v", ref)
			}

			// Verify refutes edge
			out, err := edges.Outgoing(ctx, tx, refID)
			if err != nil {
				t.Fatalf("Outgoing: %v", err)
			}
			if len(out) != 1 || out[0].Dst != claimID || out[0].Label != "refutes" {
				t.Errorf("unexpected outgoing edges: %+v", out)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("Read verification failed: %v", err)
		}
	})
}
