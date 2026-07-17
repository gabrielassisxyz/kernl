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
	// DueDate is the day the task is due, nil when it has none. A due date is a
	// calendar day, not an instant — it is stored and carried as midnight UTC so
	// it cannot slide across midnight when read back in another timezone.
	DueDate *time.Time
}

// Task status vocabulary. DefaultTaskStatus is applied on create when unset.
const (
	TaskStatusTodo       = "todo"
	TaskStatusInProgress = "in_progress"
	TaskStatusDone       = "done"
	DefaultTaskStatus    = TaskStatusTodo
)

// DueDateLayout is the one form a due date takes outside the Task struct: in the
// attrs blob, on the wire, and in what the classifier proposes. A calendar day,
// never a timestamp.
const DueDateLayout = "2006-01-02"

// ParseDueDate reads a YYYY-MM-DD due date. An empty string means "no due date"
// (nil, no error); anything unparseable is an error, so a malformed date is
// rejected at the boundary instead of silently becoming "none".
func ParseDueDate(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	due, err := time.Parse(DueDateLayout, s)
	if err != nil {
		return nil, fmt.Errorf("parse due date %q: want %s", s, DueDateLayout)
	}
	return &due, nil
}

// FormatDueDate renders a due date as YYYY-MM-DD, or "" when there is none.
func FormatDueDate(due *time.Time) string {
	if due == nil {
		return ""
	}
	return due.Format(DueDateLayout)
}

func (t Task) Meta() *Meta { return &Meta{ID: t.ID, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt} }

// NodeAttrs serializes the task into the generic attrs blob — which is why a new
// field like dueDate needs no schema migration. The key is omitted entirely when
// there is no due date, so "unset" and "empty" cannot drift apart.
func (t Task) NodeAttrs() []byte {
	attrs := map[string]any{
		"description": t.Description,
		"status":      t.Status,
		"projectId":   t.ProjectID,
	}
	if t.DueDate != nil {
		attrs["dueDate"] = FormatDueDate(t.DueDate)
	}
	data, _ := json.Marshal(attrs)
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

// SetTaskTitle updates a task's title in place, leaving its other fields alone.
// Mirrors UpdateProjectMeta but scoped to a task and to the title only — a task
// has no separately-editable description surface today. Returns ErrNotFound when
// the task does not exist.
func SetTaskTitle(ctx context.Context, tx *graph.WriteTx, id, title string, author Author) error {
	if !author.Valid() {
		return graph.ErrAuthorRequired
	}
	res, err := tx.Exec(
		`UPDATE nodes SET title = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 WHERE id = ? AND type = 'task' AND deleted_at IS NULL`,
		title, id,
	)
	if err != nil {
		return fmt.Errorf("SetTaskTitle: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("SetTaskTitle: rows affected: %w", err)
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
	t, err := loadTaskForWrite(tx, id)
	if err != nil {
		return err
	}
	t.Tags = dedupStrings(tags)
	return updateNode(ctx, tx, *t, author)
}

// SetTaskDueDate sets or clears a task's due date, leaving its other fields
// alone. A nil due removes it. Like SetTaskTags this goes through the shared
// chokepoint, so the FTS index and the revision history stay consistent.
func SetTaskDueDate(ctx context.Context, tx *graph.WriteTx, id string, due *time.Time, author Author) error {
	t, err := loadTaskForWrite(tx, id)
	if err != nil {
		return err
	}
	t.DueDate = due
	return updateNode(ctx, tx, *t, author)
}

// loadTaskForWrite reads a task, tags included, for a read-modify-write update.
// The tags matter even to a caller that does not touch them: updateNode
// reconciles the tag set against the struct it is handed, so a task loaded
// without its tags is a task about to lose them.
func loadTaskForWrite(tx *graph.WriteTx, id string) (*Task, error) {
	var title, attrsRaw, createdAt, updatedAt sql.NullString
	err := tx.QueryRow(
		`SELECT title, attrs, created_at, updated_at FROM nodes WHERE id = ? AND type = 'task' AND deleted_at IS NULL`,
		id,
	).Scan(&title, &attrsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, graph.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("loadTaskForWrite: %w", err)
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

// DeleteTask removes a task, preserving a tombstone revision.
func DeleteTask(ctx context.Context, tx *graph.WriteTx, id string, author Author) error {
	return deleteNode(ctx, tx, id, author)
}

func scanTask(id string, title, attrsRaw, createdAt, updatedAt sql.NullString) (*Task, error) {
	var attrs struct {
		Description string `json:"description"`
		Status      string `json:"status"`
		ProjectID   string `json:"projectId"`
		DueDate     string `json:"dueDate"`
	}
	if attrsRaw.Valid && attrsRaw.String != "" {
		if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
			return nil, fmt.Errorf("scanTask: unmarshal: %w", err)
		}
	}
	if attrs.Status == "" {
		attrs.Status = DefaultTaskStatus
	}
	// A stored date we cannot read is a bug worth seeing, not a due date to drop:
	// scanTask feeds the read-modify-write updates below, so silently dropping it
	// here would erase it on the next tag or due-date edit.
	dueDate, err := ParseDueDate(attrs.DueDate)
	if err != nil {
		return nil, fmt.Errorf("scanTask: %w", err)
	}
	return &Task{
		ID:          id,
		CreatedAt:   tryParseTime(createdAt.String),
		UpdatedAt:   tryParseTime(updatedAt.String),
		Title:       title.String,
		Description: attrs.Description,
		Status:      attrs.Status,
		ProjectID:   attrs.ProjectID,
		DueDate:     dueDate,
	}, nil
}
