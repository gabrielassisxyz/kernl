package nodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

// MemoryRefutation represents a challenge to a MemoryClaim.
type MemoryRefutation struct {
	ID         string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Title      string
	ClaimID    string
	Reason     string
	Confidence float64
	Tags       []string
}

// Meta returns the common metadata for this node.
func (mr MemoryRefutation) Meta() *Meta {
	return &Meta{ID: mr.ID, CreatedAt: mr.CreatedAt, UpdatedAt: mr.UpdatedAt}
}

// NodeAttrs marshals type-specific fields for the nodes.attrs column.
func (mr MemoryRefutation) NodeAttrs() []byte {
	attrs := map[string]any{
		"claim_id":   mr.ClaimID,
		"reason":     mr.Reason,
		"confidence": mr.Confidence,
	}
	data, _ := json.Marshal(attrs)
	return data
}

// NodeTags returns the tag slice (NodeSpec requirement).
func (mr MemoryRefutation) NodeTags() []string { return mr.Tags }

// FTSFields returns full-text-searchable content.
func (mr MemoryRefutation) FTSFields() FTSFields {
	return FTSFields{Title: mr.Title, Body: mr.Reason, Tags: strings.Join(mr.Tags, " ")}
}

// MemoryRefutationFilter narrows ListMemoryRefutations results.
type MemoryRefutationFilter struct {
	ClaimID string
	Tags    []string
	Limit   int
}

// CreateMemoryRefutation inserts a new memory refutation node and returns its ID.
func CreateMemoryRefutation(ctx context.Context, tx *graph.WriteTx, mr MemoryRefutation, author Author) (string, error) {
	return createNode(ctx, tx, "memory_refutation", mr, author)
}

// GetMemoryRefutation fetches a single memory refutation by ID.
func GetMemoryRefutation(ctx context.Context, tx *graph.ReadTx, id string) (*MemoryRefutation, error) {
	var title, attrsRaw sql.NullString
	var createdAt, updatedAt sql.NullString

	err := tx.QueryRow(
		`SELECT title, attrs, created_at, updated_at FROM nodes WHERE id = ? AND type = 'memory_refutation'`,
		id,
	).Scan(&title, &attrsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, graph.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetMemoryRefutation: %w", err)
	}

	var attrs struct {
		ClaimID    string  `json:"claim_id"`
		Reason     string  `json:"reason"`
		Confidence float64 `json:"confidence"`
	}
	if attrsRaw.Valid && attrsRaw.String != "" {
		if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
			return nil, fmt.Errorf("GetMemoryRefutation: unmarshal attrs: %w", err)
		}
	}

	tags, err := selectTagsForNode(tx, id)
	if err != nil {
		return nil, err
	}

	return &MemoryRefutation{
		ID:         id,
		CreatedAt:  tryParseTime(createdAt.String),
		UpdatedAt:  tryParseTime(updatedAt.String),
		Title:      title.String,
		ClaimID:    attrs.ClaimID,
		Reason:     attrs.Reason,
		Confidence: attrs.Confidence,
		Tags:       tags,
	}, nil
}

// UpdateMemoryRefutation modifies an existing memory refutation.
func UpdateMemoryRefutation(ctx context.Context, tx *graph.WriteTx, mr MemoryRefutation, author Author) error {
	return updateNode(ctx, tx, mr, author)
}

// DeleteMemoryRefutation removes a memory refutation, preserving a tombstone revision.
func DeleteMemoryRefutation(ctx context.Context, tx *graph.WriteTx, id string, author Author) error {
	return deleteNode(ctx, tx, id, author)
}

// ListMemoryRefutations returns memory refutations matching the filter.
func ListMemoryRefutations(ctx context.Context, tx *graph.ReadTx, f MemoryRefutationFilter) ([]*MemoryRefutation, error) {
	query := `SELECT id, title, attrs, created_at, updated_at FROM nodes WHERE type = 'memory_refutation'`
	var args []any

	if f.ClaimID != "" {
		query += ` AND json_extract(attrs, '$.claim_id') = ?`
		args = append(args, f.ClaimID)
	}

	if len(f.Tags) > 0 {
		query += fmt.Sprintf(
			` AND id IN (SELECT nt.node_id FROM node_tags nt JOIN tags t ON t.id = nt.tag_id WHERE t.name IN (%s))`,
			placeholders(len(f.Tags)),
		)
		for _, tag := range f.Tags {
			args = append(args, tag)
		}
	}

	query += ` ORDER BY created_at DESC`
	if f.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, f.Limit)
	}

	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListMemoryRefutations: %w", err)
	}
	defer rows.Close()

	var out []*MemoryRefutation
	for rows.Next() {
		var id string
		var title, attrsRaw sql.NullString
		var createdAt, updatedAt sql.NullString
		if err := rows.Scan(&id, &title, &attrsRaw, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("ListMemoryRefutations: scan: %w", err)
		}

		var attrs struct {
			ClaimID    string  `json:"claim_id"`
			Reason     string  `json:"reason"`
			Confidence float64 `json:"confidence"`
		}
		if attrsRaw.Valid && attrsRaw.String != "" {
			if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
				return nil, fmt.Errorf("ListMemoryRefutations: unmarshal: %w", err)
			}
		}

		tags, err := selectTagsForNode(tx, id)
		if err != nil {
			return nil, err
		}

		out = append(out, &MemoryRefutation{
			ID:         id,
			CreatedAt:  tryParseTime(createdAt.String),
			UpdatedAt:  tryParseTime(updatedAt.String),
			Title:      title.String,
			ClaimID:    attrs.ClaimID,
			Reason:     attrs.Reason,
			Confidence: attrs.Confidence,
			Tags:       tags,
		})
	}
	return out, rows.Err()
}
