package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// seedNodeEdgeFixture plants a small neighbourhood around n1 and returns the
// router to hit it with. Edges:
//
//	n1 --links_to--> n2 (note, path projects/two.md)
//	n3 --part_of-->  n1 (task)
//	n1 --links_to--> n4 (soft-deleted note, must be excluded)
//	n4 --related-->  n1 (soft-deleted node on the other end, must be excluded)
func seedNodeEdgeFixture(t *testing.T) *app.App {
	t.Helper()
	a := newTestAppWithGraph(t)
	err := a.Graph.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		stmts := []string{
			`INSERT INTO nodes (id, type, title) VALUES ('n1','note','One')`,
			`INSERT INTO nodes (id, type, title) VALUES ('n2','note','Two')`,
			`INSERT INTO nodes (id, type, title) VALUES ('n3','task','Three')`,
			`INSERT INTO nodes (id, type, title) VALUES ('n4','note','Deleted')`,
			`INSERT INTO note_paths (uuid, path) VALUES ('n2','projects/two.md')`,
			`INSERT INTO edges (id, src, dst, label) VALUES ('e1','n1','n2','links_to')`,
			`INSERT INTO edges (id, src, dst, label) VALUES ('e2','n3','n1','part_of')`,
			`INSERT INTO edges (id, src, dst, label) VALUES ('e3','n1','n4','links_to')`,
			`INSERT INTO edges (id, src, dst, label) VALUES ('e4','n4','n1','related')`,
			`UPDATE nodes SET deleted_at = '2026-08-20T00:00:00Z' WHERE id = 'n4'`,
		}
		for _, s := range stmts {
			if _, err := tx.Exec(s); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return a
}

func getNodeEdges(t *testing.T, a *app.App, query string) []map[string]any {
	t.Helper()
	r := NewRouter(a)
	req := httptest.NewRequest("GET", "/api/nodes/n1/edges"+query, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

// The one-hop neighbourhood returns both directions, each marked correctly,
// and never the soft-deleted node - from either end of the hop.
func TestNodeEdgesResolveMarksDirectionAndDropsTombstones(t *testing.T) {
	a := seedNodeEdgeFixture(t)
	out := getNodeEdges(t, a, "?resolve=true")

	if len(out) != 2 {
		t.Fatalf("want 2 live edges (n2 out, n3 in), got %d: %+v", len(out), out)
	}
	byNeighbour := map[string]map[string]any{}
	for _, e := range out {
		byNeighbour[e["id"].(string)] = e
	}

	n2, ok := byNeighbour["n2"]
	if !ok {
		t.Fatalf("missing outbound neighbour n2: %+v", out)
	}
	if n2["direction"] != "out" {
		t.Errorf("n2 direction = %v, want out", n2["direction"])
	}
	if n2["label"] != "links_to" {
		t.Errorf("n2 label = %v, want links_to", n2["label"])
	}

	n3, ok := byNeighbour["n3"]
	if !ok {
		t.Fatalf("missing inbound neighbour n3: %+v", out)
	}
	if n3["direction"] != "in" {
		t.Errorf("n3 direction = %v, want in", n3["direction"])
	}
	if n3["label"] != "part_of" {
		t.Errorf("n3 label = %v, want part_of", n3["label"])
	}

	for _, e := range out {
		if e["id"] == "n4" {
			t.Errorf("soft-deleted neighbour n4 must be excluded, got: %+v", out)
		}
	}
}

// resolve=true describes the NEIGHBOUR, not the asked-about node. n1's title
// and path differ from its neighbours', so this is where a backwards
// implementation shows itself.
func TestNodeEdgesResolveDescribesTheNeighbourNotTheAskedNode(t *testing.T) {
	a := seedNodeEdgeFixture(t)
	out := getNodeEdges(t, a, "?resolve=true")

	for _, e := range out {
		if e["id"] == "n1" {
			t.Errorf("resolved rows must describe the neighbour, got the asked node back: %+v", e)
		}
		if e["title"] == "One" {
			t.Errorf("resolved title must be the neighbour's, got %q for %v", e["title"], e["id"])
		}
		if e["via"] != "n1" {
			t.Errorf("via must be the asked node, got %v", e["via"])
		}
		if e["depth"] != float64(1) {
			t.Errorf("depth must be 1, got %v", e["depth"])
		}
	}

	var n2 map[string]any
	for _, e := range out {
		if e["id"] == "n2" {
			n2 = e
		}
	}
	if n2 == nil {
		t.Fatalf("n2 not in resolved output: %+v", out)
	}
	if n2["title"] != "Two" || n2["type"] != "note" || n2["path"] != "projects/two.md" {
		t.Errorf("n2 resolved as %+v, want title=Two type=note path=projects/two.md", n2)
	}
}

// label and type each narrow the result, and combine.
func TestNodeEdgesLabelAndTypeNarrowAndCombine(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  []string // neighbour ids, any order
	}{
		{name: "label only", query: "?label=links_to", want: []string{"n2"}},
		{name: "type only", query: "?type=task", want: []string{"n3"}},
		{name: "label and type match", query: "?label=part_of&type=task", want: []string{"n3"}},
		{name: "label and type disjoint", query: "?label=links_to&type=task", want: []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := seedNodeEdgeFixture(t)
			out := getNodeEdges(t, a, tc.query)
			got := make([]string, 0, len(out))
			for _, e := range out {
				// Unresolved shape: {id,src,dst,label}; the neighbour is the
				// endpoint that is not n1.
				src, _ := e["src"].(string)
				dst, _ := e["dst"].(string)
				if src == "n1" {
					got = append(got, dst)
				} else {
					got = append(got, src)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			wantSet := map[string]bool{}
			for _, w := range tc.want {
				wantSet[w] = true
			}
			for _, g := range got {
				if !wantSet[g] {
					t.Errorf("unexpected neighbour %q in %v, want %v", g, got, tc.want)
				}
			}
		})
	}
}

// resolve=false returns exactly the unscoped route's shape: {id,src,dst,label}
// and nothing else, filtered to the node.
func TestNodeEdgesUnresolvedMatchesUnscopedShape(t *testing.T) {
	a := seedNodeEdgeFixture(t)
	out := getNodeEdges(t, a, "")

	if len(out) != 2 {
		t.Fatalf("want 2 live edges, got %d: %+v", len(out), out)
	}
	for _, e := range out {
		for key := range e {
			switch key {
			case "id", "src", "dst", "label":
			default:
				t.Errorf("unresolved row carries off-contract key %q: %+v", key, e)
			}
		}
		if e["src"] != "n1" && e["dst"] != "n1" {
			t.Errorf("edge %v does not touch n1", e)
		}
		if e["id"] == "n4" {
			t.Errorf("edge touching soft-deleted n4 must be excluded: %+v", e)
		}
	}
}

// A node that does not exist must not answer like a node with no edges. This
// route is made to be WALKED, so a caller following a wrong id has to get an
// error rather than silence - the same distinction the vault convention draws
// between a broken link and a placeholder one.
func TestNodeEdgesUnknownNodeIs404NotEmpty(t *testing.T) {
	a := seedNodeEdgeFixture(t)
	r := NewRouter(a)

	for _, q := range []string{"", "?resolve=true"} {
		req := httptest.NewRequest("GET", "/api/nodes/does-not-exist/edges"+q, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("unknown node with query %q: got %d %s, want 404",
				q, w.Code, w.Body.String())
		}
	}
}

// A soft-deleted node is gone, not empty: asking about it must 404 too, or the
// tombstone reads as a node that simply has no edges.
func TestNodeEdgesTombstonedNodeIs404(t *testing.T) {
	a := seedNodeEdgeFixture(t)
	r := NewRouter(a)

	req := httptest.NewRequest("GET", "/api/nodes/n4/edges", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("tombstoned node: got %d %s, want 404", w.Code, w.Body.String())
	}
}

// A live node with every edge filtered out still answers 200 with an empty
// list. Empty and absent are different answers and this pins both.
func TestNodeEdgesLiveNodeWithNoMatchesIsEmpty200(t *testing.T) {
	a := seedNodeEdgeFixture(t)
	out := getNodeEdges(t, a, "?label=no-such-label")
	if len(out) != 0 {
		t.Errorf("want an empty list, got %+v", out)
	}
}

func TestListEdges(t *testing.T) {
	a := newTestAppWithGraphWithLLM(t)
	ctx := context.Background()

	var srcID, dstID string
	if err := a.Graph.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		if srcID, err = nodes.CreateNote(ctx, tx, nodes.Note{Title: "Source"}, nodes.Author{Name: "test"}); err != nil {
			return err
		}
		if dstID, err = nodes.CreateNote(ctx, tx, nodes.Note{Title: "Target"}, nodes.Author{Name: "test"}); err != nil {
			return err
		}
		_, err = edges.Create(ctx, tx, edges.Edge{Src: srcID, Dst: dstID, Type: edges.EdgeTypeLinksTo}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	r := NewRouter(a)
	req := httptest.NewRequest("GET", "/api/edges", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var list []struct {
		ID    string `json:"id"`
		Src   string `json:"src"`
		Dst   string `json:"dst"`
		Label string `json:"label"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 edge, got %d: %+v", len(list), list)
	}
	e := list[0]
	if e.Src != srcID || e.Dst != dstID || e.Label != "links_to" {
		t.Errorf("unexpected edge: %+v (want src=%s dst=%s label=links_to)", e, srcID, dstID)
	}
}
