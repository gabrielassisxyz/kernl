package orchestration

import (
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

func TestIsAgentClaimable(t *testing.T) {
	tests := []struct {
		kind string
		want bool
	}{
		{"agent", true},
		{"human", false},
		{"agent_review", false},
	}

	for _, tt := range tests {
		step := &Step{Kind: tt.kind}
		got := IsAgentClaimable(step)
		if got != tt.want {
			t.Errorf("IsAgentClaimable(%q): got %v, want %v", tt.kind, got, tt.want)
		}
	}
}

func TestRequiresHumanAction(t *testing.T) {
	tests := []struct {
		kind string
		want bool
	}{
		{"human", true},
		{"agent", false},
	}

	for _, tt := range tests {
		step := &Step{Kind: tt.kind}
		got := RequiresHumanAction(step)
		if got != tt.want {
			t.Errorf("RequiresHumanAction(%q): got %v, want %v", tt.kind, got, tt.want)
		}
	}
}

func TestIsQueueOrTerminal(t *testing.T) {
	tests := []struct {
		state string
		want  bool
	}{
		{"ready_for_implementation", true},
		{"ready_for_planning", true},
		{"ready_for_plan_review", true},
		{"shipped", true},
		{"abandoned", true},
		{"deferred", true},
		{"implementation", false},
		{"planning", false},
		{"plan_review", false},
	}

	for _, tt := range tests {
		got := IsQueueOrTerminal(tt.state)
		if got != tt.want {
			t.Errorf("IsQueueOrTerminal(%q): got %v, want %v", tt.state, got, tt.want)
		}
	}
}

func TestIsQueueOrTerminalWorkflow(t *testing.T) {
	wf := autopilotWorkflow()

	queueStates := []string{
		"ready_for_planning", "ready_for_plan_review",
		"ready_for_implementation", "ready_for_implementation_review",
		"ready_for_shipment", "ready_for_shipment_review",
	}
	for _, s := range queueStates {
		if !IsQueueOrTerminalWorkflow(s, &wf) {
			t.Errorf("IsQueueOrTerminalWorkflow(%q): expected true", s)
		}
	}

	terminalStates := []string{"shipped", "abandoned"}
	for _, s := range terminalStates {
		if !IsQueueOrTerminalWorkflow(s, &wf) {
			t.Errorf("IsQueueOrTerminalWorkflow(%q): expected true", s)
		}
	}

	if !IsQueueOrTerminalWorkflow("deferred", &wf) {
		t.Error("IsQueueOrTerminalWorkflow(deferred): expected true")
	}

	activeStates := []string{"planning", "plan_review", "implementation", "implementation_review", "shipment", "shipment_review"}
	for _, s := range activeStates {
		if IsQueueOrTerminalWorkflow(s, &wf) {
			t.Errorf("IsQueueOrTerminalWorkflow(%q): expected false for active state", s)
		}
	}

	if !IsQueueOrTerminalWorkflow("unknown_state", &wf) {
		t.Error("IsQueueOrTerminalWorkflow(unknown_state): expected true (unknown = not active)")
	}
	if !IsQueueOrTerminalWorkflow("", &wf) {
		t.Error("IsQueueOrTerminalWorkflow(''): expected true (empty = not active)")
	}
}

func TestResolveStep(t *testing.T) {
	steps := []Step{
		{ID: "step-1", Name: "Planning", Kind: "agent"},
		{ID: "step-2", Name: "Review", Kind: "human"},
	}

	got, err := ResolveStep(steps, "step-1")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if got.ID != "step-1" {
		t.Errorf("expected step-1, got %s", got.ID)
	}

	_, err = ResolveStep(steps, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent step")
	}
}

func TestResolveStepForWorkflow(t *testing.T) {
	wf := autopilotWorkflow()

	queueTests := []struct {
		state       string
		wantStep    string
		wantPhase   StepPhase
	}{
		{"ready_for_planning", "planning", PhaseQueued},
		{"ready_for_plan_review", "plan_review", PhaseQueued},
		{"ready_for_implementation", "implementation", PhaseQueued},
		{"ready_for_implementation_review", "implementation_review", PhaseQueued},
		{"ready_for_shipment", "shipment", PhaseQueued},
		{"ready_for_shipment_review", "shipment_review", PhaseQueued},
	}
	for _, tt := range queueTests {
		rs := ResolveStepForWorkflow(tt.state, &wf)
		if rs == nil {
			t.Fatalf("ResolveStepForWorkflow(%q): expected non-nil", tt.state)
		}
		if rs.Step != tt.wantStep {
			t.Errorf("ResolveStepForWorkflow(%q).Step: got %q, want %q", tt.state, rs.Step, tt.wantStep)
		}
		if rs.Phase != tt.wantPhase {
			t.Errorf("ResolveStepForWorkflow(%q).Phase: got %q, want %q", tt.state, rs.Phase, tt.wantPhase)
		}
	}

	activeTests := []struct {
		state     string
		wantPhase StepPhase
	}{
		{"planning", PhaseActive},
		{"plan_review", PhaseActive},
		{"implementation", PhaseActive},
		{"implementation_review", PhaseActive},
		{"shipment", PhaseActive},
		{"shipment_review", PhaseActive},
	}
	for _, tt := range activeTests {
		rs := ResolveStepForWorkflow(tt.state, &wf)
		if rs == nil {
			t.Fatalf("ResolveStepForWorkflow(%q): expected non-nil", tt.state)
		}
		if rs.Phase != tt.wantPhase {
			t.Errorf("ResolveStepForWorkflow(%q).Phase: got %q, want %q", tt.state, rs.Phase, tt.wantPhase)
		}
	}

	terminalTests := []string{"shipped", "abandoned", "deferred", "unknown_state", ""}
	for _, s := range terminalTests {
		rs := ResolveStepForWorkflow(s, &wf)
		if rs != nil {
			t.Errorf("ResolveStepForWorkflow(%q): expected nil for terminal/unknown state, got %+v", s, rs)
		}
	}
}

func TestDeriveWorkflowRuntimeState(t *testing.T) {
	wf := DefaultWorkflowDescriptor()

	t.Run("queue state derives runtime", func(t *testing.T) {
		rs := DeriveWorkflowRuntimeState(&wf, "ready_for_planning")
		if rs.State != "ready_for_planning" {
			t.Errorf("expected state ready_for_planning, got %q", rs.State)
		}
		if rs.NextActionOwnerKind != backend.ActionOwnerAgent {
			t.Errorf("expected agent owner, got %q", rs.NextActionOwnerKind)
		}
		if rs.RequiresHumanAction {
			t.Error("expected RequiresHumanAction=false for agent queue state")
		}
		if !rs.IsAgentClaimable {
			t.Error("expected IsAgentClaimable=true for agent queue state")
		}
	})

	t.Run("active state derives runtime", func(t *testing.T) {
		rs := DeriveWorkflowRuntimeState(&wf, "implementation")
		if rs.IsAgentClaimable {
			t.Error("expected IsAgentClaimable=false for active state")
		}
	})

	t.Run("terminal state derives runtime", func(t *testing.T) {
		rs := DeriveWorkflowRuntimeState(&wf, "shipped")
		if rs.NextActionOwnerKind != backend.ActionOwnerNone {
			t.Errorf("expected none owner for terminal state, got %q", rs.NextActionOwnerKind)
		}
	})

	t.Run("semiauto human-owned step", func(t *testing.T) {
		semiauto := BuiltinProfileDescriptor("semiauto")
		rs := DeriveWorkflowRuntimeState(&semiauto, "ready_for_plan_review")
		if rs.State != "ready_for_plan_review" {
			t.Errorf("expected state ready_for_plan_review, got %q", rs.State)
		}
		if !rs.RequiresHumanAction {
			t.Error("expected RequiresHumanAction=true for human-owned queue state")
		}
		if rs.IsAgentClaimable {
			t.Error("expected IsAgentClaimable=false for human-owned queue state")
		}
	})

	t.Run("undefined state normalizes to initial", func(t *testing.T) {
		rs := DeriveWorkflowRuntimeState(&wf, "")
		if rs.State != wf.InitialState {
			t.Errorf("expected initial state %q, got %q", wf.InitialState, rs.State)
		}
	})
}

func TestIsReviewStepForWorkflow(t *testing.T) {
	wf := autopilotWorkflow()

	reviewSteps := []string{"plan_review", "implementation_review", "shipment_review"}
	for _, step := range reviewSteps {
		if !IsReviewStepForWorkflow(step, &wf) {
			t.Errorf("expected %q to be review step", step)
		}
	}

	actionSteps := []string{"planning", "implementation", "shipment"}
	for _, step := range actionSteps {
		if IsReviewStepForWorkflow(step, &wf) {
			t.Errorf("expected %q NOT to be review step", step)
		}
	}
}

func TestPriorActionStep(t *testing.T) {
	wf := autopilotWorkflow()

	tests := []struct {
		step string
		want string
	}{
		{"plan_review", "planning"},
		{"implementation_review", "implementation"},
		{"shipment_review", "shipment"},
		{"planning", ""},
		{"implementation", ""},
	}
	for _, tt := range tests {
		got := PriorActionStep(tt.step, &wf)
		if got != tt.want {
			t.Errorf("PriorActionStep(%q): got %q, want %q", tt.step, got, tt.want)
		}
	}
}

func TestCompareWorkflowStatePriority(t *testing.T) {
	t.Run("orders known states by pipeline order", func(t *testing.T) {
		states := []string{"shipment_review", "ready_for_implementation", "planning", "ready_for_planning", "implementation"}
		expected := []string{"ready_for_planning", "planning", "ready_for_implementation", "implementation", "shipment_review"}

		for i := 0; i < len(states)-1; i++ {
			for j := i + 1; j < len(states); j++ {
				cmp := CompareWorkflowStatePriority(states[i], states[j])
				wantNegative := false
				for _, e := range expected {
					if e == states[i] {
						wantNegative = true
						break
					}
					if e == states[j] {
						break
					}
				}
				if wantNegative && cmp >= 0 {
					t.Errorf("CompareWorkflowStatePriority(%q, %q) = %d, want negative", states[i], states[j], cmp)
				}
			}
		}
	})

	t.Run("unknown states sort after known states", func(t *testing.T) {
		cmp := CompareWorkflowStatePriority("custom_alpha", "ready_for_shipment")
		if cmp <= 0 {
			t.Errorf("expected unknown state to sort after known state, got cmp=%d", cmp)
		}
	})
}

func TestIsRollbackTransition(t *testing.T) {
	tests := []struct {
		from string
		to   string
		want bool
	}{
		{"plan_review", "ready_for_planning", true},
		{"implementation_review", "ready_for_implementation", true},
		{"shipment_review", "ready_for_implementation", true},
		{"shipment_review", "ready_for_shipment", true},
		{"ready_for_planning", "planning", false},
		{"planning", "ready_for_plan_review", false},
		{"implementation", "ready_for_implementation_review", false},
		{"shipment_review", "shipped", false},
		{"planning", "planning", false},
		{"unknown", "planning", false},
		{"planning", "unknown", false},
		{"deferred", "planning", false},
	}
	for _, tt := range tests {
		got := IsRollbackTransition(tt.from, tt.to)
		if got != tt.want {
			t.Errorf("IsRollbackTransition(%q, %q): got %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestForwardTransitionTarget(t *testing.T) {
	wf := autopilotWorkflow()

	tests := []struct {
		state string
		want  string
	}{
		{"ready_for_planning", "planning"},
		{"planning", "ready_for_plan_review"},
		{"implementation", "ready_for_implementation_review"},
	}
	for _, tt := range tests {
		got := ForwardTransitionTarget(tt.state, &wf)
		if got != tt.want {
			t.Errorf("ForwardTransitionTarget(%q): got %q, want %q", tt.state, got, tt.want)
		}
	}

	t.Run("terminal state returns empty", func(t *testing.T) {
		got := ForwardTransitionTarget("shipped", &wf)
		if got != "" {
			t.Errorf("ForwardTransitionTarget(shipped): got %q, want empty", got)
		}
	})
}

func TestNormalizeStateForWorkflow(t *testing.T) {
	wf := DefaultWorkflowDescriptor()

	t.Run("returns initial state for undefined input", func(t *testing.T) {
		got := NormalizeStateForWorkflow("", &wf)
		if got != wf.InitialState {
			t.Errorf("expected %q, got %q", wf.InitialState, got)
		}
	})

	t.Run("passes through valid workflow states", func(t *testing.T) {
		got := NormalizeStateForWorkflow("implementation", &wf)
		if got != "implementation" {
			t.Errorf("expected implementation, got %q", got)
		}
	})

	t.Run("remaps legacy open state", func(t *testing.T) {
		got := NormalizeStateForWorkflow("open", &wf)
		if got != wf.InitialState {
			t.Errorf("expected %q, got %q", wf.InitialState, got)
		}
	})

	t.Run("remaps impl shorthand", func(t *testing.T) {
		got := NormalizeStateForWorkflow("impl", &wf)
		if got != "implementation" {
			t.Errorf("expected implementation, got %q", got)
		}
	})

	t.Run("remaps ready_for_review", func(t *testing.T) {
		got := NormalizeStateForWorkflow("ready_for_review", &wf)
		if got != "ready_for_implementation_review" {
			t.Errorf("expected ready_for_implementation_review, got %q", got)
		}
	})

	t.Run("remaps legacy terminal states", func(t *testing.T) {
		if NormalizeStateForWorkflow("closed", &wf) != "shipped" {
			t.Error("expected closed -> shipped")
		}
		if NormalizeStateForWorkflow("done", &wf) != "shipped" {
			t.Error("expected done -> shipped")
		}
		if NormalizeStateForWorkflow("approved", &wf) != "shipped" {
			t.Error("expected approved -> shipped")
		}
	})

	t.Run("preserves shipped/abandoned even when omitted from states", func(t *testing.T) {
		limitedWf := wf
		var filteredStates []string
		for _, s := range limitedWf.States {
			if s != "shipped" && s != "abandoned" {
				filteredStates = append(filteredStates, s)
			}
		}
		limitedWf.States = filteredStates

		if NormalizeStateForWorkflow("shipped", &limitedWf) != "shipped" {
			t.Error("expected shipped preserved")
		}
		if NormalizeStateForWorkflow("abandoned", &limitedWf) != "abandoned" {
			t.Error("expected abandoned preserved")
		}
	})

	t.Run("remaps legacy retake states", func(t *testing.T) {
		for _, s := range []string{"retake", "retry", "rejected", "refining", "rework"} {
			got := NormalizeStateForWorkflow(s, &wf)
			if got != wf.RetakeState {
				t.Errorf("NormalizeStateForWorkflow(%q): got %q, want %q", s, got, wf.RetakeState)
			}
		}
	})

	t.Run("handles case normalization", func(t *testing.T) {
		got := NormalizeStateForWorkflow("  IMPLEMENTATION  ", &wf)
		if got != "implementation" {
			t.Errorf("expected implementation, got %q", got)
		}
	})

	t.Run("unknown state returns initial", func(t *testing.T) {
		got := NormalizeStateForWorkflow("totally_unknown", &wf)
		if got != wf.InitialState {
			t.Errorf("expected %q, got %q", wf.InitialState, got)
		}
	})
}

func TestInferWorkflowMode(t *testing.T) {
	tests := []struct {
		id     string
		desc   string
		states []string
		want   string
	}{
		{"semiauto-flow", "", nil, "coarse_human_gated"},
		{"some-coarse-id", "", nil, "coarse_human_gated"},
		{"custom", "human gated flow", nil, "coarse_human_gated"},
		{"custom", "pull request output", nil, "coarse_human_gated"},
		{"autopilot", "", nil, "granular_autonomous"},
		{"custom", "agent-owned flow", nil, "granular_autonomous"},
	}
	for _, tt := range tests {
		got := InferWorkflowMode(tt.id, tt.desc, tt.states)
		if got != tt.want {
			t.Errorf("InferWorkflowMode(%q, %q, %v): got %q, want %q", tt.id, tt.desc, tt.states, got, tt.want)
		}
	}
}

func TestInferFinalCutState(t *testing.T) {
	tests := []struct {
		states []string
		want   string
	}{
		{[]string{"ready_for_plan_review", "ready_for_implementation_review"}, "ready_for_plan_review"},
		{[]string{"ready_for_implementation_review", "ready_for_shipment_review"}, "ready_for_implementation_review"},
		{[]string{"ready_for_shipment_review"}, "ready_for_shipment_review"},
		{[]string{"reviewing"}, "reviewing"},
		{[]string{"open", "closed"}, ""},
		{[]string{}, ""},
	}
	for _, tt := range tests {
		got := InferFinalCutState(tt.states)
		if got != tt.want {
			t.Errorf("InferFinalCutState(%v): got %q, want %q", tt.states, got, tt.want)
		}
	}
}

func TestInferRetakeState(t *testing.T) {
	tests := []struct {
		states       []string
		initialState string
		want         string
	}{
		{[]string{"ready_for_implementation", "retake"}, "open", "ready_for_implementation"},
		{[]string{"retake", "retry"}, "open", "retake"},
		{[]string{"retry", "rejected"}, "open", "retry"},
		{[]string{"refining"}, "open", "refining"},
		{[]string{"open", "closed"}, "open", "open"},
	}
	for _, tt := range tests {
		got := InferRetakeState(tt.states, tt.initialState)
		if got != tt.want {
			t.Errorf("InferRetakeState(%v, %q): got %q, want %q", tt.states, tt.initialState, got, tt.want)
		}
	}
}

func TestBeatRequiresHumanAction(t *testing.T) {
	descriptors := BuiltinWorkflowDescriptors()
	workflowsByID := WorkflowDescriptorByID(descriptors)

	t.Run("returns true when beat.RequiresHumanAction is true", func(t *testing.T) {
		beat := &backend.Beat{ID: "1", State: "plan_review", ProfileID: "autopilot", RequiresHumanAction: true}
		if !BeatRequiresHumanAction(beat, workflowsByID) {
			t.Error("expected true")
		}
	})

	t.Run("returns false when beat.RequiresHumanAction is false", func(t *testing.T) {
		beat := &backend.Beat{ID: "2", State: "planning", ProfileID: "semiauto", RequiresHumanAction: false}
		if BeatRequiresHumanAction(beat, workflowsByID) {
			t.Error("expected false")
		}
	})

	t.Run("derives from workflow when RequiresHumanAction not set", func(t *testing.T) {
		beat := &backend.Beat{ID: "3", State: "ready_for_plan_review", ProfileID: "semiauto"}
		if !BeatRequiresHumanAction(beat, workflowsByID) {
			t.Error("expected true for semiauto ready_for_plan_review")
		}
	})

	t.Run("returns false when workflow not found and no explicit flag", func(t *testing.T) {
		beat := &backend.Beat{ID: "4", State: "planning", ProfileID: "nonexistent"}
		if BeatRequiresHumanAction(beat, workflowsByID) {
			t.Error("expected false for missing workflow")
		}
	})
}

func TestBeatInRetake(t *testing.T) {
	descriptors := BuiltinWorkflowDescriptors()
	workflowsByID := WorkflowDescriptorByID(descriptors)

	t.Run("returns true for legacy retake states", func(t *testing.T) {
		for _, s := range []string{"retake", "retry", "rejected", "refining", "rework"} {
			beat := &backend.Beat{ID: "r", State: s, ProfileID: "autopilot"}
			if !BeatInRetake(beat, workflowsByID) {
				t.Errorf("expected true for state %q", s)
			}
		}
	})

	t.Run("returns false when workflow not found and not in legacy retake", func(t *testing.T) {
		beat := &backend.Beat{ID: "r2", State: "implementation", ProfileID: "nonexistent"}
		if BeatInRetake(beat, workflowsByID) {
			t.Error("expected false for missing workflow and non-retake state")
		}
	})
}

func TestWorkflowDescriptorByID(t *testing.T) {
	descriptors := BuiltinWorkflowDescriptors()
	m := WorkflowDescriptorByID(descriptors)

	t.Run("registers by id and backingId", func(t *testing.T) {
		if m["autopilot"] == nil {
			t.Error("expected autopilot to be registered")
		}
		if m["semiauto"] == nil {
			t.Error("expected semiauto to be registered")
		}
	})

	t.Run("registers legacy aliases", func(t *testing.T) {
		if m["beads-coarse"] != m["autopilot"] {
			t.Error("beads-coarse should map to autopilot")
		}
		if m["knots-granular"] != m["autopilot"] {
			t.Error("knots-granular should map to autopilot")
		}
		if m["knots-coarse"] != m["semiauto"] {
			t.Error("knots-coarse should map to semiauto")
		}
	})
}

func TestNormalizeProfileID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "autopilot"},
		{"autopilot", "autopilot"},
		{"beads-coarse", "autopilot"},
		{"beads-coarse-human-gated", "semiauto"},
		{"automatic", "autopilot"},
		{"workflow", "semiauto"},
		{"knots-granular", "autopilot"},
		{"knots-granular-autonomous", "autopilot"},
		{"knots-coarse", "semiauto"},
		{"knots-coarse-human-gated", "semiauto"},
		{"  Semiauto  ", "semiauto"},
	}
	for _, tt := range tests {
		got := NormalizeProfileID(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeProfileID(%q): got %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLabelHelpers(t *testing.T) {
	t.Run("IsWorkflowStateLabel", func(t *testing.T) {
		if !IsWorkflowStateLabel("wf:state:planning") {
			t.Error("expected true for state label")
		}
		if IsWorkflowStateLabel("wf:profile:autopilot") {
			t.Error("expected false for profile label")
		}
	})

	t.Run("IsWorkflowProfileLabel", func(t *testing.T) {
		if !IsWorkflowProfileLabel("wf:profile:autopilot") {
			t.Error("expected true for profile label")
		}
		if IsWorkflowProfileLabel("wf:state:planning") {
			t.Error("expected false for state label")
		}
	})

	t.Run("ExtractWorkflowStateLabel", func(t *testing.T) {
		if got := ExtractWorkflowStateLabel([]string{"wf:state:implementation"}); got != "implementation" {
			t.Errorf("expected implementation, got %q", got)
		}
		if got := ExtractWorkflowStateLabel([]string{"other-label"}); got != "" {
			t.Errorf("expected empty, got %q", got)
		}
		if got := ExtractWorkflowStateLabel([]string{"wf:state:", "wf:state:planning"}); got != "planning" {
			t.Errorf("expected planning (skip empty), got %q", got)
		}
	})

	t.Run("ExtractWorkflowProfileLabel", func(t *testing.T) {
		if got := ExtractWorkflowProfileLabel([]string{"wf:profile:semiauto"}); got != "semiauto" {
			t.Errorf("expected semiauto, got %q", got)
		}
		if got := ExtractWorkflowProfileLabel([]string{"wf:profile:beads-coarse"}); got != "autopilot" {
			t.Errorf("expected autopilot (normalized), got %q", got)
		}
		if got := ExtractWorkflowProfileLabel([]string{"wf:state:planning"}); got != "" {
			t.Errorf("expected empty for non-profile label, got %q", got)
		}
	})
}

func TestDeriveProfileID(t *testing.T) {
	t.Run("from metadata profileId", func(t *testing.T) {
		got := DeriveProfileID(nil, map[string]any{"profileId": "semiauto"})
		if got != "semiauto" {
			t.Errorf("expected semiauto, got %q", got)
		}
	})

	t.Run("prefers profileId over kernlProfileId", func(t *testing.T) {
		got := DeriveProfileID(nil, map[string]any{"profileId": "autopilot", "kernlProfileId": "semiauto"})
		if got != "autopilot" {
			t.Errorf("expected autopilot, got %q", got)
		}
	})

	t.Run("from labels", func(t *testing.T) {
		got := DeriveProfileID([]string{"wf:profile:semiauto"}, nil)
		if got != "semiauto" {
			t.Errorf("expected semiauto, got %q", got)
		}
	})

	t.Run("default", func(t *testing.T) {
		got := DeriveProfileID(nil, nil)
		if got != DefaultProfileID {
			t.Errorf("expected %q, got %q", DefaultProfileID, got)
		}
	})
}

func TestBuiltinProfileDescriptor(t *testing.T) {
	t.Run("autopilot", func(t *testing.T) {
		desc := BuiltinProfileDescriptor("autopilot")
		if desc.ID != "autopilot" {
			t.Errorf("expected autopilot, got %q", desc.ID)
		}
	})

	t.Run("no-planning starts at ready_for_implementation", func(t *testing.T) {
		desc := BuiltinProfileDescriptor("autopilot_no_planning")
		if desc.InitialState != "ready_for_implementation" {
			t.Errorf("expected ready_for_implementation, got %q", desc.InitialState)
		}
	})

	t.Run("fallback to autopilot for unknown profile", func(t *testing.T) {
		desc := BuiltinProfileDescriptor("completely-unknown-profile")
		if desc.ID != "autopilot" {
			t.Errorf("expected autopilot fallback, got %q", desc.ID)
		}
	})
}

func TestWorkflowActionStateForState(t *testing.T) {
	wf := autopilotWorkflow()

	t.Run("active state returns itself", func(t *testing.T) {
		got := WorkflowActionStateForState(&wf, "implementation")
		if got != "implementation" {
			t.Errorf("expected implementation, got %q", got)
		}
	})

	t.Run("queue state returns its action", func(t *testing.T) {
		got := WorkflowActionStateForState(&wf, "ready_for_implementation")
		if got != "implementation" {
			t.Errorf("expected implementation, got %q", got)
		}
	})
}

func TestWorkflowQueueStateForState(t *testing.T) {
	wf := autopilotWorkflow()

	t.Run("queue state returns itself", func(t *testing.T) {
		got := WorkflowQueueStateForState(&wf, "ready_for_implementation")
		if got != "ready_for_implementation" {
			t.Errorf("expected ready_for_implementation, got %q", got)
		}
	})

	t.Run("active state returns its queue", func(t *testing.T) {
		got := WorkflowQueueStateForState(&wf, "implementation")
		if got != "ready_for_implementation" {
			t.Errorf("expected ready_for_implementation, got %q", got)
		}
	})
}

func TestDeriveWorkflowStructure(t *testing.T) {
	owners := map[string]backend.ActionOwnerKind{
		"planning":                backend.ActionOwnerAgent,
		"plan_review":             backend.ActionOwnerAgent,
		"implementation":          backend.ActionOwnerAgent,
		"implementation_review":   backend.ActionOwnerAgent,
		"shipment":                backend.ActionOwnerAgent,
		"shipment_review":         backend.ActionOwnerAgent,
	}
	transitions := []backend.WorkflowTransition{
		{From: "ready_for_planning", To: "planning"},
		{From: "planning", To: "ready_for_plan_review"},
		{From: "ready_for_plan_review", To: "plan_review"},
		{From: "plan_review", To: "ready_for_implementation"},
		{From: "ready_for_implementation", To: "implementation"},
		{From: "implementation", To: "ready_for_implementation_review"},
		{From: "ready_for_implementation_review", To: "implementation_review"},
		{From: "implementation_review", To: "ready_for_shipment"},
		{From: "ready_for_shipment", To: "shipment"},
		{From: "shipment", To: "ready_for_shipment_review"},
		{From: "ready_for_shipment_review", To: "shipment_review"},
		{From: "shipment_review", To: "shipped"},
	}
	states := []string{
		"ready_for_planning", "planning",
		"ready_for_plan_review", "plan_review",
		"ready_for_implementation", "implementation",
		"ready_for_implementation_review", "implementation_review",
		"ready_for_shipment", "shipment",
		"ready_for_shipment_review", "shipment_review",
		"shipped", "abandoned",
	}
	terminalStates := []string{"shipped", "abandoned"}

	queueStates, actionStates, queueActions := DeriveWorkflowStructure(states, transitions, owners, terminalStates)

	expectedQueue := []string{
		"ready_for_planning", "ready_for_plan_review",
		"ready_for_implementation", "ready_for_implementation_review",
		"ready_for_shipment", "ready_for_shipment_review",
	}
	if len(queueStates) != len(expectedQueue) {
		t.Errorf("expected %d queue states, got %d: %v", len(expectedQueue), len(queueStates), queueStates)
	}
	queueSet := make(map[string]bool, len(queueStates))
	for _, q := range queueStates {
		queueSet[q] = true
	}
	for _, q := range expectedQueue {
		if !queueSet[q] {
			t.Errorf("expected queue state %q not found", q)
		}
	}

	expectedAction := []string{
		"planning", "plan_review", "implementation",
		"implementation_review", "shipment", "shipment_review",
	}
	if len(actionStates) != len(expectedAction) {
		t.Errorf("expected %d action states, got %d: %v", len(expectedAction), len(actionStates), actionStates)
	}
	actionSet := make(map[string]bool, len(actionStates))
	for _, a := range actionStates {
		actionSet[a] = true
	}
	for _, a := range expectedAction {
		if !actionSet[a] {
			t.Errorf("expected action state %q not found", a)
		}
	}

	expectedQueueActions := map[string]string{
		"ready_for_planning":              "planning",
		"ready_for_plan_review":            "plan_review",
		"ready_for_implementation":        "implementation",
		"ready_for_implementation_review": "implementation_review",
		"ready_for_shipment":              "shipment",
		"ready_for_shipment_review":      "shipment_review",
	}
	for k, v := range expectedQueueActions {
		if queueActions[k] != v {
			t.Errorf("queueActions[%q]: expected %q, got %q", k, v, queueActions[k])
		}
	}
}

func TestWorkflowStatePhase(t *testing.T) {
	wf := autopilotWorkflow()

	t.Run("terminal state returns terminal phase", func(t *testing.T) {
		if WorkflowStatePhase(&wf, "shipped") != PhaseTerminal {
			t.Error("expected PhaseTerminal for shipped")
		}
		if WorkflowStatePhase(&wf, "abandoned") != PhaseTerminal {
			t.Error("expected PhaseTerminal for abandoned")
		}
	})

	t.Run("deferred returns terminal", func(t *testing.T) {
		if WorkflowStatePhase(&wf, "deferred") != PhaseTerminal {
			t.Error("expected PhaseTerminal for deferred")
		}
	})

	t.Run("queue state returns queued phase", func(t *testing.T) {
		queueStates := []string{"ready_for_planning", "ready_for_implementation", "ready_for_shipment"}
		for _, s := range queueStates {
			if WorkflowStatePhase(&wf, s) != PhaseQueued {
				t.Errorf("expected PhaseQueued for %q, got %q", s, WorkflowStatePhase(&wf, s))
			}
		}
	})

	t.Run("active state returns active phase", func(t *testing.T) {
		activeStates := []string{"planning", "implementation", "shipment"}
		for _, s := range activeStates {
			if WorkflowStatePhase(&wf, s) != PhaseActive {
				t.Errorf("expected PhaseActive for %q, got %q", s, WorkflowStatePhase(&wf, s))
			}
		}
	})

	t.Run("empty state defaults to queued", func(t *testing.T) {
		if WorkflowStatePhase(&wf, "") != PhaseQueued {
			t.Error("expected PhaseQueued for empty state")
		}
	})

	t.Run("unknown state defaults to queued", func(t *testing.T) {
		if WorkflowStatePhase(&wf, "totally_unknown_state") != PhaseQueued {
			t.Error("expected PhaseQueued for unknown state")
		}
	})
}

func TestWfToSteps(t *testing.T) {
	wf := DefaultWorkflowDescriptor()
	steps := WfToSteps(&wf)

	if len(steps) == 0 {
		t.Fatal("expected non-empty steps")
	}

	stepSet := make(map[string]bool, len(steps))
	for _, s := range steps {
		stepSet[s.ID] = true
	}

	t.Run("every step maps through queued phase except terminals", func(t *testing.T) {
		terminalSet := make(map[string]bool, len(wf.TerminalStates))
		for _, s := range wf.TerminalStates {
			terminalSet[s] = true
		}
		for _, s := range steps {
			if terminalSet[s.ID] {
				rs := ResolveStepForWorkflow(s.ID, &wf)
				if rs != nil {
					t.Errorf("terminal step %q: expected nil from ResolveStep, got %+v", s.ID, rs)
				}
				continue
			}
			if s.ID == "deferred" {
				rs := ResolveStepForWorkflow(s.ID, &wf)
				if rs != nil {
					t.Errorf("deferred step %q: expected nil from ResolveStep, got %+v", s.ID, rs)
				}
				continue
			}
			rs := ResolveStepForWorkflow(s.ID, &wf)
			if rs == nil {
				queueState := WorkflowQueueStateForState(&wf, s.ID)
				if queueState != "" {
					rs2 := ResolveStepForWorkflow(queueState, &wf)
					if rs2 == nil {
						t.Errorf("non-terminal step %q: queue state %q also not resolvable", s.ID, queueState)
					}
				} else {
					t.Errorf("non-terminal step %q: not resolvable and no queue state", s.ID)
				}
			}
		}
	})

	t.Run("every step has kind from owners", func(t *testing.T) {
		for _, s := range steps {
			if s.Kind == "" {
				t.Errorf("step %q has empty kind", s.ID)
			}
		}
	})

	t.Run("nil workflow returns nil", func(t *testing.T) {
		if WfToSteps(nil) != nil {
			t.Error("expected nil for nil workflow")
		}
	})
}

func TestResolveWorkflowForBeat(t *testing.T) {
	descriptors := BuiltinWorkflowDescriptors()
	workflowsByID := WorkflowDescriptorByID(descriptors)

	t.Run("resolves by profileId", func(t *testing.T) {
		beat := &backend.Beat{ID: "1", ProfileID: "autopilot"}
		wf := ResolveWorkflowForBeat(beat, workflowsByID)
		if wf == nil || wf.ID != "autopilot" {
			t.Errorf("expected autopilot, got %v", wf)
		}
	})

	t.Run("prefers profileId over workflowId", func(t *testing.T) {
		beat := &backend.Beat{ID: "3", ProfileID: "autopilot", WorkflowID: "semiauto"}
		wf := ResolveWorkflowForBeat(beat, workflowsByID)
		if wf == nil || wf.ID != "autopilot" {
			t.Errorf("expected autopilot (profileId wins), got %v", wf)
		}
	})

	t.Run("uses workflowId when profileId missing", func(t *testing.T) {
		beat := &backend.Beat{ID: "2", ProfileID: "nonexistent_profile", WorkflowID: "semiauto"}
		wf := ResolveWorkflowForBeat(beat, workflowsByID)
		if wf == nil || wf.ID != "semiauto" {
			t.Errorf("expected semiauto via workflowId, got %v", wf)
		}
	})

	t.Run("returns nil for unknown workflow", func(t *testing.T) {
		beat := &backend.Beat{ID: "4", ProfileID: "nonexistent"}
		wf := ResolveWorkflowForBeat(beat, workflowsByID)
		if wf != nil {
			t.Errorf("expected nil for unknown workflow, got %v", wf)
		}
	})

	t.Run("uses fallback when provided", func(t *testing.T) {
		beat := &backend.Beat{ID: "5", ProfileID: "nonexistent"}
		fallback := DefaultWorkflowDescriptor()
		wf := ResolveWorkflowForBeat(beat, workflowsByID, &fallback)
		if wf == nil || wf.ID != "autopilot" {
			t.Errorf("expected fallback autopilot, got %v", wf)
		}
	})

	t.Run("resolves legacy profile IDs", func(t *testing.T) {
		beat := &backend.Beat{ID: "6", ProfileID: "beads-coarse"}
		wf := ResolveWorkflowForBeat(beat, workflowsByID)
		if wf == nil || wf.ID != "autopilot" {
			t.Errorf("expected autopilot for legacy beads-coarse, got %v", wf)
		}
	})
}