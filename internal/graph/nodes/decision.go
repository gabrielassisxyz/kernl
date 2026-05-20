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

// Decision represents a conclusion reached after deliberation.
type Decision struct {
	ID        string
	CreatedAt time.Time
	UpdatedAt time.Time
	Title     string
	Body      string
	Context   string
	Outcome   string
	DecidedAt time.Time
	Tags      []string
}

// Meta returns the common metadata for this node.
func (d Decision) Meta() *Meta {
	return &Meta{ID: d.ID, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
}

// NodeAttrs marshals type-specific fields for the nodes.attrs column.
func (d Decision) NodeAttrs() []byte {
	attrs := map[string]any{
		"body":       d.Body,
		"context":    d.Context,
		"outcome":    d.Outcome,
		"decided_at": d.DecidedAt,
	}
	data, _ := json.Marshal(attrs)
	return data
}

// NodeTags returns the tag slice (NodeSpec requirement).
func (d Decision) NodeTags() []string { return d.Tags }

// FTSFields returns full-text-searchable content.
func (d Decision) FTSFields() FTSFields {
	body := strings.Join([]string{d.Body, d.Context, d.Outcome}, " ")
	return FTSFields{Title: d.Title, Body: body, Tags: strings.Join(d.Tags, " ")}
}

// DecisionFilter narrows ListDecisions results.
type DecisionFilter struct {
	Since *time.Time
	Tags  []string
	Limit int
}

// CreateDecision inserts a new decision node and returns its ID.
func CreateDecision(ctx context.Context, tx *graph.WriteTx, d Decision, author Author) (string, error) {
	return createNode(ctx, tx, "decision", d, author)
}

// GetDecision fetches a single decision by ID.
func GetDecision(ctx context.Context, tx *graph.ReadTx, id string) (*Decision, error) {
	var title, attrsRaw sql.NullString
	var createdAt, updatedAt sql.NullString

	err := tx.QueryRow(
		`SELECT title, attrs, created_at, updated_at FROM nodes WHERE id = ? AND type = 'decision'`,
		id,
	).Scan(&title, &attrsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, graph.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetDecision: %w", err)
	}

	var attrs struct {
		Body      string    `json:"body"`
		Context   string    `json:"context"`
		Outcome   string    `json:"outcome"`
		DecidedAt time.Time `json:"decided_at"`
	}
	if attrsRaw.Valid && attrsRaw.String != "" {
		if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
			return nil, fmt.Errorf("GetDecision: unmarshal attrs: %w", err)
		}
	}

	tags, err := selectTagsForNode(tx, id)
	if err != nil {
		return nil, err
	}

	return &Decision{
		ID:        id,
		CreatedAt: tryParseTime(createdAt.String),
		UpdatedAt: tryParseTime(updatedAt.String),
		Title:     title.String,
		Body:      attrs.Body,
		Context:   attrs.Context,
		Outcome:   attrs.Outcome,
		DecidedAt: attrs.DecidedAt,
		Tags:      tags,
	}, nil
}

// UpdateDecision modifies an existing decision.
func UpdateDecision(ctx context.Context, tx *graph.WriteTx, d Decision, author Author) error {
	return updateNode(ctx, tx, d, author)
}

// DeleteDecision removes a decision, preserving a tombstone revision.
func DeleteDecision(ctx context.Context, tx *graph.WriteTx, id string, author Author) error {
	return deleteNode(ctx, tx, id, author)
}

// ListDecisions returns decisions matching the filter.
func ListDecisions(ctx context.Context, tx *graph.ReadTx, f DecisionFilter) ([]*Decision, error) {
	query := `SELECT id, title, attrs, created_at, updated_at FROM nodes WHERE type = 'decision'`
	var args []any

	if f.Since != nil {
		query += ` AND json_extract(attrs, '$.decided_at') >= ?`
		args = append(args, f.Since.Format(time.RFC3339))
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
		return nil, fmt.Errorf("ListDecisions: %w", err)
	}
	defer rows.Close()

	var out []*Decision
	for rows.Next() {
		var id string
		var title, attrsRaw sql.NullString
		var createdAt, updatedAt sql.NullString
		if err := rows.Scan(&id, &title, &attrsRaw, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("ListDecisions: scan: %w", err)
		}

		var attrs struct {
			Body      string    `json:"body"`
			Context   string    `json:"context"`
			Outcome   string    `json:"outcome"`
			DecidedAt time.Time `json:"decided_at"`
		}
		if attrsRaw.Valid && attrsRaw.String != "" {
			if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
				return nil, fmt.Errorf("ListDecisions: unmarshal: %w", err)
			}
		}

		tags, err := selectTagsForNode(tx, id)
		if err != nil {
			return nil, err
		}

		out = append(out, &Decision{
			ID:        id,
			CreatedAt: tryParseTime(createdAt.String),
			UpdatedAt: tryParseTime(updatedAt.String),
			Title:     title.String,
			Body:      attrs.Body,
			Context:   attrs.Context,
			Outcome:   attrs.Outcome,
			DecidedAt: attrs.DecidedAt,
			Tags:      tags,
		})
	}
	return out, rows.Err()
}
