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

// Diff constants for Note body revision storage.
// A line diff is stored when both old and new bodies are ≤ MaxDiffBytes
// and the change ratio (|old-new| / max(old,new,1)) ≤ MaxChangeRatio.
// Otherwise a full snapshot is stored tagged with kind="snapshot".
const (
	MaxDiffBytes   = 256 * 1024 // 256 KiB
	MaxChangeRatio = 0.5        // 50% of the larger body
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

// DeleteNote hard-deletes a note (non-note-safe path), preserving a tombstone revision.
func DeleteNote(ctx context.Context, tx *graph.WriteTx, id string, author Author) error {
	return deleteNode(ctx, tx, id, author)
}

// SoftDeleteNoteTx tombstones a note in the caller's transaction.
// The node row is kept; deleted_at is set; the FTS row is removed; incoming
// links_to edges are degraded to dangling_links rows; and a tombstone revision
// is appended to the history.
// stem is the filename stem (without extension); title is the display title.
func SoftDeleteNoteTx(ctx context.Context, tx *graph.WriteTx, id, stem, title string, author Author) error {
	return softDeleteNote(ctx, tx, id, stem, title, author)
}

// ReviveNoteTx reverses a soft-delete in the caller's transaction.
// deleted_at is cleared; the FTS row is re-inserted; a revival revision is
// appended; and dangling links matching stem/title are re-promoted to edges.
func ReviveNoteTx(ctx context.Context, tx *graph.WriteTx, id, stem, title string, author Author) error {
	return reviveNote(ctx, tx, id, stem, title, author)
}

// rowQuerier is satisfied by both *graph.ReadTx and *graph.WriteTx.
type rowQuerier interface {
	QueryRow(string, ...any) *sql.Row
}

// IsNoteTombstoned reports whether the note with the given ID has been soft-deleted.
// Accepts both *graph.ReadTx and *graph.WriteTx.
// Returns false, nil when the node does not exist.
func IsNoteTombstoned(_ context.Context, tx rowQuerier, id string) (bool, error) {
	var deletedAt sql.NullString
	err := tx.QueryRow(
		`SELECT deleted_at FROM nodes WHERE id = ? AND type = 'note'`,
		id,
	).Scan(&deletedAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("IsNoteTombstoned: %w", err)
	}
	return deletedAt.Valid, nil
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

// DiffBody implements DiffableNode for Note, producing a self-describing
// revision payload. For small text bodies it returns a line-oriented forward
// diff; for large/binary bodies or high change ratios it falls back to a
// full snapshot tagged with kind="snapshot".
func (n Note) DiffBody(prev NodeSpec) []byte {
	p, ok := prev.(Note)
	if !ok {
		// Unknown prev type — fall back to snapshot.
		return n.snapshotDiff()
	}

	// Detect binary / threshold exceeded.
	if isBinaryBody(n.Body) || isBinaryBody(p.Body) ||
		len(n.Body) > MaxDiffBytes || len(p.Body) > MaxDiffBytes {
		return n.snapshotDiff()
	}

	// Only apply the change-ratio heuristic when both bodies are non-empty.
	// Empty↔nonempty transitions always have a ratio of 1.0, but the diff is
	// trivially compact (a handful of insert/delete ops), so skip the check.
	if p.Body != "" && n.Body != "" {
		maxLen := len(p.Body)
		if len(n.Body) > maxLen {
			maxLen = len(n.Body)
		}

		delta := len(n.Body) - len(p.Body)
		if delta < 0 {
			delta = -delta
		}
		if float64(delta)/float64(maxLen) > MaxChangeRatio {
			return n.snapshotDiff()
		}
	}

	// Line-oriented forward diff.
	ops := lineDiff(p.Body, n.Body)
	payload := diffLinePayload{Ops: ops}
	data, _ := json.Marshal(payload)
	return data
}

// snapshotDiff returns a full-snapshot payload tagged with kind="snapshot".
func (n Note) snapshotDiff() []byte {
	payload := diffSnapshotPayload{
		Title: n.Title,
		Attrs: string(n.NodeAttrs()),
		Tags:  n.Tags,
	}
	data, _ := json.Marshal(payload)
	return data
}

// isBinaryBody returns true if data contains a NUL byte (binary indicator).
func isBinaryBody(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return true
		}
	}
	return false
}

// lineDiffOp represents a single operation in a line-based forward diff.
type lineDiffOp struct {
	Op   string `json:"op"` // "+" or "-"
	Line int    `json:"ln"` // 0-based line index
	Text string `json:"t"`  // line content
}

// diffLinePayload is the self-describing diff payload stored in revisions.
// kind is implicitly "line-diff" when Ops is non-nil; absence of kind
// in legacy payloads is interpreted as snapshot.
type diffLinePayload struct {
	Ops []lineDiffOp `json:"ops"`
}

// diffSnapshotPayload is the snapshot payload stored in revisions.
type diffSnapshotPayload struct {
	Title string   `json:"title"`
	Attrs string   `json:"attrs"`
	Tags  []string `json:"tags"`
}

// lineDiff computes a minimal line-oriented forward diff from old to new.
// Uses a simple LCS-based approach that produces "+" and "-" operations.
func lineDiff(oldBody, newBody string) []lineDiffOp {
	oldLines := splitLines(oldBody)
	newLines := splitLines(newBody)

	// Compute LCS table.
	m, n := len(oldLines), len(newLines)
	lcs := make([][]int, m+1)
	for i := range lcs {
		lcs[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if oldLines[i-1] == newLines[j-1] {
				lcs[i][j] = lcs[i-1][j-1] + 1
			} else {
				if lcs[i-1][j] >= lcs[i][j-1] {
					lcs[i][j] = lcs[i-1][j]
				} else {
					lcs[i][j] = lcs[i][j-1]
				}
			}
		}
	}

	// Backtrack to produce the diff ops.
	var ops []lineDiffOp
	i, j := m, n
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && oldLines[i-1] == newLines[j-1] {
			i--
			j--
		} else if j > 0 && (i == 0 || lcs[i][j-1] >= lcs[i-1][j]) {
			ops = append(ops, lineDiffOp{Op: "+", Line: j - 1, Text: newLines[j-1]})
			j--
		} else if i > 0 {
			ops = append(ops, lineDiffOp{Op: "-", Line: i - 1, Text: oldLines[i-1]})
			i--
		}
	}
	return ops
}

// splitLines splits s into lines, handling the empty-string case.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	return lines
}
