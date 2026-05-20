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

// Bookmark represents a saved URL in the knowledge graph.
type Bookmark struct {
	ID          string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Title       string
	URL         string
	Description string
	ArchivedAt  *time.Time
	Excerpt     string
	Tags        []string
}

// Meta returns the common metadata for this node.
func (b Bookmark) Meta() *Meta {
	return &Meta{ID: b.ID, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt}
}

// NodeAttrs marshals type-specific fields for the nodes.attrs column.
func (b Bookmark) NodeAttrs() []byte {
	attrs := map[string]any{
		"url":         b.URL,
		"description": b.Description,
		"excerpt":     b.Excerpt,
	}
	if b.ArchivedAt != nil {
		attrs["archived_at"] = b.ArchivedAt.Format(time.RFC3339)
	}
	data, _ := json.Marshal(attrs)
	return data
}

// NodeTags returns the tag slice (NodeSpec requirement).
func (b Bookmark) NodeTags() []string { return b.Tags }

// FTSFields returns full-text-searchable content.
func (b Bookmark) FTSFields() FTSFields {
	body := b.Description
	if b.Excerpt != "" {
		body += " " + b.Excerpt
	}
	return FTSFields{Title: b.Title, Body: body, Tags: strings.Join(b.Tags, " ")}
}

// BookmarkFilter narrows ListBookmarks results.
type BookmarkFilter struct {
	IncludeArchived bool
	Tags            []string
	Limit           int
}

// CreateBookmark inserts a new bookmark node and returns its ID.
func CreateBookmark(ctx context.Context, tx *graph.WriteTx, b Bookmark, author Author) (string, error) {
	return createNode(ctx, tx, "bookmark", b, author)
}

// GetBookmark fetches a single bookmark by ID, reconstructing fields and tags.
func GetBookmark(ctx context.Context, tx *graph.ReadTx, id string) (*Bookmark, error) {
	var title, attrsRaw sql.NullString
	var createdAt, updatedAt sql.NullString

	err := tx.QueryRow(
		`SELECT title, attrs, created_at, updated_at FROM nodes WHERE id = ? AND type = 'bookmark'`,
		id,
	).Scan(&title, &attrsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, graph.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetBookmark: %w", err)
	}

	var attrs struct {
		URL         string     `json:"url"`
		Description string     `json:"description"`
		ArchivedAt  *time.Time `json:"archived_at,omitempty"`
		Excerpt     string     `json:"excerpt,omitempty"`
	}
	if attrsRaw.Valid && attrsRaw.String != "" {
		if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
			return nil, fmt.Errorf("GetBookmark: unmarshal attrs: %w", err)
		}
	}

	tags, err := selectTagsForNode(tx, id)
	if err != nil {
		return nil, err
	}

	return &Bookmark{
		ID:          id,
		CreatedAt:   tryParseTime(createdAt.String),
		UpdatedAt:   tryParseTime(updatedAt.String),
		Title:       title.String,
		URL:         attrs.URL,
		Description: attrs.Description,
		ArchivedAt:  attrs.ArchivedAt,
		Excerpt:     attrs.Excerpt,
		Tags:        tags,
	}, nil
}

// UpdateBookmark modifies an existing bookmark.
func UpdateBookmark(ctx context.Context, tx *graph.WriteTx, b Bookmark, author Author) error {
	return updateNode(ctx, tx, b, author)
}

// DeleteBookmark removes a bookmark, preserving a tombstone revision.
func DeleteBookmark(ctx context.Context, tx *graph.WriteTx, id string, author Author) error {
	return deleteNode(ctx, tx, id, author)
}

// ListBookmarks returns bookmarks matching the filter.
func ListBookmarks(ctx context.Context, tx *graph.ReadTx, f BookmarkFilter) ([]*Bookmark, error) {
	query := `SELECT id, title, attrs, created_at, updated_at FROM nodes WHERE type = 'bookmark'`
	var args []any

	if !f.IncludeArchived {
		query += ` AND json_extract(attrs, '$.archived_at') IS NULL`
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
		return nil, fmt.Errorf("ListBookmarks: %w", err)
	}
	defer rows.Close()

	var out []*Bookmark
	for rows.Next() {
		var id string
		var title, attrsRaw sql.NullString
		var createdAt, updatedAt sql.NullString
		if err := rows.Scan(&id, &title, &attrsRaw, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("ListBookmarks: scan: %w", err)
		}

		var attrs struct {
			URL         string     `json:"url"`
			Description string     `json:"description"`
			ArchivedAt  *time.Time `json:"archived_at,omitempty"`
			Excerpt     string     `json:"excerpt,omitempty"`
		}
		if attrsRaw.Valid && attrsRaw.String != "" {
			if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
				return nil, fmt.Errorf("ListBookmarks: unmarshal: %w", err)
			}
		}

		tags, err := selectTagsForNode(tx, id)
		if err != nil {
			return nil, err
		}

		out = append(out, &Bookmark{
			ID:          id,
			CreatedAt:   tryParseTime(createdAt.String),
			UpdatedAt:   tryParseTime(updatedAt.String),
			Title:       title.String,
			URL:         attrs.URL,
			Description: attrs.Description,
			ArchivedAt:  attrs.ArchivedAt,
			Excerpt:     attrs.Excerpt,
			Tags:        tags,
		})
	}
	return out, rows.Err()
}
