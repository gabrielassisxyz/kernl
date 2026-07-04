package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// AddMemoryClaim creates a DA-authored MemoryClaim node and a 'source' edge
// pointing to the given sessionID. The Source attr is set to "da" so the UI can
// show human-readable provenance without walking the source edge.
func AddMemoryClaim(ctx context.Context, tx *graph.WriteTx, sessionID, topic, content string) (string, error) {
	author := nodes.AuthorAgent("da")

	mc := nodes.MemoryClaim{
		Subject:    topic,
		Statement:  content,
		Confidence: 1.0,
		Source:     "da",
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

// AddUserMemoryClaim creates a MemoryClaim the user asserted directly (not via
// the DA-learned flow). It is authored by the human and tagged with "user"
// provenance so the surface can distinguish user rules from DA suggestions.
func AddUserMemoryClaim(ctx context.Context, tx *graph.WriteTx, subject, statement string) (string, error) {
	mc := nodes.MemoryClaim{
		Subject:    subject,
		Statement:  statement,
		Confidence: 1.0,
		Source:     "user",
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	claimID, err := nodes.CreateMemoryClaim(ctx, tx, mc, nodes.Author{Name: "human"})
	if err != nil {
		return "", fmt.Errorf("AddUserMemoryClaim: create node: %w", err)
	}
	return claimID, nil
}

// ActiveSubjects returns the distinct subjects that currently have at least one
// non-refuted claim. Feeding these to the learned-candidate extractor lets it
// reuse an existing topic instead of minting a near-duplicate for the same
// concept (the "two topics for one idea" bug).
func ActiveSubjects(ctx context.Context, g *graph.Graph) ([]string, error) {
	var subjects []string
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		rows, err := tx.Query(`
			SELECT DISTINCT json_extract(n.attrs, '$.subject')
			FROM nodes n
			WHERE n.type = 'memory_claim'
			  AND json_extract(n.attrs, '$.subject') IS NOT NULL
			  AND n.deleted_at IS NULL
			  AND NOT EXISTS (
			      SELECT 1 FROM edges e WHERE e.label = 'refutes' AND e.dst = n.id
			  )`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				return err
			}
			if s != "" {
				subjects = append(subjects, s)
			}
		}
		return rows.Err()
	})
	return subjects, err
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
