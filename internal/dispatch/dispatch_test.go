package dispatch

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/session"
)

func makeSDLCWorkflow() *backend.WorkflowDescriptor {
	return &backend.WorkflowDescriptor{
		ID:               "sdlc",
		ProfileID:        "sdlc",
		BackingWorkflowID: "sdlc",
		Label:            "SDLC",
		Mode:             "granular_autonomous",
		InitialState:      "ready_for_planning",
		States:           []string{"ready_for_planning", "planning", "ready_for_plan_review", "plan_review", "ready_for_implementation", "implementation"},
		TerminalStates:   []string{},
		Transitions: []backend.WorkflowTransition{
			{From: "ready_for_planning", To: "planning"},
			{From: "planning", To: "ready_for_plan_review"},
			{From: "ready_for_plan_review", To: "plan_review"},
			{From: "plan_review", To: "ready_for_implementation"},
			{From: "plan_review", To: "ready_for_planning"},
		},
		FinalCutState: "",
		RetakeState:   "ready_for_planning",
		PromptProfileID: "sdlc",
		StateOwners: map[string]backend.ActionOwnerKind{
			"ready_for_planning":   backend.ActionOwnerAgent,
			"planning":            backend.ActionOwnerAgent,
			"ready_for_plan_review": backend.ActionOwnerAgent,
			"plan_review":         backend.ActionOwnerAgent,
			"ready_for_implementation": backend.ActionOwnerAgent,
			"implementation":      backend.ActionOwnerAgent,
		},
		QueueStates:    []string{"ready_for_planning", "ready_for_plan_review", "ready_for_implementation"},
		ActionStates:   []string{"planning", "plan_review", "implementation"},
		QueueActions: map[string]string{
			"ready_for_planning":     "planning",
			"ready_for_plan_review":  "plan_review",
			"ready_for_implementation": "implementation",
		},
		ReviewQueueStates: []string{"ready_for_plan_review"},
		HumanQueueStates:  []string{},
	}
}

func makeBaseSettings() *config.Settings {
	return &config.Settings{
		Agents: map[string]config.AgentConfig{
			"claude": {
				Command:  "/usr/local/bin/claude",
				Type:     "cli",
				Vendor:   "claude",
				Provider: "Claude",
				AgentName: "Claude",
				Label:    "Claude",
			},
			"codex": {
				Command:  "/usr/local/bin/codex",
				Type:     "cli",
				Vendor:   "codex",
				Provider: "Codex",
				AgentName: "Codex",
				Label:    "Codex",
			},
		},
		Pools: map[string]config.PoolConfig{
			"planning":      {Agents: []config.WeightedAgent{{AgentID: "claude", Weight: 1}, {AgentID: "codex", Weight: 1}}},
			"plan_review":   {Agents: []config.WeightedAgent{{AgentID: "claude", Weight: 1}, {AgentID: "codex", Weight: 1}}},
			"implementation": {Agents: []config.WeightedAgent{{AgentID: "claude", Weight: 1}, {AgentID: "codex", Weight: 1}}},
		},
	}
}

func collectStderr(events []session.TerminalEvent) string {
	var b strings.Builder
	for _, e := range events {
		if e.Type == "stderr" {
			b.WriteString(e.Content)
		}
	}
	return b.String()
}

func collectAgentFailures(events []session.TerminalEvent) []string {
	var result []string
	for _, e := range events {
		if e.Type == "agent_failure" {
			result = append(result, e.Content)
		}
	}
	return result
}

func TestRunDispatch_CrossAgentReviewFallback(t *testing.T) {
	wf := makeSDLCWorkflow()
	settings := makeBaseSettings()
	tracker := NewStepAgentTracker()
	tracker.Record("bead-1", "planning", "claude")

	var events []session.TerminalEvent
	ctx := &TakeLoopDispatchContext{
		ID:                       "term-test",
		BeadID:                   "bead-1",
		PushEvent:                func(evt session.TerminalEvent) { events = append(events, evt) },
		FailedAgentsPerQueueType: map[string]map[string]bool{"plan_review": {"codex": true}},
		ClaimsPerQueueType:       map[string]int{},
		LastAgentPerQueueType:    map[string]string{},
	}

	result := RunDispatch(DispatchArgs{
		Ctx:              ctx,
		Settings:         settings,
		Workflow:          wf,
		State:            "ready_for_plan_review",
		PoolKey:           "plan_review",
		QueueType:         "plan_review",
		ExcludeAgentIDs:   []string{"codex", "claude"},
		IsErrorRetry:      true,
		IsReview:          true,
		PriorAction:       "planning",
		FailedAgentID:     "codex",
		MaxClaims:         10,
		StepAgentTracker:  tracker,
	})

	if result.Stopped {
		t.Fatal("expected fallback to succeed, got stop")
	}
	if result.StepAgentOverride == nil {
		t.Fatal("expected stepAgentOverride to be set")
	}
	if result.StepAgentID != "claude" {
		t.Errorf("expected fallback agent to be claude, got %s", result.StepAgentID)
	}
	if !result.FallbackUsed {
		t.Error("expected FallbackUsed=true for cross-agent review fallback")
	}
	stderr := collectStderr(events)
	if !strings.Contains(stderr, "Cross-agent review fallback") {
		t.Errorf("expected 'Cross-agent review fallback' banner in stderr, got: %s", stderr)
	}
}

func TestRunDispatch_CrossAgentReviewNoAlternative(t *testing.T) {
	wf := makeSDLCWorkflow()
	settings := &config.Settings{
		Agents: map[string]config.AgentConfig{
			"claude": {
				Command:  "/usr/local/bin/claude",
				Type:     "cli",
				Vendor:   "claude",
				Provider: "Claude",
				AgentName: "Claude",
				Label:    "Claude",
			},
		},
		Pools: map[string]config.PoolConfig{
			"plan_review": {Agents: []config.WeightedAgent{{AgentID: "claude", Weight: 1}}},
		},
	}

	tracker := NewStepAgentTracker()
	tracker.Record("bead-1", "planning", "claude")

	var events []session.TerminalEvent
	ctx := &TakeLoopDispatchContext{
		ID:                       "term-test",
		BeadID:                   "bead-1",
		PushEvent:                func(evt session.TerminalEvent) { events = append(events, evt) },
		FailedAgentsPerQueueType: map[string]map[string]bool{"plan_review": {"claude": true}},
		ClaimsPerQueueType:       map[string]int{},
		LastAgentPerQueueType:    map[string]string{},
	}

	result := RunDispatch(DispatchArgs{
		Ctx:              ctx,
		Settings:         settings,
		Workflow:          wf,
		State:            "ready_for_plan_review",
		PoolKey:           "plan_review",
		QueueType:         "plan_review",
		ExcludeAgentIDs:   []string{"claude"},
		IsErrorRetry:      true,
		IsReview:          true,
		PriorAction:       "planning",
		FailedAgentID:     "claude",
		MaxClaims:         10,
		StepAgentTracker:  tracker,
	})

	if !result.Stopped {
		t.Error("expected stop when single agent is hard-excluded and no alternative exists")
	}
}

func TestRunDispatch_CrossAgentInvariantHonored(t *testing.T) {
	wf := makeSDLCWorkflow()
	settings := &config.Settings{
		Agents: map[string]config.AgentConfig{
			"claude": {Command: "/usr/local/bin/claude", Type: "cli", Vendor: "claude", Provider: "Claude", AgentName: "Claude", Label: "Claude"},
			"codex":  {Command: "/usr/local/bin/codex", Type: "cli", Vendor: "codex", Provider: "Codex", AgentName: "Codex", Label: "Codex"},
			"opencode": {Command: "/usr/local/bin/opencode", Type: "cli", Vendor: "opencode", Provider: "OpenCode", AgentName: "OpenCode", Label: "OpenCode"},
		},
		Pools: map[string]config.PoolConfig{
			"plan_review": {Agents: []config.WeightedAgent{{AgentID: "claude", Weight: 1}, {AgentID: "codex", Weight: 1}, {AgentID: "opencode", Weight: 1}}},
		},
	}

	tracker := NewStepAgentTracker()
	tracker.Record("bead-1", "planning", "claude")

	var events []session.TerminalEvent
	ctx := &TakeLoopDispatchContext{
		ID:                       "term-test",
		BeadID:                   "bead-1",
		PushEvent:                func(evt session.TerminalEvent) { events = append(events, evt) },
		FailedAgentsPerQueueType: map[string]map[string]bool{},
		ClaimsPerQueueType:       map[string]int{},
		LastAgentPerQueueType:    map[string]string{},
	}

	result := RunDispatch(DispatchArgs{
		Ctx:              ctx,
		Settings:         settings,
		Workflow:          wf,
		State:            "ready_for_plan_review",
		PoolKey:           "plan_review",
		QueueType:         "plan_review",
		ExcludeAgentIDs:   []string{"claude"},
		IsErrorRetry:      false,
		IsReview:          true,
		PriorAction:       "planning",
		FailedAgentID:     "",
		MaxClaims:         10,
		StepAgentTracker:  tracker,
	})

	if result.Stopped {
		t.Fatal("expected dispatch to succeed without fallback")
	}
	if result.StepAgentOverride == nil {
		t.Fatal("expected stepAgentOverride")
	}
	if result.StepAgentID == "claude" {
		t.Error("claude should be excluded for cross-agent review when alternatives exist")
	}
	if result.FallbackUsed {
		t.Error("fallback should NOT be used when alternatives exist")
	}
	stderr := collectStderr(events)
	if strings.Contains(stderr, "Cross-agent review fallback") {
		t.Error("expected NO fallback banner when alternatives exist")
	}
}

func TestRunDispatch_NormalSelection(t *testing.T) {
	wf := makeSDLCWorkflow()
	settings := makeBaseSettings()

	var events []session.TerminalEvent
	ctx := &TakeLoopDispatchContext{
		ID:                       "term-test",
		BeadID:                   "bead-1",
		PushEvent:                func(evt session.TerminalEvent) { events = append(events, evt) },
		FailedAgentsPerQueueType: map[string]map[string]bool{},
		ClaimsPerQueueType:       map[string]int{},
		LastAgentPerQueueType:    map[string]string{},
	}

	result := RunDispatch(DispatchArgs{
		Ctx:              ctx,
		Settings:         settings,
		Workflow:          wf,
		State:            "ready_for_implementation",
		PoolKey:           "implementation",
		QueueType:         "implementation",
		ExcludeAgentIDs:   []string{},
		IsErrorRetry:      false,
		IsReview:          false,
		PriorAction:       "",
		FailedAgentID:     "",
		MaxClaims:         10,
		StepAgentTracker:  NewStepAgentTracker(),
	})

	if result.Stopped {
		t.Fatal("expected dispatch to succeed for normal selection")
	}
	if result.StepAgentOverride == nil {
		t.Fatal("expected an agent to be selected")
	}
	if result.StepAgentID == "" {
		t.Error("expected non-empty agentID")
	}
}

func TestIsReviewQueueState(t *testing.T) {
	wf := makeSDLCWorkflow()
	if !IsReviewQueueState(wf, "plan_review") {
		t.Error("expected plan_review (pool key/action) to be a review queue state")
	}
	if IsReviewQueueState(wf, "implementation") {
		t.Error("expected implementation NOT to be a review queue state")
	}
	if IsReviewQueueState(nil, "plan_review") {
		t.Error("expected nil workflow to return false")
	}
}

func TestPriorActionStep(t *testing.T) {
	wf := makeSDLCWorkflow()
	prior := PriorActionStep(wf, "plan_review")
	if prior != "planning" {
		t.Errorf("expected prior action 'planning', got %q", prior)
	}
}

func TestDerivePoolKey(t *testing.T) {
	wf := makeSDLCWorkflow()
	pk := DerivePoolKey(wf, "ready_for_implementation")
	if pk != "implementation" {
		t.Errorf("expected 'implementation', got %q", pk)
	}
	pk = DerivePoolKey(nil, "ready_for_implementation")
	if pk != "" {
		t.Errorf("expected empty for nil workflow, got %q", pk)
	}
}

func TestSelectStepAgent_ErrorRetryExcludesFailedAgent(t *testing.T) {
	wf := makeSDLCWorkflow()
	settings := makeBaseSettings()
	tracker := NewStepAgentTracker()

	var events []session.TerminalEvent
	loopCtx := &TakeLoopContext{
		ID:                       "term-test",
		BeadID:                   "bead-1",
		AgentID:                  "codex",
		FailedAgentsPerQueueType: map[string]map[string]bool{},
		ClaimsPerQueueType:       map[string]int{},
		LastAgentPerQueueType:    map[string]string{},
		PushEvent:                func(evt session.TerminalEvent) { events = append(events, evt) },
		FinishSession:            func(exitCode int) {},
	}

	result := SelectStepAgent(loopCtx, wf, "ready_for_implementation", "implementation", settings.Agents, settings.Pools, settings, tracker, false, "codex")
	if result.Stopped {
		t.Fatal("expected dispatch to find an alternative agent")
	}
	if result.StepAgentOverride == nil {
		t.Fatal("expected an agent to be selected")
	}
	if result.StepAgentID == "codex" {
		t.Error("expected codex to be excluded as the failed agent")
	}
}

func TestSelectStepAgent_ReviewExcludesCurrentAndPrior(t *testing.T) {
	wf := makeSDLCWorkflow()
	settings := makeBaseSettings()
	tracker := NewStepAgentTracker()
	tracker.Record("bead-1", "planning", "claude")

	var events []session.TerminalEvent
	loopCtx := &TakeLoopContext{
		ID:                       "term-test",
		BeadID:                   "bead-1",
		AgentID:                  "current-agent",
		FailedAgentsPerQueueType: map[string]map[string]bool{},
		ClaimsPerQueueType:       map[string]int{},
		LastAgentPerQueueType:    map[string]string{},
		PushEvent:                func(evt session.TerminalEvent) { events = append(events, evt) },
		FinishSession:            func(exitCode int) {},
	}

	result := SelectStepAgent(loopCtx, wf, "ready_for_plan_review", "plan_review", settings.Agents, settings.Pools, settings, tracker, false, "")
	// With claude excluded (prior action) and current-agent excluded, codex should be selected
	if result.Stopped {
		t.Fatal("expected dispatch to find an alternative agent")
	}
	if result.StepAgentOverride == nil {
		t.Fatal("expected an agent to be selected")
	}
	if result.StepAgentID == "claude" {
		t.Error("expected claude to be excluded as prior action agent in review")
	}
}

func TestSelectStepAgent_SoftRotation(t *testing.T) {
	wf := makeSDLCWorkflow()
	settings := makeBaseSettings()
	tracker := NewStepAgentTracker()

	var events []session.TerminalEvent
	loopCtx := &TakeLoopContext{
		ID:                       "term-test",
		BeadID:                   "bead-1",
		AgentID:                  "",
		FailedAgentsPerQueueType: map[string]map[string]bool{},
		ClaimsPerQueueType:       map[string]int{},
		LastAgentPerQueueType:    map[string]string{"implementation": "codex"},
		PushEvent:                func(evt session.TerminalEvent) { events = append(events, evt) },
		FinishSession:            func(exitCode int) {},
	}

	result := SelectStepAgent(loopCtx, wf, "ready_for_implementation", "implementation", settings.Agents, settings.Pools, settings, tracker, false, "")
	if result.Stopped {
		t.Fatal("expected dispatch to succeed")
	}
	if result.StepAgentOverride == nil {
		t.Fatal("expected an agent to be selected")
	}
	if result.StepAgentID == "codex" {
		t.Error("expected codex to be excluded for soft rotation (lastAgentPerQueueType)")
	}
}

func TestHandleMaxClaims(t *testing.T) {
	var events []session.TerminalEvent
	ctx := &TakeLoopDispatchContext{
		ID:        "term-test",
		BeadID:    "bead-1",
		PushEvent: func(evt session.TerminalEvent) { events = append(events, evt) },
	}

	HandleMaxClaims(ctx, "implementation", 3, 3)
	stderr := collectStderr(events)
	if len(stderr) > 0 {
		// Should not be in stderr, it's in stdout
		t.Error("HandleMaxClaims should log to stdout, not stderr")
	}
	found := false
	for _, e := range events {
		if e.Type == "stdout" && strings.Contains(e.Content, "max claims per queue type") {
			found = true
		}
	}
	if !found {
		t.Error("expected HandleMaxClaims to emit max claims banner to stdout")
	}
}

func TestComputeStepExclusions_FailedAndErrorAgents(t *testing.T) {
	loopCtx := &TakeLoopContext{
		ID:                       "term-test",
		BeadID:                   "bead-1",
		AgentID:                  "",
		FailedAgentsPerQueueType: map[string]map[string]bool{"implementation": {"agent-a": true}},
		ClaimsPerQueueType:       map[string]int{},
		LastAgentPerQueueType:    map[string]string{},
	}

	exclusions := computeStepExclusions(loopCtx, "implementation", false, "agent-b", "", nil)
	foundA := false
	foundB := false
	for _, id := range exclusions {
		if id == "agent-a" {
			foundA = true
		}
		if id == "agent-b" {
			foundB = true
		}
	}
	if !foundA {
		t.Error("expected agent-a (failed agent) in exclusions")
	}
	if !foundB {
		t.Error("expected agent-b (error agent) in exclusions")
	}
}

func TestComputeStepExclusions_ReviewExcludesCurrentAndPrior(t *testing.T) {
	tracker := NewStepAgentTracker()
	tracker.Record("bead-1", "planning", "prior-agent")

	loopCtx := &TakeLoopContext{
		ID:                       "term-test",
		BeadID:                   "bead-1",
		AgentID:                  "my-agent",
		FailedAgentsPerQueueType: map[string]map[string]bool{},
		ClaimsPerQueueType:       map[string]int{},
		LastAgentPerQueueType:    map[string]string{},
	}

	exclusions := computeStepExclusions(loopCtx, "plan_review", true, "", "planning", tracker)
	foundCurrent := false
	foundPrior := false
	for _, id := range exclusions {
		if id == "my-agent" {
			foundCurrent = true
		}
		if id == "prior-agent" {
			foundPrior = true
		}
	}
	if !foundCurrent {
		t.Error("expected current agent excluded in review mode")
	}
	if !foundPrior {
		t.Error("expected prior action agent excluded in review mode")
	}
}

func TestComputeStepExclusions_SoftRotation(t *testing.T) {
	loopCtx := &TakeLoopContext{
		ID:                       "term-test",
		BeadID:                   "bead-1",
		AgentID:                  "",
		FailedAgentsPerQueueType: map[string]map[string]bool{},
		ClaimsPerQueueType:       map[string]int{},
		LastAgentPerQueueType:    map[string]string{"implementation": "last-agent"},
	}

	exclusions := computeStepExclusions(loopCtx, "implementation", false, "", "", nil)
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

func TestRunDispatch_DispatchFailureEmitsBanner(t *testing.T) {
	wf := makeSDLCWorkflow()
	settings := &config.Settings{
		Agents: map[string]config.AgentConfig{},
		Pools: map[string]config.PoolConfig{
			"implementation": {Agents: []config.WeightedAgent{{AgentID: "nonexistent", Weight: 1}}},
		},
	}

	var events []session.TerminalEvent
	ctx := &TakeLoopDispatchContext{
		ID:                       "term-test",
		BeadID:                   "bead-1",
		PushEvent:                func(evt session.TerminalEvent) { events = append(events, evt) },
		FailedAgentsPerQueueType: map[string]map[string]bool{},
		ClaimsPerQueueType:       map[string]int{},
		LastAgentPerQueueType:    map[string]string{},
	}

	result := RunDispatch(DispatchArgs{
		Ctx:              ctx,
		Settings:         settings,
		Workflow:          wf,
		State:            "ready_for_implementation",
		PoolKey:           "implementation",
		QueueType:         "implementation",
		ExcludeAgentIDs:   []string{},
		IsErrorRetry:      false,
		IsReview:          false,
		MaxClaims:         10,
		StepAgentTracker:  NewStepAgentTracker(),
	})

	if !result.Stopped {
		t.Error("expected dispatch to stop when pool has dangling agent")
	}
	stderr := collectStderr(events)
	if !strings.Contains(stderr, "KERNL DISPATCH FAILURE") {
		t.Errorf("expected dispatch failure banner in stderr, got: %s", stderr)
	}
}