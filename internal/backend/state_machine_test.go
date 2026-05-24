package backend

import (
	"fmt"
	"strings"
	"testing"
)

type stateMachineBackend struct {
	beads   map[string]*Bead
	updates map[string]UpdateBeadInput
}

func newStateMachineBackend() *stateMachineBackend {
	return &stateMachineBackend{
		beads:   make(map[string]*Bead),
		updates: make(map[string]UpdateBeadInput),
	}
}

func (s *stateMachineBackend) seedBead(id, state, profileID string) {
	s.beads[id] = &Bead{ID: id, State: state, ProfileID: profileID}
}

func (s *stateMachineBackend) ListWorkflows(repoPath string) ([]WorkflowDescriptor, error) {
	return BuiltinWorkflowDescriptors(), nil
}
func (s *stateMachineBackend) List(filters *BeadListFilters, repoPath string) ([]Bead, error) {
	return nil, nil
}
func (s *stateMachineBackend) ListReady(filters *BeadListFilters, repoPath string) ([]Bead, error) {
	return nil, nil
}
func (s *stateMachineBackend) Get(id string, repoPath string) (*Bead, error) {
	b, ok := s.beads[id]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
	}
	return b, nil
}
func (s *stateMachineBackend) Create(input CreateBeadInput, repoPath string) (*Bead, error) {
	return nil, nil
}
func (s *stateMachineBackend) Update(id string, input UpdateBeadInput, repoPath string) error {
	s.updates[id] = input
	if b, ok := s.beads[id]; ok {
		if input.State != "" {
			b.State = input.State
		}
	}
	return nil
}
func (s *stateMachineBackend) Delete(id string, repoPath string) error { return nil }
func (s *stateMachineBackend) Close(id string, reason string, repoPath string) (*TerminalState, error) {
	return nil, nil
}
func (s *stateMachineBackend) MarkTerminal(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (s *stateMachineBackend) Reopen(id string, reason string, repoPath string) error { return nil }
func (s *stateMachineBackend) Rewind(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (s *stateMachineBackend) Search(query string, filters *BeadListFilters, repoPath string) ([]Bead, error) {
	return nil, nil
}
func (s *stateMachineBackend) Query(expression string, options *BeadQueryOptions, repoPath string) ([]Bead, error) {
	return nil, nil
}
func (s *stateMachineBackend) AddDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (s *stateMachineBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (s *stateMachineBackend) ListDependencies(id string, repoPath string, options *DependencyListOptions) ([]BeadDependency, error) {
	return nil, nil
}
func (s *stateMachineBackend) BuildTakePrompt(beadID string, options *TakePromptOptions, repoPath string) (*TakePromptResult, error) {
	return nil, nil
}
func (s *stateMachineBackend) BuildPollPrompt(options *PollPromptOptions, repoPath string) (*PollPromptResult, error) {
	return nil, nil
}
func (s *stateMachineBackend) Comment(id string, body string, repoPath string) error { return nil }
func (s *stateMachineBackend) Capabilities() BackendCapabilities { return FullCapabilities }

func isExpectedStateMismatchError(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "expected state") && strings.Contains(lower, "currently")
}

func TestNextBead_AdvancesFromCurrentState(t *testing.T) {
	backend := newStateMachineBackend()
	backend.seedBead("bead-1", "implementation", "autopilot")
	result, err := NextBead(backend, "bead-1", "implementation", "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NextState != "ready_for_implementation_review" {
		t.Errorf("expected next state 'ready_for_implementation_review', got %q", result.NextState)
	}
}

func TestNextBead_ThrowsStateMismatch(t *testing.T) {
	backend := newStateMachineBackend()
	backend.seedBead("bead-1", "implementation", "autopilot")
	_, err := NextBead(backend, "bead-1", "planning", "/repo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isExpectedStateMismatchError(err.Error()) {
		t.Errorf("expected state mismatch error, got: %v", err)
	}
}

func TestNextBead_PersistsNewState(t *testing.T) {
	backend := newStateMachineBackend()
	backend.seedBead("bead-1", "planning", "autopilot")
	_, err := NextBead(backend, "bead-1", "planning", "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend.beads["bead-1"].State != "ready_for_plan_review" {
		t.Errorf("expected state 'ready_for_plan_review', got %q", backend.beads["bead-1"].State)
	}
}

func TestNextBead_ThrowsWhenBeadNotFound(t *testing.T) {
	backend := newStateMachineBackend()
	_, err := NextBead(backend, "nonexistent", "planning", "/repo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestNextBead_ThrowsTerminalStateNoForwardTransition(t *testing.T) {
	backend := newStateMachineBackend()
	backend.seedBead("bead-1", "shipped", "autopilot")
	_, err := NextBead(backend, "bead-1", "shipped", "/repo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no forward transition") && !strings.Contains(err.Error(), "terminal") {
		t.Errorf("expected no forward transition / terminal error, got: %v", err)
	}
}

func TestNextBead_AdvancesFromQueuedToActive(t *testing.T) {
	backend := newStateMachineBackend()
	backend.seedBead("bead-1", "ready_for_implementation", "autopilot")
	result, err := NextBead(backend, "bead-1", "ready_for_implementation", "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NextState != "implementation" {
		t.Errorf("expected next state 'implementation', got %q", result.NextState)
	}
}

func TestNextBead_MismatchErrorCompatible(t *testing.T) {
	backend := newStateMachineBackend()
	backend.seedBead("bead-1", "implementation", "autopilot")
	_, err := NextBead(backend, "bead-1", "shipment", "/repo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "expected state") {
		t.Errorf("expected 'expected state' in error, got: %s", msg)
	}
	if !strings.Contains(msg, "currently") {
		t.Errorf("expected 'currently' in error, got: %s", msg)
	}
}

func TestClaimBead_TransitionsQueuedToActive(t *testing.T) {
	backend := newStateMachineBackend()
	backend.seedBead("bead-1", "ready_for_implementation", "autopilot")
	result, err := ClaimBead(backend, "bead-1", "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NextState != "implementation" {
		t.Errorf("expected next state 'implementation', got %q", result.NextState)
	}
}

func TestClaimBead_PersistsActiveState(t *testing.T) {
	backend := newStateMachineBackend()
	backend.seedBead("bead-1", "ready_for_planning", "autopilot")
	_, err := ClaimBead(backend, "bead-1", "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if backend.beads["bead-1"].State != "planning" {
		t.Errorf("expected state 'planning', got %q", backend.beads["bead-1"].State)
	}
}

func TestClaimBead_ThrowsWhenAlreadyActive(t *testing.T) {
	backend := newStateMachineBackend()
	backend.seedBead("bead-1", "implementation", "autopilot")
	_, err := ClaimBead(backend, "bead-1", "/repo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isExpectedStateMismatchError(err.Error()) {
		t.Errorf("expected state mismatch error, got: %v", err)
	}
}

func TestClaimBead_ThrowsWhenTerminal(t *testing.T) {
	backend := newStateMachineBackend()
	backend.seedBead("bead-1", "shipped", "autopilot")
	_, err := ClaimBead(backend, "bead-1", "/repo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isExpectedStateMismatchError(err.Error()) {
		t.Errorf("expected state mismatch error, got: %v", err)
	}
}

func TestClaimBead_ThrowsWhenBeadNotFound(t *testing.T) {
	backend := newStateMachineBackend()
	_, err := ClaimBead(backend, "nonexistent", "/repo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestClaimBead_ThrowsWhenHumanOwnedQueue(t *testing.T) {
	backend := newStateMachineBackend()
	backend.seedBead("bead-1", "ready_for_plan_review", "semiauto")
	_, err := ClaimBead(backend, "bead-1", "/repo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !isExpectedStateMismatchError(err.Error()) {
		t.Errorf("expected state mismatch error, got: %v", err)
	}
}

func TestClaimBead_ClaimsShipmentQueue(t *testing.T) {
	backend := newStateMachineBackend()
	backend.seedBead("bead-1", "ready_for_shipment", "autopilot")
	result, err := ClaimBead(backend, "bead-1", "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NextState != "shipment" {
		t.Errorf("expected next state 'shipment', got %q", result.NextState)
	}
}

func TestBuiltinProfileDescriptor_Autopilot(t *testing.T) {
	wf := BuiltinProfileDescriptor("autopilot")
	if wf.ID != "autopilot" {
		t.Errorf("expected ID autopilot, got %s", wf.ID)
	}
	if wf.InitialState != "ready_for_planning" {
		t.Errorf("expected initialState ready_for_planning, got %s", wf.InitialState)
	}
}

func TestBuiltinProfileDescriptor_Semiauto(t *testing.T) {
	wf := BuiltinProfileDescriptor("semiauto")
	if wf.ID != "semiauto" {
		t.Errorf("expected ID semiauto, got %s", wf.ID)
	}
	found := false
	for _, s := range wf.States {
		if s == "ready_for_plan_review" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected semiauto to include ready_for_plan_review state")
	}
}

func TestBuiltinProfileDescriptor_NoPlanning(t *testing.T) {
	wf := BuiltinProfileDescriptor("autopilot_no_planning")
	if wf.ID != "autopilot_no_planning" {
		t.Errorf("expected ID autopilot_no_planning, got %s", wf.ID)
	}
	for _, s := range wf.States {
		if s == "planning" {
			t.Error("autopilot_no_planning should not include planning state")
		}
	}
	if wf.InitialState != "ready_for_implementation" {
		t.Errorf("expected initialState ready_for_implementation, got %s", wf.InitialState)
	}
}

func TestBuiltinProfileDescriptor_DefaultFallback(t *testing.T) {
	wf := BuiltinProfileDescriptor("unknown_profile")
	if wf.ID != "autopilot" {
		t.Errorf("expected fallback to autopilot, got %s", wf.ID)
	}
}

func TestBuiltinProfileDescriptor_Normalization(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"beads-coarse", "autopilot"},
		{"automatic", "autopilot"},
		{"workflow", "semiauto"},
		{"knots-granular", "autopilot"},
		{"knots-coarse", "semiauto"},
		{"", "autopilot"},
	}
	for _, tt := range tests {
		wf := BuiltinProfileDescriptor(tt.input)
		if wf.ID != tt.want {
			t.Errorf("BuiltinProfileDescriptor(%q): expected %s, got %s", tt.input, tt.want, wf.ID)
		}
	}
}

func TestForwardTransitionTarget_SDLCTransitions(t *testing.T) {
	wf := BuiltinWorkflowDescriptors()[0]
	tests := []struct {
		from string
		want string
		ok   bool
	}{
		{"ready_for_implementation", "implementation", true},
		{"implementation", "ready_for_review", true},
		{"ready_for_review", "review", true},
		{"review", "ready_for_shipment", true},
		{"ready_for_shipment", "shipment", true},
		{"shipment", "shipped", true},
		{"shipped", "", false},
	}
	for _, tt := range tests {
		got, ok := ForwardTransitionTarget(tt.from, wf)
		if ok != tt.ok {
			t.Errorf("ForwardTransitionTarget(%q): got ok=%v, want ok=%v", tt.from, ok, tt.ok)
		}
		if got != tt.want {
			t.Errorf("ForwardTransitionTarget(%q): got %q, want %q", tt.from, got, tt.want)
		}
	}
}

func TestForwardTransitionTarget_AutopilotTransitions(t *testing.T) {
	wf := BuiltinProfileDescriptor("autopilot")
	tests := []struct {
		from string
		want string
		ok   bool
	}{
		{"ready_for_planning", "planning", true},
		{"planning", "ready_for_plan_review", true},
		{"ready_for_plan_review", "plan_review", true},
		{"plan_review", "ready_for_implementation", true},
		{"ready_for_implementation", "implementation", true},
		{"implementation", "ready_for_implementation_review", true},
		{"ready_for_implementation_review", "implementation_review", true},
		{"implementation_review", "ready_for_integration", true},
		{"ready_for_shipment", "shipment", true},
		{"shipment", "ready_for_shipment_review", true},
		{"ready_for_shipment_review", "shipment_review", true},
		{"shipment_review", "shipped", true},
	}
	for _, tt := range tests {
		got, ok := ForwardTransitionTarget(tt.from, wf)
		if ok != tt.ok {
			t.Errorf("ForwardTransitionTarget(%q): got ok=%v, want ok=%v", tt.from, ok, tt.ok)
		}
		if got != tt.want {
			t.Errorf("ForwardTransitionTarget(%q): got %q, want %q", tt.from, got, tt.want)
		}
	}
}

func TestResolveStepForWorkflow_ActiveState(t *testing.T) {
	wf := BuiltinProfileDescriptor("autopilot")
	step, err := ResolveStepForWorkflow("implementation", wf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if step.Step != "implementation" {
		t.Errorf("expected step 'implementation', got %q", step.Step)
	}
	if step.Phase != StepPhaseActive {
		t.Errorf("expected phase 'active', got %q", step.Phase)
	}
}

func TestResolveStepForWorkflow_QueuedState(t *testing.T) {
	wf := BuiltinProfileDescriptor("autopilot")
	step, err := ResolveStepForWorkflow("ready_for_implementation", wf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if step.Step != "implementation" {
		t.Errorf("expected step 'implementation', got %q", step.Step)
	}
	if step.Phase != StepPhaseQueued {
		t.Errorf("expected phase 'queued', got %q", step.Phase)
	}
}

func TestResolveStepForWorkflow_TerminalStateFails(t *testing.T) {
	wf := BuiltinProfileDescriptor("autopilot")
	_, err := ResolveStepForWorkflow("shipped", wf)
	if err == nil {
		t.Error("expected error for terminal state, got nil")
	}
}

func TestDeriveWorkflowRuntimeState_AgentClaimable(t *testing.T) {
	wf := BuiltinProfileDescriptor("autopilot")
	rt := DeriveWorkflowRuntimeState(wf, "ready_for_implementation")
	if !rt.IsAgentClaimable {
		t.Error("expected ready_for_implementation to be agent-claimable in autopilot")
	}
	if rt.RequiresHumanAction {
		t.Error("expected ready_for_implementation NOT to require human action in autopilot")
	}
}

func TestDeriveWorkflowRuntimeState_HumanOwned(t *testing.T) {
	wf := BuiltinProfileDescriptor("semiauto")
	rt := DeriveWorkflowRuntimeState(wf, "ready_for_plan_review")
	if rt.IsAgentClaimable {
		t.Error("expected ready_for_plan_review NOT to be agent-claimable in semiauto")
	}
	if !rt.RequiresHumanAction {
		t.Error("expected ready_for_plan_review to require human action in semiauto")
	}
}

func TestDeriveWorkflowRuntimeState_ActiveState(t *testing.T) {
	wf := BuiltinProfileDescriptor("autopilot")
	rt := DeriveWorkflowRuntimeState(wf, "implementation")
	if rt.IsAgentClaimable {
		t.Error("expected implementation NOT to be agent-claimable (it's active)")
	}
	if rt.NextActionState != "implementation" {
		t.Errorf("expected nextActionState 'implementation', got %q", rt.NextActionState)
	}
}

func TestClaimBead_SemiautoPlanReview_Throws(t *testing.T) {
	backend := newStateMachineBackend()
	backend.seedBead("bead-1", "ready_for_plan_review", "semiauto")
	_, err := ClaimBead(backend, "bead-1", "/repo")
	if err == nil {
		t.Fatal("expected error for human-owned queue state, got nil")
	}
	if !isExpectedStateMismatchError(err.Error()) {
		if !strings.Contains(err.Error(), "not claimable") {
			t.Errorf("expected state mismatch or not-claimable error, got: %v", err)
		}
	}
}

func TestClaimBead_SemiautoImplementation_Claims(t *testing.T) {
	backend := newStateMachineBackend()
	backend.seedBead("bead-1", "ready_for_implementation", "semiauto")
	result, err := ClaimBead(backend, "bead-1", "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NextState != "implementation" {
		t.Errorf("expected 'implementation', got %q", result.NextState)
	}
}

func TestIsTerminalState(t *testing.T) {
	wf := BuiltinProfileDescriptor("autopilot")
	tests := []struct {
		state string
		want  bool
	}{
		{"shipped", true},
		{"abandoned", true},
		{"deferred", true},
		{"implementation", false},
		{"ready_for_implementation", false},
	}
	for _, tt := range tests {
		got := isTerminalState(tt.state, wf)
		if got != tt.want {
			t.Errorf("isTerminalState(%q): got %v, want %v", tt.state, got, tt.want)
		}
	}
}

func TestCustomWorkflowRegistry(t *testing.T) {
	// Clean up after test
	defer ClearWorkflowRegistry()

	t.Run("Consult registry first and fallback", func(t *testing.T) {
		ClearWorkflowRegistry()
		// Case 1: no custom workflow -> returns built-in autopilot
		bead := &Bead{ProfileID: "my-custom-wf"}
		desc := ResolveWorkflow(bead)
		if desc.ID != "autopilot" {
			t.Errorf("expected fallback to autopilot, got ID: %s", desc.ID)
		}

		// Case 2: register custom workflow with matching ID
		customDesc := WorkflowDescriptor{
			ID: "My-Custom-WF",
			Label: "My Custom Workflow Label",
		}
		RegisterWorkflow(customDesc)

		desc2 := ResolveWorkflow(bead)
		if desc2.ID != "My-Custom-WF" || desc2.Label != "My Custom Workflow Label" {
			t.Errorf("expected resolved custom workflow, got ID: %s, Label: %s", desc2.ID, desc2.Label)
		}
	})

	t.Run("Mixed case and legacy alias normalization", func(t *testing.T) {
		ClearWorkflowRegistry()
		// Beads-coarse is normalized to autopilot.
		// Let's register a custom descriptor with ID "autopilot" (which "beads-coarse" resolves to)
		customDesc := WorkflowDescriptor{
			ID: "Autopilot",
			Label: "Custom Autopilot Override",
		}
		RegisterWorkflow(customDesc)

		bead := &Bead{ProfileID: "beads-coarse"}
		desc := ResolveWorkflow(bead)
		if desc.Label != "Custom Autopilot Override" {
			t.Errorf("expected custom autopilot override to win, got Label: %s", desc.Label)
		}
	})
}