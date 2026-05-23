package tags

import (
	"context"
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

// NodesAll returns node IDs that carry ALL of the given tags.
// Results are ordered by node_id ASC. An empty tag list returns an empty slice
// and a nil error.
func NodesAll(ctx context.Context, tx *graph.ReadTx, tags ...string) ([]string, error) {
	if len(tags) == 0 {
		return []string{}, nil
	}

	query := `SELECT nt.node_id FROM node_tags nt` +
		` JOIN tags t ON t.id = nt.tag_id` +
		` WHERE t.name IN (` + placeholders(len(tags)) + `)` +
		` GROUP BY nt.node_id` +
		` HAVING COUNT(DISTINCT t.name) = ?` +
		` ORDER BY nt.node_id ASC`

	args := make([]any, 0, len(tags)+1)
	for _, tag := range tags {
		args = append(args, tag)
	}
	args = append(args, len(tags))

	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("tags.NodesAll: query: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("tags.NodesAll: scan: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// NodesAny returns node IDs that carry ANY of the given tags.
// Results are ordered by node_id ASC, deduplicated. An empty tag list
// returns an empty slice and a nil error.
func NodesAny(ctx context.Context, tx *graph.ReadTx, tags ...string) ([]string, error) {
	if len(tags) == 0 {
		return []string{}, nil
	}

	query := `SELECT DISTINCT nt.node_id FROM node_tags nt` +
		` JOIN tags t ON t.id = nt.tag_id` +
		` WHERE t.name IN (` + placeholders(len(tags)) + `)` +
		` ORDER BY nt.node_id ASC`

	args := make([]any, len(tags))
	for i, tag := range tags {
		args[i] = tag
	}

	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("tags.NodesAny: query: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("tags.NodesAny: scan: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// placeholders returns a comma-separated list of n ? placeholders.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]string, n)
	for i := range out {
		out[i] = "?"
	}
	return strings.Join(out, ",")
}
