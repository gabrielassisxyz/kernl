package backend

import (
	"testing"
)

func TestNormalizeBead_FieldMappingAndDefaults(t *testing.T) {
	raw := RawBead{
		ID:                 "proj.epic.1",
		Title:              "Implement widget",
		Description:        "Build the widget component",
		Notes:              "See design doc",
		AcceptanceCriteria: "Widget renders correctly",
		IssueType:          "feature",
		Status:             "in_progress",
		Priority:           3,
		Labels:             []string{"frontend", "v2"},
		Assignee:           "alice",
		Owner:              "bob",
		Parent:             "proj.epic",
		Due:                "2026-03-01",
		EstimatedMinutes:   120,
		CreatedAt:          "2026-01-01T00:00:00Z",
		UpdatedAt:          "2026-02-01T00:00:00Z",
		ClosedAt:           "2026-02-15T00:00:00Z",
		CloseReason:        "completed",
		Metadata:           map[string]any{"source": "import"},
	}

	bead := NormalizeBead(raw)

	if bead.ID != "proj.epic.1" {
		t.Errorf("expected ID proj.epic.1, got %s", bead.ID)
	}
	if bead.Title != "Implement widget" {
		t.Errorf("expected Title 'Implement widget', got %s", bead.Title)
	}
	if bead.Description != "Build the widget component" {
		t.Errorf("expected Description, got %s", bead.Description)
	}
	if bead.Notes != "See design doc" {
		t.Errorf("expected Notes 'See design doc', got %s", bead.Notes)
	}
	if bead.Acceptance != "Widget renders correctly" {
		t.Errorf("expected Acceptance 'Widget renders correctly', got %s", bead.Acceptance)
	}
	if bead.Type != "feature" {
		t.Errorf("expected Type feature, got %s", bead.Type)
	}
	if bead.Priority != 3 {
		t.Errorf("expected Priority 3, got %d", bead.Priority)
	}
	if len(bead.Labels) != 2 || bead.Labels[0] != "frontend" || bead.Labels[1] != "v2" {
		t.Errorf("expected Labels [frontend, v2], got %v", bead.Labels)
	}
	if bead.Assignee != "alice" {
		t.Errorf("expected Assignee alice, got %s", bead.Assignee)
	}
	if bead.Owner != "bob" {
		t.Errorf("expected Owner bob, got %s", bead.Owner)
	}
	if bead.ParentID != "proj.epic" {
		t.Errorf("expected ParentID proj.epic, got %s", bead.ParentID)
	}
	if bead.Due != "2026-03-01" {
		t.Errorf("expected Due 2026-03-01, got %s", bead.Due)
	}
	if bead.Estimate != 120 {
		t.Errorf("expected Estimate 120, got %d", bead.Estimate)
	}
	if bead.CreatedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("expected CreatedAt, got %s", bead.CreatedAt)
	}
	if bead.UpdatedAt != "2026-02-01T00:00:00Z" {
		t.Errorf("expected UpdatedAt, got %s", bead.UpdatedAt)
	}
	if bead.ClosedAt != "2026-02-15T00:00:00Z" {
		t.Errorf("expected ClosedAt, got %s", bead.ClosedAt)
	}
	if bead.Metadata == nil {
		t.Error("expected Metadata to be non-nil")
	}
	if bead.Metadata["close_reason"] != "completed" {
		t.Errorf("expected close_reason in metadata, got %v", bead.Metadata)
	}
	if bead.Metadata["source"] != "import" {
		t.Errorf("expected source in metadata, got %v", bead.Metadata)
	}
}

func TestNormalizeBead_ParentInference_DottedID(t *testing.T) {
	raw := RawBead{ID: "a.b.c", Title: "Child"}
	bead := NormalizeBead(raw)
	if bead.ParentID != "a.b" {
		t.Errorf("expected inferred parent a.b, got %s", bead.ParentID)
	}
}

func TestNormalizeBead_ParentInference_ExplicitOverridesDotted(t *testing.T) {
	raw := RawBead{ID: "a.b.c", Title: "Child", Parent: "x.y"}
	bead := NormalizeBead(raw)
	if bead.ParentID != "x.y" {
		t.Errorf("expected explicit parent x.y, got %s", bead.ParentID)
	}
}

func TestNormalizeBead_ParentInference_TopLevelNoParent(t *testing.T) {
	raw := RawBead{ID: "toplevel", Title: "Root"}
	bead := NormalizeBead(raw)
	if bead.ParentID != "" {
		t.Errorf("expected empty parent, got %s", bead.ParentID)
	}
}

func TestNormalizeBead_DefaultsInvalidType(t *testing.T) {
	raw := RawBead{ID: "x", Title: "T", IssueType: "banana"}
	bead := NormalizeBead(raw)
	if bead.Type != "task" {
		t.Errorf("expected default type task, got %s", bead.Type)
	}
}

func TestNormalizeBead_DefaultsInvalidStatus(t *testing.T) {
	raw := RawBead{ID: "x", Title: "T", Status: "limbo"}
	bead := NormalizeBead(raw)
	if bead.State != "ready_for_implementation" {
		t.Errorf("expected default state ready_for_implementation, got %s", bead.State)
	}
}

func TestNormalizeBead_DefaultsOutOfRangePriority(t *testing.T) {
	raw := RawBead{ID: "x", Title: "T", Priority: 99}
	bead := NormalizeBead(raw)
	if bead.Priority != 2 {
		t.Errorf("expected default priority 2, got %d", bead.Priority)
	}
}

func TestNormalizeBead_MinimalFields(t *testing.T) {
	raw := RawBead{ID: "minimal", Title: "Bare minimum"}
	bead := NormalizeBead(raw)

	if bead.ID != "minimal" {
		t.Errorf("expected ID minimal, got %s", bead.ID)
	}
	if bead.Title != "Bare minimum" {
		t.Errorf("expected Title, got %s", bead.Title)
	}
	if bead.Type != "task" {
		t.Errorf("expected default type task, got %s", bead.Type)
	}
	if bead.Priority != 2 {
		t.Errorf("expected default priority 2, got %d", bead.Priority)
	}
	if len(bead.Labels) != 0 {
		t.Errorf("expected empty labels, got %v", bead.Labels)
	}
	if bead.Acceptance != "" {
		t.Errorf("expected empty acceptance, got %s", bead.Acceptance)
	}
}

func TestNormalizeBead_FilterEmptyLabels(t *testing.T) {
	raw := RawBead{ID: "x", Title: "T", Labels: []string{"a", "", "  ", "b"}}
	bead := NormalizeBead(raw)
	expected := []string{"a", "b"}
	if len(bead.Labels) != len(expected) {
		t.Fatalf("expected %d labels, got %d: %v", len(expected), len(bead.Labels), bead.Labels)
	}
	for i, v := range expected {
		if bead.Labels[i] != v {
			t.Errorf("label[%d]: expected %s, got %s", i, v, bead.Labels[i])
		}
	}
}

func TestNormalizeBead_AcceptanceCriteriaPreferredOverAcceptance(t *testing.T) {
	raw := RawBead{
		ID:                 "x",
		Title:              "T",
		AcceptanceCriteria: "Primary",
		Acceptance:         "Fallback",
	}
	bead := NormalizeBead(raw)
	if bead.Acceptance != "Primary" {
		t.Errorf("expected acceptance_criteria preferred, got %s", bead.Acceptance)
	}
}

func TestNormalizeBead_AcceptanceFallback(t *testing.T) {
	raw := RawBead{
		ID:         "x",
		Title:      "T",
		Acceptance: "Fallback",
	}
	bead := NormalizeBead(raw)
	if bead.Acceptance != "Fallback" {
		t.Errorf("expected acceptance fallback, got %s", bead.Acceptance)
	}
}

func TestNormalizeBead_EstimatedMinutesPreferredOverEstimate(t *testing.T) {
	raw := RawBead{
		ID:               "x",
		Title:            "T",
		EstimatedMinutes: 90,
		Estimate:         60,
	}
	bead := NormalizeBead(raw)
	if bead.Estimate != 90 {
		t.Errorf("expected estimated_minutes preferred (90), got %d", bead.Estimate)
	}
}

func TestNormalizeBead_EstimateFallback(t *testing.T) {
	raw := RawBead{
		ID:        "x",
		Title:     "T",
		Estimate:  60,
	}
	bead := NormalizeBead(raw)
	if bead.Estimate != 60 {
		t.Errorf("expected estimate fallback (60), got %d", bead.Estimate)
	}
}

func TestNormalizeBead_CreatedUpdatedAtFallbacks(t *testing.T) {
	raw := RawBead{
		ID:      "x",
		Title:   "T",
		Created: "2026-01-01T00:00:00Z",
		Updated: "2026-02-01T00:00:00Z",
	}
	bead := NormalizeBead(raw)
	if bead.CreatedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("expected created fallback, got %s", bead.CreatedAt)
	}
	if bead.UpdatedAt != "2026-02-01T00:00:00Z" {
		t.Errorf("expected updated fallback, got %s", bead.UpdatedAt)
	}
}

func TestNormalizeBead_InvariantParsing(t *testing.T) {
	raw := RawBead{
		ID:    "inv-1",
		Title: "Invariant parse",
		Notes: "Operator note\n\n[Invariants]\nScope: src/lib\nState: remain queued\n\nTail note",
	}
	bead := NormalizeBead(raw)

	if len(bead.Invariants) != 2 {
		t.Fatalf("expected 2 invariants, got %d", len(bead.Invariants))
	}
	if bead.Invariants[0].Kind != InvariantKindScope || bead.Invariants[0].Condition != "src/lib" {
		t.Errorf("expected Scope: src/lib, got %v", bead.Invariants[0])
	}
	if bead.Invariants[1].Kind != InvariantKindState || bead.Invariants[1].Condition != "remain queued" {
		t.Errorf("expected State: remain queued, got %v", bead.Invariants[1])
	}
	if bead.Notes != "Operator note\n\nTail note" {
		t.Errorf("expected cleaned notes, got %q", bead.Notes)
	}
}

func TestNormalizeBead_InvariantOnlyNotes(t *testing.T) {
	raw := RawBead{
		ID:    "inv-2",
		Title: "Invariant only",
		Notes: "[Invariants]\nScope: src/lib",
	}
	bead := NormalizeBead(raw)
	if len(bead.Invariants) != 1 {
		t.Fatalf("expected 1 invariant, got %d", len(bead.Invariants))
	}
	if bead.Invariants[0].Condition != "src/lib" {
		t.Errorf("expected src/lib, got %s", bead.Invariants[0].Condition)
	}
	if bead.Notes != "" {
		t.Errorf("expected empty notes, got %q", bead.Notes)
	}
}

func TestNormalizeBead_InvalidInvariantHeaderNoLines(t *testing.T) {
	raw := RawBead{
		ID:    "inv-invalid-1",
		Title: "Invariant marker only",
		Notes: "Operator note\n\n[Invariants]\nnot-an-invariant\nTail note",
	}
	bead := NormalizeBead(raw)
	if bead.Invariants != nil {
		t.Errorf("expected nil invariants, got %v", bead.Invariants)
	}
	if bead.Notes != "Operator note\n\n[Invariants]\nnot-an-invariant\nTail note" {
		t.Errorf("expected unchanged notes, got %q", bead.Notes)
	}
}

func TestNormalizeBead_TrimmedInvariantConditions(t *testing.T) {
	raw := RawBead{
		ID:    "inv-trim-1",
		Title: "Invariant trim",
		Notes: "[Invariants]\nScope:   src/lib/components   \nState:   must remain queued   ",
	}
	bead := NormalizeBead(raw)
	if len(bead.Invariants) != 2 {
		t.Fatalf("expected 2 invariants, got %d", len(bead.Invariants))
	}
	if bead.Invariants[0].Condition != "src/lib/components" {
		t.Errorf("expected trimmed condition, got %s", bead.Invariants[0].Condition)
	}
	if bead.Invariants[1].Condition != "must remain queued" {
		t.Errorf("expected trimmed condition, got %s", bead.Invariants[1].Condition)
	}
}

func TestNormalizeBead_WorkflowStateLabelAuthoritative(t *testing.T) {
	raw := RawBead{
		ID:      "x",
		Title:   "T",
		Status:  "open",
		Labels:  []string{"wf:state:plan_review", "wf:profile:semiauto"},
	}
	bead := NormalizeBead(raw)
	if bead.State != "plan_review" {
		t.Errorf("expected state plan_review from label, got %s", bead.State)
	}
	if bead.ProfileID != "semiauto" {
		t.Errorf("expected profileId semiauto from label, got %s", bead.ProfileID)
	}
}

func TestNormalizeBead_CloseReasonInMetadata(t *testing.T) {
	raw := RawBead{
		ID:          "x",
		Title:       "T",
		CloseReason: "completed",
		Metadata:    map[string]any{"source": "import"},
	}
	bead := NormalizeBead(raw)
	if bead.Metadata["close_reason"] != "completed" {
		t.Errorf("expected close_reason in metadata, got %v", bead.Metadata)
	}
	if bead.Metadata["source"] != "import" {
		t.Errorf("expected source preserved in metadata, got %v", bead.Metadata)
	}
}

func TestNormalizeBead_EmptyLabelsNil(t *testing.T) {
	raw := RawBead{ID: "x", Title: "T"}
	bead := NormalizeBead(raw)
	if bead.Labels == nil {
		t.Error("expected non-nil empty labels slice")
	}
	if len(bead.Labels) != 0 {
		t.Errorf("expected 0 labels, got %d", len(bead.Labels))
	}
}

func TestNormalizeBead_ParentFromDependencies(t *testing.T) {
	raw := RawBead{
		ID:    "proj.1.2",
		Title: "Child",
		Dependencies: []RawDependency{
			{SourceID: "proj.1", TargetID: "proj.1.2", DepType: "parent-child"},
		},
	}
	bead := NormalizeBead(raw)
	if bead.ParentID != "proj.1" {
		t.Errorf("expected parent from dependency proj.1, got %s", bead.ParentID)
	}
	raw2 := RawBead{
		ID:    "proj.1.2",
		Title: "Child",
		Parent: "explicit.parent",
		Dependencies: []RawDependency{
			{SourceID: "proj.1", TargetID: "proj.1.2", DepType: "parent-child"},
		},
	}
	beat2 := NormalizeBead(raw2)
	if beat2.ParentID != "explicit.parent" {
		t.Errorf("explicit parent should override dependency parent, got %s", beat2.ParentID)
	}
}

// ── DenormalizeBead tests ─────────────────────────────────────────

func TestDenormalizeBead_FieldMapping(t *testing.T) {
	bead := Bead{
		ID:          "proj.epic.1",
		Title:       "Implement widget",
		Description: "Build the widget component",
		Notes:       "See design doc",
		Acceptance:  "Widget renders correctly",
		Type:        "feature",
		State:       "implementation",
		Priority:    3,
		Labels:      []string{"frontend", "v2"},
		Assignee:    "alice",
		Owner:       "bob",
		ParentID:    "proj.epic",
		Due:         "2026-03-01",
		Estimate:    120,
		CreatedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:   "2026-02-01T00:00:00Z",
		ClosedAt:    "2026-02-15T00:00:00Z",
		Metadata:    map[string]any{"source": "import"},
	}

	raw := DenormalizeBead(bead)

	if raw.ID != "proj.epic.1" {
		t.Errorf("expected ID, got %s", raw.ID)
	}
	if raw.Title != "Implement widget" {
		t.Errorf("expected Title, got %s", raw.Title)
	}
	if raw.Description != "Build the widget component" {
		t.Errorf("expected Description, got %s", raw.Description)
	}
	if raw.Notes != "See design doc" {
		t.Errorf("expected Notes, got %s", raw.Notes)
	}
	if raw.AcceptanceCriteria != "Widget renders correctly" {
		t.Errorf("expected AcceptanceCriteria, got %s", raw.AcceptanceCriteria)
	}
	if raw.IssueType != "feature" {
		t.Errorf("expected IssueType feature, got %s", raw.IssueType)
	}
	if raw.Status != "in_progress" {
		t.Errorf("expected Status in_progress, got %s", raw.Status)
	}
	if raw.Priority != 3 {
		t.Errorf("expected Priority 3, got %d", raw.Priority)
	}
	if raw.Assignee != "alice" {
		t.Errorf("expected Assignee alice, got %s", raw.Assignee)
	}
	if raw.Owner != "bob" {
		t.Errorf("expected Owner bob, got %s", raw.Owner)
	}
	if raw.Parent != "proj.epic" {
		t.Errorf("expected Parent proj.epic, got %s", raw.Parent)
	}
	if raw.Due != "2026-03-01" {
		t.Errorf("expected Due, got %s", raw.Due)
	}
	if raw.EstimatedMinutes != 120 {
		t.Errorf("expected EstimatedMinutes 120, got %d", raw.EstimatedMinutes)
	}
	if raw.CreatedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("expected CreatedAt, got %s", raw.CreatedAt)
	}
	if raw.UpdatedAt != "2026-02-01T00:00:00Z" {
		t.Errorf("expected UpdatedAt, got %s", raw.UpdatedAt)
	}
	if raw.ClosedAt != "2026-02-15T00:00:00Z" {
		t.Errorf("expected ClosedAt, got %s", raw.ClosedAt)
	}
	if raw.Metadata["source"] != "import" {
		t.Errorf("expected metadata source, got %v", raw.Metadata)
	}
}

func TestDenormalizeBead_OmitsUndefinedFields(t *testing.T) {
	bead := Bead{
		ID:        "minimal",
		Title:     "Bare",
		Type:      "task",
		State:     "open",
		Priority:  2,
		Labels:    []string{},
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}
	raw := DenormalizeBead(bead)

	if raw.Description != "" {
		t.Errorf("expected empty Description, got %s", raw.Description)
	}
	if raw.Notes != "" {
		t.Errorf("expected empty Notes, got %s", raw.Notes)
	}
	if raw.AcceptanceCriteria != "" {
		t.Errorf("expected empty AcceptanceCriteria, got %s", raw.AcceptanceCriteria)
	}
	if raw.Assignee != "" {
		t.Errorf("expected empty Assignee, got %s", raw.Assignee)
	}
	if raw.Owner != "" {
		t.Errorf("expected empty Owner, got %s", raw.Owner)
	}
	if raw.Parent != "" {
		t.Errorf("expected empty Parent, got %s", raw.Parent)
	}
	if raw.Due != "" {
		t.Errorf("expected empty Due, got %s", raw.Due)
	}
	if raw.EstimatedMinutes != 0 {
		t.Errorf("expected 0 EstimatedMinutes, got %d", raw.EstimatedMinutes)
	}
	if raw.ClosedAt != "" {
		t.Errorf("expected empty ClosedAt, got %s", raw.ClosedAt)
	}
}

func TestDenormalizeBead_InvariantEmbedding(t *testing.T) {
	bead := Bead{
		ID:         "inv-3",
		Title:      "Invariant write",
		Notes:      "Operator note",
		Type:       "task",
		State:      "open",
		Priority:   2,
		Labels:     []string{},
		Invariants: []Invariant{
			{Kind: InvariantKindScope, Condition: "src/lib"},
			{Kind: InvariantKindState, Condition: "remain queued"},
		},
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}

	raw := DenormalizeBead(bead)
	expected := "Operator note\n\n[Invariants]\nScope: src/lib\nState: remain queued"
	if raw.Notes != expected {
		t.Errorf("expected embedded invariants, got %q", raw.Notes)
	}
}

func TestDenormalizeBead_InvariantSectionOnly(t *testing.T) {
	bead := Bead{
		ID:        "inv-4",
		Title:     "Invariant section only",
		Type:      "task",
		State:     "open",
		Priority:  2,
		Labels:    []string{},
		Invariants: []Invariant{
			{Kind: InvariantKindScope, Condition: "src/lib"},
		},
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}

	raw := DenormalizeBead(bead)
	if raw.Notes != "[Invariants]\nScope: src/lib" {
		t.Errorf("expected invariant section only, got %q", raw.Notes)
	}
}

func TestDenormalizeBead_SkipsBlankInvariantConditions(t *testing.T) {
	bead := Bead{
		ID:        "inv-5",
		Title:     "Invariant blank",
		Notes:     "Operator note",
		Type:      "task",
		State:     "open",
		Priority:  2,
		Labels:    []string{},
		Invariants: []Invariant{
			{Kind: InvariantKindScope, Condition: "   "},
			{Kind: InvariantKindState, Condition: " must remain queued "},
		},
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}

	raw := DenormalizeBead(bead)
	expected := "Operator note\n\n[Invariants]\nState: must remain queued"
	if raw.Notes != expected {
		t.Errorf("expected blank invariants skipped, got %q", raw.Notes)
	}
}

func TestDenormalizeBead_AddsWorkflowLabels(t *testing.T) {
	bead := Bead{
		ID:        "x",
		Title:     "T",
		Type:      "task",
		State:     "implementation",
		Priority:  2,
		Labels:    []string{"frontend"},
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}
	raw := DenormalizeBead(bead)

	hasStateLabel := false
	hasProfileLabel := false
	for _, l := range raw.Labels {
		if l == "wf:state:implementation" {
			hasStateLabel = true
		}
		if l == "wf:profile:autopilot_no_planning" {
			hasProfileLabel = true
		}
	}
	if !hasStateLabel {
		t.Error("expected wf:state:implementation label to be added")
	}
	if !hasProfileLabel {
		t.Error("expected wf:profile:autopilot_no_planning label to be added")
	}
}

// ── Round-trip tests ──────────────────────────────────────────────

func TestRoundTrip_PreservesAllFields(t *testing.T) {
	raw := RawBead{
		ID:                 "proj.epic.1",
		Title:              "Implement widget",
		Description:        "Build the widget component",
		Notes:              "See design doc",
		AcceptanceCriteria: "Widget renders correctly",
		IssueType:          "feature",
		Status:             "in_progress",
		Priority:           3,
		Labels:             []string{"frontend", "v2"},
		Assignee:           "alice",
		Owner:              "bob",
		Parent:             "proj.epic",
		Due:                "2026-03-01",
		EstimatedMinutes:   120,
		CreatedAt:          "2026-01-01T00:00:00Z",
		UpdatedAt:          "2026-02-01T00:00:00Z",
		ClosedAt:           "2026-02-15T00:00:00Z",
		CloseReason:        "completed",
		Metadata:           map[string]any{"source": "import"},
	}

	domain := NormalizeBead(raw)
	serialized := DenormalizeBead(domain)
	restored := NormalizeBead(serialized)

	if restored.ID != domain.ID {
		t.Errorf("round-trip ID: expected %s, got %s", domain.ID, restored.ID)
	}
	if restored.Title != domain.Title {
		t.Errorf("round-trip Title: expected %s, got %s", domain.Title, restored.Title)
	}
	if restored.Description != domain.Description {
		t.Errorf("round-trip Description: expected %s, got %s", domain.Description, restored.Description)
	}
	if restored.Acceptance != domain.Acceptance {
		t.Errorf("round-trip Acceptance: expected %s, got %s", domain.Acceptance, restored.Acceptance)
	}
	if restored.Type != domain.Type {
		t.Errorf("round-trip Type: expected %s, got %s", domain.Type, restored.Type)
	}
	if restored.Priority != domain.Priority {
		t.Errorf("round-trip Priority: expected %d, got %d", domain.Priority, restored.Priority)
	}
	if restored.Assignee != domain.Assignee {
		t.Errorf("round-trip Assignee: expected %s, got %s", domain.Assignee, restored.Assignee)
	}
	if restored.Owner != domain.Owner {
		t.Errorf("round-trip Owner: expected %s, got %s", domain.Owner, restored.Owner)
	}
	if restored.ParentID != domain.ParentID {
		t.Errorf("round-trip ParentID: expected %s, got %s", domain.ParentID, restored.ParentID)
	}
	if restored.Due != domain.Due {
		t.Errorf("round-trip Due: expected %s, got %s", domain.Due, restored.Due)
	}
	if restored.Estimate != domain.Estimate {
		t.Errorf("round-trip Estimate: expected %d, got %d", domain.Estimate, restored.Estimate)
	}
	if restored.CreatedAt != domain.CreatedAt {
		t.Errorf("round-trip CreatedAt: expected %s, got %s", domain.CreatedAt, restored.CreatedAt)
	}
	if restored.UpdatedAt != domain.UpdatedAt {
		t.Errorf("round-trip UpdatedAt: expected %s, got %s", domain.UpdatedAt, restored.UpdatedAt)
	}
	if restored.ClosedAt != domain.ClosedAt {
		t.Errorf("round-trip ClosedAt: expected %s, got %s", domain.ClosedAt, restored.ClosedAt)
	}
}

func TestRoundTrip_MinimalBead(t *testing.T) {
	minimal := RawBead{ID: "solo", Title: "Lone bead"}
	domain := NormalizeBead(minimal)
	serialized := DenormalizeBead(domain)
	restored := NormalizeBead(serialized)

	if restored.ID != domain.ID {
		t.Errorf("round-trip ID: expected %s, got %s", domain.ID, restored.ID)
	}
	if restored.Title != domain.Title {
		t.Errorf("round-trip Title: expected %s, got %s", domain.Title, restored.Title)
	}
	if restored.Type != domain.Type {
		t.Errorf("round-trip Type: expected %s, got %s", domain.Type, restored.Type)
	}
	if restored.Priority != domain.Priority {
		t.Errorf("round-trip Priority: expected %d, got %d", domain.Priority, restored.Priority)
	}
	if restored.ParentID != domain.ParentID {
		t.Errorf("round-trip ParentID: expected %s, got %s", domain.ParentID, restored.ParentID)
	}
}

func TestRoundTrip_AcceptanceCriteria(t *testing.T) {
	raw := RawBead{ID: "x", Title: "T", AcceptanceCriteria: "Must pass all tests"}
	domain := NormalizeBead(raw)
	if domain.Acceptance != "Must pass all tests" {
		t.Errorf("expected acceptance, got %s", domain.Acceptance)
	}
	serialized := DenormalizeBead(domain)
	if serialized.AcceptanceCriteria != "Must pass all tests" {
		t.Errorf("expected acceptance_criteria, got %s", serialized.AcceptanceCriteria)
	}
	restored := NormalizeBead(serialized)
	if restored.Acceptance != "Must pass all tests" {
		t.Errorf("round-trip acceptance: expected Must pass all tests, got %s", restored.Acceptance)
	}
}

func TestRoundTrip_EstimatedMinutes(t *testing.T) {
	raw := RawBead{ID: "x", Title: "T", EstimatedMinutes: 45}
	domain := NormalizeBead(raw)
	if domain.Estimate != 45 {
		t.Errorf("expected estimate 45, got %d", domain.Estimate)
	}
	serialized := DenormalizeBead(domain)
	if serialized.EstimatedMinutes != 45 {
		t.Errorf("expected estimated_minutes 45, got %d", serialized.EstimatedMinutes)
	}
	restored := NormalizeBead(serialized)
	if restored.Estimate != 45 {
		t.Errorf("round-trip estimate: expected 45, got %d", restored.Estimate)
	}
}

func TestRoundTrip_Invariants(t *testing.T) {
	bead := Bead{
		ID:         "inv-5",
		Title:      "Invariant round-trip",
		Notes:      "Visible note",
		Type:       "task",
		State:      "ready_for_planning",
		Priority:   2,
		Labels:     []string{},
		Invariants: []Invariant{{Kind: InvariantKindState, Condition: "must end queued"}},
		CreatedAt:  "2026-01-01T00:00:00Z",
		UpdatedAt:  "2026-01-01T00:00:00Z",
	}

	serialized := DenormalizeBead(bead)
	restored := NormalizeBead(serialized)

	if restored.Notes != "Visible note" {
		t.Errorf("round-trip notes: expected 'Visible note', got %q", restored.Notes)
	}
	if len(restored.Invariants) != 1 || restored.Invariants[0].Condition != "must end queued" {
		t.Errorf("round-trip invariants: expected 1 invariant 'must end queued', got %v", restored.Invariants)
	}
}

func TestRoundTrip_Timestamps(t *testing.T) {
	raw := RawBead{
		ID:        "x",
		Title:     "T",
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-02-01T00:00:00Z",
		ClosedAt:  "2026-03-01T00:00:00Z",
	}
	domain := NormalizeBead(raw)
	if domain.CreatedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("expected created, got %s", domain.CreatedAt)
	}
	if domain.UpdatedAt != "2026-02-01T00:00:00Z" {
		t.Errorf("expected updated, got %s", domain.UpdatedAt)
	}
	if domain.ClosedAt != "2026-03-01T00:00:00Z" {
		t.Errorf("expected closed, got %s", domain.ClosedAt)
	}

	serialized := DenormalizeBead(domain)
	if serialized.CreatedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("expected created_at, got %s", serialized.CreatedAt)
	}
	if serialized.UpdatedAt != "2026-02-01T00:00:00Z" {
		t.Errorf("expected updated_at, got %s", serialized.UpdatedAt)
	}
	if serialized.ClosedAt != "2026-03-01T00:00:00Z" {
		t.Errorf("expected closed_at, got %s", serialized.ClosedAt)
	}

	restored := NormalizeBead(serialized)
	if restored.CreatedAt != "2026-01-01T00:00:00Z" {
		t.Errorf("round-trip created: expected 2026-01-01, got %s", restored.CreatedAt)
	}
	if restored.UpdatedAt != "2026-02-01T00:00:00Z" {
		t.Errorf("round-trip updated: expected 2026-02-01, got %s", restored.UpdatedAt)
	}
	if restored.ClosedAt != "2026-03-01T00:00:00Z" {
		t.Errorf("round-trip closed: expected 2026-03-01, got %s", restored.ClosedAt)
	}
}

// ── Compat status mapping ────────────────────────────────────────

func TestMapBeadStateToCompatStatus(t *testing.T) {
	tests := []struct {
		state    string
		expected string
	}{
		{"deferred", "deferred"},
		{"blocked", "blocked"},
		{"rejected", "blocked"},
		{"shipped", "closed"},
		{"abandoned", "closed"},
		{"closed", "closed"},
		{"done", "closed"},
		{"approved", "closed"},
		{"ready_for_implementation", "open"},
		{"ready_for_planning", "open"},
		{"ready_for_review", "open"},
		{"planning", "in_progress"},
		{"implementation", "in_progress"},
		{"shipment_review", "in_progress"},
		{"plan_review", "in_progress"},
		{"unknown_state", "open"},
	}
	for _, tt := range tests {
		got := mapBeadStateToCompatStatus(tt.state)
		if got != tt.expected {
			t.Errorf("mapBeadStateToCompatStatus(%q) = %q, want %q", tt.state, got, tt.expected)
		}
	}
}

func TestRoundTrip_ExplicitParent(t *testing.T) {
	raw := RawBead{
		ID:     "a.b.c",
		Title:  "Reparented",
		Parent: "x.y",
	}
	domain := NormalizeBead(raw)
	if domain.ParentID != "x.y" {
		t.Errorf("expected explicit parent x.y, got %s", domain.ParentID)
	}
	serialized := DenormalizeBead(domain)
	if serialized.Parent != "x.y" {
		t.Errorf("expected serialized parent x.y, got %s", serialized.Parent)
	}
	restored := NormalizeBead(serialized)
	if restored.ParentID != "x.y" {
		t.Errorf("round-trip parent: expected x.y, got %s", restored.ParentID)
	}
}

func TestDenormalizeBead_CloseReasonInMetadata(t *testing.T) {
	bead := Bead{
		ID:        "x",
		Title:     "Closed bead",
		Type:      "task",
		State:     "shipped",
		Priority:  2,
		Labels:    []string{},
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
		Metadata:  map[string]any{"close_reason": "completed"},
	}
	raw := DenormalizeBead(bead)
	if raw.CloseReason != "completed" {
		t.Errorf("expected close_reason in raw, got %s", raw.CloseReason)
	}
}