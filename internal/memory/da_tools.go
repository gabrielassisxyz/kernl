package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// AddMemoryClaim creates a new MemoryClaim node and a 'source' edge pointing to the given sessionID.
func AddMemoryClaim(ctx context.Context, tx *graph.WriteTx, sessionID, topic, content string) (string, error) {
	author := nodes.AuthorAgent("da")

	mc := nodes.MemoryClaim{
		Subject:    topic,
		Statement:  content,
		Confidence: 1.0,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	claimID, err := nodes.CreateMemoryClaim(ctx, tx, mc, author)
	if err != nil {
		return "", fmt.Errorf("AddMemoryClaim: create node: %w", err)
	}

	if sessionID != "" {
		_, err = edges.Create(ctx, tx, edges.Edge{
			Src:   claimID,
			Dst:   sessionID,
			Label: "source",
			Type:  "source",
		}, author)
		if err != nil {
			return "", fmt.Errorf("AddMemoryClaim: create source edge: %w", err)
		}
	}

	return claimID, nil
}

// RefuteMemoryClaim creates a new MemoryRefutation node and a 'refutes' edge pointing to the targetClaimID.
func RefuteMemoryClaim(ctx context.Context, tx *graph.WriteTx, targetClaimID, reason string) (string, error) {
	author := nodes.AuthorAgent("da")

	mr := nodes.MemoryRefutation{
		ClaimID:    targetClaimID,
		Reason:     reason,
		Confidence: 1.0,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	refID, err := nodes.CreateMemoryRefutation(ctx, tx, mr, author)
	if err != nil {
		return "", fmt.Errorf("RefuteMemoryClaim: create node: %w", err)
	}

	_, err = edges.Create(ctx, tx, edges.Edge{
		Src:   refID,
		Dst:   targetClaimID,
		Label: "refutes",
		Type:  "refutes",
	}, author)
	if err != nil {
		return "", fmt.Errorf("RefuteMemoryClaim: create refutes edge: %w", err)
	}

	return refID, nil
}
