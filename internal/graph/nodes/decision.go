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

// Decision represents a conclusion reached after deliberation.
type Decision struct {
	ID        string
	CreatedAt time.Time
	UpdatedAt time.Time
	Title     string
	Body      string
	Context   string
	Outcome   string
	// ImpactOnUse is the decision record's fifth part - impact on using the
	// tool - written by a different actor (the run's composer) at a
	// different time (run close) than the other four, which the implementer
	// writes at decision time. nil means the composer has not run yet
	// (awaiting); a non-nil pointer, even to "", means the composer already
	// wrote something - the two must stay distinguishable, or a later reader
	// cannot tell "not written yet" from "nothing to report" even though
	// they mean different things.
	ImpactOnUse *string
	DecidedAt   time.Time
	Tags        []string
	// RevertedAt and RevertReason record that the operator judged this
	// decision wrong after the fact (the decision model's §6 middle row: the
	// work stands, the decision does not). Both nil means never reverted;
	// both set means it was. The node is marked, never deleted or
	// recreated - the ID is content-addressed over the run, bead, epic and
	// section bodies (see decisionRecordNodeID), so a delete-and-rewrite of
	// identical content would converge on the exact same ID, and a node
	// that once existed then silently vanished at that ID is a worse
	// outcome than one that stays and says why. "This decision was reverted
	// on <date> because <reason>" is itself information a reader needs
	// months later, which is the same reason the record was written to the
	// graph in the first place.
	RevertedAt   *time.Time
	RevertReason *string
}

// Meta returns the common metadata for this node.
func (d Decision) Meta() *Meta {
	return &Meta{ID: d.ID, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
}

// NodeAttrs marshals type-specific fields for the nodes.attrs column.
// ImpactOnUse is stored as-is: a nil pointer marshals to a JSON null (the
// composer has not run yet), a pointer to "" marshals to "" (the composer
// ran and had nothing to add) - the two are distinct values on read back,
// not both empty-string.
func (d Decision) NodeAttrs() []byte {
	attrs := map[string]any{
		"body":          d.Body,
		"context":       d.Context,
		"outcome":       d.Outcome,
		"decided_at":    d.DecidedAt,
		"impact_on_use": d.ImpactOnUse,
		"reverted_at":   d.RevertedAt,
		"revert_reason": d.RevertReason,
	}
	data, _ := json.Marshal(attrs)
	return data
}

// NodeTags returns the tag slice (NodeSpec requirement).
func (d Decision) NodeTags() []string { return d.Tags }

// FTSFields returns full-text-searchable content.
func (d Decision) FTSFields() FTSFields {
	parts := []string{d.Body, d.Context, d.Outcome}
	if d.ImpactOnUse != nil {
		parts = append(parts, *d.ImpactOnUse)
	}
	if d.RevertReason != nil {
		parts = append(parts, *d.RevertReason)
	}
	body := strings.Join(parts, " ")
	return FTSFields{Title: d.Title, Body: body, Tags: strings.Join(d.Tags, " ")}
}

// DecisionFilter narrows ListDecisions results.
type DecisionFilter struct {
	Since *time.Time
	Tags  []string
	Limit int
}

// CreateDecision inserts a new decision node and returns its ID.
func CreateDecision(ctx context.Context, tx *graph.WriteTx, d Decision, author Author) (string, error) {
	return createNode(ctx, tx, "decision", d, author)
}

// GetDecision fetches a single decision by ID.
func GetDecision(ctx context.Context, tx *graph.ReadTx, id string) (*Decision, error) {
	var title, attrsRaw sql.NullString
	var createdAt, updatedAt sql.NullString

	err := tx.QueryRow(
		`SELECT title, attrs, created_at, updated_at FROM nodes WHERE id = ? AND type = 'decision'`,
		id,
	).Scan(&title, &attrsRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, graph.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("GetDecision: %w", err)
	}

	var attrs struct {
		Body         string     `json:"body"`
		Context      string     `json:"context"`
		Outcome      string     `json:"outcome"`
		DecidedAt    time.Time  `json:"decided_at"`
		ImpactOnUse  *string    `json:"impact_on_use"`
		RevertedAt   *time.Time `json:"reverted_at"`
		RevertReason *string    `json:"revert_reason"`
	}
	if attrsRaw.Valid && attrsRaw.String != "" {
		if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
			return nil, fmt.Errorf("GetDecision: unmarshal attrs: %w", err)
		}
	}

	tags, err := selectTagsForNode(tx, id)
	if err != nil {
		return nil, err
	}

	return &Decision{
		ID:           id,
		CreatedAt:    tryParseTime(createdAt.String),
		UpdatedAt:    tryParseTime(updatedAt.String),
		Title:        title.String,
		Body:         attrs.Body,
		Context:      attrs.Context,
		Outcome:      attrs.Outcome,
		DecidedAt:    attrs.DecidedAt,
		ImpactOnUse:  attrs.ImpactOnUse,
		Tags:         tags,
		RevertedAt:   attrs.RevertedAt,
		RevertReason: attrs.RevertReason,
	}, nil
}

// UpdateDecision modifies an existing decision.
func UpdateDecision(ctx context.Context, tx *graph.WriteTx, d Decision, author Author) error {
	return updateNode(ctx, tx, d, author)
}

// DeleteDecision removes a decision, preserving a tombstone revision.
func DeleteDecision(ctx context.Context, tx *graph.WriteTx, id string, author Author) error {
	return deleteNode(ctx, tx, id, author)
}

// ListDecisions returns decisions matching the filter.
func ListDecisions(ctx context.Context, tx *graph.ReadTx, f DecisionFilter) ([]*Decision, error) {
	query := `SELECT id, title, attrs, created_at, updated_at FROM nodes WHERE type = 'decision'`
	var args []any

	if f.Since != nil {
		query += ` AND json_extract(attrs, '$.decided_at') >= ?`
		args = append(args, f.Since.Format(time.RFC3339))
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
		return nil, fmt.Errorf("ListDecisions: %w", err)
	}
	defer rows.Close()

	var out []*Decision
	for rows.Next() {
		var id string
		var title, attrsRaw sql.NullString
		var createdAt, updatedAt sql.NullString
		if err := rows.Scan(&id, &title, &attrsRaw, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("ListDecisions: scan: %w", err)
		}

		var attrs struct {
			Body         string     `json:"body"`
			Context      string     `json:"context"`
			Outcome      string     `json:"outcome"`
			DecidedAt    time.Time  `json:"decided_at"`
			ImpactOnUse  *string    `json:"impact_on_use"`
			RevertedAt   *time.Time `json:"reverted_at"`
			RevertReason *string    `json:"revert_reason"`
		}
		if attrsRaw.Valid && attrsRaw.String != "" {
			if err := json.Unmarshal([]byte(attrsRaw.String), &attrs); err != nil {
				return nil, fmt.Errorf("ListDecisions: unmarshal: %w", err)
			}
		}

		tags, err := selectTagsForNode(tx, id)
		if err != nil {
			return nil, err
		}

		out = append(out, &Decision{
			ID:           id,
			CreatedAt:    tryParseTime(createdAt.String),
			UpdatedAt:    tryParseTime(updatedAt.String),
			Title:        title.String,
			Body:         attrs.Body,
			Context:      attrs.Context,
			Outcome:      attrs.Outcome,
			DecidedAt:    attrs.DecidedAt,
			ImpactOnUse:  attrs.ImpactOnUse,
			Tags:         tags,
			RevertedAt:   attrs.RevertedAt,
			RevertReason: attrs.RevertReason,
		})
	}
	return out, rows.Err()
}
