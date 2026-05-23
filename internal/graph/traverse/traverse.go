package traverse

import (
	"context"
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

// MaxDepth is the hard upper bound for traversal depth.
const MaxDepth = 6

// NeighborsAtDepth returns the set of node IDs reachable within depth
// undirected hops from nodeID, excluding nodeID itself.
// If labels is non-empty only edges with those labels are traversed.
// A depth exceeding MaxDepth returns graph.ErrDepthExceeded.
// A depth <= 0 returns an empty slice.
func NeighborsAtDepth(ctx context.Context, tx *graph.ReadTx, nodeID string, depth int, labels ...string) ([]string, error) {
	if depth <= 0 {
		return []string{}, nil
	}
	if depth > MaxDepth {
		return nil, graph.ErrDepthExceeded
	}

	var b strings.Builder
	args := []any{nodeID, depth}

	b.WriteString(`WITH RECURSIVE reach(id, steps) AS (`)
	b.WriteString(` SELECT ?, 0`)
	b.WriteString(` UNION`)
	b.WriteString(` SELECT CASE WHEN e.src = r.id THEN e.dst ELSE e.src END, r.steps + 1`)
	b.WriteString(` FROM reach r`)
	b.WriteString(` JOIN edges e ON (e.src = r.id OR e.dst = r.id)`)
	b.WriteString(` WHERE r.steps < ?`)
	if len(labels) > 0 {
		b.WriteString(` AND e.label IN (`)
		ph := make([]string, len(labels))
		for i := range ph {
			ph[i] = "?"
			args = append(args, labels[i])
		}
		b.WriteString(strings.Join(ph, ","))
		b.WriteString(`)`)
	}
	b.WriteString(`)`)
	b.WriteString(` SELECT DISTINCT id FROM reach WHERE id != ? ORDER BY id ASC`)
	args = append(args, nodeID)

	rows, err := tx.Query(b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("traverse.NeighborsAtDepth: query: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("traverse.NeighborsAtDepth: scan: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
