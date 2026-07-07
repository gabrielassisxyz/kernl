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

// Capture represents content captured from an external source.
type Capture struct {
	ID              string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Title           string
	Body            string
	CapturedFrom    string
	Tags            []string
	SuggestedAction string
	// SuggestedProjectID is set by the classifier when it suggests filing the
	// capture as a task under a specific project; empty otherwise.
	SuggestedProjectID string
	// SuggestedProjectTitle/Description/InitialTasks are set when the classifier
	// suggests promoting the capture into a new project.
	SuggestedProjectTitle       string
	SuggestedProjectDescription string
	SuggestedInitialTasks       []string
	BatchID                     string
	BatchSource                 string
	BatchSequence               int
	BatchSender                 string
	BatchTimestamp              string
	BatchContextTitle           string
}

// Meta returns the common metadata for this node.
func (c Capture) Meta() *Meta {
	return &Meta{ID: c.ID, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
}

// NodeAttrs marshals type-specific fields for the nodes.attrs column.
func (c Capture) NodeAttrs() []byte {
	attrs := map[string]any{
		"body":                          c.Body,
		"captured_from":                 c.CapturedFrom,
		"suggested_action":              c.SuggestedAction,
		"suggested_project_id":          c.SuggestedProjectID,
		"suggested_project_title":       c.SuggestedProjectTitle,
		"suggested_project_description": c.SuggestedProjectDescription,
		"suggested_initial_tasks":       c.SuggestedInitialTasks,
		"batch_id":                      c.BatchID,
		"batch_source":                  c.BatchSource,
		"batch_sequence":                c.BatchSequence,
		"batch_sender":                  c.BatchSender,
		"batch_timestamp":               c.BatchTimestamp,
		"batch_context_title":           c.BatchContextTitle,
	}
	data, _ := json.Marshal(attrs)
	return data
}

// NodeTags returns the tag slice (NodeSpec requirement).
func (c Capture) NodeTags() []string { return c.Tags }

// FTSFields returns full-text-searchable content.
func (c Capture) FTSFields() FTSFields {
	return FTSFields{Title: c.Title, Body: c.Body, Tags: strings.Join(c.Tags, " ")}
}

// CaptureFilter narrows ListCaptures results.
type CaptureFilter struct {
	CapturedFromPrefix string
	Tags               []string
	BatchID            string
	Limit              int
}

type captureAttrs struct {
	Body                        string   `json:"body"`
	CapturedFrom                string   `json:"captured_from"`
	SuggestedAction             string   `json:"suggested_action"`
	SuggestedProjectID          string   `json:"suggested_project_id"`
	SuggestedProjectTitle       string   `json:"suggested_project_title"`
	SuggestedProjectDescription string   `json:"suggested_project_description"`
	SuggestedInitialTasks       []string `json:"suggested_initial_tasks"`
	BatchID                     string   `json:"batch_id"`
	BatchSource                 string   `json:"batch_source"`
	BatchSequence               int      `json:"batch_sequence"`
	BatchSender                 string   `json:"batch_sender"`
	BatchTimestamp              string   `json:"batch_timestamp"`
	BatchContextTitle           string   `json:"batch_context_title"`
}

// CreateCapture inserts a new capture node and returns its ID.
func CreateCapture(ctx context.Context, tx *graph.WriteTx, c Capture, author Author) (string, error) {
	return createNode(ctx, tx, "capture", c, author)
}

// GetCapture fetches a single capture by ID.
func GetCapture(ctx context.Context, tx *graph.ReadTx, id string) (*Capture, error) {
	var title, attrsRaw sql.NullString
	var createdAt, updatedAt sql.NullString

	err := tx.QueryRow(
		`SELECT title, attrs, created_at, updated_at FROM nodes WHERE id = ? AND type = 'capture'`,
		id,
	).Scan(&title, &attrsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, graph.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetCapture: %w", err)
	}

	var attrs captureAttrs
	if attrsRaw.Valid && attrsRaw.String != "" {
		if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
			return nil, fmt.Errorf("GetCapture: unmarshal attrs: %w", err)
		}
	}

	tags, err := selectTagsForNode(tx, id)
	if err != nil {
		return nil, err
	}

	return &Capture{
		ID:                          id,
		CreatedAt:                   tryParseTime(createdAt.String),
		UpdatedAt:                   tryParseTime(updatedAt.String),
		Title:                       title.String,
		Body:                        attrs.Body,
		CapturedFrom:                attrs.CapturedFrom,
		Tags:                        tags,
		SuggestedAction:             attrs.SuggestedAction,
		SuggestedProjectID:          attrs.SuggestedProjectID,
		SuggestedProjectTitle:       attrs.SuggestedProjectTitle,
		SuggestedProjectDescription: attrs.SuggestedProjectDescription,
		SuggestedInitialTasks:       attrs.SuggestedInitialTasks,
		BatchID:                     attrs.BatchID,
		BatchSource:                 attrs.BatchSource,
		BatchSequence:               attrs.BatchSequence,
		BatchSender:                 attrs.BatchSender,
		BatchTimestamp:              attrs.BatchTimestamp,
		BatchContextTitle:           attrs.BatchContextTitle,
	}, nil
}

// UpdateCapture modifies an existing capture.
func UpdateCapture(ctx context.Context, tx *graph.WriteTx, c Capture, author Author) error {
	return updateNode(ctx, tx, c, author)
}

// DeleteCapture removes a capture, preserving a tombstone revision.
func DeleteCapture(ctx context.Context, tx *graph.WriteTx, id string, author Author) error {
	return deleteNode(ctx, tx, id, author)
}

// ListCaptures returns captures matching the filter.
func ListCaptures(ctx context.Context, tx *graph.ReadTx, f CaptureFilter) ([]*Capture, error) {
	query := `SELECT id, title, attrs, created_at, updated_at FROM nodes WHERE type = 'capture'`
	var args []any

	if f.CapturedFromPrefix != "" {
		query += ` AND json_extract(attrs, '$.captured_from') LIKE ?`
		args = append(args, f.CapturedFromPrefix+"%")
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
	if f.BatchID != "" {
		query += ` AND json_extract(attrs, '$.batch_id') = ?`
		args = append(args, f.BatchID)
	}

	query += ` ORDER BY created_at DESC`
	if f.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, f.Limit)
	}

	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListCaptures: %w", err)
	}
	defer rows.Close()

	var out []*Capture
	for rows.Next() {
		var id string
		var title, attrsRaw sql.NullString
		var createdAt, updatedAt sql.NullString
		if err := rows.Scan(&id, &title, &attrsRaw, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("ListCaptures: scan: %w", err)
		}

		var attrs captureAttrs
		if attrsRaw.Valid && attrsRaw.String != "" {
			if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
				return nil, fmt.Errorf("ListCaptures: unmarshal: %w", err)
			}
		}

		tags, err := selectTagsForNode(tx, id)
		if err != nil {
			return nil, err
		}

		out = append(out, &Capture{
			ID:                          id,
			CreatedAt:                   tryParseTime(createdAt.String),
			UpdatedAt:                   tryParseTime(updatedAt.String),
			Title:                       title.String,
			Body:                        attrs.Body,
			CapturedFrom:                attrs.CapturedFrom,
			Tags:                        tags,
			SuggestedAction:             attrs.SuggestedAction,
			SuggestedProjectID:          attrs.SuggestedProjectID,
			SuggestedProjectTitle:       attrs.SuggestedProjectTitle,
			SuggestedProjectDescription: attrs.SuggestedProjectDescription,
			SuggestedInitialTasks:       attrs.SuggestedInitialTasks,
			BatchID:                     attrs.BatchID,
			BatchSource:                 attrs.BatchSource,
			BatchSequence:               attrs.BatchSequence,
			BatchSender:                 attrs.BatchSender,
			BatchTimestamp:              attrs.BatchTimestamp,
			BatchContextTitle:           attrs.BatchContextTitle,
		})
	}
	return out, rows.Err()
}
