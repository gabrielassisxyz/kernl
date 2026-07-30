package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/adapter"
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
	Backend   backend.BackendPort
	Driver    BeadDriver
	Config    *config.Config
	BeadID    string
	RepoPath  string
	Worktree  string
	Log       func(stage int, state string)
	MaxStages int
	// SessionID is the opencode session to resume via -s. Non-empty means
	// the bead is being resumed from a previous run rather than dispatched
	// fresh.
	SessionID string
	// VerifyCommand is how the target repository says "this works", resolved
	// once per run by epic.ResolveVerifyCommand and named in every stage
	// prompt. The prompt used to name a Go toolchain and a module path
	// instead, which is a fact about kernl, not about the repository the agent
	// is working in.
	VerifyCommand string
	// BuildPrompt, when non-nil, overrides the default per-stage prompt
	// builder. The epic driver uses it to inject integration/shipment prompts
	// that need epic-specific context (child branches, epic branch) the
	// generic StageContract prompt cannot express.
	BuildPrompt func(in StagePromptInput, wf backend.WorkflowDescriptor) string
	// AgentStateStore holds the context-store handle.
	AgentStateStore *workflow.AgentStateStore
	// StopBeforeState halts the loop rather than entering the named state,
	// leaving the bead in the state before it so a later run resumes cleanly.
	// This is how --dry-run contains shipment: containment has to be
	// structural, because an agent told not to publish can still decide that
	// publishing is what the instruction meant.
	StopBeforeState string
	// StateDir is the directory kernl writes a run's own control files into -
	// the opencode allowlist and its per-stage specializations, and (see
	// resolveArtifactDir) each bead's exit-gate artifacts. It is passed in
	// rather than derived from the home directory here, because a function
	// deep in the dispatch loop that resolves its own paths writes into the
	// operator's real state directory from every unit test that reaches it.
	StateDir string
	// TrackerCommand is how this repository's tracker is typed from inside a
	// worktree. Resolved once per run by backend.TrackerInvocation, because
	// which tracker it is and how it reaches its store are both properties of
	// the repository being worked on.
	TrackerCommand string
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

		if deps.StopBeforeState != "" && bead.State == deps.StopBeforeState {
			slog.Info("DRIVE_TRACE return stop-before", "bead", deps.BeadID, "iter", i, "state", bead.State)
			return RunBeadResult{FinalState: bead.State, Success: true}, nil
		}

		if deps.Log != nil {
			deps.Log(i, bead.State)
		}

		runtime := backend.DeriveWorkflowRuntimeState(wf, bead.State)
		activeState := bead.State
		if runtime.IsAgentClaimable {
			nextState, ok := backend.ForwardTransitionTarget(bead.State, wf)
			// Stop before claiming, not after: claiming would leave the bead
			// sitting inside the stage it never ran, which is the stranded
			// state a resume has to be reset out of by hand.
			if ok && deps.StopBeforeState != "" && nextState == deps.StopBeforeState {
				slog.Info("DRIVE_TRACE return stop-before", "bead", deps.BeadID, "iter", i, "state", bead.State, "next", nextState)
				return RunBeadResult{FinalState: bead.State, Success: true}, nil
			}
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

		epicID := epicIDFor(bead)
		artifactDir, err := resolveArtifactDir(deps.StateDir, epicID, deps.BeadID)
		if err != nil {
			return RunBeadResult{FinalState: activeState, Success: false}, err
		}

		if deps.AgentStateStore != nil && activeStage.Kind == "subprocess" {
			// Subprocess flow
			runtimeState, err := deps.AgentStateStore.Load(deps.BeadID)
			if err != nil {
				return RunBeadResult{FinalState: activeState, Success: false},
					fmt.Errorf("failed to load agent state: %w", err)
			}

			req := subprocess.HandoffRequest{
				EpicID:         epicID,
				BeadID:         deps.BeadID,
				WorktreePath:   deps.Worktree,
				ContextPayload: runtimeState.ContextPayload,
			}

			// Captured before dispatch so commit_marker gates can scope their
			// scan to what this stage produced, not the branch's prior
			// history (see resolveArtifactDir and backend.ExitGateContext).
			baseSHA := worktreeHeadSHA(deps.Worktree)
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
			gatePassed, gateReason := backend.EvaluateExitGate(wf, backend.ExitGateContext{
				FromState:       activeState,
				WorktreePath:    deps.Worktree,
				ArtifactDir:     artifactDir,
				BeadID:          deps.BeadID,
				BeadDescription: gateDesc,
				BaseSHA:         baseSHA,
			})
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
					artifactPath := resolveArtifactRef(activeState, wf.Stages, deps.BeadID, artifactDir)
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

		// An empty verify command would render rule 4 as an empty code block:
		// the agent runs nothing and declares itself done, which is the exact
		// failure this stopped being a hardcoded Go command to prevent.
		// Same reason as the verify command: an empty tracker command renders
		// the stage's own bookkeeping instructions as bare flags with no
		// program in front of them.
		if strings.TrimSpace(deps.TrackerCommand) == "" {
			return RunBeadResult{FinalState: bead.State, Success: false},
				fmt.Errorf("KERNL DISPATCH FAILURE: no tracker command for bead %s - the stage prompt would name no tracker at all - Fix: resolve it with backend.TrackerInvocation and pass it in DriveBeadDeps.TrackerCommand", deps.BeadID)
		}
		if strings.TrimSpace(deps.VerifyCommand) == "" {
			return RunBeadResult{FinalState: bead.State, Success: false},
				fmt.Errorf("KERNL DISPATCH FAILURE: no verify command for bead %s - the stage prompt would tell the agent to run nothing before declaring done - Fix: resolve it with epic.ResolveVerifyCommand and pass it in DriveBeadDeps.VerifyCommand", deps.BeadID)
		}
		promptInput := StagePromptInput{
			Bead:           bead,
			State:          activeState,
			Stages:         wf.Stages,
			RepoPath:       deps.RepoPath,
			Worktree:       deps.Worktree,
			VerifyCommand:  deps.VerifyCommand,
			TrackerCommand: deps.TrackerCommand,
			Dialect:        adapter.ResolveDialect(agentInput.Command),
			ArtifactDir:    artifactDir,
		}
		var prompt string
		if deps.BuildPrompt != nil {
			prompt = deps.BuildPrompt(promptInput, wf)
		} else {
			prompt = BuildBeadStagePrompt(promptInput)
		}
		// An agent spawned with no prompt does whatever it infers from the
		// working directory, which is the worst possible reading of "the stage
		// produced nothing". A prompt builder that declined must stop the run.
		if strings.TrimSpace(prompt) == "" {
			return RunBeadResult{FinalState: bead.State, Success: false},
				fmt.Errorf("KERNL DISPATCH FAILURE: empty prompt for bead %s at state %s - Fix: the stage prompt builder returned nothing; check the preceding error for why it declined", deps.BeadID, activeState)
		}
		agentInput.Command, agentInput.Args = BuildStageArgs(
			adapter.AgentTarget{Command: agentInput.Command, Model: agentInput.Model, ApprovalMode: agentInput.ApprovalMode},
			agentInput.Args, deps.BeadID, deps.Worktree, deps.SessionID, prompt)
		if agentInput.Env == nil {
			agentInput.Env = make(map[string]string)
		}
		if adapter.ResolveDialect(agentInput.Command) == adapter.DialectOpenCode {
			if cfgErr := applyOpencodePermissions(agentInput.Env, deps.Config, deps.StateDir, deps.BeadID, activeState, artifactDir, wf.Stages); cfgErr != nil {
				return RunBeadResult{FinalState: bead.State, Success: false}, cfgErr
			}
		}

		agentInput.BeadID = deps.BeadID
		agentInput.RepoPath = deps.RepoPath
		agentInput.Cwd = deps.Worktree

		agentInput.Env["BEAD_ID"] = deps.BeadID
		agentInput.Env["REPO_PATH"] = deps.RepoPath

		// Captured before dispatch so commit_marker gates can scope their
		// scan to what this stage produced, not the branch's prior history
		// (see resolveArtifactDir and backend.ExitGateContext).
		baseSHA := worktreeHeadSHA(deps.Worktree)
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
		gatePassed, gateReason := backend.EvaluateExitGate(wf, backend.ExitGateContext{
			FromState:       activeState,
			WorktreePath:    deps.Worktree,
			ArtifactDir:     artifactDir,
			BeadID:          deps.BeadID,
			BeadDescription: gateDesc,
			BaseSHA:         baseSHA,
		})
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
				artifactPath := resolveArtifactRef(activeState, wf.Stages, deps.BeadID, artifactDir)
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
		fmt.Errorf("KERNL DISPATCH FAILURE: bead %s exceeded max stages (%d) - Fix: check workflow for cycles", deps.BeadID, maxStages)
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

func resolveArtifactRef(state string, stages map[string]backend.StageContract, beadID, artifactDir string) string {
	if stages == nil {
		return ""
	}
	sc, ok := stages[state]
	if !ok {
		return ""
	}
	return backend.ResolveArtifactPath(sc.OutputArtifact.Path, beadID, artifactDir)
}

// epicIDFor answers which artifact-directory scope a bead belongs to: its
// own ID when it has no parent (a standalone bead, or an epic itself), or
// its parent epic's ID otherwise - so an epic's own artifacts and its
// children's don't collide under the same directory.
func epicIDFor(bead *backend.Bead) string {
	if bead.ParentID != "" {
		return bead.ParentID
	}
	return bead.ID
}

// isSafePathComponent reports whether s is usable as a single path segment
// under kernl's state directory. epicID and beadID are tracker data - in the
// scenario this project is built for, the tracker belongs to a repository
// kernl does not own - so they are attacker-reachable strings, not trusted
// identifiers. filepath.Join cleans ".." away rather than rejecting it, so
// joining an unvalidated component can walk the result outside StateDir
// entirely; resolveArtifactDir also re-checks the joined path itself, but
// this is the first line of defense and the one that names which id is bad.
func isSafePathComponent(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	return !strings.ContainsAny(s, `/\`)
}

// resolveArtifactDir is where exit-gate artifacts (plan.md, review verdicts,
// ...) live for one bead - deliberately outside the worktree, so a stage's
// own `git add <files>` can never sweep kernl's control files into the
// target repository's commits. PR #40 on archeion published
// .kernl/<bead>/*.md in a diff because these used to live inside the
// worktree instead.
func resolveArtifactDir(stateDir, epicID, beadID string) (string, error) {
	if stateDir == "" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: no state directory for bead %s, so kernl has nowhere of its own to write exit-gate artifacts outside the worktree - Fix: set DriveBeadDeps.StateDir (app.DefaultStateDir() outside tests)", beadID)
	}
	if !isSafePathComponent(epicID) {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: bead %s has an unsafe epic/parent id %q for its artifact directory - Fix: the id must be a single path segment with no '/', '\\', '.', or '..'", beadID, epicID)
	}
	if !isSafePathComponent(beadID) {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: bead id %q is unsafe for its artifact directory - Fix: the id must be a single path segment with no '/', '\\', '.', or '..'", beadID)
	}

	runRoot := filepath.Join(stateDir, "run")
	dir := filepath.Join(runRoot, epicID, beadID)
	// Belt and braces: even validated components could combine into
	// something unanticipated, so the joined result itself must still land
	// under runRoot before anything is created or granted access to it.
	rel, err := filepath.Rel(runRoot, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: artifact dir %s for bead %s escapes %s - Fix: epic/parent id %q and bead id %q must resolve to a path beneath it", dir, beadID, runRoot, epicID, beadID)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: creating artifact dir %s for bead %s: %w", dir, beadID, err)
	}
	return dir, nil
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

// applyOpencodePermissions points OPENCODE_CONFIG at the allowlist the spawned
// agent must honor, so it does not fall back to opencode's own defaults (which
// auto-reject external_directory reads like /tmp/*). When the workflow carries
// stage contracts, the allowlist is specialized per stage with that stage's
// forbidden paths denied.
//
// It fails rather than warns. A stage that runs without its deny rules is a
// stage running under a policy nobody chose, and the warning it used to emit
// was several screens away from the rejections it caused.
//
// An OPENCODE_CONFIG the caller set explicitly wins and is left alone.
func applyOpencodePermissions(env map[string]string, cfg *config.Config, kernlDir, beadID, stage, artifactDir string, stages map[string]backend.StageContract) error {
	if _, exists := env["OPENCODE_CONFIG"]; exists {
		return nil
	}
	if kernlDir == "" {
		return fmt.Errorf("KERNL DISPATCH FAILURE: no state directory for bead %s, so kernl has nowhere of its own to write the agent's allowlist - Fix: set DriveBeadDeps.StateDir (app.DefaultStateDir() outside tests)", beadID)
	}
	var configured string
	if cfg != nil {
		configured = cfg.Orchestrator.OpencodeConfigPath
	}
	basePath, err := resolveOpencodeBaseConfig(configured, kernlDir)
	if err != nil {
		return err
	}
	env["OPENCODE_CONFIG"] = basePath

	if len(stages) == 0 {
		return nil
	}
	stageDir := filepath.Join(kernlDir, "run", beadID)
	stagePath, err := writeStageOpencodeConfig(basePath, stageDir, beadID, stage, artifactDir, stages)
	if err != nil {
		return err
	}
	env["OPENCODE_CONFIG"] = stagePath
	return nil
}

// DefaultStateDir is the directory kernl owns for files that belong to a run
// but not to the repository being worked on. It is resolved at the process
// boundary and passed down, never re-derived mid-dispatch: a unit test that
// reached the derivation wrote into the operator's real ~/.kernl.
func DefaultStateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: cannot resolve the home directory kernl keeps its state in - %w - Fix: set HOME, or set orchestrator.opencodeConfigPath in kernl.yaml to an explicit path", err)
	}
	return filepath.Join(home, ".kernl"), nil
}

// appendOpencodeStageFlags adds the per-stage flags opencode needs to
// (a) work in the correct directory, (b) carry a recognizable session title
// in the agent UI, and (c) actually receive the prompt - mirroring the
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
	// Positional prompt goes LAST - opencode treats trailing positionals
	// as the message.
	out = append(out, prompt)
	return out
}
