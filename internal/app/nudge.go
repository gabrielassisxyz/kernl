package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/session"
)

type NudgeCause string

const (
	NudgeTurnEnded                NudgeCause = "turn_ended"
	NudgeResumedAfterInterruption NudgeCause = "resumed_after_interruption"
)

type NudgeInput struct {
	BeadID string
	State  string
	Cause  NudgeCause
}

func BuildNudgePrompt(input NudgeInput) string {
	core := fmt.Sprintf(
		"Either complete the action to advance the knot, or run `kno rollback` if you cannot proceed. Do not exit without advancing or rolling back.",
	)

	switch input.Cause {
	case NudgeResumedAfterInterruption:
		return fmt.Sprintf(
			"You were resumed after an interruption. Bead `%s` is in state `%s`. %s",
			input.BeadID, input.State, core,
		)
	default:
		return fmt.Sprintf(
			"Your turn ended but bead `%s` is still in state `%s`. %s",
			input.BeadID, input.State, core,
		)
	}
}

// NudgePreset names one of the built-in manual-nudge prompts. The web UI
// pre-fills its textarea with the matching DefaultNudgePrompt; users may
// edit or replace before sending.
type NudgePreset string

const (
	NudgePresetGeneric       NudgePreset = "generic"
	NudgePresetAdvanceStatus NudgePreset = "advance_status"
	NudgePresetCustom        NudgePreset = "custom"
)

type NudgeOptions struct {
	Preset NudgePreset
	Prompt string // when non-empty, used verbatim (overrides the preset default).
}

var (
	ErrNudgeUnknownSession    = errors.New("nudge: unknown session")
	ErrNudgeRunning           = errors.New("nudge: agent is still running")
	ErrNudgeNoOpencodeSession = errors.New("nudge: no captured opencode session id")
)

// Nudge re-spawns the agent for sessionID with a manual follow-up prompt,
// resuming the captured opencode conversation via `-s <ses_xxx>` so the
// agent keeps full conversation context. Returns immediately after kicking
// off a background goroutine — events stream back through the existing SCM
// SSE pipe so the web log panel updates live.
//
// Refuses to start a second run while one is in flight (ErrNudgeRunning).
func (a *App) Nudge(sessionID string, opts NudgeOptions) error {
	if a == nil || a.NudgeRegistry == nil || a.Driver == nil {
		return errors.New("nudge: app not fully initialized")
	}
	if sessionID == "" {
		return errors.New("nudge: sessionID required")
	}
	rec, ok := a.NudgeRegistry.Get(sessionID)
	if !ok {
		return ErrNudgeUnknownSession
	}
	if rec.Running {
		return ErrNudgeRunning
	}
	if rec.OpencodeSessionID == "" {
		return ErrNudgeNoOpencodeSession
	}

	prompt := opts.Prompt
	if prompt == "" {
		prompt = DefaultNudgePrompt(opts.Preset, rec.BeadID, rec.RepoPath)
	}

	agentInput, err := ResolveAgentForBead(a.Config, a.Backend, rec.BeadID, rec.RepoPath)
	if err != nil {
		return fmt.Errorf("nudge: resolve agent for %s: %w", rec.BeadID, err)
	}
	agentInput.Args = appendOpencodeStageFlags(agentInput.Args, rec.BeadID, rec.Cwd, rec.OpencodeSessionID, prompt)
	agentInput.Env = injectOpencodeConfigEnv(agentInput.Env, rec.RepoPath)
	agentInput.BeadID = rec.BeadID
	agentInput.RepoPath = rec.RepoPath
	agentInput.Cwd = rec.Cwd

	// Surface a marker in the SSE stream so the log panel shows the nudge
	// landed even before the agent's first NDJSON line arrives.
	a.SCM.HandleEvent(sessionID, session.TerminalEvent{
		Type:    "stderr",
		Content: fmt.Sprintf("[nudge] dispatching follow-up (preset=%s, opencode_sid=%s)\n", opts.Preset, rec.OpencodeSessionID),
		BeadID:  rec.BeadID,
		Time:    time.Now().UnixMilli(),
	})

	go func() {
		ctx := context.Background()
		res, err := a.Driver.RunBead(ctx, agentInput)
		if err != nil {
			slog.Error("nudge: agent run failed",
				"sessionID", sessionID,
				"bead", rec.BeadID,
				"error", err,
			)
			a.SCM.HandleEvent(sessionID, session.TerminalEvent{
				Type:    "stderr",
				Content: fmt.Sprintf("[nudge] agent run failed: %v\n", err),
				BeadID:  rec.BeadID,
				Time:    time.Now().UnixMilli(),
			})
			return
		}
		slog.Info("nudge: agent run complete",
			"sessionID", sessionID,
			"bead", rec.BeadID,
			"success", res.Success,
			"finalState", res.FinalState,
		)
	}()

	return nil
}

// DefaultNudgePrompt returns the canonical text for a given preset, with
// bead-id and repo-path substituted. The frontend pre-fills its textarea
// with the same strings so the user can edit before sending.
func DefaultNudgePrompt(preset NudgePreset, beadID, repoPath string) string {
	switch preset {
	case NudgePresetAdvanceStatus:
		return fmt.Sprintf(`Your previous turn exited cleanly but the bead status was NOT advanced. The orchestrator's polling loop will not progress until you run the required bd update --status command for this stage.

1. Run: bd -C %s show %s
   Confirm the current state and figure out which status this stage should advance to.
2. Verify your work is genuinely complete: git status, git diff, and any required tests for the files you touched. If tests do not pass, fix them first.
3. If complete: run the bd update --status <next> command for this stage and then exit.
4. If NOT complete: finish the remaining work first, then advance the status.
5. If genuinely blocked: run bd -C %s update %s --status blocked and write a one-paragraph explanation of the block to _scratch/STAGE_BLOCKED.md.

Do not start unrelated work. Do not redo work that is already on disk. Just close out this stage.`,
			repoPath, beadID, repoPath, beadID,
		)
	case NudgePresetGeneric, "":
		return fmt.Sprintf(`Your previous turn was interrupted before completion (likely an upstream API error, timeout, or rate-limit cut you off mid-task). Resume from where you left off — do NOT restart from scratch.

1. Run: bd -C %s show %s
   See the bead's current state and the most recent status transition.
2. Run git status and git diff in the worktree to see what work is already on disk.
3. Re-read the stage instructions you received earlier in this conversation.
4. Continue the work that was in progress. Do NOT start anything new and do NOT redo work that is already complete.
5. When the stage is genuinely done, run the required bd update --status <next> command and exit. If you are truly blocked, run bd -C %s update %s --status blocked and document the block.

Be defensive — your previous turn ended unexpectedly, so verify state on disk before acting on memory.`,
			repoPath, beadID, repoPath, beadID,
		)
	}
	return fmt.Sprintf("Continue from where you left off on bead %s in repo %s.", beadID, repoPath)
}
