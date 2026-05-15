package terminal

import (
	"fmt"
	"testing"

	"github.com/gastownhall/foolery/internal/backend"
	"github.com/gastownhall/foolery/internal/session"
)

type stubRollbackBackend struct {
	beats     map[string]*backend.Beat
	rewinds   []rewindCall
	rewindErr error
}

type rewindCall struct {
	beatID      string
	targetState string
	reason      string
	repoPath    string
}

func (s *stubRollbackBackend) ListWorkflows(repoPath string) ([]backend.WorkflowDescriptor, error) {
	return nil, nil
}
func (s *stubRollbackBackend) List(filters *backend.BeatListFilters, repoPath string) ([]backend.Beat, error) {
	return nil, nil
}
func (s *stubRollbackBackend) ListReady(filters *backend.BeatListFilters, repoPath string) ([]backend.Beat, error) {
	return nil, nil
}
func (s *stubRollbackBackend) Get(id string, repoPath string) (*backend.Beat, error) {
	if b, ok := s.beats[id]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("beat %s not found", id)
}
func (s *stubRollbackBackend) Create(input backend.CreateBeatInput, repoPath string) (*backend.Beat, error) {
	return nil, nil
}
func (s *stubRollbackBackend) Update(id string, input backend.UpdateBeatInput, repoPath string) error {
	return nil
}
func (s *stubRollbackBackend) Delete(id string, repoPath string) error {
	return nil
}
func (s *stubRollbackBackend) Close(id string, reason string, repoPath string) (*backend.TerminalState, error) {
	return nil, nil
}
func (s *stubRollbackBackend) MarkTerminal(id string, targetState string, reason string, repoPath string) error {
	return nil
}
func (s *stubRollbackBackend) Reopen(id string, reason string, repoPath string) error {
	return nil
}
func (s *stubRollbackBackend) Rewind(id string, targetState string, reason string, repoPath string) error {
	s.rewinds = append(s.rewinds, rewindCall{beatID: id, targetState: targetState, reason: reason, repoPath: repoPath})
	if s.rewindErr != nil {
		return s.rewindErr
	}
	if b, ok := s.beats[id]; ok {
		b.State = targetState
	}
	return nil
}
func (s *stubRollbackBackend) Search(query string, filters *backend.BeatListFilters, repoPath string) ([]backend.Beat, error) {
	return nil, nil
}
func (s *stubRollbackBackend) Query(expression string, options *backend.BeatQueryOptions, repoPath string) ([]backend.Beat, error) {
	return nil, nil
}
func (s *stubRollbackBackend) AddDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (s *stubRollbackBackend) RemoveDependency(blockerID string, blockedID string, repoPath string) error {
	return nil
}
func (s *stubRollbackBackend) ListDependencies(id string, repoPath string, options *backend.DependencyListOptions) ([]backend.BeatDependency, error) {
	return nil, nil
}
func (s *stubRollbackBackend) BuildTakePrompt(beatID string, options *backend.TakePromptOptions, repoPath string) (*backend.TakePromptResult, error) {
	return &backend.TakePromptResult{Prompt: "take"}, nil
}
func (s *stubRollbackBackend) BuildPollPrompt(options *backend.PollPromptOptions, repoPath string) (*backend.PollPromptResult, error) {
	return nil, nil
}
func (s *stubRollbackBackend) Capabilities() backend.BackendCapabilities {
	return backend.BackendCapabilities{}
}

func makeRollbackWF() *backend.WorkflowDescriptor {
	return &backend.WorkflowDescriptor{
		ID:             "wf-sdlc",
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
			"ready_for_review":        backend.ActionOwnerAgent,
			"review":                  backend.ActionOwnerAgent,
		},
	}
}

func TestEnforceQueueTerminalInvariant_AlreadySatisfied(t *testing.T) {
	sb := &stubRollbackBackend{
		beats: map[string]*backend.Beat{
			"beat-1": {ID: "beat-1", State: "ready_for_implementation"},
		},
	}
	wf := makeRollbackWF()
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "sess-1"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := &TakeLoopContext{
		ID:               "sess-1",
		BeatID:           "beat-1",
		WorkflowsByID:    map[string]*backend.WorkflowDescriptor{"wf-sdlc": wf},
		FallbackWorkflow: wf,
		Entry:            entry,
		Beat:             &backend.Beat{ID: "beat-1", State: "ready_for_implementation"},
	}

	ok, err := EnforceQueueTerminalInvariant(ctx, sb)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !ok {
		t.Error("expected invariant satisfied for queue state")
	}
	if len(sb.rewinds) != 0 {
		t.Error("expected no rewind calls for queue state")
	}
}

func TestEnforceQueueTerminalInvariant_TerminalState(t *testing.T) {
	sb := &stubRollbackBackend{
		beats: map[string]*backend.Beat{
			"beat-1": {ID: "beat-1", State: "shipped"},
		},
	}
	wf := makeRollbackWF()
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "sess-1"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := &TakeLoopContext{
		ID:               "sess-1",
		BeatID:           "beat-1",
		WorkflowsByID:    map[string]*backend.WorkflowDescriptor{"wf-sdlc": wf},
		FallbackWorkflow: wf,
		Entry:            entry,
		Beat:             &backend.Beat{ID: "beat-1", State: "shipped"},
	}

	ok, err := EnforceQueueTerminalInvariant(ctx, sb)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !ok {
		t.Error("expected invariant satisfied for terminal state")
	}
}

func TestEnforceQueueTerminalInvariant_RollsBackActiveState(t *testing.T) {
	sb := &stubRollbackBackend{
		beats: map[string]*backend.Beat{
			"beat-1": {ID: "beat-1", State: "implementation"},
		},
	}
	wf := makeRollbackWF()
	var events []session.TerminalEvent
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "sess-1"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := &TakeLoopContext{
		ID:               "sess-1",
		BeatID:           "beat-1",
		RepoPath:         "/repo",
		WorkflowsByID:    map[string]*backend.WorkflowDescriptor{"wf-sdlc": wf},
		FallbackWorkflow: wf,
		Entry:            entry,
		Beat:             &backend.Beat{ID: "beat-1", State: "implementation"},
		PushEvent: func(evt session.TerminalEvent) {
			events = append(events, evt)
		},
	}

	ok, err := EnforceQueueTerminalInvariant(ctx, sb)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !ok {
		t.Error("expected invariant satisfied after rollback")
	}
	if len(sb.rewinds) != 1 {
		t.Fatalf("expected 1 rewind call, got %d", len(sb.rewinds))
	}
	if sb.rewinds[0].targetState != "ready_for_implementation" {
		t.Errorf("expected rollback to ready_for_implementation, got %s", sb.rewinds[0].targetState)
	}
}

func TestEnforceQueueTerminalInvariant_RollbackFails(t *testing.T) {
	sb := &stubRollbackBackend{
		beats: map[string]*backend.Beat{
			"beat-1": {ID: "beat-1", State: "implementation"},
		},
		rewindErr: fmt.Errorf("rollback failed"),
	}
	wf := makeRollbackWF()
	var events []session.TerminalEvent
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "sess-1"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := &TakeLoopContext{
		ID:               "sess-1",
		BeatID:           "beat-1",
		RepoPath:         "/repo",
		WorkflowsByID:    map[string]*backend.WorkflowDescriptor{"wf-sdlc": wf},
		FallbackWorkflow: wf,
		Entry:            entry,
		Beat:             &backend.Beat{ID: "beat-1", State: "implementation"},
		PushEvent: func(evt session.TerminalEvent) {
			events = append(events, evt)
		},
	}

	ok, err := EnforceQueueTerminalInvariant(ctx, sb)
	if err == nil {
		t.Error("expected error from rollback failure")
	}
	if ok {
		t.Error("expected invariant NOT satisfied after rollback failure")
	}

	foundStderr := false
	for _, evt := range events {
		if evt.Type == "stderr" {
			foundStderr = true
		}
	}
	if !foundStderr {
		t.Error("expected stderr event for rollback failure")
	}
}

func TestHandleErrorExit_RecordsFailedAgentAndRollsBack(t *testing.T) {
	sb := &stubRollbackBackend{
		beats: map[string]*backend.Beat{
			"beat-1": {ID: "beat-1", State: "implementation"},
		},
	}
	wf := makeRollbackWF()
	var events []session.TerminalEvent
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "sess-1"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := &TakeLoopContext{
		ID:               "sess-1",
		BeatID:           "beat-1",
		RepoPath:         "/repo",
		WorkflowsByID:    map[string]*backend.WorkflowDescriptor{"wf-sdlc": wf},
		FallbackWorkflow: wf,
		Entry:            entry,
		Beat:             &backend.Beat{ID: "beat-1", State: "implementation", WorkflowID: "wf-sdlc"},
		PushEvent: func(evt session.TerminalEvent) {
			events = append(events, evt)
		},
	}

	record := OutcomeRecord{
		BeatID:       "beat-1",
		ClaimedStep:  "implementation",
		Success:      false,
		ExitCode:     1,
		PostExitState: "implementation",
	}

	HandleErrorExit(ctx, record, 1, "agent-a", "implementation", wf, sb)

	if !ctx.FailedAgentsPerQueueType["implementation"]["agent-a"] {
		t.Error("expected agent-a to be recorded as failed for implementation queue type")
	}
	if len(sb.rewinds) != 1 {
		t.Errorf("expected 1 rewind (invariant rollback), got %d", len(sb.rewinds))
	}
	if sb.rewinds[0].targetState != "ready_for_implementation" {
		t.Errorf("expected rollback to ready_for_implementation, got %s", sb.rewinds[0].targetState)
	}
}

func TestHandleErrorExit_NoRollbackWhenInQueueState(t *testing.T) {
	sb := &stubRollbackBackend{
		beats: map[string]*backend.Beat{
			"beat-1": {ID: "beat-1", State: "ready_for_implementation"},
		},
	}
	wf := makeRollbackWF()
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "sess-1"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := &TakeLoopContext{
		ID:               "sess-1",
		BeatID:           "beat-1",
		RepoPath:         "/repo",
		WorkflowsByID:    map[string]*backend.WorkflowDescriptor{"wf-sdlc": wf},
		FallbackWorkflow: wf,
		Entry:            entry,
		Beat:             &backend.Beat{ID: "beat-1", State: "ready_for_implementation", WorkflowID: "wf-sdlc"},
		PushEvent:        func(evt session.TerminalEvent) {},
	}

	record := OutcomeRecord{
		BeatID:       "beat-1",
		ClaimedStep:  "implementation",
		Success:      false,
		ExitCode:     1,
		PostExitState: "ready_for_implementation",
	}

	HandleErrorExit(ctx, record, 1, "agent-a", "ready_for_implementation", wf, sb)

	if !ctx.FailedAgentsPerQueueType["implementation"]["agent-a"] {
		t.Error("expected agent-a to be recorded as failed")
	}
	if len(sb.rewinds) != 0 {
		t.Errorf("expected 0 rewinds (beat already in queue state), got %d", len(sb.rewinds))
	}
}

func TestHandleSuccessExit_EnforcesInvariant(t *testing.T) {
	sb := &stubRollbackBackend{
		beats: map[string]*backend.Beat{
			"beat-1": {ID: "beat-1", State: "shipped"},
		},
	}
	wf := makeRollbackWF()
	finished := false
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "sess-1"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := &TakeLoopContext{
		ID:               "sess-1",
		BeatID:           "beat-1",
		RepoPath:         "/repo",
		WorkflowsByID:    map[string]*backend.WorkflowDescriptor{"wf-sdlc": wf},
		FallbackWorkflow: wf,
		Entry:            entry,
		Beat:             &backend.Beat{ID: "beat-1", State: "shipped", WorkflowID: "wf-sdlc"},
		PushEvent:        func(evt session.TerminalEvent) {},
		FinishSession: func(exitCode int) {
			finished = true
		},
		TakeIteration: &IterationCounter{Value: 1},
	}

	record := OutcomeRecord{
		BeatID:         "beat-1",
		Success:        true,
		ExitCode:       0,
		PostExitState:  "shipped",
	}

	HandleSuccessExit(ctx, record, 0, wf, sb)

	if !finished {
		t.Error("expected FinishSession to be called")
	}
}

func TestRollbackBeatState_BeatsMode(t *testing.T) {
	sb := &stubRollbackBackend{
		beats: map[string]*backend.Beat{
			"beat-1": {ID: "beat-1", State: "implementation"},
		},
	}

	err := RollbackBeatState("beat-1", "implementation", "ready_for_implementation", "/repo", "", "test rollback", sb)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(sb.rewinds) != 1 {
		t.Fatalf("expected 1 rewind, got %d", len(sb.rewinds))
	}
	if sb.rewinds[0].reason != "take_loop_rollback" {
		t.Errorf("expected reason 'take_loop_rollback', got %s", sb.rewinds[0].reason)
	}
}

func TestRollbackBeatState_KnotsMode(t *testing.T) {
	sb := &stubRollbackBackend{
		beats: map[string]*backend.Beat{
			"beat-1": {ID: "beat-1", State: "implementation"},
		},
	}

	err := RollbackBeatState("beat-1", "implementation", "ready_for_implementation", "/repo", "knots", "test knots rollback", sb)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(sb.rewinds) != 1 {
		t.Fatalf("expected 1 rewind, got %d", len(sb.rewinds))
	}
	if sb.rewinds[0].reason != "knots_rollback" {
		t.Errorf("expected reason 'knots_rollback', got %s", sb.rewinds[0].reason)
	}
}

func TestRollbackStepFailure_ActiveStateRollsBack(t *testing.T) {
	sb := &stubRollbackBackend{
		beats: map[string]*backend.Beat{
			"beat-1": {ID: "beat-1", State: "implementation", WorkflowID: "wf-sdlc"},
		},
	}
	wf := makeRollbackWF()
	var events []session.TerminalEvent
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "sess-1"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := &TakeLoopContext{
		ID:               "sess-1",
		BeatID:           "beat-1",
		RepoPath:         "/repo",
		WorkflowsByID:    map[string]*backend.WorkflowDescriptor{"wf-sdlc": wf},
		FallbackWorkflow: wf,
		Entry:            entry,
		Beat:             &backend.Beat{ID: "beat-1", State: "implementation", WorkflowID: "wf-sdlc"},
		PushEvent: func(evt session.TerminalEvent) {
			events = append(events, evt)
		},
	}

	_, err := RollbackStepFailure(ctx, sb, "agent-a")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(sb.rewinds) != 1 {
		t.Fatalf("expected 1 rewind, got %d", len(sb.rewinds))
	}
	if sb.rewinds[0].targetState != "ready_for_implementation" {
		t.Errorf("expected rollback to ready_for_implementation, got %s", sb.rewinds[0].targetState)
	}
	foundBanner := false
	for _, evt := range events {
		if evt.Type == "stdout" && len(evt.Content) > 0 {
			foundBanner = true
		}
	}
	if !foundBanner {
		t.Error("expected rollback banner event")
	}
}

func TestRollbackStepFailure_QueueStateNoRollback(t *testing.T) {
	sb := &stubRollbackBackend{
		beats: map[string]*backend.Beat{
			"beat-1": {ID: "beat-1", State: "ready_for_implementation", WorkflowID: "wf-sdlc"},
		},
	}
	wf := makeRollbackWF()
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "sess-1"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := &TakeLoopContext{
		ID:               "sess-1",
		BeatID:           "beat-1",
		RepoPath:         "/repo",
		WorkflowsByID:    map[string]*backend.WorkflowDescriptor{"wf-sdlc": wf},
		FallbackWorkflow: wf,
		Entry:            entry,
		Beat:             &backend.Beat{ID: "beat-1", State: "ready_for_implementation", WorkflowID: "wf-sdlc"},
		PushEvent:        func(evt session.TerminalEvent) {},
	}

	_, err := RollbackStepFailure(ctx, sb, "agent-a")
	if err != nil {
		t.Errorf("expected no error for queue state, got %v", err)
	}
	if len(sb.rewinds) != 0 {
		t.Errorf("expected 0 rewinds for queue state, got %d", len(sb.rewinds))
	}
}

func TestRollbackStepFailure_CannotResolveQueueState(t *testing.T) {
	wf := &backend.WorkflowDescriptor{
		ID:             "wf-minimal",
		States:         []string{"unknown_state"},
		TerminalStates: []string{},
		ActionStates:   []string{"unknown_state"},
		QueueStates:    []string{},
		Transitions:    []backend.WorkflowTransition{},
		StateOwners:   map[string]backend.ActionOwnerKind{"unknown_state": backend.ActionOwnerAgent},
	}
	sb := &stubRollbackBackend{
		beats: map[string]*backend.Beat{
			"beat-1": {ID: "beat-1", State: "unknown_state", WorkflowID: "wf-minimal"},
		},
	}
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "sess-1"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := &TakeLoopContext{
		ID:               "sess-1",
		BeatID:           "beat-1",
		RepoPath:         "/repo",
		WorkflowsByID:    map[string]*backend.WorkflowDescriptor{"wf-minimal": wf},
		FallbackWorkflow: wf,
		Entry:            entry,
		Beat:             &backend.Beat{ID: "beat-1", State: "unknown_state", WorkflowID: "wf-minimal"},
		PushEvent:        func(evt session.TerminalEvent) {},
	}

	_, err := RollbackStepFailure(ctx, sb, "agent-a")
	if err == nil {
		t.Error("expected error when queue state cannot be resolved")
	}
}

func TestEnforceQueueTerminalInvariant_SkipsOnFetchError(t *testing.T) {
	sb := &stubRollbackBackend{
		beats: map[string]*backend.Beat{},
	}
	wf := makeRollbackWF()
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "sess-1"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := &TakeLoopContext{
		ID:               "sess-1",
		BeatID:           "beat-missing",
		WorkflowsByID:    map[string]*backend.WorkflowDescriptor{"wf-sdlc": wf},
		FallbackWorkflow: wf,
		Entry:            entry,
		Beat:             &backend.Beat{ID: "beat-missing", State: "implementation"},
	}

	ok, err := EnforceQueueTerminalInvariant(ctx, sb)
	if err != nil {
		t.Errorf("expected no error on fetch failure, got %v", err)
	}
	if !ok {
		t.Error("expected invariant check to pass on fetch failure (assume ok)")
	}
}

func TestRollbackInvariantViolation_RollbackSucceeds(t *testing.T) {
	sb := &stubRollbackBackend{
		beats: map[string]*backend.Beat{
			"beat-1": {ID: "beat-1", State: "implementation"},
		},
	}
	wf := makeRollbackWF()
	var events []session.TerminalEvent
	ctx := &TakeLoopContext{
		ID:               "sess-1",
		BeatID:           "beat-1",
		RepoPath:         "/repo",
		WorkflowsByID:    map[string]*backend.WorkflowDescriptor{"wf-sdlc": wf},
		FallbackWorkflow: wf,
		PushEvent: func(evt session.TerminalEvent) {
			events = append(events, evt)
		},
	}
	current := &backend.Beat{ID: "beat-1", State: "implementation"}

	ok, err := RollbackInvariantViolation(ctx, current, wf, "[terminal-manager] [sess-1] [invariant]", sb)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !ok {
		t.Error("expected rollback to succeed")
	}
	if sb.rewinds[0].targetState != "ready_for_implementation" {
		t.Errorf("expected rollback to ready_for_implementation, got %s", sb.rewinds[0].targetState)
	}
}

func TestConcurrentAbortDuringRollback(t *testing.T) {
	sb := &stubRollbackBackend{
		beats: map[string]*backend.Beat{
			"beat-1": {ID: "beat-1", State: "implementation"},
		},
	}
	wf := makeRollbackWF()
	aborted := true
	finishCalled := false
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "sess-1"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := &TakeLoopContext{
		ID:               "sess-1",
		BeatID:           "beat-1",
		RepoPath:         "/repo",
		WorkflowsByID:    map[string]*backend.WorkflowDescriptor{"wf-sdlc": wf},
		FallbackWorkflow: wf,
		Entry:            entry,
		Beat:             &backend.Beat{ID: "beat-1", State: "implementation", WorkflowID: "wf-sdlc"},
		PushEvent:        func(evt session.TerminalEvent) {},
		SessionAborted: func() bool { return aborted },
		FinishSession: func(exitCode int) {
			finishCalled = true
		},
		TakeIteration: &IterationCounter{Value: 1},
	}

	record := OutcomeRecord{
		BeatID:       "beat-1",
		ClaimedStep:  "implementation",
		Success:      false,
		ExitCode:     1,
		PostExitState: "implementation",
	}

	HandleErrorExit(ctx, record, 1, "agent-a", "implementation", wf, sb)

	if !finishCalled {
		t.Error("expected FinishSession to be called even during abort")
	}
}

func TestHandleErrorExit_NoAlternativeAgent(t *testing.T) {
	sb := &stubRollbackBackend{
		beats: map[string]*backend.Beat{
			"beat-1": {ID: "beat-1", State: "ready_for_implementation"},
		},
	}
	wf := makeRollbackWF()
	finishCalled := false
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "sess-1"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := &TakeLoopContext{
		ID:               "sess-1",
		BeatID:           "beat-1",
		RepoPath:         "/repo",
		WorkflowsByID:    map[string]*backend.WorkflowDescriptor{"wf-sdlc": wf},
		FallbackWorkflow: wf,
		Entry:            entry,
		Beat:             &backend.Beat{ID: "beat-1", State: "ready_for_implementation", WorkflowID: "wf-sdlc"},
		PushEvent:        func(evt session.TerminalEvent) {},
		FinishSession: func(exitCode int) {
			finishCalled = true
		},
		TakeIteration: &IterationCounter{Value: 1},
	}

	record := OutcomeRecord{
		BeatID:                    "beat-1",
		ClaimedStep:               "implementation",
		Success:                   false,
		ExitCode:                  1,
		PostExitState:             "ready_for_implementation",
		AlternativeAgentAvailable: false,
	}

	HandleErrorExit(ctx, record, 1, "agent-a", "ready_for_implementation", wf, sb)

	if !finishCalled {
		t.Error("expected FinishSession to be called when no alternative agent")
	}
	if !ctx.FailedAgentsPerQueueType["implementation"]["agent-a"] {
		t.Error("expected agent-a to be recorded as failed")
	}
}

func TestRollbackBeatState_Error(t *testing.T) {
	sb := &stubRollbackBackend{
		beats: map[string]*backend.Beat{
			"beat-1": {ID: "beat-1", State: "implementation"},
		},
		rewindErr: fmt.Errorf("db connection failed"),
	}

	err := RollbackBeatState("beat-1", "implementation", "ready_for_implementation", "/repo", "", "test", sb)
	if err == nil {
		t.Error("expected error when Rewind fails")
	}
}

func TestRollbackStepFailure_RollbackError(t *testing.T) {
	sb := &stubRollbackBackend{
		beats: map[string]*backend.Beat{
			"beat-1": {ID: "beat-1", State: "implementation", WorkflowID: "wf-sdlc"},
		},
		rewindErr: fmt.Errorf("rollback failed"),
	}
	wf := makeRollbackWF()
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "sess-1"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := &TakeLoopContext{
		ID:               "sess-1",
		BeatID:           "beat-1",
		RepoPath:         "/repo",
		WorkflowsByID:    map[string]*backend.WorkflowDescriptor{"wf-sdlc": wf},
		FallbackWorkflow: wf,
		Entry:            entry,
		Beat:             &backend.Beat{ID: "beat-1", State: "implementation", WorkflowID: "wf-sdlc"},
		PushEvent:        func(evt session.TerminalEvent) {},
	}

	_, err := RollbackStepFailure(ctx, sb, "agent-a")
	if err == nil {
		t.Error("expected error when rollback fails")
	}
}

func TestMaxClaimsPerQueueTypeIsThree(t *testing.T) {
	if MaxClaimsPerQueueType != 3 {
		t.Errorf("expected MaxClaimsPerQueueType=3, got %d", MaxClaimsPerQueueType)
	}
}

func TestHandleTakeIterationClose_AbortedSession(t *testing.T) {
	sb := &stubRollbackBackend{
		beats: map[string]*backend.Beat{
			"beat-1": {ID: "beat-1", State: "implementation", WorkflowID: "wf-sdlc"},
		},
	}
	wf := makeRollbackWF()
	aborted := true
	finishCalled := false
	entry := &SessionEntry{
		Session:                  &TerminalSession{ID: "sess-1"},
		FailedAgentsPerQueueType: make(map[string]map[string]bool),
		ClaimsPerQueueType:       make(map[string]int),
		LastAgentPerQueueType:    make(map[string]string),
	}
	ctx := &TakeLoopContext{
		ID:               "sess-1",
		BeatID:           "beat-1",
		RepoPath:         "/repo",
		WorkflowsByID:    map[string]*backend.WorkflowDescriptor{"wf-sdlc": wf},
		FallbackWorkflow: wf,
		Entry:            entry,
		Beat:             &backend.Beat{ID: "beat-1", State: "implementation", WorkflowID: "wf-sdlc"},
		PushEvent:        func(evt session.TerminalEvent) {},
		SessionAborted: func() bool { return aborted },
		FinishSession: func(exitCode int) {
			finishCalled = true
		},
		TakeIteration: &IterationCounter{Value: 1},
	}

	HandleTakeIterationClose(ctx, 1, "agent-a", "Claude", "ready_for_implementation", sb)

	if !finishCalled {
		t.Error("expected FinishSession to be called for aborted session")
	}
}