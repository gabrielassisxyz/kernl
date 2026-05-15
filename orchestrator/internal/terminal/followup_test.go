package terminal

import (
	"fmt"
	"testing"

	"github.com/gastownhall/foolery/internal/backend"
	"github.com/gastownhall/foolery/internal/session"
)

type mockLeaseChecker struct {
	healthy bool
	reason  string
	state   string
	err     error
}

func (m *mockLeaseChecker) EvaluateLeaseHealth(leaseID, repoPath string) (LeaseHealthResult, error) {
	if m.err != nil {
		return LeaseHealthResult{}, m.err
	}
	return LeaseHealthResult{
		Healthy:   m.healthy,
		Reason:    m.reason,
		LeaseState: m.state,
	}, nil
}

func makeFollowUpCtx(overrides ...func(*TakeLoopContext)) *TakeLoopContext {
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "test-sess"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
		KnotsLeaseID:             "test-lease-active",
		KnotsLeaseStep:           "implementation",
	}
	ctx := NewTakeLoopContext(entry, &backend.Beat{ID: "beat-6881"}, "/tmp/foolery-test")
	ctx.WorkflowsByID = map[string]*backend.WorkflowDescriptor{}
	ctx.FallbackWorkflow = &backend.WorkflowDescriptor{
		ID:             "default",
		Label:          "default",
		States:         []string{"open", "planning", "shipped"},
		TerminalStates: []string{"shipped"},
		ActionStates:   []string{"planning"},
		QueueStates:    []string{"open"},
		Transitions: []backend.WorkflowTransition{
			{From: "open", To: "planning"},
			{From: "planning", To: "shipped"},
		},
		StateOwners: map[string]backend.ActionOwnerKind{
			"planning": backend.ActionOwnerAgent,
		},
	}
	ctx.PushEvent = func(evt session.TerminalEvent) {}
	for _, o := range overrides {
		o(ctx)
	}
	return ctx
}

func TestHandleTakeLoopTurnEnded_SendsFollowUpWhenActive(t *testing.T) {
	ctx := makeFollowUpCtx()
	sent := false
	sentPrompt := ""
	deps := FollowUpDeps{
		GetBeat: func(beatID, repoPath string) (*backend.Beat, error) {
			return &backend.Beat{ID: "beat-6881", State: "planning"}, nil
		},
		SendUserTurn: func(prompt, source string) bool { sent = true; sentPrompt = prompt; return true },
		LeaseChecker: &mockLeaseChecker{healthy: true},
	}

	result := HandleTakeLoopTurnEnded(ctx, deps)

	if !result {
		t.Error("expected true when beat is in active state")
	}
	if !sent {
		t.Error("expected follow-up prompt to be sent")
	}
	if sentPrompt == "" {
		t.Error("expected non-empty prompt")
	}
	if !containsAll(sentPrompt, "still in state", "beat-6881") {
		t.Errorf("prompt should contain beat id and state, got: %s", sentPrompt)
	}
}

func TestHandleTakeLoopTurnEnded_DoesNotSendWhenTerminal(t *testing.T) {
	ctx := makeFollowUpCtx()
	sent := false
	deps := FollowUpDeps{
		GetBeat: func(beatID, repoPath string) (*backend.Beat, error) {
			return &backend.Beat{ID: "beat-6881", State: "shipped"}, nil
		},
		SendUserTurn: func(prompt, source string) bool { sent = true; return true },
		LeaseChecker: &mockLeaseChecker{healthy: true},
	}

	result := HandleTakeLoopTurnEnded(ctx, deps)

	if result {
		t.Error("expected false when beat has advanced to terminal")
	}
	if sent {
		t.Error("expected no follow-up when beat is terminal")
	}
	if ctx.FollowUpAttempts.Count != 0 {
		t.Errorf("expected count reset to 0, got %d", ctx.FollowUpAttempts.Count)
	}
}

func TestHandleTakeLoopTurnEnded_DoesNotSendWhenQueue(t *testing.T) {
	ctx := makeFollowUpCtx()
	sent := false
	deps := FollowUpDeps{
		GetBeat: func(beatID, repoPath string) (*backend.Beat, error) {
			return &backend.Beat{ID: "beat-6881", State: "open"}, nil
		},
		SendUserTurn: func(prompt, source string) bool { sent = true; return true },
		LeaseChecker: &mockLeaseChecker{healthy: true},
	}

	result := HandleTakeLoopTurnEnded(ctx, deps)

	if result {
		t.Error("expected false when beat is in queue state")
	}
	if sent {
		t.Error("expected no follow-up when beat is in queue state")
	}
}

func TestHandleTakeLoopTurnEnded_ReturnsFalseWhenFetchFails(t *testing.T) {
	ctx := makeFollowUpCtx()
	sent := false
	deps := FollowUpDeps{
		GetBeat: func(beatID, repoPath string) (*backend.Beat, error) {
			return nil, fmt.Errorf("fetch error")
		},
		SendUserTurn: func(prompt, source string) bool { sent = true; return true },
		LeaseChecker: &mockLeaseChecker{healthy: true},
	}

	result := HandleTakeLoopTurnEnded(ctx, deps)

	if result {
		t.Error("expected false when backend fetch fails")
	}
	if sent {
		t.Error("expected no follow-up when fetch fails")
	}
}

func TestHandleTakeLoopTurnEnded_SendUserTurnFails(t *testing.T) {
	ctx := makeFollowUpCtx()
	deps := FollowUpDeps{
		GetBeat: func(beatID, repoPath string) (*backend.Beat, error) {
			return &backend.Beat{ID: "beat-6881", State: "planning"}, nil
		},
		SendUserTurn: func(prompt, source string) bool { return false },
		LeaseChecker: &mockLeaseChecker{healthy: true},
	}
	result := HandleTakeLoopTurnEnded(ctx, deps)

	if result {
		t.Error("expected false when sendUserTurn fails")
	}
}

func TestHandleTakeLoopTurnEnded_CapStopsAfterFive(t *testing.T) {
	ctx := makeFollowUpCtx()
	callCount := 0
	deps := FollowUpDeps{
		GetBeat: func(beatID, repoPath string) (*backend.Beat, error) {
			return &backend.Beat{ID: "beat-6881", State: "planning"}, nil
		},
		SendUserTurn: func(prompt, source string) bool { callCount++; return true },
		LeaseChecker: &mockLeaseChecker{healthy: true},
	}

	results := make([]bool, 7)
	for i := 0; i < 7; i++ {
		results[i] = HandleTakeLoopTurnEnded(ctx, deps)
	}

	expected := []bool{true, true, true, true, true, false, false}
	for i, exp := range expected {
		if results[i] != exp {
			t.Errorf("call %d: expected %v, got %v", i+1, exp, results[i])
		}
	}
	if callCount != 5 {
		t.Errorf("expected sendUserTurn called 5 times, got %d", callCount)
	}
}

func TestHandleTakeLoopTurnEnded_CapBannerEmitted(t *testing.T) {
	var banners []session.TerminalEvent
	ctx := makeFollowUpCtx(func(ctx *TakeLoopContext) {
		ctx.PushEvent = func(evt session.TerminalEvent) {
			if evt.Type == "stderr" {
				banners = append(banners, evt)
			}
		}
	})
	deps := FollowUpDeps{
		GetBeat: func(beatID, repoPath string) (*backend.Beat, error) {
			return &backend.Beat{ID: "beat-6881", State: "planning"}, nil
		},
		SendUserTurn: func(prompt, source string) bool { return true },
		LeaseChecker: &mockLeaseChecker{healthy: true},
	}

	for i := 0; i < 6; i++ {
		HandleTakeLoopTurnEnded(ctx, deps)
	}

	found := false
	for _, b := range banners {
		if containsAll(b.Content, "follow-up cap reached", "beat-6881", "planning") {
			found = true
		}
	}
	if !found {
		t.Error("expected stderr banner containing 'follow-up cap reached', beat id, and state")
	}
}

func TestHandleTakeLoopTurnEnded_CapResetOnStateAdvance(t *testing.T) {
	ctx := makeFollowUpCtx()
	ctx.WorkflowsByID = map[string]*backend.WorkflowDescriptor{}
	ctx.FallbackWorkflow = &backend.WorkflowDescriptor{
		ID:             "default",
		Label:          "default",
		States:         []string{"open", "planning", "review", "shipped"},
		TerminalStates: []string{"shipped"},
		ActionStates:   []string{"planning", "review"},
		QueueStates:    []string{"open"},
		Transitions: []backend.WorkflowTransition{
			{From: "open", To: "planning"},
			{From: "planning", To: "review"},
			{From: "review", To: "shipped"},
		},
		StateOwners: map[string]backend.ActionOwnerKind{
			"planning": backend.ActionOwnerAgent,
			"review":   backend.ActionOwnerAgent,
		},
	}

	states := []string{"planning", "planning", "planning", "review"}
	callIdx := 0
	deps := FollowUpDeps{
		GetBeat: func(beatID, repoPath string) (*backend.Beat, error) {
			state := states[callIdx]
			callIdx++
			return &backend.Beat{ID: "beat-6881", State: state}, nil
		},
		SendUserTurn: func(prompt, source string) bool { return true },
		LeaseChecker: &mockLeaseChecker{healthy: true},
	}

	for i := 0; i < 3; i++ {
		HandleTakeLoopTurnEnded(ctx, deps)
	}
	if ctx.FollowUpAttempts.Count != 3 {
		t.Errorf("expected count=3 after 3 stuck turns, got %d", ctx.FollowUpAttempts.Count)
	}

	result := HandleTakeLoopTurnEnded(ctx, deps)
	if !result {
		t.Error("expected true when state advances to different active state")
	}
	if ctx.FollowUpAttempts.Count != 1 {
		t.Errorf("expected count reset to 1 after state change, got %d", ctx.FollowUpAttempts.Count)
	}
	if ctx.FollowUpAttempts.LastState != "review" {
		t.Errorf("expected lastState=review, got %s", ctx.FollowUpAttempts.LastState)
	}
}

func TestHandleTakeLoopTurnEnded_CapResetOnQueueState(t *testing.T) {
	ctx := makeFollowUpCtx()
	ctx.FollowUpAttempts.Count = 7
	ctx.FollowUpAttempts.LastState = "planning"

	deps := FollowUpDeps{
		GetBeat: func(beatID, repoPath string) (*backend.Beat, error) {
			return &backend.Beat{ID: "beat-6881", State: "shipped"}, nil
		},
		SendUserTurn: func(prompt, source string) bool { return true },
		LeaseChecker: &mockLeaseChecker{healthy: true},
	}

	result := HandleTakeLoopTurnEnded(ctx, deps)

	if result {
		t.Error("expected false when beat reaches terminal state")
	}
	if ctx.FollowUpAttempts.Count != 0 {
		t.Errorf("expected count reset to 0, got %d", ctx.FollowUpAttempts.Count)
	}
	if ctx.FollowUpAttempts.LastState != "shipped" {
		t.Errorf("expected lastState=shipped, got %s", ctx.FollowUpAttempts.LastState)
	}
}

func TestHandleTakeLoopTurnEnded_LeaseHealthBlocksFollowUp(t *testing.T) {
	var stderrEvents []session.TerminalEvent
	ctx := makeFollowUpCtx(func(ctx *TakeLoopContext) {
		ctx.PushEvent = func(evt session.TerminalEvent) {
			if evt.Type == "stderr" {
				stderrEvents = append(stderrEvents, evt)
			}
		}
	})
	sent := false
	deps := FollowUpDeps{
		GetBeat: func(beatID, repoPath string) (*backend.Beat, error) {
			return &backend.Beat{ID: "beat-6881", State: "planning"}, nil
		},
		SendUserTurn: func(prompt, source string) bool { sent = true; return true },
		LeaseChecker: &mockLeaseChecker{
			healthy: false,
			reason:  "lease_terminated",
			state:   "lease_terminated",
		},
	}

	result := HandleTakeLoopTurnEnded(ctx, deps)

	if result {
		t.Error("expected false when lease health check fails")
	}
	if sent {
		t.Error("expected no follow-up sent when lease is unhealthy")
	}
	if len(stderrEvents) == 0 || !containsAll(stderrEvents[0].Content, "FOOLERY DISPATCH FAILURE") {
		t.Error("expected dispatch failure banner for dead lease")
	}
}

func TestBuildTakeLoopFollowUpPrompt(t *testing.T) {
	prompt := BuildTakeLoopFollowUpPrompt("foolery-6881", "workable")
	if !containsAll(prompt, "foolery-6881", "workable", "still in state") {
		t.Errorf("prompt should contain beat id, state, and 'still in state', got: %s", prompt)
	}
	if !containsAll(prompt, "kno rollback") {
		t.Errorf("prompt should suggest kno rollback, got: %s", prompt)
	}
}

func TestRecordFollowUpProgress(t *testing.T) {
	fa := &FollowUpCounter{}

	count := RecordFollowUpProgress(fa, "planning")
	if count != 1 {
		t.Errorf("expected count=1, got %d", count)
	}
	if fa.LastState != "planning" {
		t.Errorf("expected lastState=planning, got %s", fa.LastState)
	}

	count = RecordFollowUpProgress(fa, "planning")
	if count != 2 {
		t.Errorf("expected count=2, got %d", count)
	}

	count = RecordFollowUpProgress(fa, "review")
	if count != 1 {
		t.Errorf("expected count reset to 1 on state change, got %d", count)
	}
	if fa.LastState != "review" {
		t.Errorf("expected lastState=review, got %s", fa.LastState)
	}
}

func TestShouldContinueShipFollowUp(t *testing.T) {
	tests := []struct {
		name     string
		input    ShipFollowUpInput
		expected bool
	}{
		{
			name:     "clean close continues",
			input:    ShipFollowUpInput{ExitCode: 0, ExitReason: "raw_close", ExecutionPromptSent: true, ShipCompletionPromptSent: false, AutoShipCompletionPrompt: "follow up"},
			expected: true,
		},
		{
			name:     "fatal exit reason blocks",
			input:    ShipFollowUpInput{ExitCode: 0, ExitReason: "timeout", ExecutionPromptSent: true, ShipCompletionPromptSent: false, AutoShipCompletionPrompt: "follow up"},
			expected: false,
		},
		{
			name:     "non-zero exit code blocks",
			input:    ShipFollowUpInput{ExitCode: 1, ExitReason: "raw_close", ExecutionPromptSent: true, ShipCompletionPromptSent: false, AutoShipCompletionPrompt: "follow up"},
			expected: false,
		},
		{
			name:     "already sent ship prompt blocks",
			input:    ShipFollowUpInput{ExitCode: 0, ExitReason: "raw_close", ExecutionPromptSent: true, ShipCompletionPromptSent: true, AutoShipCompletionPrompt: "follow up"},
			expected: false,
		},
		{
			name:     "no execution prompt sent blocks",
			input:    ShipFollowUpInput{ExitCode: 0, ExitReason: "raw_close", ExecutionPromptSent: false, ShipCompletionPromptSent: false, AutoShipCompletionPrompt: "follow up"},
			expected: false,
		},
		{
			name:     "no auto ship prompt blocks",
			input:    ShipFollowUpInput{ExitCode: 0, ExitReason: "raw_close", ExecutionPromptSent: true, ShipCompletionPromptSent: false, AutoShipCompletionPrompt: ""},
			expected: false,
		},
		{
			name:     "spawn_error blocks",
			input:    ShipFollowUpInput{ExitCode: 0, ExitReason: "spawn_error", ExecutionPromptSent: true, ShipCompletionPromptSent: false, AutoShipCompletionPrompt: "follow up"},
			expected: false,
		},
		{
			name:     "external_abort blocks",
			input:    ShipFollowUpInput{ExitCode: 0, ExitReason: "external_abort", ExecutionPromptSent: true, ShipCompletionPromptSent: false, AutoShipCompletionPrompt: "follow up"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldContinueShipFollowUp(tt.input)
			if got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestIsQueueOrTerminalState(t *testing.T) {
	wf := &backend.WorkflowDescriptor{
		ID:             "wf-1",
		States:         []string{"ready_for_implementation", "implementation", "ready_for_review", "shipped"},
		TerminalStates: []string{"shipped"},
		ActionStates:   []string{"implementation"},
		QueueStates:    []string{"ready_for_implementation", "ready_for_review"},
		Transitions: []backend.WorkflowTransition{
			{From: "ready_for_implementation", To: "implementation"},
			{From: "implementation", To: "ready_for_review"},
			{From: "ready_for_review", To: "shipped"},
		},
	}

	tests := []struct {
		state    string
		expected bool
	}{
		{"ready_for_implementation", true},
		{"ready_for_review", true},
		{"shipped", true},
		{"implementation", false},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got := IsQueueOrTerminalState(tt.state, wf)
			if got != tt.expected {
				t.Errorf("IsQueueOrTerminalState(%q) = %v, want %v", tt.state, got, tt.expected)
			}
		})
	}
}

func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || containsAt(s, sub))
}

func containsAt(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}