package terminal

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/orchestration"
	"github.com/gabrielassisxyz/kernl/internal/session"
)

type NextTakeResult struct {
	Prompt        string
	BeadState     string
	AgentOverride string
	MaxClaims     int
}

type RetryDecision struct {
	ShouldRetry    bool
	NextPrompt     *NextTakeResult
	IterationAgent string
}

type OutcomeRecord struct {
	BeadID                    string    `json:"beadId"`
	SessionID                 string    `json:"sessionId"`
	Iteration                 int       `json:"iteration"`
	Success                   bool      `json:"success"`
	ExitCode                  int       `json:"exitCode"`
	ClaimedState              string    `json:"claimedState"`
	ClaimedStep               string    `json:"claimedStep,omitempty"`
	PostExitState             string    `json:"postExitState"`
	RolledBack                bool      `json:"rolledBack"`
	AlternativeAgentAvailable bool      `json:"alternativeAgentAvailable"`
	AgentID                   string    `json:"agentId,omitempty"`
	AgentLabel                string    `json:"agentLabel,omitempty"`
	Timestamp                 time.Time `json:"timestamp"`
	DurationMs                int64     `json:"durationMs,omitempty"`
}

type TakeLoopContext struct {
	ID                       string
	BeadID                   string
	Bead                     *backend.Bead
	RepoPath                 string
	ResolvedRepoPath         string
	WorkflowsByID            map[string]*backend.WorkflowDescriptor
	FallbackWorkflow         *backend.WorkflowDescriptor
	AgentID                  string
	AgentLabel               string
	AgentCommand             string
	AgentProvider            string
	AgentModel               string
	AgentVersion             string
	AgentFlavor              string
	Entry                    *SessionEntry
	InteractionLog           InteractionLog
	PushEvent                func(evt session.TerminalEvent)
	FinishSession            func(exitCode int)
	SessionAborted           func() bool
	TakeIteration            *IterationCounter
	ClaimsPerQueueType       map[string]int
	LastAgentPerQueueType    map[string]string
	FailedAgentsPerQueueType map[string]map[string]bool
	FollowUpAttempts         *FollowUpCounter
	MemoryManagerType        string
	ClaimedAt                *time.Time
}

type IterationCounter struct {
	Value int
}

type FollowUpCounter struct {
	Count     int
	LastState string
}

type BackendPortProvider func() backend.BackendPort

func NewTakeLoopContext(entry *SessionEntry, bead *backend.Bead, repoPath string) *TakeLoopContext {
	return &TakeLoopContext{
		ID:                       entry.Session.ID,
		BeadID:                   bead.ID,
		Bead:                     bead,
		RepoPath:                 repoPath,
		ResolvedRepoPath:         repoPath,
		Entry:                    entry,
		TakeIteration:            &IterationCounter{Value: 1},
		ClaimsPerQueueType:       entry.ClaimsPerQueueType,
		LastAgentPerQueueType:    entry.LastAgentPerQueueType,
		FailedAgentsPerQueueType: entry.FailedAgentsPerQueueType,
		FollowUpAttempts:         &FollowUpCounter{},
	}
}

func (ctx *TakeLoopContext) RecordFailedAgent(queueType string, agentID string) {
	if queueType == "" || agentID == "" {
		return
	}
	if ctx.FailedAgentsPerQueueType == nil {
		ctx.FailedAgentsPerQueueType = make(map[string]map[string]bool)
	}
	failed, ok := ctx.FailedAgentsPerQueueType[queueType]
	if !ok {
		failed = make(map[string]bool)
		ctx.FailedAgentsPerQueueType[queueType] = failed
	}
	failed[agentID] = true
}

func (ctx *TakeLoopContext) IncrementClaims(queueType string) int {
	if ctx.ClaimsPerQueueType == nil {
		ctx.ClaimsPerQueueType = make(map[string]int)
	}
	ctx.ClaimsPerQueueType[queueType]++
	return ctx.ClaimsPerQueueType[queueType]
}

func (ctx *TakeLoopContext) GetClaimCount(queueType string) int {
	if ctx.ClaimsPerQueueType == nil {
		return 0
	}
	return ctx.ClaimsPerQueueType[queueType]
}

func (ctx *TakeLoopContext) ComputeExclusions(queueType string, isReview bool, currentAgentID string, priorActionAgentID string, lastErrorAgentID string) []string {
	exclusions := make([]string, 0)

	if ctx.FailedAgentsPerQueueType != nil {
		for agentID := range ctx.FailedAgentsPerQueueType[queueType] {
			exclusions = append(exclusions, agentID)
		}
	}

	if lastErrorAgentID != "" {
		found := false
		for _, id := range exclusions {
			if id == lastErrorAgentID {
				found = true
				break
			}
		}
		if !found {
			exclusions = append(exclusions, lastErrorAgentID)
		}
	}

	if isReview {
		if currentAgentID != "" {
			exclusions = append(exclusions, currentAgentID)
		}
		if priorActionAgentID != "" {
			exclusions = append(exclusions, priorActionAgentID)
		}
	} else {
		if last, ok := ctx.LastAgentPerQueueType[queueType]; ok && last != "" {
			exclusions = append(exclusions, last)
		}
	}

	return exclusions
}

func ClassifyIterationSuccess(exitCode int, claimedState string, postExitState string, wf *backend.WorkflowDescriptor) bool {
	if exitCode != 0 {
		return false
	}
	if postExitState == "unknown" {
		return false
	}

	targets := ExpectedSuccessTargets(wf, claimedState)
	if len(targets) > 0 {
		for _, t := range targets {
			if postExitState == t {
				return true
			}
		}
		return false
	}

	step := FindStepForState(wf, claimedState)
	if step == nil {
		return false
	}

	nextQueue := NextQueueStateFromState(wf, step.State)
	if nextQueue != "" && postExitState == nextQueue {
		return true
	}

	priorQueue := PriorQueueStateFromState(wf, step.State)
	if priorQueue != "" && postExitState == priorQueue {
		return true
	}

	return false
}

func ExpectedSuccessTargets(wf *backend.WorkflowDescriptor, claimedState string) []string {
	actionState := orchestration.WorkflowActionStateForState(wf, claimedState)
	if actionState == "" {
		actionState = claimedState
	}

	var targets []string
	for _, t := range wf.Transitions {
		if t.From != actionState {
			continue
		}
		phase := orchestration.WorkflowStatePhase(wf, t.To)
		if (phase == orchestration.PhaseQueued || phase == orchestration.PhaseTerminal) && t.To != claimedState {
			targets = append(targets, t.To)
		}
	}

	return targets
}

func FindStepForState(wf *backend.WorkflowDescriptor, state string) *orchestration.Step {
	resolved, err := orchestration.ResolveStep(orchestration.WfToSteps(wf), state)
	if err != nil {
		return nil
	}
	return resolved
}

func NextQueueStateFromState(wf *backend.WorkflowDescriptor, state string) string {
	for _, t := range wf.Transitions {
		if t.From == state {
			phase := orchestration.WorkflowStatePhase(wf, t.To)
			if phase == orchestration.PhaseQueued && t.To != state {
				return t.To
			}
		}
	}
	return ""
}

func PriorQueueStateFromState(wf *backend.WorkflowDescriptor, state string) string {
	for _, t := range wf.Transitions {
		if t.To == state {
			phase := orchestration.WorkflowStatePhase(wf, t.From)
			if phase == orchestration.PhaseQueued {
				return t.From
			}
		}
	}
	return ""
}

func EnforceQueueTerminalInvariant(ctx *TakeLoopContext, backendPort backend.BackendPort) (bool, error) {
	tag := fmt.Sprintf("[terminal-manager] [%s] [invariant]", ctx.ID)

	current, err := backendPort.Get(ctx.BeadID, ctx.RepoPath)
	if err != nil {
		slog.Warn(fmt.Sprintf("%s failed to fetch bead state for invariant check", tag), "error", err)
		return true, nil
	}

	wf := orchestration.ResolveWorkflowForBead(current, ctx.WorkflowsByID, ctx.FallbackWorkflow)

	if IsIsQueueOrTerminalWithWorkflow(current.State, wf) {
		slog.Info(fmt.Sprintf("%s bead=%s state=%s — invariant satisfied", tag, ctx.BeadID, current.State))
		return true, nil
	}

	return RollbackInvariantViolation(ctx, current, wf, tag, backendPort)
}

func IsIsQueueOrTerminalWithWorkflow(state string, wf *backend.WorkflowDescriptor) bool {
	if orchestration.IsQueueOrTerminal(state) {
		return true
	}
	phase := orchestration.WorkflowStatePhase(wf, state)
	return phase == orchestration.PhaseQueued || phase == orchestration.PhaseTerminal
}

func RollbackInvariantViolation(ctx *TakeLoopContext, current *backend.Bead, wf *backend.WorkflowDescriptor, tag string, backendPort backend.BackendPort) (bool, error) {
	slog.Warn(fmt.Sprintf("%s [WARN] bead=%s state=%s — VIOLATION: action state on exit", tag, ctx.BeadID, current.State))

	ctx.PushEvent(session.TerminalEvent{
		Type:   "stdout",
		BeadID: ctx.BeadID,
		Content: fmt.Sprintf("\x1b[33m--- Invariant violation: bead %s in action state \"%s\" after agent exit ---\x1b[0m\n", ctx.BeadID, current.State),
		Time:   time.Now().UnixMilli(),
	})

	rollbackState := orchestration.WorkflowQueueStateForState(wf, current.State)
	if rollbackState == "" {
		slog.Error(fmt.Sprintf("%s cannot resolve queue state for \"%s\" — skipping rollback", tag, current.State))
		return false, nil
	}

	slog.Warn(fmt.Sprintf("%s [WARN] rolling back from \"%s\" to \"%s\"", tag, current.State, rollbackState))

	err := backendPort.Rewind(ctx.BeadID, rollbackState, "take_loop_invariant_rollback", ctx.RepoPath)
	if err != nil {
		slog.Error(fmt.Sprintf("%s rollback failed: %v", tag, err))
		ctx.PushEvent(session.TerminalEvent{
			Type:   "stderr",
			BeadID: ctx.BeadID,
			Content: fmt.Sprintf("Invariant enforcement: failed to roll back %s from %s to %s: %v\n", ctx.BeadID, current.State, rollbackState, err),
			Time:   time.Now().UnixMilli(),
		})
		return false, err
	}

	ctx.PushEvent(session.TerminalEvent{
		Type:   "stdout",
		BeadID: ctx.BeadID,
		Content: fmt.Sprintf("\x1b[33m--- Invariant fix: rolled back %s from \"%s\" to \"%s\" ---\x1b[0m\n", ctx.BeadID, current.State, rollbackState),
		Time:   time.Now().UnixMilli(),
	})
	slog.Warn(fmt.Sprintf("%s [WARN] rollback succeeded for %s", tag, ctx.BeadID))

	refreshed, err := backendPort.Get(ctx.BeadID, ctx.RepoPath)
	if err != nil {
		slog.Error(fmt.Sprintf("%s failed to verify rollback", tag), "error", err)
		return false, err
	}
	refreshedWf := orchestration.ResolveWorkflowForBead(refreshed, ctx.WorkflowsByID, ctx.FallbackWorkflow)
	if IsIsQueueOrTerminalWithWorkflow(refreshed.State, refreshedWf) {
		slog.Info(fmt.Sprintf("%s bead=%s state=%s — invariant satisfied after rollback", tag, ctx.BeadID, refreshed.State))
		return true, nil
	}

	slog.Error(fmt.Sprintf("%s bead=%s state=%s — STILL VIOLATED after rollback", tag, ctx.BeadID, refreshed.State))
	return false, fmt.Errorf("bead %s still in action state %s after rollback", ctx.BeadID, refreshed.State)
}

const MaxClaimsPerQueueType = 3

func BuildOutcomeRecord(ctx *TakeLoopContext, iterationAgentID string, iterationAgentLabel string, claimedState string, claimedStep string, exitCode int, postExitState string, altAvailable bool, success bool) OutcomeRecord {
	var durationMs int64
	if ctx.ClaimedAt != nil {
		durationMs = time.Since(*ctx.ClaimedAt).Milliseconds()
	}
	return OutcomeRecord{
		BeadID:                    ctx.BeadID,
		SessionID:                 ctx.ID,
		Iteration:                 ctx.TakeIteration.Value,
		Success:                   success,
		ExitCode:                  exitCode,
		ClaimedState:              claimedState,
		ClaimedStep:               claimedStep,
		PostExitState:             postExitState,
		RolledBack:                false,
		AlternativeAgentAvailable: altAvailable,
		AgentID:                   iterationAgentID,
		AgentLabel:                iterationAgentLabel,
		Timestamp:                 time.Now().UTC(),
		DurationMs:                durationMs,
	}
}

func HandleTakeIterationClose(ctx *TakeLoopContext, exitCode int, iterationAgentID string, iterationAgentLabel string, claimedState string, backendPort backend.BackendPort) error {
	tag := fmt.Sprintf("[terminal-manager] [%s] [take-loop]", ctx.ID)
	code := exitCode

	if ctx.SessionAborted != nil && ctx.SessionAborted() {
		slog.Info(fmt.Sprintf("%s STOP: session was aborted", tag))
		if ctx.FinishSession != nil {
			ctx.FinishSession(code)
		}
		return nil
	}

	postExitState := "unknown"
	refreshed, err := backendPort.Get(ctx.BeadID, ctx.RepoPath)
	if err == nil && refreshed != nil {
		postExitState = refreshed.State
		slog.Info(fmt.Sprintf("%s post-close bead state: bead=%s state=%s", tag, ctx.BeadID, postExitState))

		ctx.PushEvent(session.TerminalEvent{
			Type:    "beat_state_observed",
			BeadID:   ctx.BeadID,
			Content:  fmt.Sprintf(`{"beadId":"%s","state":"%s","reason":"post_exit_state_observed"}`, ctx.BeadID, postExitState),
			Time:    time.Now().UnixMilli(),
		})
	}

	wf := orchestration.ResolveWorkflowForBead(ctx.Bead, ctx.WorkflowsByID, ctx.FallbackWorkflow)
	resolvedStep, _ := orchestration.ResolveStep(orchestration.WfToSteps(wf), claimedState)
	claimedStep := ""
	if resolvedStep != nil {
		claimedStep = resolvedStep.State
	}

	success := ClassifyIterationSuccess(code, claimedState, postExitState, wf)

	altAvailable := CheckAlternativeAgent(ctx, iterationAgentID, resolvedStep, wf)

	record := BuildOutcomeRecord(ctx, iterationAgentID, iterationAgentLabel, claimedState, claimedStep, code, postExitState, altAvailable, success)

	if code != 0 {
		return HandleErrorExit(ctx, record, code, iterationAgentID, postExitState, wf, backendPort)
	}

	return HandleSuccessExit(ctx, record, code, wf, backendPort)
}

func CheckAlternativeAgent(ctx *TakeLoopContext, iterationAgentID string, resolvedStep *orchestration.Step, wf *backend.WorkflowDescriptor) bool {
	if resolvedStep == nil || iterationAgentID == "" {
		return false
	}

	queueType := ""
	if wf.QueueActions != nil {
		if qt, ok := wf.QueueActions[resolvedStep.State]; ok {
			queueType = qt
		}
	}
	if queueType == "" {
		queueType = resolvedStep.State
	}

	exclusions := ctx.ComputeExclusions(queueType, orchestration.IsReviewStep(resolvedStep), iterationAgentID, "", "")
	if len(exclusions) == 0 {
		return true
	}
	return len(exclusions) < len(wf.States)
}

func HandleErrorExit(ctx *TakeLoopContext, record OutcomeRecord, code int, iterationAgentID string, postExitState string, wf *backend.WorkflowDescriptor, backendPort backend.BackendPort) error {
	tag := fmt.Sprintf("[terminal-manager] [%s] [take-loop]", ctx.ID)
	slog.Info(fmt.Sprintf("%s non-zero exit code=%d — attempting rollback and retry", tag, code))

	queueType := record.ClaimedStep
	if queueType == "" {
		queueType = "unknown"
	}
	ctx.RecordFailedAgent(queueType, iterationAgentID)

	rollbackNeeded := false
	if postExitState != "unknown" {
		refreshed, fetchErr := backendPort.Get(ctx.BeadID, ctx.RepoPath)
		if fetchErr == nil && refreshed != nil {
			rollbackNeeded = !IsIsQueueOrTerminalWithWorkflow(refreshed.State, wf)
		}
	}

	invariantOk, _ := EnforceQueueTerminalInvariant(ctx, backendPort)
	record.RolledBack = rollbackNeeded && invariantOk

	if ctx.InteractionLog != nil {
		ctx.InteractionLog.LogBeadState(ctx.BeadID, postExitState, "after_prompt", fmt.Sprintf("iteration=%d", record.Iteration))
	}

	slog.Info(fmt.Sprintf("%s error outcome: success=%v rolledBack=%v altAvailable=%v", tag, record.Success, record.RolledBack, record.AlternativeAgentAvailable))

	if !record.AlternativeAgentAvailable {
		if iterationAgentID != "" {
			slog.Info(fmt.Sprintf("%s STOP: no alternative agent available for retry", tag))
		} else {
			slog.Info(fmt.Sprintf("%s STOP: no agentId for error retry exclusion", tag))
		}
	} else {
		slog.Info(fmt.Sprintf("%s alternative agent available — would retry with exclusion of agent=%s", tag, iterationAgentID))
		slog.Info(fmt.Sprintf("%s STOP: retry spawning not yet wired (buildNextTakePrompt pending)", tag))
	}

	if ctx.FinishSession != nil {
		ctx.FinishSession(code)
	}
	return nil
}

func HandleSuccessExit(ctx *TakeLoopContext, record OutcomeRecord, code int, wf *backend.WorkflowDescriptor, backendPort backend.BackendPort) error {
	tag := fmt.Sprintf("[terminal-manager] [%s] [take-loop]", ctx.ID)

	if ctx.InteractionLog != nil {
		ctx.InteractionLog.LogEnd(code, "completed")
	}

	slog.Info(fmt.Sprintf("%s evaluating next iteration (code=0, iteration=%d)", tag, ctx.TakeIteration.Value))

	_, _ = EnforceQueueTerminalInvariant(ctx, backendPort)

	if ctx.FinishSession != nil {
		ctx.FinishSession(code)
	}
	return nil
}

func RollbackBeadState(beadID string, fromState string, toState string, repoPath string, memoryManagerType string, reason string, backendPort backend.BackendPort) error {
	tag := "[terminal-manager] [rollback]"

	if memoryManagerType == "knots" {
		slog.Info(fmt.Sprintf("%s using knots rollback for bead=%s from=%s to=%s", tag, beadID, fromState, toState))
		return backendPort.Rewind(beadID, toState, "knots_rollback", repoPath)
	}

	slog.Info(fmt.Sprintf("%s rolling back bead=%s from=%s to=%s reason=%q", tag, beadID, fromState, toState, reason))
	return backendPort.Rewind(beadID, toState, "take_loop_rollback", repoPath)
}

func RollbackStepFailure(ctx *TakeLoopContext, backendPort backend.BackendPort, failedAgentLabel string) (*NextTakeResult, error) {
	tag := fmt.Sprintf("[terminal-manager] [%s] [step-failure]", ctx.ID)

	current, err := backendPort.Get(ctx.BeadID, ctx.RepoPath)
	if err != nil {
		slog.Error(fmt.Sprintf("%s failed to fetch bead for step-failure rollback", tag), "error", err)
		return nil, fmt.Errorf("fetch bead for step-failure rollback: %w", err)
	}

	wf := orchestration.ResolveWorkflowForBead(current, ctx.WorkflowsByID, ctx.FallbackWorkflow)
	if wf == nil {
		slog.Error(fmt.Sprintf("%s no workflow for bead=%s", tag, ctx.BeadID))
		return nil, fmt.Errorf("no workflow for bead %s", ctx.BeadID)
	}

	phase := orchestration.WorkflowStatePhase(wf, current.State)
	if phase != orchestration.PhaseActive {
		slog.Info(fmt.Sprintf("%s bead=%s state=%s is not active phase — no step-failure rollback needed", tag, ctx.BeadID, current.State))
		return nil, nil
	}

	step, stepErr := orchestration.ResolveStep(orchestration.WfToSteps(wf), current.State)
	if stepErr != nil || step == nil {
		slog.Error(fmt.Sprintf("%s cannot resolve step for state=%s", tag, current.State))
		return nil, nil
	}

	ownerKind := wf.StateOwners[current.State]
	if ownerKind != backend.ActionOwnerAgent {
		slog.Info(fmt.Sprintf("%s state=%s owner=%s is not agent-owned — no step-failure rollback", tag, current.State, ownerKind))
		return nil, nil
	}

	rollbackState := orchestration.WorkflowQueueStateForState(wf, current.State)
	if rollbackState == "" {
		slog.Error(fmt.Sprintf("%s cannot resolve queue state for %s — cannot rollback", tag, current.State))
		ctx.PushEvent(session.TerminalEvent{
			Type:   "stderr",
			BeadID: ctx.BeadID,
			Content: fmt.Sprintf("\x1b[31mKERNL DISPATCH FAILURE: cannot resolve queue state for \"%s\" — cannot rollback step failure\x1b[0m\n", current.State),
			Time:   time.Now().UnixMilli(),
		})
		return nil, fmt.Errorf("cannot resolve queue state for %s", current.State)
	}

	reason := fmt.Sprintf("take-loop: rolled back from %s to %s — agent left bead in action state", current.State, rollbackState)

	ctx.PushEvent(session.TerminalEvent{
		Type:   "stdout",
		BeadID: ctx.BeadID,
		Content: fmt.Sprintf("\x1b[33m--- Step failure: rolling back %s from \"%s\" to \"%s\" ---\x1b[0m\n", ctx.BeadID, current.State, rollbackState),
		Time:   time.Now().UnixMilli(),
	})

	err = RollbackBeadState(ctx.BeadID, current.State, rollbackState, ctx.RepoPath, ctx.MemoryManagerType, reason, backendPort)
	if err != nil {
		slog.Error(fmt.Sprintf("%s step-failure rollback failed: %v", tag, err))
		ctx.PushEvent(session.TerminalEvent{
			Type:   "stderr",
			BeadID: ctx.BeadID,
			Content: fmt.Sprintf("Step failure rollback failed for %s: %v\n", ctx.BeadID, err),
			Time:   time.Now().UnixMilli(),
		})
		return nil, fmt.Errorf("step-failure rollback: %w", err)
	}

	refreshed, err := backendPort.Get(ctx.BeadID, ctx.RepoPath)
	if err != nil {
		slog.Error(fmt.Sprintf("%s failed to re-fetch bead after step-failure rollback", tag), "error", err)
		return nil, fmt.Errorf("re-fetch after rollback: %w", err)
	}

	refreshedWf := orchestration.ResolveWorkflowForBead(refreshed, ctx.WorkflowsByID, ctx.FallbackWorkflow)
	if !IsIsQueueOrTerminalWithWorkflow(refreshed.State, refreshedWf) {
		slog.Error(fmt.Sprintf("%s bead=%s state=%s — STILL in action state after step-failure rollback", tag, ctx.BeadID, refreshed.State))
		return nil, fmt.Errorf("bead %s still in action state %s after step-failure rollback", ctx.BeadID, refreshed.State)
	}

	slog.Info(fmt.Sprintf("%s step-failure rollback succeeded: bead=%s now in state=%s", tag, ctx.BeadID, refreshed.State))

	ctx.Bead = refreshed
	ctx.PushEvent(session.TerminalEvent{
		Type:   "stdout",
		BeadID: ctx.BeadID,
		Content: fmt.Sprintf("\x1b[33m--- Step failure resolved: %s rolled back to \"%s\" ---\x1b[0m\n", ctx.BeadID, refreshed.State),
		Time:   time.Now().UnixMilli(),
	})

	return nil, nil
}