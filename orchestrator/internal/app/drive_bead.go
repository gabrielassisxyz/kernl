package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

// BeadDriver is the orchestrator-internal contract for spawning a single
// agent against a single bead stage. SessionDriver implements it; tests can
// supply fakes.
type BeadDriver interface {
	RunBead(ctx context.Context, input RunBeadInput) (RunBeadResult, error)
}

// DriveBeadDeps wires the inputs the per-bead workflow loop needs.
type DriveBeadDeps struct {
	Backend            backend.BackendPort
	Driver             BeadDriver
	Config             *config.Config
	BeadID             string
	RepoPath           string
	Worktree           string
	Log                func(stage int, state string)
	MaxStages          int
	// StageRetryAttempts is how many times to re-spawn the same opencode session
	// when the agent exits rc=0 but did not advance the bead's status. Defaults
	// to orchestrator.stageRetryAttempts from kernl.yaml (or 2 if unset).
	StageRetryAttempts int
}

// DriveBeadToTerminal advances a single bead through every agent-claimable
// stage of its workflow until it reaches a terminal state, a human-owned
// gate, or a hard failure. Per VISION §8.1 the orchestrator drives the
// per-bead loop and per-stage agent selection lives in ResolveAgentForBead
// which keys on the bead's current action state.
//
// Stop conditions (success): terminal state (closed, deferred, etc.);
// awaiting_integration / awaiting_pr_review; any queued state with a human
// owner.
//
// Stop conditions (failure): blocked status; agent exits non-zero; agent
// returns success but bead.state did not change (silent agent failure);
// backend / resolver / spawn error; max iterations.
func DriveBeadToTerminal(ctx context.Context, deps DriveBeadDeps) (RunBeadResult, error) {
	maxStages := deps.MaxStages
	if maxStages <= 0 {
		maxStages = 16
	}

	retryLimit := deps.StageRetryAttempts
	if retryLimit <= 0 && deps.Config != nil {
		retryLimit = deps.Config.Orchestrator.StageRetryAttempts
	}
	if retryLimit <= 0 {
		retryLimit = 2
	}

	var lastResult RunBeadResult
	prevState := ""

	for i := 0; i < maxStages; i++ {
		bead, err := deps.Backend.Get(deps.BeadID, deps.RepoPath)
		if err != nil || bead == nil {
			return lastResult, fmt.Errorf("KERNL DISPATCH FAILURE: bead %s not found in repo %s: %w", deps.BeadID, deps.RepoPath, err)
		}

		wf := backend.ResolveWorkflow(bead)

		if isWorkflowTerminal(bead.State, wf) {
			return RunBeadResult{FinalState: bead.State, Success: true}, nil
		}
		if isHumanGateOrHandoff(bead.State, wf) {
			return RunBeadResult{FinalState: bead.State, Success: true}, nil
		}
		if bead.State == string(workflow.StatusBlocked) {
			return RunBeadResult{FinalState: bead.State, Success: false}, nil
		}

		// Same state twice means the agent finished cleanly but did not
		// advance bead.status. Before failing loud, attempt up to retryLimit
		// re-spawns of the same opencode session with an explicit follow-up.
		// Exception: if the workflow has no forward transition from this state,
		// treat it as a single-stage / degenerate workflow and return success.
		if i > 0 && bead.State == prevState {
			if _, ok := backend.ForwardTransitionTarget(bead.State, wf); !ok {
				return RunBeadResult{FinalState: bead.State, Success: true}, nil
			}
			res, retryErr := retryStuckStage(ctx, deps, bead, wf, lastResult.SessionID, retryLimit)
			if retryErr != nil {
				return res, retryErr
			}
			lastResult = res
			// Re-read bead state before the next outer iteration.
			prevState = bead.State
			continue
		}

		if deps.Log != nil {
			deps.Log(i, bead.State)
		}

		// Resolve the agent BEFORE advancing state so a misconfigured pool
		// does not strand the bead at an active state with no worker.
		agentInput, err := ResolveAgentForBead(deps.Config, deps.Backend, deps.BeadID, deps.RepoPath)
		if err != nil {
			return RunBeadResult{FinalState: bead.State, Success: false},
				fmt.Errorf("KERNL DISPATCH FAILURE: bead %s at state %s: %w", deps.BeadID, bead.State, err)
		}

		// Claim only when the bead is in a queued state (ready_for_X). The
		// agent owns the transition from active (X) → next queued (ready_for_X_review);
		// claiming again from an active state would mask agent failures.
		runtime := backend.DeriveWorkflowRuntimeState(wf, bead.State)
		activeState := bead.State
		if runtime.IsAgentClaimable {
			nextState, ok := backend.ForwardTransitionTarget(bead.State, wf)
			if ok {
				newLabels := filterOutLabelPrefix(bead.Labels, "wf:state:")
				newLabels = append(newLabels, "wf:state:"+nextState)
				if err := deps.Backend.Update(deps.BeadID, backend.UpdateBeadInput{
					State:     nextState,
					SetLabels: newLabels,
				}, deps.RepoPath); err != nil {
					return RunBeadResult{FinalState: bead.State, Success: false},
						fmt.Errorf("KERNL DISPATCH FAILURE: advancing bead %s from %s to %s: %w", deps.BeadID, bead.State, nextState, err)
				}
				activeState = nextState
			}
		}

		// Build the prompt that tells the agent what to do AND how to end
		// the stage (the agent must `bd update --status <next>` so the
		// orchestrator's polling loop sees the workflow advance).
		var promptNextState string
		if nextAfterActive, ok := backend.ForwardTransitionTarget(activeState, wf); ok {
			promptNextState = nextAfterActive
		}
		prompt := BuildBeadStagePrompt(bead, activeState, promptNextState, deps.RepoPath, deps.Worktree)
		agentInput.Args = appendOpencodeStageFlags(agentInput.Args, deps.BeadID, deps.Worktree, prompt)
		// Point opencode at the orchestrator's permission allowlist so
		// `/tmp/*` writes and worktree access are not auto-rejected (which
		// silently makes agents bail mid-stage — observed in the kernl-npp
		// MVP run on 2026-05-17 where every agent gave up after hitting
		// "permission requested: external_directory (/tmp/*); auto-rejecting").
		agentInput.Env = injectOpencodeConfigEnv(agentInput.Env, deps.RepoPath)

		agentInput.BeadID = deps.BeadID
		// bd reads/writes stay on the canonical repo; the agent process is
		// chrooted into the bead's isolated worktree via Cwd.
		agentInput.RepoPath = deps.RepoPath
		agentInput.Cwd = deps.Worktree

		res, err := deps.Driver.RunBead(ctx, agentInput)
		if err != nil {
			return RunBeadResult{FinalState: res.FinalState, Success: false},
				fmt.Errorf("KERNL DISPATCH FAILURE: agent %s for bead %s: %w", agentInput.AgentName, deps.BeadID, err)
		}
		if !res.Success {
			return RunBeadResult{FinalState: res.FinalState, Success: false}, nil
		}

		lastResult = res
		prevState = bead.State
	}

	return RunBeadResult{FinalState: lastResult.FinalState, Success: false},
		fmt.Errorf("KERNL DISPATCH FAILURE: bead %s exceeded max stages (%d) — Fix: check workflow for cycles or agents that do not advance state", deps.BeadID, maxStages)
}

// retryStuckStage re-spawns the same opencode session up to maxRetries times
// when the agent exited rc=0 but the bead's status did not advance. Each
// retry sends a strong follow-up prompt via -s <sessionID> so the agent can
// finish the interrupted work without losing context.
func retryStuckStage(ctx context.Context, deps DriveBeadDeps, bead *backend.Bead, wf backend.WorkflowDescriptor, sessionID string, maxRetries int) (RunBeadResult, error) {
	var promptNextState string
	if next, ok := backend.ForwardTransitionTarget(bead.State, wf); ok {
		promptNextState = next
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		slog.Info("stage retry: agent exited without advancing bead status",
			"bead", deps.BeadID,
			"state", bead.State,
			"attempt", attempt,
			"maxRetries", maxRetries,
			"sessionID", sessionID,
		)

		agentInput, err := ResolveAgentForBead(deps.Config, deps.Backend, deps.BeadID, deps.RepoPath)
		if err != nil {
			return RunBeadResult{FinalState: bead.State, Success: false},
				fmt.Errorf("KERNL DISPATCH FAILURE: bead %s retry %d resolve agent: %w", deps.BeadID, attempt, err)
		}

		followUp := buildRetryPrompt(deps.BeadID, bead.State, promptNextState, deps.RepoPath)
		agentInput.Args = appendOpencodeRetryFlags(agentInput.Args, deps.BeadID, deps.Worktree, sessionID, followUp)
		agentInput.Env = injectOpencodeConfigEnv(agentInput.Env, deps.RepoPath)
		agentInput.BeadID = deps.BeadID
		agentInput.RepoPath = deps.RepoPath
		agentInput.Cwd = deps.Worktree

		res, err := deps.Driver.RunBead(ctx, agentInput)
		if err != nil {
			return RunBeadResult{FinalState: res.FinalState, Success: false},
				fmt.Errorf("KERNL DISPATCH FAILURE: agent %s for bead %s retry %d: %w", agentInput.AgentName, deps.BeadID, attempt, err)
		}
		if !res.Success {
			return RunBeadResult{FinalState: res.FinalState, Success: false}, nil
		}

		// Check whether the bead advanced after this retry.
		updated, getErr := deps.Backend.Get(deps.BeadID, deps.RepoPath)
		if getErr != nil || updated == nil {
			return res, fmt.Errorf("KERNL DISPATCH FAILURE: bead %s not found after retry %d: %w", deps.BeadID, attempt, getErr)
		}
		if updated.State != bead.State {
			slog.Info("stage retry succeeded: bead advanced",
				"bead", deps.BeadID,
				"from", bead.State,
				"to", updated.State,
				"attempt", attempt,
			)
			return RunBeadResult{SessionID: res.SessionID, FinalState: updated.State, Success: true}, nil
		}

		// Use the session ID from this retry for the next one (session chain).
		if res.SessionID != "" {
			sessionID = res.SessionID
		}
	}

	return RunBeadResult{FinalState: bead.State, Success: false},
		fmt.Errorf("KERNL DISPATCH FAILURE: bead %s stuck at state %q after %d retries — Fix: the agent for this state must run 'bd update --status %s' before exiting", deps.BeadID, bead.State, maxRetries, promptNextState)
}

// buildRetryPrompt returns the follow-up message sent to a session that exited
// without advancing the bead's status.
func buildRetryPrompt(beadID, currentState, nextState, repoPath string) string {
	if nextState == "" {
		return fmt.Sprintf(
			"Your previous turn ended without running `bd update --status`. "+
				"Bead %s is still at %q. Run the required `bd update --status` command now and exit. "+
				"If you cannot complete the work, run: bd update --status blocked --repo %s %s",
			beadID, currentState, repoPath, beadID,
		)
	}
	return fmt.Sprintf(
		"Your previous turn ended without running `bd update --status %s`. "+
			"Do that command now and exit: bd update --status %s --repo %s %s\n"+
			"If you cannot complete the work, write _scratch/STAGE_BLOCKED.md and run: "+
			"bd update --status blocked --repo %s %s",
		nextState, nextState, repoPath, beadID,
		repoPath, beadID,
	)
}

// appendOpencodeRetryFlags builds the arg list for a retry spawn. It reuses
// the session via -s <sessionID> when one is available so the agent retains
// its full conversation history. Falls back to --title-only if no session ID
// was captured from the prior run.
func appendOpencodeRetryFlags(args []string, beadID, worktree, sessionID, followUpPrompt string) []string {
	hasFlag := func(flag string) bool {
		for _, a := range args {
			if a == flag {
				return true
			}
		}
		return false
	}
	out := append([]string(nil), args...)
	if worktree != "" && !hasFlag("--dir") {
		out = append(out, "--dir", worktree)
	}
	if !hasFlag("--title") {
		out = append(out, "--title", "kernl:"+beadID)
	}
	if sessionID != "" && !hasFlag("-s") {
		out = append(out, "-s", sessionID)
	}
	out = append(out, followUpPrompt)
	return out
}

func isWorkflowTerminal(state string, wf backend.WorkflowDescriptor) bool {
	if state == string(workflow.StatusClosed) {
		return true
	}
	for _, ts := range wf.TerminalStates {
		if ts == state {
			return true
		}
	}
	return false
}

func isHumanGateOrHandoff(state string, wf backend.WorkflowDescriptor) bool {
	if state == string(workflow.StatusAwaitingIntegration) || state == string(workflow.StatusAwaitingPRReview) {
		return true
	}
	runtime := backend.DeriveWorkflowRuntimeState(wf, state)
	return runtime.RequiresHumanAction
}

func filterOutLabelPrefix(labels []string, prefix string) []string {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		if !strings.HasPrefix(l, prefix) {
			out = append(out, l)
		}
	}
	return out
}

// injectOpencodeConfigEnv sets OPENCODE_CONFIG to orchestrator/opencode-config.json
// (alongside go.mod inside the canonical repo) so the spawned agent honors
// the orchestrator's permission allowlist instead of opencode's defaults
// (which auto-reject external_directory writes like /tmp/*).
// Does not overwrite an explicit OPENCODE_CONFIG already set by the caller.
func injectOpencodeConfigEnv(env map[string]string, repoPath string) map[string]string {
	if env == nil {
		env = map[string]string{}
	}
	if _, exists := env["OPENCODE_CONFIG"]; exists {
		return env
	}
	env["OPENCODE_CONFIG"] = repoPath + "/orchestrator/opencode-config.json"
	return env
}

// appendOpencodeStageFlags adds the per-stage flags opencode needs to
// (a) work in the correct directory, (b) carry a recognizable session title
// in the agent UI, and (c) actually receive the prompt — mirroring the
// shape used by scripts/swarm/swarm_parallel.py:cmd().
//
// Idempotent: if a flag is already present (e.g. user configured --dir in
// kernl.yaml), it is left alone.
func appendOpencodeStageFlags(args []string, beadID, worktree, prompt string) []string {
	hasFlag := func(flag string) bool {
		for _, a := range args {
			if a == flag {
				return true
			}
		}
		return false
	}
	out := append([]string(nil), args...)
	if worktree != "" && !hasFlag("--dir") {
		out = append(out, "--dir", worktree)
	}
	if !hasFlag("--title") {
		out = append(out, "--title", "kernl:"+beadID)
	}
	// Positional prompt goes LAST — opencode treats trailing positionals
	// as the message.
	out = append(out, prompt)
	return out
}
