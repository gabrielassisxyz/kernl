package nodes

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
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
