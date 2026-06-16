// Package search provides full-text search over graph nodes via SQLite FTS5.
package search

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

// Hit represents a single search result.
type Hit struct {
	NodeID string
	Title  string
	Rank   float64
}

// options holds optional filters for a search query.
type options struct {
	tags   []string
	types  []string
	prefix bool
}

// Option modifies search behaviour.
type Option func(*options)

// WithPrefix turns the final query token into a prefix match, so a partial
// token like "lin" matches "Linktree". This enables autocomplete-as-you-type
// without affecting the default exact-phrase behaviour.
func WithPrefix() Option {
	return func(o *options) {
		o.prefix = true
	}
}

// WithTags filters results to nodes that have ALL of the given tags.
func WithTags(tags ...string) Option {
	return func(o *options) {
		o.tags = tags
	}
}

// WithTypes filters results to nodes whose type is in the given list.
func WithTypes(types ...string) Option {
	return func(o *options) {
		o.types = types
	}
}

// Search executes a full-text search against the nodes_fts virtual table.
//
// The query string is cleaned of FTS5 special characters before being
// wrapped in double-quotes to perform a phrase search.  An empty query
// returns graph.ErrFTSQuerySyntax.
//
// Errors containing the substring "fts5" in their message (as returned
// by the SQLite FTS5 extension) are wrapped with graph.ErrFTSQuerySyntax.
func Search(ctx context.Context, tx *graph.ReadTx, query string, opts ...Option) ([]Hit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, graph.ErrFTSQuerySyntax
	}

	var o options
	for _, opt := range opts {
		opt(&o)
	}

	cleaned := sanitizeFTSQuery(query)
	if cleaned == "" {
		return nil, graph.ErrFTSQuerySyntax
	}

	sqlStr, sqlArgs := buildQuery(cleaned, o)

	rows, err := tx.Query(sqlStr, sqlArgs...)
	if err != nil {
		if isFTS5Error(err) {
			return nil, fmt.Errorf("search: %w", graph.ErrFTSQuerySyntax)
		}
		return nil, fmt.Errorf("search: query: %w", err)
	}
	defer rows.Close()

	var hits []Hit
	for rows.Next() {
		var id string
		var title string
		var rank float64
		if err := rows.Scan(&id, &title, &rank); err != nil {
			return nil, fmt.Errorf("search: scan: %w", err)
		}
		hits = append(hits, Hit{NodeID: id, Title: title, Rank: rank})
	}
	if err := rows.Err(); err != nil {
		if isFTS5Error(err) {
			return nil, fmt.Errorf("search: %w", graph.ErrFTSQuerySyntax)
		}
		return nil, fmt.Errorf("search: rows: %w", err)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("search: rows: %w", rows.Err())
	}

	return hits, nil
}

// sanitizeFTSQuery strips characters that have special meaning in the
// FTS5 query syntax.  It also collapses internal double-quote usage to
// prevent the user from injecting syntax.  The result is a plain string
// safe for wrapping in double-quotes for phrase matching.
func sanitizeFTSQuery(raw string) string {
	// Remove or replace FTS5 special characters.
	replacer := strings.NewReplacer(
		`"`, " ",
		`""`, " ",
		`*`, " ",
		`^`, " ",
		`(`, " ",
		`)`, " ",
		`-`, " ",
		`+`, " ",
	)
	s := replacer.Replace(raw)

	// Collapse multiple spaces and trim.
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}

// buildQuery constructs the parameterized SQL and argument slice.
func buildQuery(cleaned string, o options) (string, []any) {
	var b strings.Builder
	args := make([]any, 0, 1+len(o.tags)+len(o.types))

	b.WriteString(`SELECT n.id, n.title, rank`)
	b.WriteString(` FROM nodes_fts(?) ft`)
	args = append(args, buildMatchExpr(cleaned, o.prefix))

	b.WriteString(` JOIN nodes n ON n.fts_rowid = ft.rowid`)

	if len(o.tags) > 0 {
		b.WriteString(` JOIN node_tags nt ON nt.node_id = n.id`)
		b.WriteString(` JOIN tags t ON t.id = nt.tag_id`)
	}

	// WHERE clause: always exclude tombstoned nodes (deleted_at IS NULL).
	// Non-note rows always have NULL deleted_at so this is a no-op for them.
	whereClauses := make([]string, 0, 3)
	whereClauses = append(whereClauses, "n.deleted_at IS NULL")

	if len(o.tags) > 0 {
		placeholders := make([]string, len(o.tags))
		for i := range o.tags {
			placeholders[i] = "?"
		}
		whereClauses = append(whereClauses,
			fmt.Sprintf("t.name IN (%s)", strings.Join(placeholders, ", ")))
		for _, tag := range o.tags {
			args = append(args, tag)
		}
	}

	if len(o.types) > 0 {
		placeholders := make([]string, len(o.types))
		for i := range o.types {
			placeholders[i] = "?"
		}
		whereClauses = append(whereClauses,
			fmt.Sprintf("n.type IN (%s)", strings.Join(placeholders, ", ")))
		for _, typ := range o.types {
			args = append(args, typ)
		}
	}

	if len(whereClauses) > 0 {
		b.WriteString(` WHERE `)
		b.WriteString(strings.Join(whereClauses, ` AND `))
	}

	if len(o.tags) > 0 {
		b.WriteString(` GROUP BY n.id HAVING COUNT(DISTINCT t.name) = `)
		b.WriteString(fmt.Sprintf("%d", len(o.tags)))
	}

	b.WriteString(` ORDER BY rank ASC`)

	return b.String(), args
}

// buildMatchExpr builds the FTS5 MATCH expression from the sanitized query.
//
// Without prefix it wraps the whole query in double-quotes for a phrase match,
// preserving the original behaviour. With prefix it quotes each token and
// appends '*' to the final token (FTS5 prefix syntax), so the last word the
// user is still typing matches by prefix while earlier words match exactly.
// The '*' is added here, after sanitizeFTSQuery has already stripped any
// user-supplied '*', so it cannot be injected.
func buildMatchExpr(cleaned string, prefix bool) string {
	if !prefix {
		return `"` + cleaned + `"`
	}
	tokens := strings.Fields(cleaned)
	parts := make([]string, len(tokens))
	for i, tok := range tokens {
		parts[i] = `"` + tok + `"`
		if i == len(tokens)-1 {
			parts[i] += "*"
		}
	}
	return strings.Join(parts, " ")
}

// isFTS5Error checks whether an error returned by the SQLite driver
// originates from the FTS5 extension.
func isFTS5Error(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "fts5") || strings.Contains(msg, "FTS5") {
		return true
	}
	if strings.Contains(msg, "malformed") && strings.Contains(msg, "match") {
		return true
	}

	// Also check for fts5-like errors in wrapped errors.
	var unwrapped error = err
	for {
		if strings.Contains(unwrapped.Error(), "fts5") {
			return true
		}
		unwrapped = errors.Unwrap(unwrapped)
		if unwrapped == nil {
			break
		}
	}

	return false
}
