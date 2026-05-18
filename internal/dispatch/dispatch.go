package dispatch

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/orchestration"
	"github.com/gabrielassisxyz/kernl/internal/session"
)

type DispatchArgs struct {
	Ctx                   *TakeLoopDispatchContext
	Settings              *config.Settings
	Workflow              *backend.WorkflowDescriptor
	State                 string
	PoolKey               string
	QueueType             string
	ExcludeAgentIDs       []string
	IsErrorRetry          bool
	StepFailureRollback   bool
	IsReview              bool
	PriorAction           string
	FailedAgentID         string
	MaxClaims             int
	StepAgentTracker      *StepAgentTracker
}

type TakeLoopDispatchContext struct {
	ID                       string
	BeadID                   string
	PushEvent                func(evt session.TerminalEvent)
	FinishSession            func(exitCode int)
	ClaimsPerQueueType       map[string]int
	LastAgentPerQueueType    map[string]string
	FailedAgentsPerQueueType map[string]map[string]bool
}

type DispatchResult struct {
	StepAgentOverride *config.AgentConfig
	StepAgentID       string
	MaxClaims         int
	Stopped           bool
	FallbackUsed      bool
}

var DispatchStop = DispatchResult{Stopped: true}

func RunDispatch(a DispatchArgs) DispatchResult {
	agents := a.Settings.Agents
	pools := a.Settings.Pools
	beadID := ""
	if a.Ctx != nil {
		beadID = a.Ctx.BeadID
	}

	_, ok := pools[a.PoolKey]
	if !ok {
		err := NewDispatchFailureError(a.PoolKey, beadID, "pool config", "add settings.pools."+a.PoolKey+" with agent entries")
		emitDispatchFailure(a.Ctx, err)
		return DispatchStop
	}

	selected, err := ResolveDispatchAgent(a.Workflow.QueueActions, a.State, agents, pools, a.ExcludeAgentIDs...)
	if err != nil {
		var dfe *DispatchFailureError
		if errors.As(err, &dfe) {
			if isPoolEmptyError(dfe) {
				if a.IsReview && a.PriorAction != "" {
					fallback := RetryWithoutCrossAgentExclusion(a, agents, pools)
					if fallback != nil {
						return *fallback
					}
				}
				emitDispatchFailure(a.Ctx, dfe)
				return DispatchStop
			}
			emitDispatchFailure(a.Ctx, dfe)
			return DispatchStop
		}
		emitDispatchFailureMsg(a.Ctx, a.PoolKey, beadID, err.Error())
		return DispatchStop
	}

	if selected != nil && selected.ID != "" {
		a.StepAgentTracker.Record(beadID, a.PoolKey, selected.ID)
	}

	logSelection(a.Ctx, a.PoolKey, selected.Config, a.IsErrorRetry, a.IsReview, a.ExcludeAgentIDs)

	return DispatchResult{
		StepAgentOverride: selected.Config,
		StepAgentID:       selected.ID,
		MaxClaims:         a.MaxClaims,
	}
}

func findAgentID(agents map[string]config.AgentConfig, cfg *config.AgentConfig) string {
	for id, ac := range agents {
		if ac.Command == cfg.Command && ac.Label == cfg.Label && ac.Provider == cfg.Provider {
			return id
		}
	}
	return ""
}

func RetryWithoutCrossAgentExclusion(a DispatchArgs, agents map[string]config.AgentConfig, pools map[string]config.PoolConfig) *DispatchResult {
	beadID := ""
	if a.Ctx != nil {
		beadID = a.Ctx.BeadID
	}

	priorAgent := ""
	if a.PriorAction != "" && a.StepAgentTracker != nil {
		if agentID, ok := a.StepAgentTracker.Get(beadID, a.PriorAction); ok {
			priorAgent = agentID
		}
	}

	hardExcluded := make([]string, 0)
	if a.Ctx != nil && a.Ctx.FailedAgentsPerQueueType != nil {
		for id := range a.Ctx.FailedAgentsPerQueueType[a.QueueType] {
			hardExcluded = append(hardExcluded, id)
		}
	}
	if a.FailedAgentID != "" {
		found := false
		for _, id := range hardExcluded {
			if id == a.FailedAgentID {
				found = true
				break
			}
		}
		if !found {
			hardExcluded = append(hardExcluded, a.FailedAgentID)
		}
	}

	selected, err := ResolveDispatchAgent(a.Workflow.QueueActions, a.State, agents, pools, hardExcluded...)
	if err != nil {
		return nil
	}

	if selected != nil {
		AnnounceCrossAgentFallback(a.Ctx, selected.Config, priorAgent, a.PoolKey)
		if selected.ID != "" && a.StepAgentTracker != nil {
			a.StepAgentTracker.Record(beadID, a.PoolKey, selected.ID)
		}
	}

	return &DispatchResult{
		StepAgentOverride: selected.Config,
		StepAgentID:       selected.ID,
		MaxClaims:         a.MaxClaims,
		FallbackUsed:      true,
	}
}

func AnnounceCrossAgentFallback(ctx *TakeLoopDispatchContext, selected *config.AgentConfig, priorAgent string, poolKey string) {
	selectedLabel := agentLabel(selected)
	priorLabel := priorAgent
	if priorLabel == "" {
		priorLabel = "the action-step agent"
	}
	tag := fmt.Sprintf("[terminal-manager] [%s] [take-loop]", ctx.ID)

	slog.Warn(fmt.Sprintf("%s cross-agent review fallback: pool=%q only %q remained after exclusions; relaxing cross-agent invariant (prior action agent=%q) rather than stalling the take", tag, poolKey, selectedLabel, priorLabel))

	ctx.PushEvent(session.TerminalEvent{
		Type:    "stderr",
		BeadID:  ctx.BeadID,
		Content: fmt.Sprintf("\x1b[33m--- Cross-agent review fallback: every other agent in pool %q was excluded (hard-failed or unavailable). Letting %q review their own %s output rather than stopping the take. Review the result carefully. ---\x1b[0m\n", poolKey, selectedLabel, poolKey),
		Time:    time.Now().UnixMilli(),
	})

	ctx.PushEvent(session.TerminalEvent{
		Type:    "agent_failure",
		BeadID:  ctx.BeadID,
		Content: fmt.Sprintf(`{"kind":"cross_agent_review_fallback","message":"Pool %q had only %q available after exclusions. Falling back to same-agent review (prior action agent=%q). Review the result.","beadId":"%s"}`, poolKey, selectedLabel, priorLabel, ctx.BeadID),
		Time:    time.Now().UnixMilli(),
	})
}

func isPoolEmptyError(dfe *DispatchFailureError) bool {
	return dfe.Missing == "non-excluded pool agents" || dfe.Missing == "pool agents with positive weight"
}

func agentLabel(cfg *config.AgentConfig) string {
	if cfg == nil {
		return "(none)"
	}
	if cfg.Label != "" {
		return cfg.Label
	}
	if cfg.AgentName != "" {
		return cfg.AgentName
	}
	return cfg.Command
}

func emitDispatchFailure(ctx *TakeLoopDispatchContext, err *DispatchFailureError) {
	banner := fmt.Sprintf("\x1b[31mKERNL DISPATCH FAILURE: %s (pool=%s bead=%s). Fix: %s\x1b[0m\n", err.Missing, err.PoolKey, err.BeadID, err.Fix)
	ctx.PushEvent(session.TerminalEvent{
		Type:    "stderr",
		BeadID:  ctx.BeadID,
		Content: banner,
		Time:    time.Now().UnixMilli(),
	})
	slog.Error("dispatch failure", "error", err.Error())
}

func emitDispatchFailureMsg(ctx *TakeLoopDispatchContext, poolKey, beadID, msg string) {
	banner := fmt.Sprintf("\x1b[31mKERNL DISPATCH FAILURE: dispatch failed for pool=%s bead=%s: %s\x1b[0m\n", poolKey, beadID, msg)
	ctx.PushEvent(session.TerminalEvent{
		Type:    "stderr",
		BeadID:  ctx.BeadID,
		Content: banner,
		Time:    time.Now().UnixMilli(),
	})
}

func logSelection(ctx *TakeLoopDispatchContext, poolKey string, selected *config.AgentConfig, isErrorRetry, isReview bool, excluded []string) {
	tag := fmt.Sprintf("[terminal-manager] [%s] [take-loop]", ctx.ID)
	reason := "pool selection"
	if isErrorRetry {
		reason = "error retry"
	} else if isReview {
		reason = "cross-agent review"
	}
	selectedLabel := "(none)"
	if selected != nil {
		selectedLabel = agentLabel(selected)
	}
	excludedStr := "none"
	if len(excluded) > 0 {
		excludedStr = joinStrings(excluded, ", ")
	}
	slog.Info(fmt.Sprintf("%s %s: pool=%q selected=%q (excluded: %s)", tag, reason, poolKey, selectedLabel, excludedStr))
}

func joinStrings(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	result := ss[0]
	for _, s := range ss[1:] {
		result += sep + s
	}
	return result
}

func IsReviewQueueState(wf *backend.WorkflowDescriptor, queueType string) bool {
	if wf == nil {
		return false
	}
	// queueType is the action/pool key (e.g. "plan_review").
	// Check if the queue state that maps to this action is a review queue state.
	if wf.QueueActions != nil {
		for queueState, action := range wf.QueueActions {
			if action == queueType {
				for _, rqs := range wf.ReviewQueueStates {
					if rqs == queueState {
						return true
					}
				}
			}
		}
	}
	// Also check if the Step for this action is a review kind.
	steps := orchestration.WfToSteps(wf)
	step, err := orchestration.ResolveStep(steps, queueType)
	if err == nil && step != nil {
		return orchestration.IsReviewStep(step)
	}
	return false
}

func PriorActionStep(wf *backend.WorkflowDescriptor, queueType string) string {
	if wf == nil || wf.QueueActions == nil {
		return ""
	}
	// queueType is the pool key, which is the action state name (e.g. "plan_review").
	// Find the queue state that maps to this action (e.g. ready_for_plan_review -> plan_review).
	var queueState string
	for qs, action := range wf.QueueActions {
		if action == queueType {
			queueState = qs
			break
		}
	}
	if queueState == "" {
		return ""
	}
	// Find the transition where something transitions INTO this queue state.
	// The FROM state of that transition is the prior action step (e.g. "planning").
	for _, t := range wf.Transitions {
		if t.To == queueState {
			phase := orchestration.WorkflowStatePhase(wf, t.From)
			if phase == orchestration.PhaseActive {
				return t.From
			}
		}
	}
	return ""
}

func DerivePoolKey(wf *backend.WorkflowDescriptor, state string) string {
	if wf == nil || wf.QueueActions == nil {
		return ""
	}
	if pk, ok := wf.QueueActions[state]; ok {
		return pk
	}
	return state
}

func SelectStepAgent(
	ctx *TakeLoopContext,
	wf *backend.WorkflowDescriptor,
	state string,
	queueType string,
	agents map[string]config.AgentConfig,
	pools map[string]config.PoolConfig,
	settings *config.Settings,
	stepAgentTracker *StepAgentTracker,
	stepFailureRollback bool,
	lastErrorAgentID string,
) DispatchResult {
	poolKey := DerivePoolKey(wf, state)
	if poolKey == "" {
		return DispatchResult{MaxClaims: MaxClaimsDefault}
	}

	maxClaims := MaxClaimsDefault

	failedAgentID := lastErrorAgentID
	if stepFailureRollback && ctx != nil && ctx.AgentID != "" {
		failedAgentID = ctx.AgentID
	}
	isErrorRetry := lastErrorAgentID != "" && !stepFailureRollback

	isReview := IsReviewQueueState(wf, queueType)
	priorAction := ""
	if isReview {
		priorAction = PriorActionStep(wf, queueType)
	}

	excludeAgentIDs := computeStepExclusions(ctx, queueType, isReview, failedAgentID, priorAction, stepAgentTracker)

	dispatchCtx := ctxToDispatchCtx(ctx)

	return RunDispatch(DispatchArgs{
		Ctx:                   dispatchCtx,
		Settings:              settings,
		Workflow:              wf,
		State:                 state,
		PoolKey:               poolKey,
		QueueType:             queueType,
		ExcludeAgentIDs:       excludeAgentIDs,
		IsErrorRetry:          isErrorRetry,
		StepFailureRollback:   stepFailureRollback,
		IsReview:              isReview,
		PriorAction:           priorAction,
		FailedAgentID:         failedAgentID,
		MaxClaims:             maxClaims,
		StepAgentTracker:      stepAgentTracker,
	})
}

func ctxToDispatchCtx(ctx *TakeLoopContext) *TakeLoopDispatchContext {
	if ctx == nil {
		return nil
	}
	return &TakeLoopDispatchContext{
		ID:                       ctx.ID,
		BeadID:                   ctx.BeadID,
		PushEvent:                ctx.PushEvent,
		FinishSession:            ctx.FinishSession,
		ClaimsPerQueueType:       ctx.ClaimsPerQueueType,
		LastAgentPerQueueType:    ctx.LastAgentPerQueueType,
		FailedAgentsPerQueueType: ctx.FailedAgentsPerQueueType,
	}
}

type TakeLoopContext struct {
	ID                       string
	BeadID                   string
	AgentID                  string
	AgentLabel               string
	ClaimsPerQueueType       map[string]int
	LastAgentPerQueueType    map[string]string
	FailedAgentsPerQueueType map[string]map[string]bool
	PushEvent                func(evt session.TerminalEvent)
	FinishSession            func(exitCode int)
}

func computeStepExclusions(
	ctx *TakeLoopContext,
	queueType string,
	isReview bool,
	failedAgentID string,
	priorAction string,
	tracker *StepAgentTracker,
) []string {
	exclusions := make([]string, 0)

	if ctx != nil && ctx.FailedAgentsPerQueueType != nil {
		for id := range ctx.FailedAgentsPerQueueType[queueType] {
			exclusions = append(exclusions, id)
		}
	}

	if failedAgentID != "" {
		found := false
		for _, id := range exclusions {
			if id == failedAgentID {
				found = true
				break
			}
		}
		if !found {
			exclusions = append(exclusions, failedAgentID)
		}
	}

	if isReview {
		if ctx != nil && ctx.AgentID != "" {
			exclusions = append(exclusions, ctx.AgentID)
		}
		if priorAction != "" && tracker != nil {
			if priorAgentID, ok := tracker.Get(ctx.BeadID, priorAction); ok && priorAgentID != "" {
				exclusions = append(exclusions, priorAgentID)
			}
		}
	} else {
		if ctx != nil {
			if last, ok := ctx.LastAgentPerQueueType[queueType]; ok && last != "" {
				found := false
				for _, id := range exclusions {
					if id == last {
						found = true
						break
					}
				}
				if !found {
					exclusions = append(exclusions, last)
				}
			}
		}
	}

	return exclusions
}

const MaxClaimsDefault = 10

func maxClaimsFromConfig(settings *config.Settings) int {
	return 0
}

func HandleMaxClaims(ctx *TakeLoopDispatchContext, queueType string, count, maxClaims int) {
	tag := fmt.Sprintf("[terminal-manager] [%s] [take-loop]", ctx.ID)
	slog.Info(fmt.Sprintf("%s STOP: max claims per queue type reached for %q (%d/%d)", tag, queueType, count, maxClaims))

	ctx.PushEvent(session.TerminalEvent{
		Type:    "stdout",
		BeadID:  ctx.BeadID,
		Content: fmt.Sprintf("\x1b[33m--- %s Take loop stopped: max claims per queue type %q reached (%d/%d) ---\x1b[0m\n", time.Now().UTC().Format(time.RFC3339), queueType, count, maxClaims),
		Time:    time.Now().UnixMilli(),
	})
}