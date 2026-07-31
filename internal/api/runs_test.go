package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func seedWorkflowRun(t *testing.T, g *graph.Graph, wr nodes.WorkflowRun) string {
	t.Helper()
	var id string
	err := g.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateWorkflowRun(context.Background(), tx, wr, nodes.Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("seeding workflow run: %v", err)
	}
	return id
}

func TestListRunsHandlerEmptyReturnsEmptyArrayNotNull(t *testing.T) {
	a := testApp()
	a.Graph = testutil.NewInMemoryTestGraph(t)
	r := NewRouter(a)

	req := httptest.NewRequest("GET", "/api/runs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	// An absent array must marshal as "[]", never "null" - a client that ranges
	// over the body without a nil guard breaks the moment the list is empty.
	if body := w.Body.String(); body != "[]\n" {
		t.Fatalf("expected empty array body, got %q", body)
	}
}

func TestListRunsHandlerHonoursFilters(t *testing.T) {
	a := testApp()
	g := testutil.NewInMemoryTestGraph(t)
	a.Graph = g
	r := NewRouter(a)

	seedWorkflowRun(t, g, nodes.WorkflowRun{
		Title: "run-a", WorkflowName: "epic-run", Status: "running", Tags: []string{"epic:e1"},
	})
	seedWorkflowRun(t, g, nodes.WorkflowRun{
		Title: "run-b", WorkflowName: "epic-run", Status: "done", Tags: []string{"epic:e2"},
	})
	seedWorkflowRun(t, g, nodes.WorkflowRun{
		Title: "run-c", WorkflowName: "bead-run", Status: "done", Tags: []string{"epic:e2"},
	})

	cases := []struct {
		name      string
		query     string
		wantCount int
	}{
		{"by workflowName", "?workflowName=epic-run", 2},
		{"by status", "?status=done", 2},
		{"by tags", "?tags=epic:e2", 2},
		{"by limit", "?limit=1", 1},
		{"combined", "?workflowName=epic-run&status=done", 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/runs"+tc.query, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != 200 {
				t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
			}
			var res []runDTO
			if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(res) != tc.wantCount {
				t.Fatalf("expected %d runs, got %d: %+v", tc.wantCount, len(res), res)
			}
		})
	}
}

func TestListRunsHandlerRejectsInvalidLimit(t *testing.T) {
	a := testApp()
	g := testutil.NewInMemoryTestGraph(t)
	a.Graph = g
	r := NewRouter(a)

	seedWorkflowRun(t, g, nodes.WorkflowRun{
		Title: "run-a", WorkflowName: "epic-run", Status: "running",
	})

	// Each of these must be rejected loudly rather than silently ignored: a
	// non-numeric value is an obvious typo, but a negative or zero limit is the
	// dangerous case, because ListWorkflowRuns only bounds the query when the
	// limit is > 0 and would otherwise return every run in the graph.
	for _, limit := range []string{"abc", "-5", "0"} {
		t.Run("limit="+limit, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/runs?limit="+limit, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != 400 {
				t.Fatalf("expected status 400 for limit=%s, got %d: %s", limit, w.Code, w.Body.String())
			}
		})
	}
}

func TestGetRunHandlerReturnsRun(t *testing.T) {
	a := testApp()
	g := testutil.NewInMemoryTestGraph(t)
	a.Graph = g
	r := NewRouter(a)

	id := seedWorkflowRun(t, g, nodes.WorkflowRun{
		Title: "run-a", WorkflowName: "epic-run", Status: "running", Tags: []string{"epic:e1"},
	})

	req := httptest.NewRequest("GET", "/api/runs/"+id, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	var res runDTO
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.ID != id || res.WorkflowName != "epic-run" || res.Status != "running" {
		t.Errorf("unexpected run: %+v", res)
	}
}

func TestGetRunHandlerMissingIDReturnsNotFound(t *testing.T) {
	a := testApp()
	a.Graph = testutil.NewInMemoryTestGraph(t)
	r := NewRouter(a)

	req := httptest.NewRequest("GET", "/api/runs/does-not-exist", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRunsJSONContract(t *testing.T) {
	a := testApp()
	g := testutil.NewInMemoryTestGraph(t)
	a.Graph = g
	r := NewRouter(a)

	seedWorkflowRun(t, g, nodes.WorkflowRun{
		Title: "run-a", WorkflowName: "epic-run", Status: "running", Tags: []string{"epic:e1"},
	})

	req := httptest.NewRequest("GET", "/api/runs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	var res []map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 run, got %d", len(res))
	}

	assertJSONKeys(t, res[0],
		[]string{"id", "createdAt", "updatedAt", "title", "workflowName", "status", "runData", "tags"},
		[]string{"ID", "CreatedAt", "UpdatedAt", "Title", "WorkflowName", "Status", "RunData", "Tags"},
	)
}
