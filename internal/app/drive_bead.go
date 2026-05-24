package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/subprocess"
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
	// SessionID is the opencode session to resume via -s. Non-empty means
	// the bead is being resumed from a previous run rather than dispatched
	// fresh.
	SessionID string
	// BuildPrompt, when non-nil, overrides the default per-stage prompt
	// builder. The epic driver uses it to inject integration/shipment prompts
	// that need epic-specific context (child branches, epic branch) the
	// generic StageContract prompt cannot express.
	BuildPrompt func(bead *backend.Bead, activeState string, wf backend.WorkflowDescriptor, repoPath, worktree string) string
	// AgentStateStore holds the context-store handle.
	AgentStateStore *workflow.AgentStateStore
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
		slog.Info("DRIVE_TRACE iter top", "bead", deps.BeadID, "iter", i, "state", bead.State, "prevState", prevState, "profile", wf.ID)

		if isWorkflowTerminal(bead.State, wf) {
			slog.Info("DRIVE_TRACE return terminal", "bead", deps.BeadID, "iter", i, "state", bead.State)
			return RunBeadResult{FinalState: bead.State, Success: true}, nil
		}
		if isHumanGateOrHandoff(bead.State, wf) {
			runtime := backend.DeriveWorkflowRuntimeState(wf, bead.State)
			slog.Info("DRIVE_TRACE return human-gate", "bead", deps.BeadID, "iter", i, "state", bead.State, "owner", runtime.NextActionOwnerKind, "reqHuman", runtime.RequiresHumanAction)
			return RunBeadResult{FinalState: bead.State, Success: true}, nil
		}
		if bead.State == string(workflow.StatusBlocked) {
			slog.Info("DRIVE_TRACE return blocked", "bead", deps.BeadID, "iter", i)
			return RunBeadResult{FinalState: bead.State, Success: false}, nil
		}

		if deps.Log != nil {
			deps.Log(i, bead.State)
		}

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
					slog.Info("DRIVE_TRACE return claim-failed", "bead", deps.BeadID, "iter", i, "from", bead.State, "to", nextState, "err", err)
					return RunBeadResult{FinalState: bead.State, Success: false},
						fmt.Errorf("KERNL DISPATCH FAILURE: advancing bead %s from %s to %s: %w", deps.BeadID, bead.State, nextState, err)
				}
				activeState = nextState
				slog.Info("DRIVE_TRACE claimed", "bead", deps.BeadID, "iter", i, "from", bead.State, "to", nextState)
			}
		}

		activeStage := wf.Stages[activeState]
		if deps.AgentStateStore != nil && activeStage.Kind == "subprocess" {
			// Subprocess flow
			runtimeState, err := deps.AgentStateStore.Load(deps.BeadID)
			if err != nil {
				return RunBeadResult{FinalState: activeState, Success: false},
					fmt.Errorf("failed to load agent state: %w", err)
			}

			epicID := bead.ParentID
			if epicID == "" {
				epicID = bead.ID
			}

			req := subprocess.HandoffRequest{
				EpicID:         epicID,
				BeadID:         deps.BeadID,
				WorktreePath:   deps.Worktree,
				ContextPayload: runtimeState.ContextPayload,
			}

			startTime := time.Now()
			resp, err := subprocess.RunSubprocessStage(ctx, activeStage, req)
			if err != nil {
				var causeStr string
				var stderr string
				var subErr *subprocess.SubprocessError
				if errors.As(err, &subErr) {
					stderr = subErr.Stderr
					switch subErr.Cause {
					case subprocess.CauseNonZeroExit:
						causeStr = "non-zero exit"
					case subprocess.CauseTimeout:
						causeStr = "timeout"
					case subprocess.CauseParseError:
						causeStr = "unparseable output"
					case subprocess.CauseOutputTooLarge:
						causeStr = "output too large"
					default:
						causeStr = string(subErr.Cause)
					}
				} else {
					causeStr = "execution failed: " + err.Error()
				}

				// Truncate stderr dumped into the bead comment at a sane limit (64KB) with a truncation marker.
				const maxStderrLen = 65536
				if len(stderr) > maxStderrLen {
					stderr = stderr[:maxStderrLen] + "\n... (truncated)"
				}

				commentBody := fmt.Sprintf("subprocess stage %s failed: %s\n\nStderr:\n%s", activeState, causeStr, stderr)

				_ = deps.Backend.Update(deps.BeadID, backend.UpdateBeadInput{State: "blocked"}, deps.RepoPath)
				_ = deps.Backend.Comment(deps.BeadID, commentBody, deps.RepoPath)
				return RunBeadResult{FinalState: "blocked", Success: false}, nil
			}

			runtimeState.ContextPayload = resp.ContextPayload
			if err := deps.AgentStateStore.Save(deps.BeadID, runtimeState); err != nil {
				return RunBeadResult{FinalState: activeState, Success: false},
					fmt.Errorf("failed to save agent state: %w", err)
			}

			duration := time.Since(startTime)
			gateDesc := ""
			if freshBead, ferr := deps.Backend.Get(deps.BeadID, deps.RepoPath); ferr == nil && freshBead != nil {
				gateDesc = freshBead.Description
			}
			gatePassed, gateReason := backend.EvaluateExitGate(wf, activeState, deps.Worktree, deps.BeadID, gateDesc)
			if gatePassed {
				nextState, ok := backend.ForwardTransitionTarget(activeState, wf)
				if ok {
					err := deps.Backend.Update(deps.BeadID, backend.UpdateBeadInput{State: nextState}, deps.RepoPath)
					if err != nil {
						beadAfter, getErr := deps.Backend.Get(deps.BeadID, deps.RepoPath)
						if getErr == nil && beadAfter != nil && beadAfter.State == nextState {
							slog.Info("DRIVE_TRACE post-spawn update idempotent", "bead", deps.BeadID, "state", nextState)
						} else {
							slog.Info("DRIVE_TRACE return advance-failed", "bead", deps.BeadID, "err", err)
							return RunBeadResult{FinalState: activeState, Success: false},
								fmt.Errorf("KERNL DISPATCH FAILURE: advancing bead %s from %s to %s after subprocess exit: %w", deps.BeadID, activeState, nextState, err)
						}
					}
					artifactPath := resolveArtifactRef(activeState, wf.Stages, deps.BeadID)
					commitSHA := worktreeHeadSHA(deps.Worktree)
					agentID := "subprocess"
					if len(activeStage.Subprocess.Command) > 0 {
						agentID = activeStage.Subprocess.Command[0]
					}
					if err := deps.Backend.Comment(deps.BeadID, buildStageComment(activeState, agentID, "", artifactPath, commitSHA, duration), deps.RepoPath); err != nil {
						slog.Warn("DRIVE_TRACE comment failed", "bead", deps.BeadID, "err", err)
					}
				}
			} else {
				_ = deps.Backend.Update(deps.BeadID, backend.UpdateBeadInput{State: "blocked"}, deps.RepoPath)
				_ = deps.Backend.Comment(deps.BeadID, "gate_failed: "+gateReason, deps.RepoPath)
				return RunBeadResult{FinalState: "blocked", Success: false}, nil
			}

			lastResult = RunBeadResult{FinalState: activeState, Success: true}
			prevState = bead.State
			continue
		}

		// Native flow
		agentInput, err := ResolveAgentForBead(deps.Config, deps.Backend, deps.BeadID, deps.RepoPath)
		if err != nil {
			return RunBeadResult{FinalState: bead.State, Success: false},
				fmt.Errorf("KERNL DISPATCH FAILURE: bead %s at state %s: %w", deps.BeadID, bead.State, err)
		}

		slog.Info("DRIVE_TRACE pre-claim", "bead", deps.BeadID, "iter", i, "state", bead.State, "claimable", runtime.IsAgentClaimable, "owner", runtime.NextActionOwnerKind, "agent", agentInput.AgentName)

		var prompt string
		if deps.BuildPrompt != nil {
			prompt = deps.BuildPrompt(bead, activeState, wf, deps.RepoPath, deps.Worktree)
		} else {
			prompt = BuildBeadStagePrompt(bead, activeState, wf.Stages, deps.RepoPath, deps.Worktree)
		}
		agentInput.Args = appendOpencodeStageFlags(agentInput.Args, deps.BeadID, deps.Worktree, deps.SessionID, prompt)
		agentInput.Env = injectOpencodeConfigEnv(agentInput.Env, deps.RepoPath)
		if agentInput.Env == nil {
			agentInput.Env = make(map[string]string)
		}
		if len(wf.Stages) > 0 {
			staticConfigPath := deps.RepoPath + "/orchestrator/opencode-config.json"
			stageCfgPath, cfgErr := writeStageOpencodeConfig(staticConfigPath, deps.Worktree, deps.BeadID, activeState, wf.Stages)
			if cfgErr != nil {
				slog.Warn("DRIVE_TRACE stage-opencode-config failed, using static config", "err", cfgErr)
			} else {
				agentInput.Env["OPENCODE_CONFIG"] = stageCfgPath
			}
		}

		agentInput.BeadID = deps.BeadID
		agentInput.RepoPath = deps.RepoPath
		agentInput.Cwd = deps.Worktree

		agentInput.Env["BEAD_ID"] = deps.BeadID
		agentInput.Env["REPO_PATH"] = deps.RepoPath

		startTime := time.Now()
		slog.Info("DRIVE_TRACE spawn", "bead", deps.BeadID, "iter", i, "activeState", activeState, "agent", agentInput.AgentName)
		res, err := deps.Driver.RunBead(ctx, agentInput)
		if err != nil {
			slog.Info("DRIVE_TRACE return agent-err", "bead", deps.BeadID, "iter", i, "err", err)
			return RunBeadResult{FinalState: res.FinalState, Success: false},
				fmt.Errorf("KERNL DISPATCH FAILURE: agent %s for bead %s: %w", agentInput.AgentName, deps.BeadID, err)
		}
		if !res.Success {
			slog.Info("DRIVE_TRACE return agent-not-success", "bead", deps.BeadID, "iter", i, "resFinalState", res.FinalState)
			return RunBeadResult{FinalState: res.FinalState, Success: false}, nil
		}
		duration := time.Since(startTime)
		slog.Info("DRIVE_TRACE post-spawn ok", "bead", deps.BeadID, "iter", i, "resFinalState", res.FinalState)

		// Re-fetch the bead so description-based exit gates (e.g. shipment's
		// pr_url marker) see what the agent just wrote, not the stale snapshot
		// from the top of this iteration.
		gateDesc := ""
		if freshBead, ferr := deps.Backend.Get(deps.BeadID, deps.RepoPath); ferr == nil && freshBead != nil {
			gateDesc = freshBead.Description
		}
		gatePassed, gateReason := backend.EvaluateExitGate(wf, activeState, deps.Worktree, deps.BeadID, gateDesc)
		if gatePassed {
			nextState, ok := backend.ForwardTransitionTarget(activeState, wf)
			if ok {
				err := deps.Backend.Update(deps.BeadID, backend.UpdateBeadInput{State: nextState}, deps.RepoPath)
				if err != nil {
					beadAfter, getErr := deps.Backend.Get(deps.BeadID, deps.RepoPath)
					if getErr == nil && beadAfter != nil && beadAfter.State == nextState {
						slog.Info("DRIVE_TRACE post-spawn update idempotent", "bead", deps.BeadID, "state", nextState)
					} else {
						slog.Info("DRIVE_TRACE return advance-failed", "bead", deps.BeadID, "err", err)
						return RunBeadResult{FinalState: activeState, Success: false},
							fmt.Errorf("KERNL DISPATCH FAILURE: advancing bead %s from %s to %s after agent exit: %w", deps.BeadID, activeState, nextState, err)
					}
				}
				artifactPath := resolveArtifactRef(activeState, wf.Stages, deps.BeadID)
				commitSHA := worktreeHeadSHA(deps.Worktree)
				if err := deps.Backend.Comment(deps.BeadID, buildStageComment(activeState, agentInput.AgentName, res.SessionID, artifactPath, commitSHA, duration), deps.RepoPath); err != nil {
					slog.Warn("DRIVE_TRACE comment failed", "bead", deps.BeadID, "err", err)
				}
			}
		} else {
			_ = deps.Backend.Update(deps.BeadID, backend.UpdateBeadInput{State: "blocked"}, deps.RepoPath)
			_ = deps.Backend.Comment(deps.BeadID, "gate_failed: "+gateReason, deps.RepoPath)
			return RunBeadResult{FinalState: "blocked", Success: false}, nil
		}

		lastResult = res
		prevState = bead.State
	}

	return RunBeadResult{FinalState: lastResult.FinalState, Success: false},
		fmt.Errorf("KERNL DISPATCH FAILURE: bead %s exceeded max stages (%d) — Fix: check workflow for cycles", deps.BeadID, maxStages)
}

func buildStageComment(state, agentID, sessionID, artifactPath, commitSHA string, duration time.Duration) string {
	return fmt.Sprintf(
		"stage: %s\nagent: %s\nsession_id: %s\nartifact: %s\ncommit: %s\nduration: %s",
		state,
		agentID,
		sessionID,
		artifactPath,
		commitSHA,
		duration.Truncate(time.Millisecond).String(),
	)
}

func resolveArtifactRef(state string, stages map[string]backend.StageContract, beadID string) string {
	if stages == nil {
		return ""
	}
	sc, ok := stages[state]
	if !ok {
		return ""
	}
	return strings.ReplaceAll(sc.OutputArtifact.Path, "<bead_id>", beadID)
}

func worktreeHeadSHA(worktree string) string {
	if worktree == "" {
		return ""
	}
	out, err := exec.Command("git", "-C", worktree, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
func appendOpencodeStageFlags(args []string, beadID, worktree, sessionID, prompt string) []string {
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
	if sessionID != "" && !hasFlag("-s") {
		out = append(out, "-s", sessionID)
	}
	if !hasFlag("--title") {
		out = append(out, "--title", "kernl:"+beadID)
	}
	// Positional prompt goes LAST — opencode treats trailing positionals
	// as the message.
	out = append(out, prompt)
	return out
}
