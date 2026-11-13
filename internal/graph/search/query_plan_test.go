package search

import (
	"context"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// The planner, not the query text, decides whether a search scales: with the join
// column unindexed the same SQL resolved each hit by walking every note, and a note
// write's link suggestion (one search per term) outlasted the client's 60 s timeout.
func TestSearchJoinsNodesThroughFTSRowidIndex(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)

	var o options
	WithTypes("note")(&o)
	sqlStr, args := buildQuery(sanitizeFTSQuery("vault"), o)

	var plan []string
	err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		rows, err := tx.Query("EXPLAIN QUERY PLAN "+sqlStr, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, parent, notused int
			var detail string
			if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
				return err
			}
			plan = append(plan, detail)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
	}

	for _, step := range plan {
		if strings.HasPrefix(step, "SEARCH n ") && strings.Contains(step, "USING INDEX idx_nodes_fts_rowid (fts_rowid=?)") {
			return
		}
	}
	t.Fatalf("the nodes lookup does not use idx_nodes_fts_rowid; plan:\n%s", strings.Join(plan, "\n"))
}
