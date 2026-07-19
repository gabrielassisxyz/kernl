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

// IngestReview represents an item in the async review queue.
//
// The json tags are load-bearing: this struct is serialized directly by
// GET /api/ingest/queue, and REST is camelCase. Without them encoding/json
// falls back to Go field names and bakes them into the wire format.
type IngestReview struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	Title        string    `json:"title"`
	SourceNodeID string    `json:"sourceNodeId"`
	Action       string    `json:"action"`
	Payload      string    `json:"payload"`
	ContentHash  string    `json:"contentHash"`
	Tags         []string  `json:"tags"`
}

// Meta returns the common metadata for this node.
func (ir IngestReview) Meta() *Meta {
	return &Meta{ID: ir.ID, CreatedAt: ir.CreatedAt, UpdatedAt: ir.UpdatedAt}
}

// NodeAttrs marshals type-specific fields for the nodes.attrs column.
func (ir IngestReview) NodeAttrs() []byte {
	attrs := map[string]any{
		"source_node_id": ir.SourceNodeID,
		"action":         ir.Action,
		"payload":        ir.Payload,
		"content_hash":   ir.ContentHash,
	}
	data, _ := json.Marshal(attrs)
	return data
}

// NodeTags returns the tag slice (NodeSpec requirement).
func (ir IngestReview) NodeTags() []string { return ir.Tags }

// FTSFields returns full-text-searchable content.
func (ir IngestReview) FTSFields() FTSFields {
	body := strings.Join([]string{ir.Action, ir.Payload, ir.ContentHash}, " ")
	return FTSFields{Title: ir.Title, Body: body, Tags: strings.Join(ir.Tags, " ")}
}

// IngestReviewFilter narrows ListIngestReviews results.
type IngestReviewFilter struct {
	Tags  []string
	Limit int
}

// CreateIngestReview inserts a new ingest_review node and returns its ID.
func CreateIngestReview(ctx context.Context, tx *graph.WriteTx, ir IngestReview, author Author) (string, error) {
	return createNode(ctx, tx, "ingest_review", ir, author)
}

// GetIngestReview fetches a single ingest_review by ID.
func GetIngestReview(ctx context.Context, tx *graph.ReadTx, id string) (*IngestReview, error) {
	var title, attrsRaw sql.NullString
	var createdAt, updatedAt sql.NullString

	err := tx.QueryRow(
		`SELECT title, attrs, created_at, updated_at FROM nodes WHERE id = ? AND type = 'ingest_review'`,
		id,
	).Scan(&title, &attrsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, graph.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetIngestReview: %w", err)
	}

	var attrs struct {
		SourceNodeID string `json:"source_node_id"`
		Action       string `json:"action"`
		Payload      string `json:"payload"`
		ContentHash  string `json:"content_hash"`
	}
	if attrsRaw.Valid && attrsRaw.String != "" {
		if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
			return nil, fmt.Errorf("GetIngestReview: unmarshal attrs: %w", err)
		}
	}

	tags, err := selectTagsForNode(tx, id)
	if err != nil {
		return nil, err
	}

	return &IngestReview{
		ID:           id,
		CreatedAt:    tryParseTime(createdAt.String),
		UpdatedAt:    tryParseTime(updatedAt.String),
		Title:        title.String,
		SourceNodeID: attrs.SourceNodeID,
		Action:       attrs.Action,
		Payload:      attrs.Payload,
		ContentHash:  attrs.ContentHash,
		Tags:         tags,
	}, nil
}

// UpdateIngestReview modifies an existing ingest_review.
func UpdateIngestReview(ctx context.Context, tx *graph.WriteTx, ir IngestReview, author Author) error {
	return updateNode(ctx, tx, ir, author)
}

// DeleteIngestReview removes an ingest_review, preserving a tombstone revision.
func DeleteIngestReview(ctx context.Context, tx *graph.WriteTx, id string, author Author) error {
	return deleteNode(ctx, tx, id, author)
}

// ListIngestReviews returns ingest_reviews matching the filter.
func ListIngestReviews(ctx context.Context, tx *graph.ReadTx, f IngestReviewFilter) ([]*IngestReview, error) {
	query := `SELECT id, title, attrs, created_at, updated_at FROM nodes WHERE type = 'ingest_review'`
	var args []any

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
		return nil, fmt.Errorf("ListIngestReviews: %w", err)
	}
	defer rows.Close()

	var out []*IngestReview
	for rows.Next() {
		var id string
		var title, attrsRaw sql.NullString
		var createdAt, updatedAt sql.NullString
		if err := rows.Scan(&id, &title, &attrsRaw, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("ListIngestReviews: scan: %w", err)
		}

		var attrs struct {
			SourceNodeID string `json:"source_node_id"`
			Action       string `json:"action"`
			Payload      string `json:"payload"`
			ContentHash  string `json:"content_hash"`
		}
		if attrsRaw.Valid && attrsRaw.String != "" {
			if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
				return nil, fmt.Errorf("ListIngestReviews: unmarshal: %w", err)
			}
		}

		tags, err := selectTagsForNode(tx, id)
		if err != nil {
			return nil, err
		}

		out = append(out, &IngestReview{
			ID:           id,
			CreatedAt:    tryParseTime(createdAt.String),
			UpdatedAt:    tryParseTime(updatedAt.String),
			Title:        title.String,
			SourceNodeID: attrs.SourceNodeID,
			Action:       attrs.Action,
			Payload:      attrs.Payload,
			ContentHash:  attrs.ContentHash,
			Tags:         tags,
		})
	}
	return out, rows.Err()
}
