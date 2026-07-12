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

// Project is a human-created organizational node (type "project"). Projects are
// distinct from orchestrator beads: they live in the graph purely to let the
// user group their own tasks. Tasks link to a project via a part_of edge.
type Project struct {
	ID          string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Title       string
	Description string
	Status      string // active | paused | done | archived
	Tags        []string
}

// DefaultProjectStatus is applied when a project is created without one.
const DefaultProjectStatus = "active"

func (p Project) Meta() *Meta { return &Meta{ID: p.ID, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt} }

func (p Project) NodeAttrs() []byte {
	data, _ := json.Marshal(map[string]any{
		"description": p.Description,
		"status":      p.Status,
	})
	return data
}

func (p Project) NodeTags() []string { return p.Tags }

func (p Project) FTSFields() FTSFields {
	return FTSFields{Title: p.Title, Body: p.Description, Tags: strings.Join(p.Tags, " ")}
}

// CreateProject inserts a new project node and returns its ID.
func CreateProject(ctx context.Context, tx *graph.WriteTx, p Project, author Author) (string, error) {
	if p.Status == "" {
		p.Status = DefaultProjectStatus
	}
	return createNode(ctx, tx, "project", p, author)
}

// GetProject fetches a single project by ID.
func GetProject(ctx context.Context, tx *graph.ReadTx, id string) (*Project, error) {
	var title, attrsRaw, createdAt, updatedAt sql.NullString
	err := tx.QueryRow(
		`SELECT title, attrs, created_at, updated_at FROM nodes WHERE id = ? AND type = 'project' AND deleted_at IS NULL`,
		id,
	).Scan(&title, &attrsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, graph.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetProject: %w", err)
	}
	p, err := scanProject(id, title, attrsRaw, createdAt, updatedAt)
	if err != nil {
		return nil, err
	}
	if p.Tags, err = selectTagsForNode(tx, id); err != nil {
		return nil, err
	}
	return p, nil
}

// ListProjects returns all non-deleted projects, newest first.
func ListProjects(ctx context.Context, tx *graph.ReadTx) ([]*Project, error) {
	rows, err := tx.Query(
		`SELECT id, title, attrs, created_at, updated_at FROM nodes WHERE type = 'project' AND deleted_at IS NULL ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("ListProjects: %w", err)
	}
	defer rows.Close()

	var out []*Project
	for rows.Next() {
		var id string
		var title, attrsRaw, createdAt, updatedAt sql.NullString
		if err := rows.Scan(&id, &title, &attrsRaw, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("ListProjects: scan: %w", err)
		}
		p, err := scanProject(id, title, attrsRaw, createdAt, updatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Hydrate tags after the cursor is closed — selectTagsForNode issues its own
	// query on the same transaction.
	for _, p := range out {
		if p.Tags, err = selectTagsForNode(tx, p.ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// SetProjectStatus updates a project's status in place. Returns ErrNotFound
// when the project does not exist.
func SetProjectStatus(ctx context.Context, tx *graph.WriteTx, id, status string, author Author) error {
	if !author.Valid() {
		return graph.ErrAuthorRequired
	}
	res, err := tx.Exec(
		`UPDATE nodes SET attrs = json_set(attrs, '$.status', ?), updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 WHERE id = ? AND type = 'project' AND deleted_at IS NULL`,
		status, id,
	)
	if err != nil {
		return fmt.Errorf("SetProjectStatus: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("SetProjectStatus: rows affected: %w", err)
	}
	if n == 0 {
		return graph.ErrNotFound
	}
	return nil
}

// UpdateProjectMeta updates a project's title and description in place.
// Returns ErrNotFound when the project does not exist.
func UpdateProjectMeta(ctx context.Context, tx *graph.WriteTx, id, title, description string, author Author) error {
	if !author.Valid() {
		return graph.ErrAuthorRequired
	}
	res, err := tx.Exec(
		`UPDATE nodes SET title = ?, attrs = json_set(attrs, '$.description', ?), updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
		 WHERE id = ? AND type = 'project' AND deleted_at IS NULL`,
		title, description, id,
	)
	if err != nil {
		return fmt.Errorf("UpdateProjectMeta: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("UpdateProjectMeta: rows affected: %w", err)
	}
	if n == 0 {
		return graph.ErrNotFound
	}
	return nil
}

// SetProjectTags replaces the tag set on a project, leaving its other fields
// alone. The project is read back and re-written through the shared chokepoint
// so tag reconciliation, the FTS index and the revision history all stay
// consistent. Callers that want to clear every tag pass an empty slice; callers
// that do not mean to touch tags must not call this at all — the update path
// reconciles against the tags it is handed, so a nil slice removes them all.
func SetProjectTags(ctx context.Context, tx *graph.WriteTx, id string, tags []string, author Author) error {
	var title, attrsRaw, createdAt, updatedAt sql.NullString
	err := tx.QueryRow(
		`SELECT title, attrs, created_at, updated_at FROM nodes WHERE id = ? AND type = 'project' AND deleted_at IS NULL`,
		id,
	).Scan(&title, &attrsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return graph.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("SetProjectTags: %w", err)
	}
	p, err := scanProject(id, title, attrsRaw, createdAt, updatedAt)
	if err != nil {
		return err
	}
	p.Tags = dedupStrings(tags)
	return updateNode(ctx, tx, *p, author)
}

// DeleteProject removes a project node, preserving history via the shared
// delete chokepoint. Tasks that referenced it keep their projectId attr and
// simply render as unassigned — deletion does not cascade.
func DeleteProject(ctx context.Context, tx *graph.WriteTx, id string, author Author) error {
	// Type check first so the generic chokepoint can't delete a non-project.
	var one int
	err := tx.QueryRow(
		`SELECT 1 FROM nodes WHERE id = ? AND type = 'project' AND deleted_at IS NULL`, id,
	).Scan(&one)
	if err == sql.ErrNoRows {
		return graph.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("DeleteProject: %w", err)
	}
	return deleteNode(ctx, tx, id, author)
}

func scanProject(id string, title, attrsRaw, createdAt, updatedAt sql.NullString) (*Project, error) {
	var attrs struct {
		Description string `json:"description"`
		Status      string `json:"status"`
	}
	if attrsRaw.Valid && attrsRaw.String != "" {
		if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
			return nil, fmt.Errorf("scanProject: unmarshal: %w", err)
		}
	}
	if attrs.Status == "" {
		attrs.Status = DefaultProjectStatus
	}
	return &Project{
		ID:          id,
		CreatedAt:   tryParseTime(createdAt.String),
		UpdatedAt:   tryParseTime(updatedAt.String),
		Title:       title.String,
		Description: attrs.Description,
		Status:      attrs.Status,
	}, nil
}
