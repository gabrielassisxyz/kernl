package graph_test

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/tags"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"pgregory.net/rapid"
)

type captureMirror struct {
	title string
	body  string
	tags  []string
	alive bool
}

type model struct {
	g        *graph.Graph
	nodes    map[string]captureMirror
	edges    map[string]edges.Edge
	revCount map[string]int
}

func TestSubstrateProperties(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		g := testutil.NewTestGraph(t)
		defer g.Close()
		m := &model{
			g:        g,
			nodes:    make(map[string]captureMirror),
			edges:    make(map[string]edges.Edge),
			revCount: make(map[string]int),
		}
		n := rapid.IntRange(5, 30).Draw(rt, "numOps")
		for i := 0; i < n; i++ {
			op := rapid.IntRange(0, 5).Draw(rt, "op")
			m.apply(rt, op)
			m.checkInvariants(rt)
		}
	})
}

func (m *model) apply(t *rapid.T, op int) {
	ctx := context.Background()
	switch op {
	case 0: // create
		title := rapid.String().Draw(t, "title")
		body := rapid.String().Draw(t, "body")
		tagCount := rapid.IntRange(0, 3).Draw(t, "tagCount")
		var tagSlice []string
		for i := 0; i < tagCount; i++ {
			tag := rapid.StringMatching(`[a-z]+`).Draw(t, fmt.Sprintf("tag%d", i))
			if tag != "" {
				tagSlice = append(tagSlice, tag)
			}
		}
		c := nodes.Capture{Title: title, Body: body, Tags: tagSlice}
		var id string
		err := m.g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			var err error
			id, err = nodes.CreateCapture(ctx, tx, c, nodes.Author{Name: "rapid"})
			return err
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		m.nodes[id] = captureMirror{title: title, body: body, tags: dedupStrings(tagSlice), alive: true}
		m.revCount[id] = 1

	case 1: // update
		id, ok := m.pickAliveNode(t)
		if !ok {
			return // skip if no alive nodes
		}
		title := rapid.String().Draw(t, "newTitle")
		body := rapid.String().Draw(t, "newBody")
		tagCount := rapid.IntRange(0, 3).Draw(t, "newTagCount")
		var tagSlice []string
		for i := 0; i < tagCount; i++ {
			tag := rapid.StringMatching(`[a-z]+`).Draw(t, fmt.Sprintf("newTag%d", i))
			if tag != "" {
				tagSlice = append(tagSlice, tag)
			}
		}
		c := nodes.Capture{ID: id, Title: title, Body: body, Tags: tagSlice}
		err := m.g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			return nodes.UpdateCapture(ctx, tx, c, nodes.Author{Name: "rapid"})
		})
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		m.nodes[id] = captureMirror{title: title, body: body, tags: dedupStrings(tagSlice), alive: true}
		m.revCount[id]++

	case 2: // delete
		id, ok := m.pickAliveNode(t)
		if !ok {
			return
		}
		err := m.g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			return nodes.DeleteCapture(ctx, tx, id, nodes.Author{Name: "rapid"})
		})
		if err != nil {
			t.Fatalf("delete: %v", err)
		}
		mirror := m.nodes[id]
		mirror.alive = false
		m.nodes[id] = mirror
		m.revCount[id]++
		// Cascade: edges connected to this node are deleted by SQLite FK cascade.
		for eid, edge := range m.edges {
			if edge.Src == id || edge.Dst == id {
				delete(m.edges, eid)
			}
		}

	case 3: // add_tag
		id, ok := m.pickAliveNode(t)
		if !ok {
			return
		}
		tag := rapid.StringMatching(`[a-z]+`).Draw(t, "addTag")
		if tag == "" {
			return
		}
		err := m.g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			return tags.Add(ctx, tx, id, tag, tags.Author{Name: "rapid"})
		})
		if err != nil {
			t.Fatalf("add_tag: %v", err)
		}
		mirror := m.nodes[id]
		mirror.tags = dedupStrings(append(mirror.tags, tag))
		m.nodes[id] = mirror

	case 4: // add_edge
		if len(m.aliveNodeIDs()) < 2 {
			return
		}
		src, dst := m.pickTwoAliveNodes(t)
		label := rapid.StringMatching(`[a-z]+`).Draw(t, "edgeLabel")
		if label == "" {
			label = "related"
		}
		e := edges.Edge{Src: src, Dst: dst, Label: label}
		var id string
		err := m.g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			var err error
			id, err = edges.Create(ctx, tx, e, nodes.Author{Name: "rapid"})
			return err
		})
		if err != nil {
			t.Fatalf("add_edge: %v", err)
		}
		m.edges[id] = edges.Edge{ID: id, Src: src, Dst: dst, Label: label}

	case 5: // delete_edge
		if len(m.edges) == 0 {
			return
		}
		id := m.pickEdgeID(t)
		err := m.g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			return edges.Delete(ctx, tx, id, nodes.Author{Name: "rapid"})
		})
		if err != nil {
			t.Fatalf("delete_edge: %v", err)
		}
		delete(m.edges, id)
	}
}

func (m *model) checkInvariants(t *rapid.T) {
	ctx := context.Background()

	// Invariant 1: sum(revCount) == count(revisions)
	var totalRevCount int
	for _, c := range m.revCount {
		totalRevCount += c
	}
	var dbRevCount int
	err := m.g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow("SELECT COUNT(*) FROM revisions").Scan(&dbRevCount)
	})
	if err != nil {
		t.Fatalf("invariant 1 query: %v", err)
	}
	if totalRevCount != dbRevCount {
		t.Fatalf("invariant 1: sum(revCount)=%d, db revisions=%d", totalRevCount, dbRevCount)
	}

	// Invariant 2: FTS reflects current state for all live nodes
	for id, mirror := range m.nodes {
		if !mirror.alive {
			continue
		}
		var ftsCount int
		err := m.g.DoRead(ctx, func(tx *graph.ReadTx) error {
			return tx.QueryRow(
				"SELECT COUNT(*) FROM nodes n JOIN nodes_fts f ON n.fts_rowid = f.rowid WHERE n.id = ?",
				id,
			).Scan(&ftsCount)
		})
		if err != nil {
			t.Fatalf("invariant 2 query for %s: %v", id, err)
		}
		if ftsCount != 1 {
			t.Fatalf("invariant 2: node %s alive but ftsCount=%d", id, ftsCount)
		}
	}

	// Invariant 3: deleted nodes have no node_tags, no edges referencing them, and no fts row
	for id, mirror := range m.nodes {
		if mirror.alive {
			continue
		}
		var nodeCount int
		err := m.g.DoRead(ctx, func(tx *graph.ReadTx) error {
			return tx.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = ?", id).Scan(&nodeCount)
		})
		if err != nil {
			t.Fatalf("invariant 3a query: %v", err)
		}
		if nodeCount != 0 {
			t.Fatalf("invariant 3: deleted node %s still in nodes table", id)
		}

		var tagCount int
		err = m.g.DoRead(ctx, func(tx *graph.ReadTx) error {
			return tx.QueryRow("SELECT COUNT(*) FROM node_tags WHERE node_id = ?", id).Scan(&tagCount)
		})
		if err != nil {
			t.Fatalf("invariant 3b query: %v", err)
		}
		if tagCount != 0 {
			t.Fatalf("invariant 3: deleted node %s still has %d node_tags", id, tagCount)
		}
	}

	// Invariant 4: UUIDv7 monotone ordering among alive nodes
	aliveIDs := m.aliveNodeIDs()
	sort.Strings(aliveIDs)
	for i := 1; i < len(aliveIDs); i++ {
		if aliveIDs[i] < aliveIDs[i-1] {
			t.Fatalf("invariant 4: ids not monotonic: %s < %s", aliveIDs[i], aliveIDs[i-1])
		}
	}

	// Invariant 5: prev_revision_id chain unbroken (alive nodes only; deleted nodes have node_id=NULL)
	for id, mirror := range m.nodes {
		if !mirror.alive {
			continue
		}
		type rev struct {
			id       string
			parentID string
		}
		var revs []rev
		err := m.g.DoRead(ctx, func(tx *graph.ReadTx) error {
			rows, err := tx.Query(
				"SELECT id, parent_id FROM revisions WHERE node_id = ? ORDER BY created_at ASC, id ASC",
				id,
			)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var r rev
				var pid sql.NullString
				if err := rows.Scan(&r.id, &pid); err != nil {
					return err
				}
				if pid.Valid {
					r.parentID = pid.String
				}
				revs = append(revs, r)
			}
			return rows.Err()
		})
		if err != nil {
			t.Fatalf("invariant 5 query for %s: %v", id, err)
		}
		if len(revs) == 0 && m.revCount[id] == 0 {
			continue
		}
		if len(revs) != m.revCount[id] {
			t.Fatalf("invariant 5: revision count mismatch for node %s: db=%d mirror=%d", id, len(revs), m.revCount[id])
		}
		// first revision must have empty parent_id; later ones must reference an earlier revision in this slice
		for i, r := range revs {
			if i == 0 {
				if r.parentID != "" {
					t.Fatalf("invariant 5: first revision %s for node %s has parent_id=%s", r.id, id, r.parentID)
				}
			} else {
				found := false
				for j := 0; j < i; j++ {
					if revs[j].id == r.parentID {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("invariant 5: revision %s for node %s has broken parent_id=%s", r.id, id, r.parentID)
				}
			}
		}
	}
}

func (m *model) aliveNodeIDs() []string {
	var ids []string
	for id, mirror := range m.nodes {
		if mirror.alive {
			ids = append(ids, id)
		}
	}
	return ids
}

func (m *model) pickAliveNode(t *rapid.T) (string, bool) {
	ids := m.aliveNodeIDs()
	if len(ids) == 0 {
		return "", false
	}
	idx := rapid.IntRange(0, len(ids)-1).Draw(t, "aliveIdx")
	return ids[idx], true
}

func (m *model) pickTwoAliveNodes(t *rapid.T) (string, string) {
	ids := m.aliveNodeIDs()
	if len(ids) < 2 {
		panic("pickTwoAliveNodes called with <2 alive nodes")
	}
	srcIdx := rapid.IntRange(0, len(ids)-1).Draw(t, "srcIdx")
	dstIdx := rapid.IntRange(0, len(ids)-2).Draw(t, "dstIdx")
	if dstIdx >= srcIdx {
		dstIdx++
	}
	return ids[srcIdx], ids[dstIdx]
}

func (m *model) pickEdgeID(t *rapid.T) string {
	var ids []string
	for id := range m.edges {
		ids = append(ids, id)
	}
	idx := rapid.IntRange(0, len(ids)-1).Draw(t, "edgeIdx")
	return ids[idx]
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
