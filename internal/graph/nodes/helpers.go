package nodes

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph/tagname"
)

// tagQuerier is satisfied by both read and write transactions.
type tagQuerier interface {
	Query(string, ...any) (*sql.Rows, error)
}

func selectTagsForNode(q tagQuerier, nodeID string) ([]string, error) {
	rows, err := q.Query(
		`SELECT t.name FROM tags t JOIN node_tags nt ON t.id = nt.tag_id WHERE nt.node_id = ?`,
		nodeID,
	)
	if err != nil {
		return nil, fmt.Errorf("selectTagsForNode: %w", err)
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("selectTagsForNode: scan: %w", err)
		}
		tags = append(tags, name)
	}
	return tags, rows.Err()
}

// normalizeTags canonicalises the tags a node spec carries, so that every tag
// entering the graph through the chokepoint obeys the same nesting and casing
// rules as one written via tags.Add. Blank entries are dropped rather than
// rejected, matching what dedupStrings has always done with them; a name that
// breaks the nesting convention is an error, because storing it would create a
// tag no query could ever reach.
func normalizeTags(in []string) ([]string, error) {
	kept := make([]string, 0, len(in))
	for _, s := range in {
		if strings.TrimSpace(s) == "" {
			continue
		}
		kept = append(kept, s)
	}
	return tagname.NormalizeAll(kept)
}

func dedupStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	var out []string
	for _, s := range in {
		if _, ok := seen[s]; !ok && s != "" {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	ph := make([]string, n)
	for i := range ph {
		ph[i] = "?"
	}
	return strings.Join(ph, ",")
}

func tryParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
