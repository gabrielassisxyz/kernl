package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// wellFormedAuditFixtureSections is shaped like what
// backend.DecisionRecordSectionBodies extracts from a real
// decision-record.md - one key per required section. Feeding it straight to
// app.WriteDecisionRecordNode, the actual production write path, proves the
// endpoint surfaces a record written by that path, not a hand-rolled
// fixture shaped only to satisfy the handler's query.
var wellFormedAuditFixtureSections = map[string]string{
	"decision":           "Use edges.EdgeTypeHasDecision for the bead/epic link.",
	"options_considered": "A bare string literal vs a new typed constant.",
	"trade_offs":         "A typed constant is one more name, but closes the set.",
	"rationale":          "Matches the existing closed edge-type set.",
}

func TestAuditDecisionsHandler(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	// A record written by this bead's actual production path.
	// WriteDecisionRecordNode creates the bead's and the epic's own
	// reference nodes itself - no stand-in seeding needed.
	bead := app.BeadRef{ID: "kb-1", Title: "bead", TrackerKind: "br", RepoPath: "/repo"}
	epic := app.BeadRef{ID: "kb-epic-1", Title: "epic", TrackerKind: "br", RepoPath: "/repo"}
	if _, err := app.WriteDecisionRecordNode(ctx, g, wellFormedAuditFixtureSections, bead, epic); err != nil {
		t.Fatalf("WriteDecisionRecordNode: %v", err)
	}

	// A record from the pre-existing, still-dead auto-approval path
	// (dispatch.LogAutonomousDecision's shape). It must NOT appear: it
	// carries a different tag and a different edge type, and this endpoint
	// is deliberately scoped to Phase 3 decision records only.
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
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
		_, err = edges.Create(ctx, tx, edges.Edge{Src: "kb-epic-1", Dst: decisionID, Type: "audit-log"}, nodes.Author{Name: "agent"})
		return err
	})
	if err != nil {
		t.Fatalf("seeding autonomous-tagged decision: %v", err)
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

	// Should only include the Phase 3 decision record, not the
	// autonomous-tagged one.
	if len(res) != 1 {
		t.Fatalf("expected 1 decision, got %d: %+v", len(res), res)
	}

	got := res[0]
	if got.Context != wellFormedAuditFixtureSections["decision"] {
		t.Errorf("Context = %q, want the decision section text", got.Context)
	}
	if got.Outcome != wellFormedAuditFixtureSections["rationale"] {
		t.Errorf("Outcome = %q, want the rationale section text", got.Outcome)
	}
	if got.ImpactOnUse != nil {
		t.Errorf("ImpactOnUse = %q, want nil (awaiting the composer)", *got.ImpactOnUse)
	}
	if len(got.RelatedIDs) != 2 {
		t.Fatalf("expected 2 related IDs (bead + epic), got %d: %v", len(got.RelatedIDs), got.RelatedIDs)
	}
	relatedSet := map[string]bool{}
	for _, id := range got.RelatedIDs {
		relatedSet[id] = true
	}
	if !relatedSet["kb-1"] || !relatedSet["kb-epic-1"] {
		t.Errorf("RelatedIDs = %v, want both kb-1 and kb-epic-1", got.RelatedIDs)
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
		[]string{"id", "createdAt", "title", "body", "context", "outcome", "impactOnUse", "tags", "relatedIds"},
		[]string{"CreatedAt", "RelatedIDs", "ImpactOnUse", "created_at", "related_ids", "impact_on_use"},
	)

	if got := w.Header().Get("X-Kernl-Truncated"); got != "false" {
		t.Errorf("X-Kernl-Truncated = %q, want %q (1 decision, well under the page limit)", got, "false")
	}
}

// TestAuditDecisionsHandler_TruncationIsSignaled proves the 100-row cap is
// no longer silent now that this endpoint has a real writer: with more than
// decisionsPageLimit records, the response still returns exactly the page
// limit (out-of-scope pagination is not being built here), but
// X-Kernl-Truncated tells a caller it is looking at the newest page rather
// than the whole result set.
func TestAuditDecisionsHandler_TruncationIsSignaled(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	bead := app.BeadRef{ID: "kb-many", Title: "bead", TrackerKind: "br", RepoPath: "/repo"}

	const seeded = 101 // one more than decisionsPageLimit
	for i := 0; i < seeded; i++ {
		sections := map[string]string{
			"decision":           fmt.Sprintf("Decision number %d.", i),
			"options_considered": wellFormedAuditFixtureSections["options_considered"],
			"trade_offs":         wellFormedAuditFixtureSections["trade_offs"],
			"rationale":          wellFormedAuditFixtureSections["rationale"],
		}
		if _, err := app.WriteDecisionRecordNode(ctx, g, sections, bead, bead); err != nil {
			t.Fatalf("WriteDecisionRecordNode(%d): %v", i, err)
		}
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
	if len(res) != 100 {
		t.Fatalf("expected exactly 100 decisions (the page limit), got %d", len(res))
	}

	if got := w.Header().Get("X-Kernl-Truncated"); got != "true" {
		t.Errorf("X-Kernl-Truncated = %q, want %q (%d records seeded, only 100 returned)", got, "true", seeded)
	}
}
