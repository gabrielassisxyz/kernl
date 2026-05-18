package app

import (
	"context"
	"fmt"
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
	Backend   backend.BackendPort
	Driver    BeadDriver
	Config    *config.Config
	BeadID    string
	RepoPath  string
	Worktree  string
	Log       func(stage int, state string)
	MaxStages int
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
		// advance bead.status — fail loud rather than spin forever.
		// Exception: if the workflow has no forward transition from this
		// state, treat it as a single-stage / degenerate workflow and
		// return success. This preserves the contract for beads whose
		// workflow profile doesn't define a multi-stage pipeline.
		if i > 0 && bead.State == prevState {
			if _, ok := backend.ForwardTransitionTarget(bead.State, wf); !ok {
				return RunBeadResult{FinalState: bead.State, Success: true}, nil
			}
			return RunBeadResult{FinalState: bead.State, Success: false},
				fmt.Errorf("KERNL DISPATCH FAILURE: bead %s stuck at state %q after agent ran — Fix: the agent for this state must update bead.status before exiting", deps.BeadID, bead.State)
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
