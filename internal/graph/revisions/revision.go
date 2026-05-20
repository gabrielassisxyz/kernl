// Package revisions provides read-only access to the node revision history.
package revisions

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

// Revision is a historical snapshot of a node mutation.
type Revision struct {
	ID        string          `json:"id"`
	NodeID    string          `json:"node_id"`
	ParentID  *string         `json:"parent_id,omitempty"`
	Diff      json.RawMessage `json:"diff"`
	Attrs     json.RawMessage `json:"attrs"`
	Author    string          `json:"author"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// List returns all revisions for a node, ordered by created_at DESC, id DESC.
// If no revisions exist, it returns an empty slice (not an error).
func List(ctx context.Context, tx *graph.ReadTx, nodeID string) ([]Revision, error) {
	if nodeID == "" {
		return nil, nil
	}

	rows, err := tx.Query(
		`SELECT id, node_id, parent_id, diff, attrs, author, created_at, updated_at
		 FROM revisions
		 WHERE node_id = ?
		 ORDER BY created_at DESC, id DESC`,
		nodeID,
	)
	if err != nil {
		return nil, fmt.Errorf("revisions.List: %w", err)
	}
	defer rows.Close()

	var revs []Revision
	for rows.Next() {
		var r Revision
		var nodeIDStr sql.NullString
		var parentID sql.NullString
		var diffStr string
		var attrsStr string
		var createdAt, updatedAt string

		if err := rows.Scan(&r.ID, &nodeIDStr, &parentID, &diffStr, &attrsStr, &r.Author, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("revisions.List: scan: %w", err)
		}

		if nodeIDStr.Valid {
			r.NodeID = nodeIDStr.String
		}
		if parentID.Valid {
			r.ParentID = &parentID.String
		}
		r.Diff = json.RawMessage(diffStr)
		r.Attrs = json.RawMessage(attrsStr)

		r.CreatedAt, err = time.Parse("2006-01-02T15:04:05Z", createdAt)
		if err != nil {
			return nil, fmt.Errorf("revisions.List: parse created_at: %w", err)
		}
		r.UpdatedAt, err = time.Parse("2006-01-02T15:04:05Z", updatedAt)
		if err != nil {
			return nil, fmt.Errorf("revisions.List: parse updated_at: %w", err)
		}

		revs = append(revs, r)
	}

	return revs, rows.Err()
}

// GetAt returns a specific revision by node_id and revision_id.
// Returns graph.ErrNotFound if no matching revision exists.
func GetAt(ctx context.Context, tx *graph.ReadTx, nodeID string, revisionID string) (Revision, error) {
	if nodeID == "" || revisionID == "" {
		return Revision{}, graph.ErrNotFound
	}

	var r Revision
	var nodeIDStr sql.NullString
	var parentID sql.NullString
	var diffStr string
	var attrsStr string
	var createdAt, updatedAt string

	err := tx.QueryRow(
		`SELECT id, node_id, parent_id, diff, attrs, author, created_at, updated_at
		 FROM revisions
		 WHERE node_id = ? AND id = ?`,
		nodeID, revisionID,
	).Scan(&r.ID, &nodeIDStr, &parentID, &diffStr, &attrsStr, &r.Author, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return Revision{}, graph.ErrNotFound
	}
	if err != nil {
		return Revision{}, fmt.Errorf("revisions.GetAt: %w", err)
	}

	if nodeIDStr.Valid {
		r.NodeID = nodeIDStr.String
	}
	if parentID.Valid {
		r.ParentID = &parentID.String
	}
	r.Diff = json.RawMessage(diffStr)
	r.Attrs = json.RawMessage(attrsStr)

	r.CreatedAt, err = time.Parse("2006-01-02T15:04:05Z", createdAt)
	if err != nil {
		return Revision{}, fmt.Errorf("revisions.GetAt: parse created_at: %w", err)
	}
	r.UpdatedAt, err = time.Parse("2006-01-02T15:04:05Z", updatedAt)
	if err != nil {
		return Revision{}, fmt.Errorf("revisions.GetAt: parse updated_at: %w", err)
	}

	return r, nil
}
