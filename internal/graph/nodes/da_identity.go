package nodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

// TypeDAIdentity is the node type constant for DA identities.
const TypeDAIdentity = "da_identity"

// DAIdentity represents a DA (Digital Assistant) identity node.
type DAIdentity struct {
	ID           string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	SystemPrompt string
	DisplayName  string
}

// Meta implements NodeSpec.
func (di DAIdentity) Meta() *Meta {
	return &Meta{ID: di.ID, CreatedAt: di.CreatedAt, UpdatedAt: di.UpdatedAt}
}

// NodeAttrs implements NodeSpec.
func (di DAIdentity) NodeAttrs() []byte {
	attrs := map[string]any{
		"system_prompt": di.SystemPrompt,
		"display_name":  di.DisplayName,
	}
	data, _ := json.Marshal(attrs)
	return data
}

// NodeTags implements NodeSpec.
func (di DAIdentity) NodeTags() []string { return nil }

// FTSFields implements NodeSpec. Title is display_name; Body is system_prompt
// truncated to 200 characters.
func (di DAIdentity) FTSFields() FTSFields {
	body := di.SystemPrompt
	if len(body) > 200 {
		body = body[:200]
	}
	return FTSFields{Title: di.DisplayName, Body: body}
}

// daIdentityAttrs is the deserialized attrs payload for a da_identity node.
type daIdentityAttrs struct {
	SystemPrompt string `json:"system_prompt"`
	DisplayName  string `json:"display_name"`
}

// CreateDAIdentity inserts a new da_identity node and returns its ID.
func CreateDAIdentity(ctx context.Context, tx *graph.WriteTx, di *DAIdentity, author Author) (string, error) {
	return createNode(ctx, tx, TypeDAIdentity, di, author)
}

// GetDAIdentity fetches the (singleton) DA identity. Returns graph.ErrNotFound
// if none exists.
func GetDAIdentity(ctx context.Context, tx *graph.ReadTx) (*DAIdentity, error) {
	var id string
	var title, attrsRaw sql.NullString
	var createdAt, updatedAt sql.NullString

	err := tx.QueryRow(
		`SELECT id, title, attrs, created_at, updated_at FROM nodes WHERE type = ? ORDER BY created_at ASC LIMIT 1`,
		TypeDAIdentity,
	).Scan(&id, &title, &attrsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, graph.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetDAIdentity: %w", err)
	}

	var attrs daIdentityAttrs
	if attrsRaw.Valid && attrsRaw.String != "" {
		if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
			return nil, fmt.Errorf("GetDAIdentity: unmarshal attrs: %w", err)
		}
	}

	di := &DAIdentity{
		ID:           id,
		CreatedAt:    tryParseTime(createdAt.String),
		UpdatedAt:    tryParseTime(updatedAt.String),
		SystemPrompt: attrs.SystemPrompt,
		DisplayName:  attrs.DisplayName,
	}

	// Fallback: if attrs didn't have display_name, use the FTS title column.
	if di.DisplayName == "" && title.Valid {
		di.DisplayName = title.String
	}

	return di, nil
}

// SaveDAIdentity updates an existing da_identity node.
func SaveDAIdentity(ctx context.Context, tx *graph.WriteTx, di *DAIdentity, author Author) error {
	if di.ID == "" {
		return fmt.Errorf("SaveDAIdentity: ID is required")
	}
	return updateNode(ctx, tx, di, author)
}

// DAIdentityMeta is a lightweight read-only query for checking existence.
type DAIdentityMeta struct {
	ID          string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DisplayName string
}

// ListDAIdentities returns all DA identities (typically zero or one).
func ListDAIdentities(ctx context.Context, tx *graph.ReadTx) ([]DAIdentityMeta, error) {
	rows, err := tx.Query(
		`SELECT id, title, created_at, updated_at FROM nodes WHERE type = ? ORDER BY created_at ASC`,
		TypeDAIdentity,
	)
	if err != nil {
		return nil, fmt.Errorf("ListDAIdentities: %w", err)
	}
	defer rows.Close()

	var out []DAIdentityMeta
	for rows.Next() {
		var id string
		var title, createdAt, updatedAt sql.NullString
		if err := rows.Scan(&id, &title, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("ListDAIdentities: scan: %w", err)
		}
		out = append(out, DAIdentityMeta{
			ID:          id,
			CreatedAt:   tryParseTime(createdAt.String),
			UpdatedAt:   tryParseTime(updatedAt.String),
			DisplayName: title.String,
		})
	}
	return out, rows.Err()
}
