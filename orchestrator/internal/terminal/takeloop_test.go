package terminal

import (
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/orchestration"
)

func makeTestWorkflow() *backend.WorkflowDescriptor {
	return &backend.WorkflowDescriptor{
		ID:             "wf-1",
		States:         []string{"ready_for_implementation", "implementation", "ready_for_review", "review", "shipped"},
		TerminalStates: []string{"shipped", "abandoned"},
		ActionStates:   []string{"implementation", "review"},
		QueueStates:    []string{"ready_for_implementation", "ready_for_review"},
		QueueActions:   map[string]string{"ready_for_implementation": "implementation", "ready_for_review": "review"},
		Transitions: []backend.WorkflowTransition{
			{From: "ready_for_implementation", To: "implementation"},
			{From: "implementation", To: "ready_for_review"},
			{From: "implementation", To: "implementation"},
			{From: "ready_for_review", To: "review"},
			{From: "review", To: "ready_for_implementation"},
			{From: "review", To: "shipped"},
		},
		StateOwners: map[string]backend.ActionOwnerKind{
			"ready_for_implementation": backend.ActionOwnerAgent,
			"implementation":           backend.ActionOwnerAgent,
			"ready_for_review":         backend.ActionOwnerAgent,
			"review":                   backend.ActionOwnerAgent,
		},
	}
}

func TestClassifyIterationSuccess_AdvancesToNextQueue(t *testing.T) {
	wf := makeTestWorkflow()
	got := ClassifyIterationSuccess(0, "ready_for_implementation", "ready_for_review", wf)
	if !got {
		t.Error("expected success when beat advances from queue to next queue via action")
	}
}

func TestClassifyIterationSuccess_NonZeroExitCode(t *testing.T) {
	wf := makeTestWorkflow()
	got := ClassifyIterationSuccess(1, "ready_for_implementation", "implementation", wf)
	if got {
		t.Error("expected failure for non-zero exit code")
	}
}

func TestClassifyIterationSuccess_UnknownPostExitState(t *testing.T) {
	wf := makeTestWorkflow()
	got := ClassifyIterationSuccess(0, "ready_for_implementation", "unknown", wf)
	if got {
		t.Error("expected failure for unknown post-exit state")
	}
}

func TestClassifyIterationSuccess_ReviewReturnsToPriorQueue(t *testing.T) {
	wf := makeTestWorkflow()
	got := ClassifyIterationSuccess(0, "ready_for_review", "ready_for_implementation", wf)
	if !got {
		t.Error("expected success when review returns to prior queue (rejection)")
	}
}

func TestClassifyIterationSuccess_StuckInActiveState(t *testing.T) {
	wf := makeTestWorkflow()
	got := ClassifyIterationSuccess(0, "implementation", "implementation", wf)
	if got {
		t.Error("expected failure when beat stays in same active state")
	}
}

func TestClassifyIterationSuccess_AdvancesToTerminal(t *testing.T) {
	wf := makeTestWorkflow()
	got := ClassifyIterationSuccess(0, "review", "shipped", wf)
	if !got {
		t.Error("expected success when review results in shipped")
	}
}

func TestTakeLoopContext_RecordFailedAgent(t *testing.T) {
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "test"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := NewTakeLoopContext(entry, &backend.Beat{ID: "beat-1"}, "/repo")

	ctx.RecordFailedAgent("implementation", "agent-a")
	ctx.RecordFailedAgent("implementation", "agent-b")

	if len(ctx.FailedAgentsPerQueueType["implementation"]) != 2 {
		t.Errorf("expected 2 failed agents, got %d", len(ctx.FailedAgentsPerQueueType["implementation"]))
	}
	if !ctx.FailedAgentsPerQueueType["implementation"]["agent-a"] {
		t.Error("expected agent-a in failed set")
	}
	if !ctx.FailedAgentsPerQueueType["implementation"]["agent-b"] {
		t.Error("expected agent-b in failed set")
	}
}

func TestTakeLoopContext_RecordFailedAgent_EmptyArgs(t *testing.T) {
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "test"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := NewTakeLoopContext(entry, &backend.Beat{ID: "beat-1"}, "/repo")

	ctx.RecordFailedAgent("", "agent-a")
	ctx.RecordFailedAgent("implementation", "")

	if len(ctx.FailedAgentsPerQueueType) != 0 {
		t.Errorf("expected 0 entries for empty args, got %d", len(ctx.FailedAgentsPerQueueType))
	}
}

func TestTakeLoopContext_IncrementClaims(t *testing.T) {
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "test"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := NewTakeLoopContext(entry, &backend.Beat{ID: "beat-1"}, "/repo")

	c1 := ctx.IncrementClaims("implementation")
	if c1 != 1 {
		t.Errorf("expected first claim count 1, got %d", c1)
	}

	c2 := ctx.IncrementClaims("implementation")
	if c2 != 2 {
		t.Errorf("expected second claim count 2, got %d", c2)
	}

	c3 := ctx.GetClaimCount("implementation")
	if c3 != 2 {
		t.Errorf("expected get count 2, got %d", c3)
	}
}

func TestTakeLoopContext_ComputeExclusions_FailedAgents(t *testing.T) {
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "test"},
		FailedAgentsPerQueueType: map[string]map[string]bool{
			"implementation": {"agent-a": true},
		},
		ClaimsPerQueueType:    make(map[string]int),
		LastAgentPerQueueType: make(map[string]string),
	}
	ctx := NewTakeLoopContext(entry, &backend.Beat{ID: "beat-1"}, "/repo")

	exclusions := ctx.ComputeExclusions("implementation", false, "", "", "")
	if len(exclusions) != 1 || exclusions[0] != "agent-a" {
		t.Errorf("expected [agent-a] in exclusions, got %v", exclusions)
	}
}

func TestTakeLoopContext_ComputeExclusions_ReviewExclusion(t *testing.T) {
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "test"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := NewTakeLoopContext(entry, &backend.Beat{ID: "beat-1"}, "/repo")

	exclusions := ctx.ComputeExclusions("review", true, "current-agent", "prior-action-agent", "")
	foundCurrent := false
	foundPrior := false
	for _, id := range exclusions {
		if id == "current-agent" {
			foundCurrent = true
		}
		if id == "prior-action-agent" {
			foundPrior = true
		}
	}
	if !foundCurrent {
		t.Error("expected current agent in exclusions for review")
	}
	if !foundPrior {
		t.Error("expected prior action agent in exclusions for review")
	}
}

func TestTakeLoopContext_ComputeExclusions_SoftRotation(t *testing.T) {
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "test"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    map[string]string{"implementation": "last-agent"},
	}
	ctx := NewTakeLoopContext(entry, &backend.Beat{ID: "beat-1"}, "/repo")

	exclusions := ctx.ComputeExclusions("implementation", false, "", "", "")
	found := false
	for _, id := range exclusions {
		if id == "last-agent" {
			found = true
		}
	}
	if !found {
		t.Error("expected last-agent in exclusions for soft rotation")
	}
}

func TestTakeLoopContext_ComputeExclusions_ErrorAgentExcluded(t *testing.T) {
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "test"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := NewTakeLoopContext(entry, &backend.Beat{ID: "beat-1"}, "/repo")

	exclusions := ctx.ComputeExclusions("implementation", false, "", "", "error-agent")
	found := false
	for _, id := range exclusions {
		if id == "error-agent" {
			found = true
		}
	}
	if !found {
		t.Error("expected error-agent in exclusions")
	}
}

func TestBuildOutcomeRecord(t *testing.T) {
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "sess-1"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	now := time.Now()
	ctx := &TakeLoopContext{
		ID:            "sess-1",
		BeatID:        "beat-1",
		Entry:         entry,
		TakeIteration: &IterationCounter{Value: 2},
		ClaimedAt:     &now,
	}

	record := BuildOutcomeRecord(ctx, "agent-a", "Claude", "ready_for_implementation", "implementation", 0, "implementation", true, true)

	if record.BeatID != "beat-1" {
		t.Errorf("expected beatId=beat-1, got %s", record.BeatID)
	}
	if record.Success != true {
		t.Error("expected success=true")
	}
	if record.ExitCode != 0 {
		t.Errorf("expected exitCode=0, got %d", record.ExitCode)
	}
	if record.ClaimedState != "ready_for_implementation" {
		t.Errorf("expected claimedState=ready_for_implementation, got %s", record.ClaimedState)
	}
	if record.PostExitState != "implementation" {
		t.Errorf("expected postExitState=implementation, got %s", record.PostExitState)
	}
	if record.AgentID != "agent-a" {
		t.Errorf("expected agentId=agent-a, got %s", record.AgentID)
	}
	if record.Iteration != 2 {
		t.Errorf("expected iteration=2, got %d", record.Iteration)
	}
}

func TestWfToSteps(t *testing.T) {
	wf := makeTestWorkflow()
	steps := orchestration.WfToSteps(wf)
	if len(steps) != 5 {
		t.Errorf("expected 5 steps, got %d", len(steps))
	}

	found := false
	for _, s := range steps {
		if s.ID == "implementation" && s.Kind == "agent" {
			found = true
		}
	}
	if !found {
		t.Error("expected implementation step with kind=agent")
	}
}

func TestClassifyIterationSuccess_TerminalFromReview(t *testing.T) {
	wf := &backend.WorkflowDescriptor{
		ID:             "wf-review",
		States:         []string{"ready_for_review", "review", "shipped"},
		TerminalStates: []string{"shipped"},
		ActionStates:   []string{"review"},
		QueueStates:    []string{"ready_for_review"},
		Transitions: []backend.WorkflowTransition{
			{From: "ready_for_review", To: "review"},
			{From: "review", To: "shipped"},
		},
		StateOwners: map[string]backend.ActionOwnerKind{
			"review": backend.ActionOwnerAgent,
		},
	}

	got := ClassifyIterationSuccess(0, "review", "shipped", wf)
	if !got {
		t.Error("expected success when review goes to shipped (terminal)")
	}
}

func TestExpectedSuccessTargets(t *testing.T) {
	wf := &backend.WorkflowDescriptor{
		ID:             "wf-1",
		States:         []string{"ready_for_implementation", "implementation", "ready_for_review"},
		TerminalStates: []string{},
		ActionStates:   []string{"implementation"},
		QueueStates:    []string{"ready_for_implementation", "ready_for_review"},
		Transitions: []backend.WorkflowTransition{
			{From: "ready_for_implementation", To: "implementation"},
			{From: "implementation", To: "ready_for_review"},
			{From: "implementation", To: "implementation"},
		},
		StateOwners: map[string]backend.ActionOwnerKind{
			"implementation": backend.ActionOwnerAgent,
		},
		QueueActions: map[string]string{"ready_for_implementation": "implementation"},
	}

	targets := ExpectedSuccessTargets(wf, "ready_for_implementation")
	found := false
	for _, tgt := range targets {
		if tgt == "ready_for_review" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'ready_for_review' in success targets, got %v", targets)
	}
}

func TestClassifyIterationSuccess_StaysSameState(t *testing.T) {
	wf := makeTestWorkflow()
	got := ClassifyIterationSuccess(0, "ready_for_implementation", "ready_for_implementation", wf)
	if got {
		t.Error("expected failure when beat stays at same queue state without advancing")
	}
}

func TestNextQueueStateFromState(t *testing.T) {
	wf := makeTestWorkflow()
	got := NextQueueStateFromState(wf, "implementation")
	if got != "ready_for_review" {
		t.Errorf("expected ready_for_review, got %s", got)
	}
}

func TestPriorQueueStateFromState(t *testing.T) {
	wf := makeTestWorkflow()
	got := PriorQueueStateFromState(wf, "implementation")
	if got != "ready_for_implementation" {
		t.Errorf("expected ready_for_implementation, got %s", got)
	}
}

func TestFindStepForState(t *testing.T) {
	wf := makeTestWorkflow()
	step := FindStepForState(wf, "implementation")
	if step == nil {
		t.Fatal("expected step for implementation state")
	}
	if step.ID != "implementation" {
		t.Errorf("expected step ID=implementation, got %s", step.ID)
	}

	step = FindStepForState(wf, "nonexistent")
	if step != nil {
		t.Error("expected nil for nonexistent state")
	}
}

func TestIsIsQueueOrTerminalWithWorkflow(t *testing.T) {
	wf := makeTestWorkflow()

	if !IsIsQueueOrTerminalWithWorkflow("ready_for_implementation", wf) {
		t.Error("expected ready_for_implementation to be queue")
	}
	if !IsIsQueueOrTerminalWithWorkflow("shipped", wf) {
		t.Error("expected shipped to be terminal")
	}
	if IsIsQueueOrTerminalWithWorkflow("implementation", wf) {
		t.Error("expected implementation to NOT be queue or terminal")
	}
}

func TestTakeLoopContext_ClaimsMaxEnforcement(t *testing.T) {
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "test"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := NewTakeLoopContext(entry, &backend.Beat{ID: "beat-1"}, "/repo")

	for i := 1; i <= MaxClaimsPerQueueType; i++ {
		ctx.IncrementClaims("implementation")
	}

	count := ctx.GetClaimCount("implementation")
	if count != MaxClaimsPerQueueType {
		t.Errorf("expected %d claims, got %d", MaxClaimsPerQueueType, count)
	}

	if count > MaxClaimsPerQueueType {
		t.Errorf("claims exceeded max: %d > %d", count, MaxClaimsPerQueueType)
	}
}

func TestMaxClaimsPerQueueTypeValue(t *testing.T) {
	if MaxClaimsPerQueueType != 3 {
		t.Errorf("expected MaxClaimsPerQueueType=3, got %d", MaxClaimsPerQueueType)
	}
}

func TestOrchestrationWorkflowIntegration(t *testing.T) {
	wf := makeTestWorkflow()

	phase := orchestration.WorkflowStatePhase(wf, "implementation")
	if phase != orchestration.PhaseActive {
		t.Errorf("expected implementation to be active, got %s", phase)
	}

	phase = orchestration.WorkflowStatePhase(wf, "ready_for_implementation")
	if phase != orchestration.PhaseQueued {
		t.Errorf("expected ready_for_implementation to be queued, got %s", phase)
	}

	phase = orchestration.WorkflowStatePhase(wf, "shipped")
	if phase != orchestration.PhaseTerminal {
		t.Errorf("expected shipped to be terminal, got %s", phase)
	}
}

func TestWorkflowActionStateForState(t *testing.T) {
	wf := makeTestWorkflow()
	got := orchestration.WorkflowActionStateForState(wf, "implementation")
	if got != "implementation" {
		t.Errorf("expected 'implementation', got %q", got)
	}
}

func TestWorkflowQueueStateForState(t *testing.T) {
	wf := makeTestWorkflow()
	got := orchestration.WorkflowQueueStateForState(wf, "implementation")
	if got != "ready_for_implementation" {
		t.Errorf("expected ready_for_implementation, got %q", got)
	}
}