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

// WorkflowRun represents an execution trace of a workflow.
type WorkflowRun struct {
	ID           string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Title        string
	WorkflowName string
	Status       string
	RunData      string
	Tags         []string
}

// Meta returns the common metadata for this node.
func (wr WorkflowRun) Meta() *Meta {
	return &Meta{ID: wr.ID, CreatedAt: wr.CreatedAt, UpdatedAt: wr.UpdatedAt}
}

// NodeAttrs marshals type-specific fields for the nodes.attrs column.
func (wr WorkflowRun) NodeAttrs() []byte {
	attrs := map[string]any{
		"workflow_name": wr.WorkflowName,
		"status":        wr.Status,
		"run_data":      wr.RunData,
	}
	data, _ := json.Marshal(attrs)
	return data
}

// NodeTags returns the tag slice (NodeSpec requirement).
func (wr WorkflowRun) NodeTags() []string { return wr.Tags }

// FTSFields returns full-text-searchable content.
func (wr WorkflowRun) FTSFields() FTSFields {
	return FTSFields{Title: wr.Title, Body: wr.WorkflowName + " " + wr.Status, Tags: strings.Join(wr.Tags, " ")}
}

// WorkflowRunFilter narrows ListWorkflowRuns results.
type WorkflowRunFilter struct {
	WorkflowName string
	Status       string
	Tags         []string
	Limit        int
}

// CreateWorkflowRun inserts a new workflow run node and returns its ID.
func CreateWorkflowRun(ctx context.Context, tx *graph.WriteTx, wr WorkflowRun, author Author) (string, error) {
	return createNode(ctx, tx, "workflow_run", wr, author)
}

// GetWorkflowRun fetches a single workflow run by ID.
func GetWorkflowRun(ctx context.Context, tx *graph.ReadTx, id string) (*WorkflowRun, error) {
	var title, attrsRaw sql.NullString
	var createdAt, updatedAt sql.NullString

	err := tx.QueryRow(
		`SELECT title, attrs, created_at, updated_at FROM nodes WHERE id = ? AND type = 'workflow_run'`,
		id,
	).Scan(&title, &attrsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, graph.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetWorkflowRun: %w", err)
	}

	var attrs struct {
		WorkflowName string `json:"workflow_name"`
		Status       string `json:"status"`
		RunData      string `json:"run_data"`
	}
	if attrsRaw.Valid && attrsRaw.String != "" {
		if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
			return nil, fmt.Errorf("GetWorkflowRun: unmarshal attrs: %w", err)
		}
	}

	tags, err := selectTagsForNode(tx, id)
	if err != nil {
		return nil, err
	}

	return &WorkflowRun{
		ID:           id,
		CreatedAt:    tryParseTime(createdAt.String),
		UpdatedAt:    tryParseTime(updatedAt.String),
		Title:        title.String,
		WorkflowName: attrs.WorkflowName,
		Status:       attrs.Status,
		RunData:      attrs.RunData,
		Tags:         tags,
	}, nil
}

// UpdateWorkflowRun modifies an existing workflow run.
func UpdateWorkflowRun(ctx context.Context, tx *graph.WriteTx, wr WorkflowRun, author Author) error {
	return updateNode(ctx, tx, wr, author)
}

// DeleteWorkflowRun removes a workflow run, preserving a tombstone revision.
func DeleteWorkflowRun(ctx context.Context, tx *graph.WriteTx, id string, author Author) error {
	return deleteNode(ctx, tx, id, author)
}

// ListWorkflowRuns returns workflow runs matching the filter.
func ListWorkflowRuns(ctx context.Context, tx *graph.ReadTx, f WorkflowRunFilter) ([]*WorkflowRun, error) {
	query := `SELECT id, title, attrs, created_at, updated_at FROM nodes WHERE type = 'workflow_run'`
	var args []any

	if f.WorkflowName != "" {
		query += ` AND json_extract(attrs, '$.workflow_name') = ?`
		args = append(args, f.WorkflowName)
	}

	if f.Status != "" {
		query += ` AND json_extract(attrs, '$.status') = ?`
		args = append(args, f.Status)
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
		return nil, fmt.Errorf("ListWorkflowRuns: %w", err)
	}
	defer rows.Close()

	var out []*WorkflowRun
	for rows.Next() {
		var id string
		var title, attrsRaw sql.NullString
		var createdAt, updatedAt sql.NullString
		if err := rows.Scan(&id, &title, &attrsRaw, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("ListWorkflowRuns: scan: %w", err)
		}

		var attrs struct {
			WorkflowName string `json:"workflow_name"`
			Status       string `json:"status"`
			RunData      string `json:"run_data"`
		}
		if attrsRaw.Valid && attrsRaw.String != "" {
			if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
				return nil, fmt.Errorf("ListWorkflowRuns: unmarshal: %w", err)
			}
		}

		tags, err := selectTagsForNode(tx, id)
		if err != nil {
			return nil, err
		}

		out = append(out, &WorkflowRun{
			ID:           id,
			CreatedAt:    tryParseTime(createdAt.String),
			UpdatedAt:    tryParseTime(updatedAt.String),
			Title:        title.String,
			WorkflowName: attrs.WorkflowName,
			Status:       attrs.Status,
			RunData:      attrs.RunData,
			Tags:         tags,
		})
	}
	return out, rows.Err()
}
