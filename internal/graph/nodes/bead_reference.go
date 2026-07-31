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

// BeadReference is a stub that lets an edge point at an orchestrator bead or
// epic without mirroring its tracker state into this graph. It carries only
// the facts an edge endpoint needs and nothing that can drift: no status, no
// labels, no dependencies. The tracker (br or bd) stays the sole source of
// truth about what the bead is doing; this node is just the anchor a
// has_decision edge requires in order to exist. See CreateBeadReference for
// why it is never updated.
type BeadReference struct {
	ID        string
	CreatedAt time.Time
	UpdatedAt time.Time
	// Title is the bead's title at the moment its first reference was
	// created - a label for display, not a live mirror; it is never
	// refreshed on a later write (see CreateBeadReference).
	Title string
	// TrackerKind is which tracker CLI owns this bead: "br" or "bd" (see
	// backend.MemoryManagerType / backend.TrackerBinary).
	TrackerKind string
	// Repository is the local path of the repository this bead's tracker
	// belongs to.
	Repository string
}

// Meta returns the common metadata for this node.
func (b BeadReference) Meta() *Meta {
	return &Meta{ID: b.ID, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt}
}

// NodeAttrs marshals type-specific fields for the nodes.attrs column. Title
// is not included here: like every other node type, it lives in the nodes
// table's own title column.
func (b BeadReference) NodeAttrs() []byte {
	attrs := map[string]any{
		"tracker_kind": b.TrackerKind,
		"repository":   b.Repository,
	}
	data, _ := json.Marshal(attrs)
	return data
}

// NodeTags returns the tag slice (NodeSpec requirement). A reference node
// carries no tags - tags are a mutable classification, and this node stores
// nothing that changes.
func (b BeadReference) NodeTags() []string { return nil }

// FTSFields returns full-text-searchable content.
func (b BeadReference) FTSFields() FTSFields {
	return FTSFields{Title: b.Title, Body: b.TrackerKind + " " + b.Repository}
}

// CreateBeadReference inserts a new bead reference node and returns its ID,
// or is a no-op returning the existing id if one already exists.
//
// Unlike every other node type in this package, the ID is required and is
// never generated: it must be the bead's own tracker id, because the entire
// point of this node is to be the thing edges.Create finds when a caller
// links a Decision to that bead. A caller supplying an empty ID, or leaving
// Title, TrackerKind, or Repository empty, gets an error naming what is
// missing rather than a node written with a blank stood in for it -
// app.ensureBeadReferenceNode already checks this with a more actionable
// message for the one caller that exists today, but that caller is not the
// only thing standing between an empty BeadReference and the graph: this
// check is the one a future caller cannot bypass by skipping that layer.
//
// The insert is atomic and idempotent (createNodeIfAbsent, INSERT OR
// IGNORE) rather than a separate existence check followed by an
// unconditional insert: every child bead of the same epic calls this for
// the same epic id, so a check-then-insert here would race across the
// separate *graph.Graph each caller opens - two callers observing absence
// before either commits, with only one insert succeeding and the other
// failing on the primary-key collision instead of both converging on
// success. A reference node is never updated once created (nothing on it
// ever needs a more current answer), so "already exists" is success at any
// concurrency, not a conflict to reconcile.
func CreateBeadReference(ctx context.Context, tx *graph.WriteTx, b BeadReference, author Author) (string, error) {
	if b.ID == "" {
		return "", fmt.Errorf("nodes.CreateBeadReference: id is required and must be the bead's own tracker id - it is never generated")
	}
	var missing []string
	if b.Title == "" {
		missing = append(missing, "title")
	}
	if b.TrackerKind == "" {
		missing = append(missing, "tracker kind")
	}
	if b.Repository == "" {
		missing = append(missing, "repository")
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("nodes.CreateBeadReference: bead %s: missing %s", b.ID, strings.Join(missing, ", "))
	}
	id, _, err := createNodeIfAbsent(ctx, tx, "bead_reference", b, author)
	return id, err
}

// GetBeadReference fetches a single bead reference by ID.
func GetBeadReference(ctx context.Context, tx *graph.ReadTx, id string) (*BeadReference, error) {
	var title, attrsRaw sql.NullString
	var createdAt, updatedAt sql.NullString

	err := tx.QueryRow(
		`SELECT title, attrs, created_at, updated_at FROM nodes WHERE id = ? AND type = 'bead_reference'`,
		id,
	).Scan(&title, &attrsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, graph.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetBeadReference: %w", err)
	}

	var attrs struct {
		TrackerKind string `json:"tracker_kind"`
		Repository  string `json:"repository"`
	}
	if attrsRaw.Valid && attrsRaw.String != "" {
		if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
			return nil, fmt.Errorf("GetBeadReference: unmarshal attrs: %w", err)
		}
	}

	return &BeadReference{
		ID:          id,
		CreatedAt:   tryParseTime(createdAt.String),
		UpdatedAt:   tryParseTime(updatedAt.String),
		Title:       title.String,
		TrackerKind: attrs.TrackerKind,
		Repository:  attrs.Repository,
	}, nil
}
