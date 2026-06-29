package ingest

import (
	"context"
	"testing"
)

func TestProposeMergeHunksParsesStringArray(t *testing.T) {
	llm := &fakeLLM{content: `["First addition.", "  ", "Second addition."]`}

	hunks, err := proposeMergeHunks(context.Background(), llm, "existing body", "new content")
	if err != nil {
		t.Fatalf("proposeMergeHunks: %v", err)
	}
	if len(hunks) != 2 {
		t.Fatalf("expected 2 hunks (blank dropped), got %d", len(hunks))
	}
	if hunks[0].Content != "First addition." || hunks[1].Content != "Second addition." {
		t.Errorf("unexpected hunks: %+v", hunks)
	}
	if hunks[0].ID == hunks[1].ID {
		t.Errorf("expected distinct hunk ids, got %q and %q", hunks[0].ID, hunks[1].ID)
	}
}

func TestProposeMergeHunksGarbageReturnsEmpty(t *testing.T) {
	llm := &fakeLLM{content: "I'm sorry, I can't help."}

	hunks, err := proposeMergeHunks(context.Background(), llm, "existing", "new")
	if err != nil {
		t.Fatalf("proposeMergeHunks: %v", err)
	}
	if len(hunks) != 0 {
		t.Errorf("expected 0 hunks, got %d", len(hunks))
	}
}

func TestPlanMergeNoTargetReturnsEmptyPlan(t *testing.T) {
	g := openGraph(t)
	reviewID := seedReviewWith(t, g, "Orphan", "Completely unmatched subject matter.", "")

	plan, err := PlanMerge(context.Background(), g, &fakeLLM{content: "[]"}, reviewID)
	if err != nil {
		t.Fatalf("PlanMerge: %v", err)
	}
	if plan.TargetNoteID != "" {
		t.Errorf("expected empty target (fallback), got %q", plan.TargetNoteID)
	}
}

func TestPlanMergeResolvesTargetAndHunks(t *testing.T) {
	g := openGraph(t)
	targetID := createNote(t, g, "Photosynthesis", "Plants convert light into chemical energy.")
	reviewID := seedReviewWith(t, g, "Photosynthesis detail",
		"Photosynthesis also releases oxygen as a byproduct.", "")

	llm := &fakeLLM{content: `["Releases oxygen as a byproduct."]`}
	plan, err := PlanMerge(context.Background(), g, llm, reviewID)
	if err != nil {
		t.Fatalf("PlanMerge: %v", err)
	}
	if plan.TargetNoteID != targetID {
		t.Errorf("expected target %q, got %q", targetID, plan.TargetNoteID)
	}
	if plan.TargetTitle != "Photosynthesis" {
		t.Errorf("expected target title, got %q", plan.TargetTitle)
	}
	if len(plan.Hunks) != 1 || plan.Hunks[0].Content != "Releases oxygen as a byproduct." {
		t.Errorf("unexpected hunks: %+v", plan.Hunks)
	}
}
