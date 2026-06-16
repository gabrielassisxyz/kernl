package edges

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/internal/ids"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// EdgeType enumerates the kinds of relationships edges represent.
type EdgeType string

const (
	EdgeTypeRelated   EdgeType = "related"
	EdgeTypeDependsOn EdgeType = "depends_on"
	EdgeTypeBlocks    EdgeType = "blocks"
	EdgeTypePartOf    EdgeType = "part_of"
	EdgeTypeLinksTo   EdgeType = "links_to"
)

// Edge models a directed relationship between two nodes.
type Edge struct {
	ID         string          `json:"id"`
	Src        string          `json:"src"`
	Dst        string          `json:"dst"`
	Label      string          `json:"label"`
	Type       EdgeType        `json:"type"`
	OwnerID    string          `json:"owner_id"`
	Visibility string          `json:"visibility"`
	Attrs      json.RawMessage `json:"attrs"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// edgeOptions holds optional query filters.
type edgeOptions struct {
	types []EdgeType
}

// EdgeOption is a functional option for edge queries.
type EdgeOption func(*edgeOptions)

// WithType adds an EdgeType filter to the query.
// Multiple calls accumulate; edges matching any listed type are returned.
func WithType(t EdgeType) EdgeOption {
	return func(o *edgeOptions) {
		o.types = append(o.types, t)
	}
}

// Create inserts a new edge after validating src, dst, and author.
// Returns the edge ID. Generates a UUIDv7 if edge.ID is empty.
func Create(ctx context.Context, tx *graph.WriteTx, edge Edge, author nodes.Author) (string, error) {
	if !author.Valid() {
		return "", graph.ErrAuthorRequired
	}

	// Validate src node exists
	var srcExists int
	err := tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id = ?`, edge.Src).Scan(&srcExists)
	if err != nil {
		return "", fmt.Errorf("edges.Create: check src: %w", err)
	}
	if srcExists == 0 {
		return "", fmt.Errorf("edges.Create: src node %q: %w", edge.Src, graph.ErrNotFound)
	}

	// Validate dst node exists
	var dstExists int
	err = tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id = ?`, edge.Dst).Scan(&dstExists)
	if err != nil {
		return "", fmt.Errorf("edges.Create: check dst: %w", err)
	}
	if dstExists == 0 {
		return "", fmt.Errorf("edges.Create: dst node %q: %w", edge.Dst, graph.ErrNotFound)
	}

	// Assign ID if empty
	id := edge.ID
	if id == "" {
		id = ids.New()
	}

	// Derive label from type; fall back to explicit Label if Type is empty
	label := string(edge.Type)
	if label == "" {
		label = edge.Label
	}

	attrs := edge.Attrs
	if len(attrs) == 0 {
		attrs = json.RawMessage("{}")
	}

	_, err = tx.Exec(
		`INSERT INTO edges(id, src, dst, label, owner_id, visibility, attrs) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, edge.Src, edge.Dst, label, edge.OwnerID, edge.Visibility, string(attrs),
	)
	if err != nil {
		return "", fmt.Errorf("edges.Create: insert: %w", err)
	}

	return id, nil
}

// Delete removes an edge by ID. Validates author before deletion.
func Delete(ctx context.Context, tx *graph.WriteTx, id string, author nodes.Author) error {
	if !author.Valid() {
		return graph.ErrAuthorRequired
	}

	result, err := tx.Exec(`DELETE FROM edges WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("edges.Delete: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("edges.Delete: rows affected: %w", err)
	}
	if n == 0 {
		return graph.ErrNotFound
	}
	return nil
}

// Outgoing returns all edges originating from nodeID, optionally filtered by EdgeType.
// Results are ordered by created_at ASC, id ASC for determinism.
func Outgoing(ctx context.Context, tx *graph.ReadTx, nodeID string, opts ...EdgeOption) ([]Edge, error) {
	return queryEdges(ctx, tx, `src = ?`, nodeID, opts)
}

// Incoming returns all edges targeting nodeID, optionally filtered by EdgeType.
// Results are ordered by created_at ASC, id ASC for determinism.
func Incoming(ctx context.Context, tx *graph.ReadTx, nodeID string, opts ...EdgeOption) ([]Edge, error) {
	return queryEdges(ctx, tx, `dst = ?`, nodeID, opts)
}

// queryEdges executes the edge query with optional type filtering.
func queryEdges(ctx context.Context, tx *graph.ReadTx, directionClause string, nodeID string, opts []EdgeOption) ([]Edge, error) {
	o := &edgeOptions{}
	for _, opt := range opts {
		opt(o)
	}

	query := fmt.Sprintf(
		`SELECT id, src, dst, label, owner_id, visibility, attrs, created_at, updated_at FROM edges WHERE %s`,
		directionClause,
	)
	args := []any{nodeID}

	if len(o.types) > 0 {
		placeholders := make([]string, len(o.types))
		for i, t := range o.types {
			placeholders[i] = "?"
			args = append(args, string(t))
		}
		query += fmt.Sprintf(" AND label IN (%s)", strings.Join(placeholders, ", "))
	}

	query += " ORDER BY created_at ASC, id ASC"

	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("edges.queryEdges: %w", err)
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		e, err := scanEdge(rows)
		if err != nil {
			return nil, err
		}
		edges = append(edges, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("edges.queryEdges: rows: %w", err)
	}

	return edges, nil
}

// scanEdge reads a single edge row into an Edge struct.
func scanEdge(rows *sql.Rows) (Edge, error) {
	var (
		e          Edge
		label      string
		attrsStr   string
		createdStr string
		updatedStr string
		ownerID    sql.NullString
		visibility sql.NullString
	)

	err := rows.Scan(&e.ID, &e.Src, &e.Dst, &label, &ownerID, &visibility, &attrsStr, &createdStr, &updatedStr)
	if err != nil {
		return Edge{}, fmt.Errorf("edges.scanEdge: %w", err)
	}

	e.Label = label
	e.Type = EdgeType(label)
	if ownerID.Valid {
		e.OwnerID = ownerID.String
	}
	if visibility.Valid {
		e.Visibility = visibility.String
	}
	e.Attrs = json.RawMessage(attrsStr)

	e.CreatedAt, err = parseTime(createdStr)
	if err != nil {
		return Edge{}, fmt.Errorf("edges.scanEdge: parse created_at: %w", err)
	}
	e.UpdatedAt, err = parseTime(updatedStr)
	if err != nil {
		return Edge{}, fmt.Errorf("edges.scanEdge: parse updated_at: %w", err)
	}

	return e, nil
}

// parseTime parses a SQLite TEXT timestamp into time.Time.
func parseTime(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02T15:04:05Z", s)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}
