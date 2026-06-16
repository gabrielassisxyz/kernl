package testutil

import (
	"math/rand"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/internal/ids"
)

// SyntheticGraph generates a deterministic seeded graph with realistic tag and
// edge distributions for benchmarking.
type SyntheticGraph struct {
	Graph    *graph.Graph
	Nodes    []SyntheticNode
	Edges    []SyntheticEdge
	TagNames []string
	NodeTags map[string][]string // node id -> tag slice
}

// SyntheticNode holds metadata for a generated node.
type SyntheticNode struct {
	ID    string
	Type  string
	Title string
}

// SyntheticEdge holds metadata for a generated edge.
type SyntheticEdge struct {
	Src   string
	Dst   string
	Label string
}

// GenerateSynthetic creates a graph of size nodeCount with deterministic seed.
// The generated graph includes a mix of related, depends_on, part_of edges plus
// some provenance-labeled edges so source-overlap is non-zero.
func GenerateSynthetic(t testing.TB, seed int64, nodeCount int) *SyntheticGraph {
	g := NewInMemoryTestGraph(t)

	rng := rand.New(rand.NewSource(seed))
	tags := []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta"}
	labels := []string{"related", "depends_on", "part_of", "blocks", "generated_from", "processed_from"}
	nodeTypes := []string{"note", "bead", "capture", "bookmark"}

	sg := &SyntheticGraph{
		Graph:    g,
		TagNames: tags,
		NodeTags: make(map[string][]string),
	}

	// Create nodes.
	_ = g.DoWrite(t.Context(), func(tx *graph.WriteTx) error {
		for i := 0; i < nodeCount; i++ {
			id := ids.New()
			typ := nodeTypes[rng.Intn(len(nodeTypes))]
			_, err := tx.Exec(`INSERT INTO nodes(id, type, title) VALUES (?, ?, ?)`, id, typ, "n")
			if err != nil {
				return err
			}
			sg.Nodes = append(sg.Nodes, SyntheticNode{ID: id, Type: typ, Title: "n"})
		}
		return nil
	})

	// Assign tags to a subset of nodes.
	_ = g.DoWrite(t.Context(), func(tx *graph.WriteTx) error {
		for _, tag := range tags {
			_, err := tx.Exec(`INSERT OR IGNORE INTO tags(id, name) VALUES (?, ?)`, tag, tag)
			if err != nil {
				return err
			}
		}
		for _, n := range sg.Nodes {
			// Random tag count 0-3.
			tc := rng.Intn(4)
			for j := 0; j < tc; j++ {
				tag := tags[rng.Intn(len(tags))]
				_, err := tx.Exec(`INSERT OR IGNORE INTO node_tags(node_id, tag_id) SELECT ?, id FROM tags WHERE name = ?`, n.ID, tag)
				if err != nil {
					return err
				}
				sg.NodeTags[n.ID] = append(sg.NodeTags[n.ID], tag)
			}
		}
		return nil
	})

	// Create edges: target density around 3 edges per node.
	_ = g.DoWrite(t.Context(), func(tx *graph.WriteTx) error {
		edgeCount := nodeCount * 3
		for i := 0; i < edgeCount; i++ {
			src := sg.Nodes[rng.Intn(len(sg.Nodes))]
			dst := sg.Nodes[rng.Intn(len(sg.Nodes))]
			if src.ID == dst.ID {
				continue
			}
			lbl := labels[rng.Intn(len(labels))]
			_, err := tx.Exec(`INSERT INTO edges(id, src, dst, label) VALUES (?, ?, ?, ?)`, ids.New(), src.ID, dst.ID, lbl)
			if err != nil {
				return err
			}
			sg.Edges = append(sg.Edges, SyntheticEdge{Src: src.ID, Dst: dst.ID, Label: lbl})
		}
		return nil
	})

	return sg
}

// DeterminismCheck returns true if both runs produce the same node and edge counts.
func DeterminismCheck(t testing.TB, seed int64, nodeCount int) bool {
	g1 := NewInMemoryTestGraph(t)
	g2 := NewInMemoryTestGraph(t)

	rng1 := rand.New(rand.NewSource(seed))
	rng2 := rand.New(rand.NewSource(seed))

	var count1, count2 int
	var edges1, edges2 int
	_ = g1.DoWrite(t.Context(), func(tx *graph.WriteTx) error {
		for i := 0; i < nodeCount; i++ {
			id := ids.New()
			_, err := tx.Exec(`INSERT INTO nodes(id, type, title) VALUES (?, 'note', ?)`, id, "n")
			if err != nil {
				return err
			}
		}
		var nodeIDs []string
		rows, err := tx.Query(`SELECT id FROM nodes`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			nodeIDs = append(nodeIDs, id)
		}
		for i := 0; i < nodeCount; i++ {
			s := nodeIDs[rng1.Intn(len(nodeIDs))]
			d := nodeIDs[rng1.Intn(len(nodeIDs))]
			if s == d {
				continue
			}
			_, err := tx.Exec(`INSERT INTO edges(id, src, dst, label) VALUES (?, ?, ?, 'related')`, ids.New(), s, d)
			if err != nil {
				return err
			}
			edges1++
		}
		return nil
	})
	_ = g1.DoRead(t.Context(), func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes`).Scan(&count1)
	})
	_ = g2.DoWrite(t.Context(), func(tx *graph.WriteTx) error {
		for i := 0; i < nodeCount; i++ {
			id := ids.New()
			_, err := tx.Exec(`INSERT INTO nodes(id, type, title) VALUES (?, 'note', ?)`, id, "n")
			if err != nil {
				return err
			}
		}
		var nodeIDs []string
		rows, err := tx.Query(`SELECT id FROM nodes`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			nodeIDs = append(nodeIDs, id)
		}
		for i := 0; i < nodeCount; i++ {
			s := nodeIDs[rng2.Intn(len(nodeIDs))]
			d := nodeIDs[rng2.Intn(len(nodeIDs))]
			if s == d {
				continue
			}
			_, err := tx.Exec(`INSERT INTO edges(id, src, dst, label) VALUES (?, ?, ?, 'related')`, ids.New(), s, d)
			if err != nil {
				return err
			}
			edges2++
		}
		return nil
	})
	_ = g2.DoRead(t.Context(), func(tx *graph.ReadTx) error {
		return tx.QueryRow(`SELECT COUNT(*) FROM nodes`).Scan(&count2)
	})

	if count1 != count2 || edges1 != edges2 {
		return false
	}
	return true
}
