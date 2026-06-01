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

	if len(claims) == 0 {
		return nil, nil
	}

	var ids []string
	var args []any
	for _, c := range claims {
		ids = append(ids, c.ID)
		args = append(args, c.ID)
	}

	placeholders := make([]string, len(ids))
	for i := range placeholders {
		placeholders[i] = "?"
	}

	query := fmt.Sprintf(`SELECT dst FROM edges WHERE label = 'refutes' AND dst IN (%s)`, strings.Join(placeholders, ", "))

	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("SynthesizeTopic: query refutes: %w", err)
	}
	defer rows.Close()

	refuted := make(map[string]bool)
	for rows.Next() {
		var dst string
		if err := rows.Scan(&dst); err != nil {
			return nil, fmt.Errorf("SynthesizeTopic: scan refutes: %w", err)
		}
		refuted[dst] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("SynthesizeTopic: rows: %w", err)
	}

	var active []*nodes.MemoryClaim
	for _, c := range claims {
		if !refuted[c.ID] {
			active = append(active, c)
		}
	}

	return active, nil
}
