package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

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
