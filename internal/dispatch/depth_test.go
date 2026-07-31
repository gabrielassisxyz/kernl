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

// TestClassifyDepth_FullPipelineWhenOnlyHowIsOpen is the contrasting case:
// a bead whose correct behaviour is already determined - its acceptance
// criteria could be written without choosing anything - but whose
// implementation approach is genuinely open. That openness is real design
// work for a planner, not grounds to refuse the bead outright.
func TestClassifyDepth_FullPipelineWhenOnlyHowIsOpen(t *testing.T) {
	b := backend.Bead{
		ID:          "arch-hkk",
		Type:        "task",
		Title:       "Bring the retry backoff in line with the documented contract",
		Description: "The existing design doc already specifies the required backoff behaviour end to end.",
	}

	got := ClassifyDepth(b)

	if got.Depth != DepthFullPipeline {
		t.Fatalf("ClassifyDepth(%s).Depth = %q, want %q", b.ID, got.Depth, DepthFullPipeline)
	}
}

// TestClassifyDepth_ShortFlowForBugWithAcceptance covers the other named
// depth: a localized defect where a failing test already defines what
// correct means needs one implementer, not a planner.
func TestClassifyDepth_ShortFlowForBugWithAcceptance(t *testing.T) {
	b := backend.Bead{
		ID:          "bug-off-by-one",
		Type:        "bug",
		Title:       "Paginator drops the last page",
		Description: "The last page of results never renders.",
		Acceptance:  "TestPaginator_LastPageRenders passes",
	}

	got := ClassifyDepth(b)

	if got.Depth != DepthShortFlow {
		t.Fatalf("ClassifyDepth(%s).Depth = %q, want %q", b.ID, got.Depth, DepthShortFlow)
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

// TestClassifyDepth_GateBeatsBugAcceptance pins the order the two rules are
// checked in: a bug with acceptance criteria that still declares its own
// design open must gate, not short-flow. Acceptance criteria describing the
// desired outcome does not resolve an undecided approach.
func TestClassifyDepth_GateBeatsBugAcceptance(t *testing.T) {
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
