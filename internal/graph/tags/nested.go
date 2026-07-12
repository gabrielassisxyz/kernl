package tags

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/tagname"
)

// NodesUnder returns the node IDs tagged with name or with any of its
// descendants — `homelab` also answers for `homelab/nas` and `homelab/nas/zfs`.
// Results are ordered by node_id ASC and deduplicated: a node carrying both the
// parent and a child tag appears once.
func NodesUnder(ctx context.Context, tx *graph.ReadTx, name string) ([]string, error) {
	normalized, err := tagname.Normalize(name)
	if err != nil {
		return nil, fmt.Errorf("tags.NodesUnder: %w", err)
	}

	// The prefix goes into a LIKE pattern, so its wildcards have to be defused:
	// a tag literally named `50%_off` must not start matching everything.
	pattern := escapeLike(normalized) + tagname.Separator + "%"

	rows, err := tx.Query(
		`SELECT DISTINCT nt.node_id FROM node_tags nt`+
			` JOIN tags t ON t.id = nt.tag_id`+
			` WHERE t.name = ? OR t.name LIKE ? ESCAPE '\'`+
			` ORDER BY nt.node_id ASC`,
		normalized, pattern,
	)
	if err != nil {
		return nil, fmt.Errorf("tags.NodesUnder: query: %w", err)
	}
	defer rows.Close()

	nodeIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("tags.NodesUnder: scan: %w", err)
		}
		nodeIDs = append(nodeIDs, id)
	}
	return nodeIDs, rows.Err()
}

// TreeNode is one level of the tag hierarchy derived from the `/` convention.
type TreeNode struct {
	// Name is the full tag name, e.g. "homelab/nas".
	Name string
	// Segment is the last level only, e.g. "nas" — what a tree UI renders.
	Segment string
	// Children are the tags nested directly under this one, sorted by name.
	Children []*TreeNode
}

// Tree builds the nested structure implied by a flat list of tag names. Parents
// that no node carries are synthesised: `homelab/nas` alone still yields a
// `homelab` branch, otherwise its child would have nowhere to hang.
//
// It is a pure function so the tree is derived in one place — the API — rather
// than being re-implemented by every client that renders it.
func Tree(names []string) []*TreeNode {
	roots := []*TreeNode{}
	byName := map[string]*TreeNode{}

	// Sorting first means children are appended to a parent in name order, so
	// the result needs no second sorting pass.
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)

	for _, name := range sorted {
		normalized, err := tagname.Normalize(name)
		if err != nil {
			continue
		}
		ensureBranch(normalized, byName, &roots)
	}
	return roots
}

// ensureBranch materialises name and every ancestor it implies, returning the
// node for name itself.
func ensureBranch(name string, byName map[string]*TreeNode, roots *[]*TreeNode) *TreeNode {
	if existing, ok := byName[name]; ok {
		return existing
	}

	segments := tagname.Segments(name)
	node := &TreeNode{
		Name:     name,
		Segment:  segments[len(segments)-1],
		Children: []*TreeNode{},
	}
	byName[name] = node

	if len(segments) == 1 {
		*roots = append(*roots, node)
		return node
	}

	parentName := strings.Join(segments[:len(segments)-1], tagname.Separator)
	parent := ensureBranch(parentName, byName, roots)
	parent.Children = append(parent.Children, node)
	return node
}

// escapeLike neutralises the LIKE metacharacters in a literal prefix, for a
// query using ESCAPE '\'.
func escapeLike(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(s)
}
