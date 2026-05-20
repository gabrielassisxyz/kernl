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

// BookmarkList represents a curated list of bookmarks in the knowledge graph.
type BookmarkList struct {
	ID          string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Title       string
	Description string
	Tags        []string
}

// Meta returns the common metadata for this node.
func (bl BookmarkList) Meta() *Meta {
	return &Meta{ID: bl.ID, CreatedAt: bl.CreatedAt, UpdatedAt: bl.UpdatedAt}
}

// NodeAttrs marshals type-specific fields for the nodes.attrs column.
func (bl BookmarkList) NodeAttrs() []byte {
	attrs := map[string]any{
		"description": bl.Description,
	}
	data, _ := json.Marshal(attrs)
	return data
}

// NodeTags returns the tag slice (NodeSpec requirement).
func (bl BookmarkList) NodeTags() []string { return bl.Tags }

// FTSFields returns full-text-searchable content.
func (bl BookmarkList) FTSFields() FTSFields {
	return FTSFields{Title: bl.Title, Body: bl.Description, Tags: strings.Join(bl.Tags, " ")}
}

// BookmarkListFilter narrows ListBookmarkLists results.
type BookmarkListFilter struct {
	Tags  []string
	Limit int
}

// CreateBookmarkList inserts a new bookmark_list node and returns its ID.
func CreateBookmarkList(ctx context.Context, tx *graph.WriteTx, bl BookmarkList, author Author) (string, error) {
	return createNode(ctx, tx, "bookmark_list", bl, author)
}

// GetBookmarkList fetches a single bookmark_list by ID, reconstructing fields and tags.
func GetBookmarkList(ctx context.Context, tx *graph.ReadTx, id string) (*BookmarkList, error) {
	var title, attrsRaw sql.NullString
	var createdAt, updatedAt sql.NullString

	err := tx.QueryRow(
		`SELECT title, attrs, created_at, updated_at FROM nodes WHERE id = ? AND type = 'bookmark_list'`,
		id,
	).Scan(&title, &attrsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, graph.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetBookmarkList: %w", err)
	}

	var attrs struct {
		Description string `json:"description"`
	}
	if attrsRaw.Valid && attrsRaw.String != "" {
		if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
			return nil, fmt.Errorf("GetBookmarkList: unmarshal attrs: %w", err)
		}
	}

	tags, err := selectTagsForNode(tx, id)
	if err != nil {
		return nil, err
	}

	return &BookmarkList{
		ID:          id,
		CreatedAt:   tryParseTime(createdAt.String),
		UpdatedAt:   tryParseTime(updatedAt.String),
		Title:       title.String,
		Description: attrs.Description,
		Tags:        tags,
	}, nil
}

// UpdateBookmarkList modifies an existing bookmark_list.
func UpdateBookmarkList(ctx context.Context, tx *graph.WriteTx, bl BookmarkList, author Author) error {
	return updateNode(ctx, tx, bl, author)
}

// DeleteBookmarkList removes a bookmark_list, preserving a tombstone revision.
func DeleteBookmarkList(ctx context.Context, tx *graph.WriteTx, id string, author Author) error {
	return deleteNode(ctx, tx, id, author)
}

// ListBookmarkLists returns bookmark_lists matching the filter.
func ListBookmarkLists(ctx context.Context, tx *graph.ReadTx, f BookmarkListFilter) ([]*BookmarkList, error) {
	query := `SELECT id, title, attrs, created_at, updated_at FROM nodes WHERE type = 'bookmark_list'`
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
		return nil, fmt.Errorf("ListBookmarkLists: %w", err)
	}
	defer rows.Close()

	var out []*BookmarkList
	for rows.Next() {
		var id string
		var title, attrsRaw sql.NullString
		var createdAt, updatedAt sql.NullString
		if err := rows.Scan(&id, &title, &attrsRaw, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("ListBookmarkLists: scan: %w", err)
		}

		var attrs struct {
			Description string `json:"description"`
		}
		if attrsRaw.Valid && attrsRaw.String != "" {
			if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
				return nil, fmt.Errorf("ListBookmarkLists: unmarshal: %w", err)
			}
		}

		tags, err := selectTagsForNode(tx, id)
		if err != nil {
			return nil, err
		}

		out = append(out, &BookmarkList{
			ID:          id,
			CreatedAt:   tryParseTime(createdAt.String),
			UpdatedAt:   tryParseTime(updatedAt.String),
			Title:       title.String,
			Description: attrs.Description,
			Tags:        tags,
		})
	}
	return out, rows.Err()
}
