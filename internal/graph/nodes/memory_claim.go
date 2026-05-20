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

// MemoryClaim represents a factual assertion in the knowledge graph.
type MemoryClaim struct {
	ID         string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Title      string
	Statement  string
	Confidence float64
	Subject    string
	Source     string
	Tags       []string
}

// Meta returns the common metadata for this node.
func (mc MemoryClaim) Meta() *Meta {
	return &Meta{ID: mc.ID, CreatedAt: mc.CreatedAt, UpdatedAt: mc.UpdatedAt}
}

// NodeAttrs marshals type-specific fields for the nodes.attrs column.
func (mc MemoryClaim) NodeAttrs() []byte {
	attrs := map[string]any{
		"statement":  mc.Statement,
		"confidence": mc.Confidence,
		"subject":    mc.Subject,
		"source":     mc.Source,
	}
	data, _ := json.Marshal(attrs)
	return data
}

// NodeTags returns the tag slice (NodeSpec requirement).
func (mc MemoryClaim) NodeTags() []string { return mc.Tags }

// FTSFields returns full-text-searchable content.
func (mc MemoryClaim) FTSFields() FTSFields {
	return FTSFields{Title: mc.Title, Body: mc.Statement, Tags: strings.Join(mc.Tags, " ")}
}

// MemoryClaimFilter narrows ListMemoryClaims results.
type MemoryClaimFilter struct {
	Subject       string
	MinConfidence float64
	Tags          []string
	Limit         int
}

// CreateMemoryClaim inserts a new memory claim node and returns its ID.
func CreateMemoryClaim(ctx context.Context, tx *graph.WriteTx, mc MemoryClaim, author Author) (string, error) {
	return createNode(ctx, tx, "memory_claim", mc, author)
}

// GetMemoryClaim fetches a single memory claim by ID.
func GetMemoryClaim(ctx context.Context, tx *graph.ReadTx, id string) (*MemoryClaim, error) {
	var title, attrsRaw sql.NullString
	var createdAt, updatedAt sql.NullString

	err := tx.QueryRow(
		`SELECT title, attrs, created_at, updated_at FROM nodes WHERE id = ? AND type = 'memory_claim'`,
		id,
	).Scan(&title, &attrsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, graph.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetMemoryClaim: %w", err)
	}

	var attrs struct {
		Statement  string  `json:"statement"`
		Confidence float64 `json:"confidence"`
		Subject    string  `json:"subject"`
		Source     string  `json:"source"`
	}
	if attrsRaw.Valid && attrsRaw.String != "" {
		if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
			return nil, fmt.Errorf("GetMemoryClaim: unmarshal attrs: %w", err)
		}
	}

	tags, err := selectTagsForNode(tx, id)
	if err != nil {
		return nil, err
	}

	return &MemoryClaim{
		ID:         id,
		CreatedAt:  tryParseTime(createdAt.String),
		UpdatedAt:  tryParseTime(updatedAt.String),
		Title:      title.String,
		Statement:  attrs.Statement,
		Confidence: attrs.Confidence,
		Subject:    attrs.Subject,
		Source:     attrs.Source,
		Tags:       tags,
	}, nil
}

// UpdateMemoryClaim modifies an existing memory claim.
func UpdateMemoryClaim(ctx context.Context, tx *graph.WriteTx, mc MemoryClaim, author Author) error {
	return updateNode(ctx, tx, mc, author)
}

// DeleteMemoryClaim removes a memory claim, preserving a tombstone revision.
func DeleteMemoryClaim(ctx context.Context, tx *graph.WriteTx, id string, author Author) error {
	return deleteNode(ctx, tx, id, author)
}

// ListMemoryClaims returns memory claims matching the filter.
func ListMemoryClaims(ctx context.Context, tx *graph.ReadTx, f MemoryClaimFilter) ([]*MemoryClaim, error) {
	query := `SELECT id, title, attrs, created_at, updated_at FROM nodes WHERE type = 'memory_claim'`
	var args []any

	if f.Subject != "" {
		query += ` AND json_extract(attrs, '$.subject') = ?`
		args = append(args, f.Subject)
	}

	if f.MinConfidence > 0 {
		query += ` AND CAST(json_extract(attrs, '$.confidence') AS REAL) >= ?`
		args = append(args, f.MinConfidence)
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
		return nil, fmt.Errorf("ListMemoryClaims: %w", err)
	}
	defer rows.Close()

	var out []*MemoryClaim
	for rows.Next() {
		var id string
		var title, attrsRaw sql.NullString
		var createdAt, updatedAt sql.NullString
		if err := rows.Scan(&id, &title, &attrsRaw, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("ListMemoryClaims: scan: %w", err)
		}

		var attrs struct {
			Statement  string  `json:"statement"`
			Confidence float64 `json:"confidence"`
			Subject    string  `json:"subject"`
			Source     string  `json:"source"`
		}
		if attrsRaw.Valid && attrsRaw.String != "" {
			if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
				return nil, fmt.Errorf("ListMemoryClaims: unmarshal: %w", err)
			}
		}

		tags, err := selectTagsForNode(tx, id)
		if err != nil {
			return nil, err
		}

		out = append(out, &MemoryClaim{
			ID:         id,
			CreatedAt:  tryParseTime(createdAt.String),
			UpdatedAt:  tryParseTime(updatedAt.String),
			Title:      title.String,
			Statement:  attrs.Statement,
			Confidence: attrs.Confidence,
			Subject:    attrs.Subject,
			Source:     attrs.Source,
			Tags:       tags,
		})
	}
	return out, rows.Err()
}
