package traverse

import (
	"context"
	"fmt"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

// Path represents the result of a ShortestPath query.
type Path struct {
	Found  bool
	Length int
	Nodes  []string
}

// ShortestPath returns the shortest undirected path between a and b under
// the given label filter. If a == b the result is a trivial zero-length
// path [a]. If no path exists within MaxDepth hops the result is an
// explicit no-path value (Found == false) rather than an error.
func ShortestPath(ctx context.Context, tx *graph.ReadTx, a, b string, labels ...string) (Path, error) {
	if a == b {
		return Path{Found: true, Length: 0, Nodes: []string{a}}, nil
	}

	// Resolve edge adjacency from the database.
	labelFilter := make(map[string]struct{}, len(labels))
	for _, l := range labels {
		labelFilter[l] = struct{}{}
	}

	// Fetch all relevant edges for a simple in-memory undirected BFS.
	// The result set is bounded by MaxDepth hops anyway.
	rows, err := tx.Query(`SELECT src, dst, label FROM edges`)
	if err != nil {
		return Path{}, fmt.Errorf("traverse.ShortestPath: query edges: %w", err)
	}
	defer rows.Close()

	adj := make(map[string][]string)
	for rows.Next() {
		var src, dst, label string
		if err := rows.Scan(&src, &dst, &label); err != nil {
			return Path{}, fmt.Errorf("traverse.ShortestPath: scan edge: %w", err)
		}
		if len(labelFilter) > 0 {
			if _, ok := labelFilter[label]; !ok {
				continue
			}
		}
		adj[src] = append(adj[src], dst)
		adj[dst] = append(adj[dst], src) // undirected
	}
	if err := rows.Err(); err != nil {
		return Path{}, fmt.Errorf("traverse.ShortestPath: rows: %w", err)
	}

	// BFS
	type node struct {
		id    string
		prev  *node
		depth int
	}

	visited := make(map[string]bool)
	queue := []*node{{id: a, depth: 0}}
	visited[a] = true

	var target *node
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if curr.depth >= MaxDepth {
			break
		}

		for _, nbr := range adj[curr.id] {
			if visited[nbr] {
				continue
			}
			visited[nbr] = true
			next := &node{id: nbr, prev: curr, depth: curr.depth + 1}
			if nbr == b {
				target = next
				queue = nil
				break
			}
			queue = append(queue, next)
		}
	}

	if target == nil {
		return Path{Found: false}, nil
	}

	// Reconstruct path.
	path := []string{}
	for n := target; n != nil; n = n.prev {
		path = append([]string{n.id}, path...)
	}
	return Path{Found: true, Length: len(path) - 1, Nodes: path}, nil
}
