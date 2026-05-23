package nodes

import (
	"context"
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

// byTypeOptions holds optional filters for ByType.
type byTypeOptions struct {
	tags  []string
	limit int
}

// ByTypeOption is a functional option for ByType queries.
type ByTypeOption func(*byTypeOptions)

// WithTags filters results to nodes that carry ALL of the given tags.
func WithTags(tags ...string) ByTypeOption {
	return func(o *byTypeOptions) {
		o.tags = append(o.tags, tags...)
	}
}

// WithLimit caps the result set.
func WithLimit(n int) ByTypeOption {
	return func(o *byTypeOptions) {
		o.limit = n
	}
}

// ByType returns metadata for nodes of the specified type.
// Results are ordered by updated_at DESC, id ASC for determinism.
// An empty result returns an empty slice and a nil error.
func ByType(ctx context.Context, tx *graph.ReadTx, typ string, opts ...ByTypeOption) ([]Meta, error) {
	o := &byTypeOptions{}
	for _, opt := range opts {
		opt(o)
	}

	var b strings.Builder
	args := []any{typ}

	b.WriteString(`SELECT n.id, n.created_at, n.updated_at FROM nodes n WHERE n.type = ?`)

	if len(o.tags) > 0 {
		b.WriteString(` AND n.id IN (`)
		b.WriteString(`SELECT nt.node_id FROM node_tags nt`)
		b.WriteString(` JOIN tags t ON t.id = nt.tag_id`)
		b.WriteString(` WHERE t.name IN (`)
		b.WriteString(placeholders(len(o.tags)))
		b.WriteString(`)`)
		b.WriteString(` GROUP BY nt.node_id HAVING COUNT(DISTINCT t.name) = ?`)
		b.WriteString(`)`)
		for _, tag := range o.tags {
			args = append(args, tag)
		}
		args = append(args, len(o.tags))
	}

	b.WriteString(` ORDER BY n.updated_at DESC, n.id ASC`)

	if o.limit > 0 {
		b.WriteString(` LIMIT ?`)
		args = append(args, o.limit)
	}

	rows, err := tx.Query(b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("nodes.ByType: query: %w", err)
	}
	defer rows.Close()

	var out []Meta
	for rows.Next() {
		var id, createdAt, updatedAt string
		if err := rows.Scan(&id, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("nodes.ByType: scan: %w", err)
		}
		out = append(out, Meta{
			ID:        id,
			CreatedAt: tryParseTime(createdAt),
			UpdatedAt: tryParseTime(updatedAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("nodes.ByType: rows: %w", err)
	}

	return out, nil
}
