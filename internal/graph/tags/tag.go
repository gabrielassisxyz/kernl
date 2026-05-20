package tags

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/internal/ids"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// Author is re-exported from the nodes package.
type Author = nodes.Author

// Add adds a tag to a node. The tag is created in the tags table if it does not
// already exist. Returns graph.ErrAuthorRequired if author is invalid,
// graph.ErrEmptyTag if tag is empty, and graph.ErrNotFound if nodeID does not
// exist. Duplicate tag assignments are silently ignored.
func Add(ctx context.Context, tx *graph.WriteTx, nodeID string, tag string, author Author) error {
	if !author.Valid() {
		return graph.ErrAuthorRequired
	}
	if tag == "" {
		return graph.ErrEmptyTag
	}

	// Ensure tag exists in the tags table.
	if _, err := tx.Exec(`INSERT OR IGNORE INTO tags(id, name) VALUES (?, ?)`, ids.New(), tag); err != nil {
		return fmt.Errorf("tags.Add: upsert tag: %w", err)
	}

	// Lookup the tag id.
	var tagID string
	if err := tx.QueryRow(`SELECT id FROM tags WHERE name = ?`, tag).Scan(&tagID); err != nil {
		return fmt.Errorf("tags.Add: select tag id: %w", err)
	}

	// Insert the node_tags link. The FK on node_id will fail if the node does
	// not exist; we translate that into graph.ErrNotFound.
	_, err := tx.Exec(`INSERT OR IGNORE INTO node_tags(node_id, tag_id) VALUES (?, ?)`, nodeID, tagID)
	if err != nil {
		if isForeignKeyError(err) {
			return graph.ErrNotFound
		}
		return fmt.Errorf("tags.Add: insert node_tags: %w", err)
	}

	return nil
}

// Remove removes a tag from a node. Returns graph.ErrAuthorRequired if author
// is invalid, graph.ErrNotFound if the node_tags row did not exist (either the
// node or tag does not exist, or the tag was not assigned to the node). If the
// tag becomes orphaned — no other node_tags reference it — it is removed from
// the tags table.
func Remove(ctx context.Context, tx *graph.WriteTx, nodeID string, tag string, author Author) error {
	if !author.Valid() {
		return graph.ErrAuthorRequired
	}

	// Lookup the tag id. If the tag does not exist at all, the node_tags row
	// cannot exist either.
	var tagID string
	err := tx.QueryRow(`SELECT id FROM tags WHERE name = ?`, tag).Scan(&tagID)
	if err == sql.ErrNoRows {
		return graph.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("tags.Remove: select tag id: %w", err)
	}

	// Remove the node_tags row.
	result, err := tx.Exec(`DELETE FROM node_tags WHERE node_id = ? AND tag_id = ?`, nodeID, tagID)
	if err != nil {
		return fmt.Errorf("tags.Remove: delete node_tags: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("tags.Remove: rows affected: %w", err)
	}
	if n == 0 {
		return graph.ErrNotFound
	}

	// Remove orphaned tag row if no other node references it.
	var count int
	err = tx.QueryRow(`SELECT COUNT(*) FROM node_tags WHERE tag_id = ?`, tagID).Scan(&count)
	if err != nil {
		return fmt.Errorf("tags.Remove: count node_tags: %w", err)
	}
	if count == 0 {
		if _, err := tx.Exec(`DELETE FROM tags WHERE id = ?`, tagID); err != nil {
			return fmt.Errorf("tags.Remove: delete orphaned tag: %w", err)
		}
	}

	return nil
}

// List returns all tag names for a node, ordered by name ASC.
func List(ctx context.Context, tx *graph.ReadTx, nodeID string) ([]string, error) {
	rows, err := tx.Query(
		`SELECT t.name FROM tags t JOIN node_tags nt ON t.id = nt.tag_id WHERE nt.node_id = ? ORDER BY t.name ASC`,
		nodeID,
	)
	if err != nil {
		return nil, fmt.Errorf("tags.List: query: %w", err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("tags.List: scan: %w", err)
		}
		tags = append(tags, name)
	}
	return tags, rows.Err()
}

// Nodes returns all node IDs associated with a tag, ordered by node_id ASC.
func Nodes(ctx context.Context, tx *graph.ReadTx, tag string) ([]string, error) {
	rows, err := tx.Query(
		`SELECT nt.node_id FROM node_tags nt JOIN tags t ON t.id = nt.tag_id WHERE t.name = ? ORDER BY nt.node_id ASC`,
		tag,
	)
	if err != nil {
		return nil, fmt.Errorf("tags.Nodes: query: %w", err)
	}
	defer rows.Close()

	var nodeIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("tags.Nodes: scan: %w", err)
		}
		nodeIDs = append(nodeIDs, id)
	}
	return nodeIDs, rows.Err()
}

// isForeignKeyError returns true when the error is a SQLite foreign-key
// constraint violation.
func isForeignKeyError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}
