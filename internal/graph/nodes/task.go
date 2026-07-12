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

// Task is a human-created organizational node (type "task"). Like Project, a
// Task is distinct from an orchestrator bead. A task may belong to a project;
// the canonical link is a part_of edge (task -> project) created at the API
// layer, while ProjectID is mirrored here for cheap filtering.
type Task struct {
	ID          string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Title       string
	Description string
	Status      string // todo | in_progress | done
	ProjectID   string // empty when the task is not assigned to a project
	Tags        []string
}

// Task status vocabulary. DefaultTaskStatus is applied on create when unset.
const (
	TaskStatusTodo       = "todo"
	TaskStatusInProgress = "in_progress"
	TaskStatusDone       = "done"
	DefaultTaskStatus    = TaskStatusTodo
)

func (t Task) Meta() *Meta { return &Meta{ID: t.ID, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt} }

func (t Task) NodeAttrs() []byte {
	data, _ := json.Marshal(map[string]any{
		"description": t.Description,
		"status":      t.Status,
		"projectId":   t.ProjectID,
	})
	return data
}

func (t Task) NodeTags() []string { return t.Tags }

func (t Task) FTSFields() FTSFields {
	return FTSFields{Title: t.Title, Body: t.Description, Tags: strings.Join(t.Tags, " ")}
}

// CreateTask inserts a new task node and returns its ID. The caller is
// responsible for creating the part_of edge to the project (if any).
func CreateTask(ctx context.Context, tx *graph.WriteTx, t Task, author Author) (string, error) {
	if t.Status == "" {
		t.Status = DefaultTaskStatus
	}
	return createNode(ctx, tx, "task", t, author)
}

// GetTask fetches a single task by ID.
func GetTask(ctx context.Context, tx *graph.ReadTx, id string) (*Task, error) {
	var title, attrsRaw, createdAt, updatedAt sql.NullString
	err := tx.QueryRow(
		`SELECT title, attrs, created_at, updated_at FROM nodes WHERE id = ? AND type = 'task' AND deleted_at IS NULL`,
		id,
	).Scan(&title, &attrsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, graph.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetTask: %w", err)
	}
	t, err := scanTask(id, title, attrsRaw, createdAt, updatedAt)
	if err != nil {
		return nil, err
	}
	if t.Tags, err = selectTagsForNode(tx, id); err != nil {
		return nil, err
	}
	return t, nil
}

// ListTasks returns non-deleted tasks, newest first. When projectID is
// non-empty, only tasks belonging to that project are returned.
func ListTasks(ctx context.Context, tx *graph.ReadTx, projectID string) ([]*Task, error) {
	query := `SELECT id, title, attrs, created_at, updated_at FROM nodes WHERE type = 'task' AND deleted_at IS NULL`
	var args []any
	if projectID != "" {
		query += ` AND json_extract(attrs, '$.projectId') = ?`
		args = append(args, projectID)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListTasks: %w", err)
	}
	defer rows.Close()

	var out []*Task
	for rows.Next() {
		var id string
		var title, attrsRaw, createdAt, updatedAt sql.NullString
		if err := rows.Scan(&id, &title, &attrsRaw, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("ListTasks: scan: %w", err)
		}
		t, err := scanTask(id, title, attrsRaw, createdAt, updatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Hydrate tags after the cursor is closed — selectTagsForNode issues its own
	// query on the same transaction.
	for _, t := range out {
		if t.Tags, err = selectTagsForNode(tx, t.ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// SetTaskStatus updates a task's status in place. Returns ErrNotFound when the
// task does not exist.
func SetTaskStatus(ctx context.Context, tx *graph.WriteTx, id, status string, author Author) error {
	if !author.Valid() {
		return graph.ErrAuthorRequired
	}
	res, err := tx.Exec(
		`UPDATE nodes SET attrs = json_set(attrs, '$.status', ?), updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 WHERE id = ? AND type = 'task' AND deleted_at IS NULL`,
		status, id,
	)
	if err != nil {
		return fmt.Errorf("SetTaskStatus: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("SetTaskStatus: rows affected: %w", err)
	}
	if n == 0 {
		return graph.ErrNotFound
	}
	return nil
}

// SetTaskTags replaces the tag set on a task, leaving its other fields alone.
// The task is read back and re-written through the shared chokepoint so tag
// reconciliation, the FTS index and the revision history all stay consistent.
// Callers that want to clear every tag pass an empty slice; callers that do not
// mean to touch tags must not call this at all — the update path reconciles
// against the tags it is handed, so a nil slice removes them all.
func SetTaskTags(ctx context.Context, tx *graph.WriteTx, id string, tags []string, author Author) error {
	var title, attrsRaw, createdAt, updatedAt sql.NullString
	err := tx.QueryRow(
		`SELECT title, attrs, created_at, updated_at FROM nodes WHERE id = ? AND type = 'task' AND deleted_at IS NULL`,
		id,
	).Scan(&title, &attrsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return graph.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("SetTaskTags: %w", err)
	}
	t, err := scanTask(id, title, attrsRaw, createdAt, updatedAt)
	if err != nil {
		return err
	}
	t.Tags = dedupStrings(tags)
	return updateNode(ctx, tx, *t, author)
}

// DeleteTask removes a task, preserving a tombstone revision.
func DeleteTask(ctx context.Context, tx *graph.WriteTx, id string, author Author) error {
	return deleteNode(ctx, tx, id, author)
}

func scanTask(id string, title, attrsRaw, createdAt, updatedAt sql.NullString) (*Task, error) {
	var attrs struct {
		Description string `json:"description"`
		Status      string `json:"status"`
		ProjectID   string `json:"projectId"`
	}
	if attrsRaw.Valid && attrsRaw.String != "" {
		if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
			return nil, fmt.Errorf("scanTask: unmarshal: %w", err)
		}
	}
	if attrs.Status == "" {
		attrs.Status = DefaultTaskStatus
	}
	return &Task{
		ID:          id,
		CreatedAt:   tryParseTime(createdAt.String),
		UpdatedAt:   tryParseTime(updatedAt.String),
		Title:       title.String,
		Description: attrs.Description,
		Status:      attrs.Status,
		ProjectID:   attrs.ProjectID,
	}, nil
}
