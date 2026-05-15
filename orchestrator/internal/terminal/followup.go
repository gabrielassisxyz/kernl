package terminal

import (
	"fmt"
	"log/slog"

	"github.com/gastownhall/foolery/internal/backend"
	"github.com/gastownhall/foolery/internal/orchestration"
	"github.com/gastownhall/foolery/internal/session"
)

const MaxFollowUpsPerState = 5

const FollowUpSource = "take_loop_follow_up"

func BuildTakeLoopFollowUpPrompt(beatID, state string) string {
	return fmt.Sprintf(
		"Your turn ended but beat `%s` is still in state `%s`. Either complete the action to advance the knot, or run `kno rollback` if you cannot proceed. Do not exit without advancing or rolling back.",
		beatID, state,
	)
}

func RecordFollowUpProgress(fa *FollowUpCounter, state string) int {
	if fa.LastState != state {
		fa.Count = 0
		fa.LastState = state
	}
	fa.Count++
	return fa.Count
}

func IsQueueOrTerminalState(state string, wf *backend.WorkflowDescriptor) bool {
	if orchestration.IsQueueOrTerminal(state) {
		return true
	}
	if wf != nil {
		phase := orchestration.WorkflowStatePhase(wf, state)
		return phase == orchestration.PhaseQueued || phase == orchestration.PhaseTerminal
	}
	return false
}

type SendUserTurnFunc func(prompt, source string) bool

type LeaseHealthChecker interface {
	EvaluateLeaseHealth(leaseID, repoPath string) (LeaseHealthResult, error)
}

type LeaseHealthResult struct {
	Healthy   bool
	Reason    string
	LeaseState string
	Detail    string
}

type DefaultLeaseHealthChecker struct{}

func (d *DefaultLeaseHealthChecker) EvaluateLeaseHealth(leaseID, repoPath string) (LeaseHealthResult, error) {
	return LeaseHealthResult{Healthy: true}, nil
}

type FollowUpDeps struct {
	GetBeat          func(beatID, repoPath string) (*backend.Beat, error)
	SendUserTurn     SendUserTurnFunc
	LeaseChecker     LeaseHealthChecker
	InteractionLog   InteractionLog
}

func HandleTakeLoopTurnEnded(ctx *TakeLoopContext, deps FollowUpDeps) bool {
	tag := fmt.Sprintf("[terminal-manager] [%s] [take-loop]", ctx.ID)

	state, err := fetchBeatState(ctx, deps)
	if err != nil || state == "" {
		slog.Warn(fmt.Sprintf("%s onTurnEnded beat fetch failed", tag), "error", err)
		return false
	}

	wf := orchestration.ResolveWorkflowForBeat(ctx.Beat, ctx.WorkflowsByID, ctx.FallbackWorkflow)
	if IsQueueOrTerminalState(state, wf) {
		ctx.FollowUpAttempts.Count = 0
		ctx.FollowUpAttempts.LastState = state
		return false
	}

	count := RecordFollowUpProgress(ctx.FollowUpAttempts, state)
	if count > MaxFollowUpsPerState {
		slog.Warn(fmt.Sprintf(
			"%s follow-up cap reached for beat=%s state=%s count=%d — stopping in-iteration follow-ups",
			tag, ctx.BeatID, state, count,
		))
		emitFollowUpCapBanner(ctx, ctx.BeatID, state, count)
		return false
	}

	health, healthErr := deps.LeaseChecker.EvaluateLeaseHealth(ctx.Entry.KnotsLeaseID, ctx.RepoPath)
	if healthErr != nil || !health.Healthy {
		reason := "lease_state_unknown"
		if healthErr != nil {
			reason = healthErr.Error()
		} else if health.Reason != "" {
			reason = health.Reason
		}
		slog.Warn(fmt.Sprintf(
			"%s refusing follow-up: lease not healthy beat=%s leaseId=%s reason=%s",
			tag, ctx.BeatID, ctx.Entry.KnotsLeaseID, reason,
		))
		emitLeaseDeadBanner(ctx, state, health)
		return false
	}

	prompt := BuildTakeLoopFollowUpPrompt(ctx.BeatID, state)
	sent := deps.SendUserTurn(prompt, FollowUpSource)
	if !sent {
		slog.Warn(fmt.Sprintf("%s failed to send follow-up prompt for beat=%s state=%s", tag, ctx.BeatID, state))
		return false
	}

	emitFollowUpPushEvent(ctx, ctx.BeatID, state)
	return true
}

func fetchBeatState(ctx *TakeLoopContext, deps FollowUpDeps) (string, error) {
	beat, err := deps.GetBeat(ctx.BeatID, ctx.RepoPath)
	if err != nil {
		return "", err
	}
	if beat == nil {
		return "", fmt.Errorf("beat %s not found", ctx.BeatID)
	}
	return beat.State, nil
}

func emitFollowUpPushEvent(ctx *TakeLoopContext, beatID, state string) {
	ctx.PushEvent(session.TerminalEvent{
		Type:   "stdout",
		BeatID: beatID,
		Content: fmt.Sprintf(
			"\x1b[33m--- Take-loop follow-up prompt sent because knot %s in state %s ---\x1b[0m\n",
			beatID, state,
		),
	})
}

func emitFollowUpCapBanner(ctx *TakeLoopContext, beatID, state string, count int) {
	ctx.PushEvent(session.TerminalEvent{
		Type:   "stderr",
		BeatID: beatID,
		Content: fmt.Sprintf(
			"\x1b[31m--- Take-loop follow-up cap reached: knot %s stuck in state %s after %d consecutive follow-up prompts. Closing session so the take loop can reassess. ---\x1b[0m\n",
			beatID, state, count,
		),
	})
}

func emitLeaseDeadBanner(ctx *TakeLoopContext, state string, health LeaseHealthResult) {
	ctx.PushEvent(session.TerminalEvent{
		Type:   "stderr",
		BeatID: ctx.BeatID,
		Content: fmt.Sprintf(
			"\x1b[31mFOOLERY DISPATCH FAILURE: refusing follow-up for beat %s — lease %s is %s (reason: %s)\x1b[0m\n",
			ctx.BeatID, ctx.Entry.KnotsLeaseID, health.LeaseState, health.Reason,
		),
	})
}

type ShipFollowUpInput struct {
	ExitCode                 int
	ExitReason               string
	ExecutionPromptSent      bool
	ShipCompletionPromptSent bool
	AutoShipCompletionPrompt string
}

func ShouldContinueShipFollowUp(input ShipFollowUpInput) bool {
	if input.ExitCode != 0 {
		return false
	}
	if !input.ExecutionPromptSent {
		return false
	}
	if input.ShipCompletionPromptSent {
		return false
	}
	if input.AutoShipCompletionPrompt == "" {
		return false
	}
	return !isFatalFollowUpExitReason(input.ExitReason)
}

func isFatalFollowUpExitReason(exitReason string) bool {
	return exitReason == "timeout" || exitReason == "spawn_error" || exitReason == "external_abort"
}