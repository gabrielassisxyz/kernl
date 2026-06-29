package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// SynthesizeTopic returns active MemoryClaim nodes for the given topic,
// filtering out any claim that is the target of a 'refutes' edge.
// Results are returned newest-first.
func SynthesizeTopic(ctx context.Context, tx *graph.ReadTx, topic string) ([]*nodes.MemoryClaim, error) {
	claims, err := nodes.ListMemoryClaims(ctx, tx, nodes.MemoryClaimFilter{
		Subject: topic,
	})
	if err != nil {
		return nil, fmt.Errorf("SynthesizeTopic: %w", err)
	}

	return FilterRefuted(ctx, tx, claims)
}

// FilterRefuted returns the subset of claims that are NOT the target of a
// 'refutes' edge, preserving input order. It is the shared "active claim"
// gate used both by topic synthesis and by planning-context retrieval, so a
// refuted claim is excluded everywhere claims are surfaced.
func FilterRefuted(ctx context.Context, tx *graph.ReadTx, claims []*nodes.MemoryClaim) ([]*nodes.MemoryClaim, error) {
	if len(claims) == 0 {
		return nil, nil
	}

	args := make([]any, 0, len(claims))
	for _, c := range claims {
		args = append(args, c.ID)
	}

	placeholders := make([]string, len(claims))
	for i := range placeholders {
		placeholders[i] = "?"
	}

	query := fmt.Sprintf(`SELECT dst FROM edges WHERE label = 'refutes' AND dst IN (%s)`, strings.Join(placeholders, ", "))

	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("FilterRefuted: query refutes: %w", err)
	}
	defer rows.Close()

	refuted := make(map[string]bool)
	for rows.Next() {
		var dst string
		if err := rows.Scan(&dst); err != nil {
			return nil, fmt.Errorf("FilterRefuted: scan refutes: %w", err)
		}
		refuted[dst] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FilterRefuted: rows: %w", err)
	}

	var active []*nodes.MemoryClaim
	for _, c := range claims {
		if !refuted[c.ID] {
			active = append(active, c)
		}
	}

	return active, nil
}
