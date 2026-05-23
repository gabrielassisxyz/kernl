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

// Note represents a vault note node — all notes share type "note".
// The user-vs-generated distinction is read from frontmatter author/origin,
// not folders (KTD-1, R20).
type Note struct {
	ID        string
	CreatedAt time.Time
	UpdatedAt time.Time
	Origin    string // frontmatter origin, empty for user notes
	Author    string // frontmatter author, empty for user notes
	Title     string // from frontmatter title or filename stem
	Body      string // re-derivable cache stored in attrs (FTS source)
	Tags      []string
}

// Meta returns the common metadata for this node.
func (n Note) Meta() *Meta {
	return &Meta{ID: n.ID, CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt}
}

// NodeAttrs marshals type-specific fields for the nodes.attrs column.
func (n Note) NodeAttrs() []byte {
	attrs := map[string]any{
		"body":   n.Body,
		"origin": n.Origin,
		"author": n.Author,
	}
	data, _ := json.Marshal(attrs)
	return data
}

// NodeTags returns the tag slice (NodeSpec requirement).
func (n Note) NodeTags() []string { return n.Tags }

// FTSFields returns full-text-searchable content.
func (n Note) FTSFields() FTSFields {
	return FTSFields{Title: n.Title, Body: n.Body, Tags: strings.Join(n.Tags, " ")}
}

// NoteFilter narrows ListNotes results.
type NoteFilter struct {
	Origin         string
	Author         string
	Tags           []string
	Limit          int
	IncludeDeleted bool // opt-in to include tombstoned notes
}

// CreateNote inserts a new note node and returns its ID.
func CreateNote(ctx context.Context, tx *graph.WriteTx, n Note, author Author) (string, error) {
	return createNode(ctx, tx, "note", n, author)
}

// GetNote fetches a single note by ID.
func GetNote(ctx context.Context, tx *graph.ReadTx, id string) (*Note, error) {
	var title, attrsRaw sql.NullString
	var createdAt, updatedAt sql.NullString

	err := tx.QueryRow(
		`SELECT title, attrs, created_at, updated_at FROM nodes WHERE id = ? AND type = 'note' AND deleted_at IS NULL`,
		id,
	).Scan(&title, &attrsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, graph.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetNote: %w", err)
	}

	var attrs struct {
		Body   string `json:"body"`
		Origin string `json:"origin"`
		Author string `json:"author"`
	}
	if attrsRaw.Valid && attrsRaw.String != "" {
		if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
			return nil, fmt.Errorf("GetNote: unmarshal attrs: %w", err)
		}
	}

	tags, err := selectTagsForNode(tx, id)
	if err != nil {
		return nil, err
	}

	return &Note{
		ID:        id,
		CreatedAt: tryParseTime(createdAt.String),
		UpdatedAt: tryParseTime(updatedAt.String),
		Origin:    attrs.Origin,
		Author:    attrs.Author,
		Title:     title.String,
		Body:      attrs.Body,
		Tags:      tags,
	}, nil
}

// UpdateNote modifies an existing note.
func UpdateNote(ctx context.Context, tx *graph.WriteTx, n Note, author Author) error {
	return updateNode(ctx, tx, n, author)
}

// DeleteNote removes a note, preserving a tombstone revision.
func DeleteNote(ctx context.Context, tx *graph.WriteTx, id string, author Author) error {
	return deleteNode(ctx, tx, id, author)
}

// ListNotes returns notes matching the filter.
// Tombstoned notes (deleted_at IS NOT NULL) are excluded by default.
func ListNotes(ctx context.Context, tx *graph.ReadTx, f NoteFilter) ([]*Note, error) {
	query := `SELECT id, title, attrs, created_at, updated_at FROM nodes WHERE type = 'note'`
	var args []any

	if !f.IncludeDeleted {
		query += ` AND deleted_at IS NULL`
	}

	if f.Origin != "" {
		query += ` AND json_extract(attrs, '$.origin') = ?`
		args = append(args, f.Origin)
	}

	if f.Author != "" {
		query += ` AND json_extract(attrs, '$.author') = ?`
		args = append(args, f.Author)
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
		return nil, fmt.Errorf("ListNotes: %w", err)
	}
	defer rows.Close()

	var out []*Note
	for rows.Next() {
		var id string
		var title, attrsRaw sql.NullString
		var createdAt, updatedAt sql.NullString
		if err := rows.Scan(&id, &title, &attrsRaw, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("ListNotes: scan: %w", err)
		}

		var attrs struct {
			Body   string `json:"body"`
			Origin string `json:"origin"`
			Author string `json:"author"`
		}
		if attrsRaw.Valid && attrsRaw.String != "" {
			if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
				return nil, fmt.Errorf("ListNotes: unmarshal: %w", err)
			}
		}

		tags, err := selectTagsForNode(tx, id)
		if err != nil {
			return nil, err
		}

		out = append(out, &Note{
			ID:        id,
			CreatedAt: tryParseTime(createdAt.String),
			UpdatedAt: tryParseTime(updatedAt.String),
			Origin:    attrs.Origin,
			Author:    attrs.Author,
			Title:     title.String,
			Body:      attrs.Body,
			Tags:      tags,
		})
	}
	return out, rows.Err()
}
