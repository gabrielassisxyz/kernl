package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func TestAuditDecisionsHandler(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	// Seed some decisions
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		// Non-autonomous decision
		_, err := nodes.CreateDecision(ctx, tx, nodes.Decision{
			Title: "Human Decision",
			Tags:  []string{"audit"},
		}, nodes.Author{Name: "user"})
		if err != nil {
			return err
		}

		// Autonomous decision
		decisionID, err := nodes.CreateDecision(ctx, tx, nodes.Decision{
			Title:     "Autonomous Decision",
			Body:      "prompt",
			Context:   "action",
			Outcome:   "success",
			DecidedAt: time.Now(),
			Tags:      []string{"audit", "autonomous"},
		}, nodes.Author{Name: "agent"})
		if err != nil {
			return err
		}

		// Create edge for bead ID
		epicNodeID, err := nodes.CreateMemoryClaim(ctx, tx, nodes.MemoryClaim{
			Title: "epic1",
		}, nodes.Author{Name: "test"})
		if err != nil {
			return err
		}

		_, err = edges.Create(ctx, tx, edges.Edge{
			Src:  epicNodeID,
			Dst:  decisionID,
			Type: "audit-log",
		}, nodes.Author{Name: "agent"})

		return err
	})
	if err != nil {
		t.Fatalf("seeding graph: %v", err)
	}

	a := testApp()
	a.Graph = g
	r := NewRouter(a)

	req := httptest.NewRequest("GET", "/api/audit/decisions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var res []DecisionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should only include the autonomous decision
	if len(res) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(res))
	}

	if res[0].Title != "Autonomous Decision" {
		t.Errorf("unexpected title: %q", res[0].Title)
	}
	if len(res[0].RelatedIDs) != 1 {
		t.Errorf("expected 1 related ID, got %d", len(res[0].RelatedIDs))
	}

	// Decoding into DecisionResponse above passes regardless of the tag names,
	// since encoding/json matches struct field names too; the raw keys are the
	// only thing that proves the wire format is camelCase.
	var rawRes []map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &rawRes); err != nil {
		t.Fatalf("failed to decode raw response: %v", err)
	}
	if len(rawRes) != 1 {
		t.Fatalf("expected 1 raw decision, got %d", len(rawRes))
	}
	assertJSONKeys(t, rawRes[0],
		[]string{"id", "createdAt", "title", "body", "context", "outcome", "tags", "relatedIds"},
		[]string{"CreatedAt", "RelatedIDs", "created_at", "related_ids"},
	)
}
