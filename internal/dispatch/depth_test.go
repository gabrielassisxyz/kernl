package dispatch

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// TestClassifyDepth_GateOnOpenDesignLanguage reproduces the real case that
// calibrates this classifier: a bead disqualified itself, in its own
// description, as not yet a decided piece of work. A depth router that sent
// this bead into the autonomous pipeline would be wrong - this bead is a
// decision gate for the operator, not a candidate for autonomous work.
func TestClassifyDepth_GateOnOpenDesignLanguage(t *testing.T) {
	b := backend.Bead{
		ID:          "arch-3ff",
		Type:        "task",
		Title:       "Reduce p99 latency on the ingest path",
		Description: "The shape of a fix is not yet chosen. Worth measuring before it is built.",
	}

	got := ClassifyDepth(b)

	if got.Depth != DepthGate {
		t.Fatalf("ClassifyDepth(%s).Depth = %q, want %q", b.ID, got.Depth, DepthGate)
	}
	if got.Reason == "" {
		t.Fatal("ClassifyDepth returned DepthGate with no reason - the reason is what lets the operator check the classification against the bead's own text")
	}
}

// TestClassifyDepth_FullPipelineWhenNoAcceptanceStated is the positive
// condition for DepthFullPipeline: no acceptance criteria are stated at
// all, so what "correct" means for this bead was never settled - that is
// real design work for a planner, not something a default can paper over.
func TestClassifyDepth_FullPipelineWhenNoAcceptanceStated(t *testing.T) {
	b := backend.Bead{
		ID:          "arch-hkk",
		Type:        "task",
		Title:       "Bring the retry backoff in line with the documented contract",
		Description: "The existing design doc describes the intent, but no acceptance criteria are written down yet.",
	}

	got := ClassifyDepth(b)

	if got.Depth != DepthFullPipeline {
		t.Fatalf("ClassifyDepth(%s).Depth = %q, want %q", b.ID, got.Depth, DepthFullPipeline)
	}
	if got.Reason == "" {
		t.Fatal("ClassifyDepth returned DepthFullPipeline with no reason")
	}
}

// TestClassifyDepth_ShortFlowIsTheDefaultWhenAcceptanceIsStated pins the new
// default: a bead that exists has already been planned, so once it states
// its own acceptance criteria and does not declare its design open, one
// implementer can go straight to it - regardless of bead type.
func TestClassifyDepth_ShortFlowIsTheDefaultWhenAcceptanceIsStated(t *testing.T) {
	cases := []struct {
		name string
		bead backend.Bead
	}{
		{
			name: "bug with acceptance",
			bead: backend.Bead{
				ID:          "bug-off-by-one",
				Type:        "bug",
				Title:       "Paginator drops the last page",
				Description: "The last page of results never renders.",
				Acceptance:  "TestPaginator_LastPageRenders passes",
			},
		},
		{
			name: "task with acceptance",
			bead: backend.Bead{
				ID:          "task-with-acceptance",
				Type:        "task",
				Title:       "Rename the export endpoint",
				Description: "The endpoint's name no longer matches what it does.",
				Acceptance:  "GET /api/export-v2 returns the same payload the old /api/export did",
			},
		},
		{
			name: "feature with acceptance",
			bead: backend.Bead{
				ID:          "feature-with-acceptance",
				Type:        "feature",
				Title:       "Add a dark mode toggle",
				Description: "Users have asked for a dark theme.",
				Acceptance:  "A toggle in settings switches the whole app to a dark palette",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyDepth(tc.bead)
			if got.Depth != DepthShortFlow {
				t.Fatalf("ClassifyDepth(%s).Depth = %q, want %q", tc.bead.ID, got.Depth, DepthShortFlow)
			}
		})
	}
}

// TestClassifyDepth_BugWithoutAcceptanceIsNotShortFlow guards the other
// side of the short-flow rule: a bug alone is not enough. Without stated
// acceptance criteria there is no failing test standing in for "correct",
// so this still needs a planner.
func TestClassifyDepth_BugWithoutAcceptanceIsNotShortFlow(t *testing.T) {
	b := backend.Bead{
		ID:          "bug-vague",
		Type:        "bug",
		Title:       "Search feels slow sometimes",
		Description: "Users have reported search feeling sluggish under some conditions.",
	}

	got := ClassifyDepth(b)

	if got.Depth != DepthFullPipeline {
		t.Fatalf("ClassifyDepth(%s).Depth = %q, want %q", b.ID, got.Depth, DepthFullPipeline)
	}
}

// TestClassifyDepth_GateBeatsAcceptance pins the order the two rules are
// checked in: a bead with acceptance criteria that still declares its own
// design open must gate, not short-flow. Acceptance criteria describing the
// desired outcome does not resolve an undecided approach.
func TestClassifyDepth_GateBeatsAcceptance(t *testing.T) {
	b := backend.Bead{
		ID:          "bug-with-open-design",
		Type:        "bug",
		Title:       "Memory leak in the vault watcher",
		Description: "Root cause has two candidate explanations; which one to pursue is not yet chosen.",
		Acceptance:  "Memory stays flat over a 24h soak test",
	}

	got := ClassifyDepth(b)

	if got.Depth != DepthGate {
		t.Fatalf("ClassifyDepth(%s).Depth = %q, want %q", b.ID, got.Depth, DepthGate)
	}
}

// TestProposeDepths_ClassifiesEachCandidateIndependently is the list
// contract the operator's "what can be worked on today?" question needs: a
// depth and a reason per item, order preserved, one candidate's depth never
// leaking into another's.
func TestProposeDepths_ClassifiesEachCandidateIndependently(t *testing.T) {
	candidates := []backend.Bead{
		{ID: "gate-1", Type: "task", Description: "Not yet decided which store to use."},
		{ID: "short-1", Type: "bug", Description: "Off-by-one in the exporter.", Acceptance: "TestExporterCount passes"},
		{ID: "full-1", Type: "feature", Description: "Add a dark mode toggle."},
	}

	got := ProposeDepths(candidates)

	if len(got) != len(candidates) {
		t.Fatalf("ProposeDepths returned %d proposals, want %d", len(got), len(candidates))
	}

	want := map[string]Depth{"gate-1": DepthGate, "short-1": DepthShortFlow, "full-1": DepthFullPipeline}
	for i, p := range got {
		if p.ID != candidates[i].ID {
			t.Errorf("proposal[%d].ID = %q, want %q (order not preserved)", i, p.ID, candidates[i].ID)
		}
		if p.Depth != want[p.ID] {
			t.Errorf("proposal for %q: Depth = %q, want %q", p.ID, p.Depth, want[p.ID])
		}
		if strings.TrimSpace(p.Reason) == "" {
			t.Errorf("proposal for %q has no reason", p.ID)
		}
	}
}
