package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/adapter"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/subprocess"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

// LLMSessionEnvVar names the affinity key kernl hands a dispatched agent so
// the LLM proxy in front of it can keep one conversation on one upstream
// account. It exists because the accounts behind that proxy cache prompt
// prefixes per account: a stage whose ~40-150 model calls are load-balanced
// across three of them lands cold on most of those calls and reprocesses its
// whole history, in latency and in plan quota. The routing that buys
// throughput is what destroys cache locality, and only the caller knows
// which calls belong together.
//
// The value is the bead id, which is the granularity that gets both
// properties at once. Finer (one key per stage attempt) would drop the warm
// account between a rejected attempt and its retry, where the stage prompt
// and repo context - the prefix that actually caches - are near-identical.
// Coarser (one key per epic) would pin every bead of a run to a single
// account and leave the others idle, which is the same failure as hardcoding
// a pinned alias. Different beads still spread, because their prefixes
// genuinely differ and pinning them together would buy nothing.
//
// kernl only names the session; it deliberately does not choose an account.
// Mapping the key to a deployment and expiring it is the proxy's job, and
// reimplementing that here would mean tracking session-to-account state and
// its TTL in application code.
//
// It is set for every dialect rather than gated on one, because it is inert
// for an agent whose configuration never reads it, and gating would bake in
// the assumption that only today's agent talks to a proxy. Consuming it is
// one line of agent configuration - for pi, a `headers` entry sending
// `x-litellm-session-id` from this variable. An agent run outside kernl
// simply has it unset and falls back to the proxy's normal load balancing.
const LLMSessionEnvVar = "KERNL_LLM_SESSION_ID"

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
	// RunID is the workflow_run node id StartWorkflowRun returned for this
	// dispatch. recordDecisionIfGateType threads it into
	// WriteDecisionRecordNode so a decision is linked to the run that
	// produced it by an edge, not by inference from which beads that run
	// happened to drive - see WriteDecisionRecordNode's own doc comment.
	// Both CLI call sites (cmd/kernl/epic.go, cmd/kernl/bead.go) always have
	// one by the time they call DriveBeadToTerminal.
	RunID string
	// HeadSHAResolver reports a worktree's current HEAD SHA for the ledger
	// and gate context. Nil defaults to GitHeadSHAResolver{} (the real
	// git-shelling implementation, mirroring StageAttemptInput.DiffStats in
	// attempt_ledger.go) - production call sites never need to set this;
	// only tests inject a fake, so exercising this loop never requires a
	// real git binary on the host (AGENTS.md §4).
	HeadSHAResolver HeadSHAResolver
	// DA is the actor an implementer's fork handover is asked. Nil means no
	// DA is configured - the fork gate is off, and an implementer keeps
	// deciding every fork alone, exactly as it does today. This is a
	// supported state, not a failure: see app.NewDA's own doc comment on the
	// same "nil, nil means off" contract app.NewOracle already established.
	DA DA
	// ContextDocs is registry.repos[].contextDocs for this repository -
	// resolved once per run by the CLI call site, exactly like VerifyCommand
	// and TrackerCommand, rather than this package re-deriving a RepoEntry
	// lookup from Config.Registry.Repos by matching RepoPath. It feeds the
	// same app.AssembleContext the Oracle already reads (Unit A of
	// local/artifacts/plans/2026-08-01-composer-context-and-fork-gate-plan.md
	// §2.3), reused here for the DA rather than a second context path.
	ContextDocs []string
}

// HeadSHAResolver reports a worktree's current HEAD short SHA, or "" when it
// cannot be determined. It exists as a seam so the bead-driving loop's own
// tests do not have to shell out to the host's git binary just to get a
// stable answer - the same reason attempt_ledger.go's DiffStatter exists.
type HeadSHAResolver interface {
	HeadSHA(worktree string) string
}

// GitHeadSHAResolver is the production HeadSHAResolver: git rev-parse
// --short HEAD against the given worktree.
type GitHeadSHAResolver struct{}

// HeadSHA delegates to worktreeHeadSHA, the pre-existing implementation.
func (GitHeadSHAResolver) HeadSHA(worktree string) string {
	return worktreeHeadSHA(worktree)
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
	headSHA := deps.HeadSHAResolver
	if headSHA == nil {
		headSHA = GitHeadSHAResolver{}
	}

	var lastResult RunBeadResult
	prevState := ""
	// reviewRewinds counts how many times a rejected implementation has been
	// handed back to an implementer in THIS call - the budget that stops a
	// reviewer and an implementer disagreeing in a loop. See
	// implementationReviewRewindLimit.
	reviewRewinds := 0
	// forkGateCalls counts how many fork handovers THIS call has already
	// decided (re-entering the same stage each time) - the budget that stops
	// an implementer and the DA looping forever. See forkHandoverLimit.
	forkGateCalls := 0
	// pendingRework carries the review artifact of a rejection this call just
	// rewound, so the dispatch that answers it records what it is answering
	// instead of leaving the ledger to work that out from the rows around it
	// (StageAttemptRecord.CausedBy). Empty except between a rewind and the
	// one attempt that follows it.
	pendingRework := ""
	// retryIterations is how many of this call's iterations exist only because
	// a mechanical block was retried, rather than because the workflow moved
	// forward. They are added to the ceiling instead of being charged against
	// it: maxStages exists to catch a workflow that cycles, and its error text
	// says so, so a run that spent its iterations on legitimate retries must
	// not be reported as a cycle. The total stays bounded - the retry budget
	// is per stage entry (mechanicalBlockRetryLimit, reset only by a fresh
	// claim), so this can add at most that many per stage.
	retryIterations := 0
	// takeRework hands that artifact to the attempt being recorded and
	// forgets it. Every ledger write in this loop takes it, so a rewind can
	// only ever mark the FIRST attempt after it - including an attempt that
	// died before its gate, which is a retake that failed, not a rewind still
	// waiting to be claimed by a later stage.
	takeRework := func() string {
		claimed := pendingRework
		pendingRework = ""
		return claimed
	}

	for i := 0; i < maxStages+retryIterations; i++ {
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
			// This bead was already blocked BEFORE this call - a retry of a
			// prior attempt, not a fresh gate failure this iteration
			// produced. The transition that set State to "blocked" never
			// touches the wf:state:* label, so it is still whatever it was
			// the moment before the block - the one place that information
			// survives, since "blocked" itself carries no memory of which
			// stage caused it. blockBeadWithCause's own wf:blocked:* label
			// (blocked_cause.go) is what now says WHY, and that decides
			// whether this call may retry the stage on its own or must
			// leave the bead exactly as it found it - a judgment block (or
			// one with no recorded cause at all, e.g. blocked by hand) is
			// never resumed silently; only that decision is the one this
			// bead is asking for.
			blockedAt, resumable := mechanicalResumeAllowed(bead.Labels)
			cause := BlockedCauseFromLabels(bead.Labels)
			retries := blockedRetryCountFromLabels(bead.Labels)
			if resumable {
				newLabels := filterOutLabelPrefix(bead.Labels, blockedCauseLabelPrefix)
				newLabels = filterOutLabelPrefix(newLabels, blockedRetryLabelPrefix)
				newLabels = append(newLabels, blockedRetryLabelPrefix+strconv.Itoa(retries+1))
				if err := deps.Backend.Update(deps.BeadID, backend.UpdateBeadInput{
					State:     blockedAt,
					SetLabels: newLabels,
				}, deps.RepoPath); err != nil {
					return RunBeadResult{FinalState: bead.State, Success: false},
						fmt.Errorf("KERNL DISPATCH FAILURE: resuming bead %s from a %s block back to %s: %w", deps.BeadID, cause, blockedAt, err)
				}
				slog.Info("DRIVE_TRACE resume mechanical block", "bead", deps.BeadID, "iter", i, "cause", cause, "retry", retries+1, "limit", mechanicalBlockRetryLimit, "resumeState", blockedAt)
				// Neither this iteration nor the dispatch it leads to is a
				// stage this workflow advanced through, so neither may be
				// charged to maxStages - see retryIterations. The clamp is a
				// backstop, not the bound: the budget label is what stops
				// this, and it lives in the tracker, so a tracker that
				// silently dropped the label would otherwise raise the
				// ceiling on every pass and spawn agents forever. Reaching
				// the clamp ends the run as an exceeded ceiling, which is
				// what a bead resuming without ever spending its budget is.
				retryIterations = min(retryIterations+2, maxStages*mechanicalBlockRetryLimit)
				prevState = bead.State
				continue
			}
			if cause.IsMechanical() && blockedAt != "" {
				slog.Info("DRIVE_TRACE mechanical block retries exhausted", "bead", deps.BeadID, "iter", i, "cause", cause, "retries", retries, "limit", mechanicalBlockRetryLimit)
			}
			slog.Info("DRIVE_TRACE return blocked", "bead", deps.BeadID, "iter", i, "blockedAtState", blockedAt, "cause", cause)
			return RunBeadResult{FinalState: bead.State, Success: false, BlockedAtState: blockedAt, GateFailureReason: string(cause)}, nil
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
		// Whether this iteration is entering a genuinely new stage, as
		// opposed to re-entering one the bead is already sitting in (a
		// mechanical-block resume, a fork handover coming back). It scopes
		// the exit gate's base commit - see resolveStageEpochBase.
		claimedFreshStage := false
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
				// A fresh claim means a genuinely NEW stage is starting -
				// any wf:blocked-retries:* left over from a previous,
				// unrelated stage's mechanical block must not count against
				// this one (see mechanicalBlockRetryLimit).
				newLabels = filterOutLabelPrefix(newLabels, blockedRetryLabelPrefix)
				if err := deps.Backend.Update(deps.BeadID, backend.UpdateBeadInput{
					State:     nextState,
					SetLabels: newLabels,
				}, deps.RepoPath); err != nil {
					slog.Info("DRIVE_TRACE return claim-failed", "bead", deps.BeadID, "iter", i, "from", bead.State, "to", nextState, "err", err)
					return RunBeadResult{FinalState: bead.State, Success: false},
						fmt.Errorf("KERNL DISPATCH FAILURE: advancing bead %s from %s to %s: %w", deps.BeadID, bead.State, nextState, err)
				}
				activeState = nextState
				claimedFreshStage = true
				slog.Info("DRIVE_TRACE claimed", "bead", deps.BeadID, "iter", i, "from", bead.State, "to", nextState)
			}
		}

		activeStage := wf.Stages[activeState]

		epicID := epicIDFor(bead)
		artifactDir, err := resolveArtifactDir(deps.StateDir, epicID, deps.BeadID)
		if err != nil {
			return RunBeadResult{FinalState: activeState, Success: false}, err
		}

		// Before any agent is spawned for this bead: a bead whose own text
		// says its design is still open is not autonomous work, and
		// dispatching it would have an implementer choose an approach
		// nobody committed to. handleDepthGate is a no-op for every other
		// bead, and asks at most once per bead (see its own doc comment).
		gate, err := handleDepthGate(ctx, deps, bead, epicID, artifactDir, activeState, forkGateCalls)
		if err != nil {
			return RunBeadResult{FinalState: activeState, Success: false}, err
		}
		forkGateCalls = gate.ForkGateCalls
		if gate.Blocked {
			return gate.Result, nil
		}

		// A decision_record gate on this stage needs the epic's own title
		// to build its reference node (see recordDecisionIfGateType).
		// Fetched here, before the agent runs, so a transient tracker
		// failure or a concurrently deleted epic is discovered before an
		// agent invocation is spent - rather than discarding a successful
		// agent run and a passed gate afterward, the way a fetch made only
		// after the gate had already passed would.
		epicTitle := ""
		if epicID != "" && epicID != deps.BeadID {
			if _, ok := backend.FindExitGateByType(wf, activeState, "decision_record"); ok {
				epicBead, epicErr := deps.Backend.Get(epicID, deps.RepoPath)
				if epicErr != nil {
					return RunBeadResult{FinalState: activeState, Success: false},
						fmt.Errorf("KERNL DISPATCH FAILURE: bead %s at stage %s needs its epic %s to record a decision, but the epic could not be fetched from %s: %w - Fix: confirm the epic bead still exists in the tracker at that repo path", deps.BeadID, activeState, epicID, deps.RepoPath, epicErr)
				}
				if epicBead == nil {
					return RunBeadResult{FinalState: activeState, Success: false},
						fmt.Errorf("KERNL DISPATCH FAILURE: bead %s at stage %s needs its epic %s to record a decision, but the epic was not found in %s - Fix: confirm the epic bead still exists in the tracker at that repo path", deps.BeadID, activeState, epicID, deps.RepoPath)
				}
				epicTitle = epicBead.Title
			}
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
			// history (see resolveArtifactDir and backend.ExitGateContext) -
			// and captured once per stage ENTRY rather than per dispatch, so
			// re-entering a stage does not measure it against its own commit
			// (see resolveStageEpochBase).
			baseSHA, err := resolveStageEpochBase(artifactDir, activeState, deps.Worktree, claimedFreshStage, headSHA)
			if err != nil {
				return RunBeadResult{FinalState: activeState, Success: false}, err
			}
			startTime := time.Now()
			subprocessAgentID := "subprocess"
			if len(activeStage.Subprocess.Command) > 0 {
				subprocessAgentID = activeStage.Subprocess.Command[0]
			}
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

				// A subprocess that timed out, ran over its output cap, or
				// exited non-zero is a real attempt with a real outcome -
				// not recording it here (the only place this branch ever
				// reaches an exit-gate-shaped result) would bias the
				// ledger toward subprocess stages that happened to work.
				// No structured exit code is available from
				// subprocess.SubprocessError - only ExitCode: nil is
				// honest here, not a guess reconstructed from error text.
				if ledgerErr := AppendStageAttempt(deps.StateDir, epicID, BuildStageAttemptRecord(StageAttemptInput{
					AgentID:           subprocessAgentID,
					Dialect:           "subprocess",
					BeadID:            deps.BeadID,
					Stage:             activeState,
					StartedAt:         startTime,
					Duration:          time.Since(startTime),
					BaseSHA:           baseSHA,
					CommitSHA:         headSHA.HeadSHA(deps.Worktree),
					Worktree:          deps.Worktree,
					GatePassed:        false,
					GateFailureReason: "subprocess_" + causeStr,
					CausedBy:          takeRework(),
				})); ledgerErr != nil {
					slog.Error("DRIVE_TRACE attempt ledger write failed", "bead", deps.BeadID, "err", ledgerErr)
				}

				// Truncate stderr dumped into the bead comment at a sane limit (64KB) with a truncation marker.
				const maxStderrLen = 65536
				if len(stderr) > maxStderrLen {
					stderr = stderr[:maxStderrLen] + "\n... (truncated)"
				}

				commentBody := fmt.Sprintf("subprocess stage %s failed: %s\n\nStderr:\n%s", activeState, causeStr, stderr)

				blockBeadWithCause(deps.Backend, deps.BeadID, deps.RepoPath, BlockedCauseSubprocess)
				_ = deps.Backend.Comment(deps.BeadID, commentBody, deps.RepoPath)
				return RunBeadResult{FinalState: "blocked", Success: false, BlockedAtState: activeState, GateFailureReason: "subprocess_" + causeStr}, nil
			}

			runtimeState.ContextPayload = resp.ContextPayload
			if err := deps.AgentStateStore.Save(deps.BeadID, runtimeState); err != nil {
				return RunBeadResult{FinalState: activeState, Success: false},
					fmt.Errorf("failed to save agent state: %w", err)
			}

			duration := time.Since(startTime)
			gateDesc := ""
			freshBead, ferr := deps.Backend.Get(deps.BeadID, deps.RepoPath)
			if ferr == nil && freshBead != nil {
				gateDesc = freshBead.Description
			}
			commitSHA := headSHA.HeadSHA(deps.Worktree)
			agentID := subprocessAgentID
			// RunSubprocessStage only reaches here when the subprocess's own
			// cmd.Run() returned no error, so it did exit cleanly - unlike
			// the failure branch above, a real exit code (0) is available.
			cleanExit := 0

			// The fork gate is checked BEFORE the exit gate is ever
			// evaluated, and skips it entirely when a handover applies: an
			// implementer that handed a fork over stopped deliberately
			// without committing, so decision_record/commit_marker would
			// fail it - turning the handover into exactly the interruption
			// this gate exists to prevent. See handleForkGate's own doc
			// comment.
			forkHandled, forkErr := handleForkGate(ctx, forkGateAttemptContext{
				Deps:        deps,
				WF:          wf,
				Bead:        freshBead,
				ActiveState: activeState,
				ArtifactDir: artifactDir,
				EpicID:      epicID,
				CallsUsed:   forkGateCalls,
				LedgerInput: StageAttemptInput{
					AgentID:   agentID,
					Dialect:   "subprocess",
					BeadID:    deps.BeadID,
					Stage:     activeState,
					StartedAt: startTime,
					Duration:  duration,
					ExitCode:  &cleanExit,
					BaseSHA:   baseSHA,
					CommitSHA: commitSHA,
					Worktree:  deps.Worktree,
				},
			})
			if forkErr != nil {
				return RunBeadResult{FinalState: activeState, Success: false}, forkErr
			}
			if forkHandled.Applied {
				if forkHandled.Reenter {
					forkGateCalls++
					prevState = bead.State
					continue
				}
				return forkHandled.Result, nil
			}

			gateCtx := backend.ExitGateContext{
				FromState:       activeState,
				WorktreePath:    deps.Worktree,
				ArtifactDir:     artifactDir,
				BeadID:          deps.BeadID,
				BeadDescription: gateDesc,
				BaseSHA:         baseSHA,
			}
			gatePassed, gateReason, gateEvidence := backend.EvaluateExitGateWithEvidence(wf, gateCtx)
			if err := AppendStageAttempt(deps.StateDir, epicID, BuildStageAttemptRecord(StageAttemptInput{
				AgentID:           agentID,
				Dialect:           "subprocess",
				BeadID:            deps.BeadID,
				Stage:             activeState,
				StartedAt:         startTime,
				Duration:          duration,
				ExitCode:          &cleanExit,
				BaseSHA:           baseSHA,
				CommitSHA:         commitSHA,
				Worktree:          deps.Worktree,
				GatePassed:        gatePassed,
				GateFailureReason: gateReason,
				GateEvidence:      gateEvidence,
				ReviewVerdict:     reviewVerdictForGate(wf, gateCtx),
				CausedBy:          takeRework(),
			})); err != nil {
				slog.Error("DRIVE_TRACE attempt ledger write failed", "bead", deps.BeadID, "err", err)
			}
			if gatePassed {
				// Mirrors a decision_record gate's record into the graph
				// before the bead is allowed to move on - a no-op for every
				// other gate type. Run before the state transition, not
				// after: a bead must not advance past a stage whose
				// reasoning failed to persist (see recordDecisionIfGateType).
				if err := recordDecisionIfGateType(ctx, wf, gateCtx, deps, bead, epicID, epicTitle); err != nil {
					return RunBeadResult{FinalState: activeState, Success: false}, err
				}
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
					if err := deps.Backend.Comment(deps.BeadID, buildStageComment(activeState, agentID, "", artifactPath, commitSHA, duration), deps.RepoPath); err != nil {
						slog.Warn("DRIVE_TRACE comment failed", "bead", deps.BeadID, "err", err)
					}
				}
			} else {
				// A reviewer that deliberately rejected the work has somewhere
				// to send it: back to the implementer (today's behavior), or,
				// when it classified the rejection as needing an explicit
				// approved decision, to the same DA a proactive fork handover
				// reaches. Every other gate failure still blocks. Both
				// dispatch paths (this one, subprocess; and the native flow
				// below) call the SAME handleGateFailure rather than each
				// inlining this decision - see that function's own doc
				// comment for why.
				handled, handleErr := handleGateFailure(ctx, gateFailureContext{
					Deps:          deps,
					WF:            wf,
					Bead:          freshBead,
					ActiveState:   activeState,
					ArtifactDir:   artifactDir,
					EpicID:        epicID,
					GateReason:    gateReason,
					ReviewRewinds: reviewRewinds,
					ForkGateCalls: forkGateCalls,
				})
				if handleErr != nil {
					return RunBeadResult{FinalState: activeState, Success: false}, handleErr
				}
				if handled.Reenter {
					// Every re-entry handleGateFailure returns today is a
					// rewind, so this guard never fires. It states the
					// dependency rather than assuming it holds forever: the
					// proactive fork gate above already re-enters WITHOUT
					// sending work back, and a second such path returning
					// from here would otherwise be recorded as rework
					// silently.
					if handled.ReviewRewinds > reviewRewinds {
						pendingRework = reworkArtifact(gateReason)
					}
					reviewRewinds = handled.ReviewRewinds
					forkGateCalls = handled.ForkGateCalls
					prevState = bead.State
					continue
				}
				return handled.Result, nil
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
		// Fetched only for the native flow (deps.BuildPrompt == nil): that is
		// both production shapes that ever render a bead's own generic
		// implementer prompt - `kernl bead run` and an epic's child beads
		// (cmd/kernl/epic.go's per-child RunBead, which drives implementation
		// through this same loop with no override). The epic BEAD's own
		// integration/integration_review/shipment stages set BuildPrompt and
		// render a wholly different template (internal/prompt) that does not
		// read StagePromptInput.RelevantDecisions at all - those stages merge
		// and ship code that was already decided, they do not make new design
		// decisions, so paying for a graph lookup whose result would be
		// silently discarded there bought nothing.
		var relevantDecisions []RelevantDecision
		if deps.BuildPrompt == nil {
			relevantDecisions = relatedDecisionsForPrompt(ctx, deps, bead)
		}
		// A rewind is worth nothing if the implementer cannot see why the
		// work came back. The review artifact says so itself - it is only
		// carried when it ends in a REJECT - so no separate "this is a
		// retry" flag has to be kept in sync with the bead's state.
		var rejectedReview string
		if activeState == "implementation" {
			rejectedReview = readRejectedReview(
				backend.ResolveArtifactPath("<artifact_dir>/implementation-review.md", deps.BeadID, artifactDir))
		}
		// A stage that cannot hand a fork over must never be told it can
		// (see StagePromptInput.ForkHandoverPath): the INVITATION stays
		// gated on forkHandoverArmed, which is false for every reviewer
		// stage and every run with no DA configured - today's behavior.
		//
		// The ANSWER is not gated the same way, and the two were conflated
		// until the depth gate started producing answers of its own. A
		// decision that has already been taken must reach whoever
		// implements it, always: gating the read on this stage declaring a
		// decision_record exit gate meant a profile without one silently
		// dropped a settled decision on the floor, and the implementer
		// re-decided it. Reading an answer that exists costs nothing when
		// there is none; not reading one that does is how the decision the
		// operator was spared gets made twice.
		forkAnswer := readForkAnswerArtifact(deps.BeadID, artifactDir)
		var forkHandoverPath string
		if forkHandoverArmed(wf, activeState, deps.DA) {
			forkHandoverPath = resolvedForkHandoverPath(deps.BeadID, artifactDir)
		}
		promptInput := StagePromptInput{
			Bead:              bead,
			State:             activeState,
			Stages:            wf.Stages,
			RepoPath:          deps.RepoPath,
			Worktree:          deps.Worktree,
			VerifyCommand:     deps.VerifyCommand,
			TrackerCommand:    deps.TrackerCommand,
			Dialect:           adapter.ResolveDialect(agentInput.Command),
			ArtifactDir:       artifactDir,
			RelevantDecisions: relevantDecisions,
			RejectedReview:    rejectedReview,
			PriorGateFailure:  LastGateFailure(deps.StateDir, epicID, deps.BeadID, activeState),
			ForkHandoverPath:  forkHandoverPath,
			ForkAnswer:        forkAnswer,
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
		agentInput.Env[LLMSessionEnvVar] = deps.BeadID

		// Captured before dispatch so commit_marker gates can scope their
		// scan to what this stage produced, not the branch's prior history
		// (see resolveArtifactDir and backend.ExitGateContext) - and captured
		// once per stage ENTRY rather than per dispatch, so re-entering a
		// stage does not measure it against its own commit (see
		// resolveStageEpochBase).
		baseSHA, err := resolveStageEpochBase(artifactDir, activeState, deps.Worktree, claimedFreshStage, headSHA)
		if err != nil {
			return RunBeadResult{FinalState: activeState, Success: false}, err
		}
		startTime := time.Now()
		slog.Info("DRIVE_TRACE spawn", "bead", deps.BeadID, "iter", i, "activeState", activeState, "agent", agentInput.AgentName)
		res, err := deps.Driver.RunBead(ctx, agentInput)
		attemptDialect := string(adapter.ResolveDialect(agentInput.Command))
		if err != nil {
			if ledgerErr := AppendStageAttempt(deps.StateDir, epicID, BuildStageAttemptRecord(StageAttemptInput{
				AgentID:           agentInput.AgentName,
				Dialect:           attemptDialect,
				ConfiguredModel:   agentInput.Model,
				Pool:              agentInput.Pool,
				BeadID:            deps.BeadID,
				Stage:             activeState,
				SessionID:         res.SessionID,
				StartedAt:         startTime,
				Duration:          time.Since(startTime),
				ExitCode:          res.ExitCode,
				BaseSHA:           baseSHA,
				CommitSHA:         headSHA.HeadSHA(deps.Worktree),
				Worktree:          deps.Worktree,
				GatePassed:        false,
				GateFailureReason: err.Error(),
				Failure:           res.Failure,
				FollowUpCount:     res.FollowUpCount,
				Nudged:            res.Nudged,
				Usage:             res.Usage,
				Turns:             res.Turns,
			})); ledgerErr != nil {
				slog.Error("DRIVE_TRACE attempt ledger write failed", "bead", deps.BeadID, "err", ledgerErr)
			}
			slog.Info("DRIVE_TRACE return agent-err", "bead", deps.BeadID, "iter", i, "err", err)
			return RunBeadResult{FinalState: res.FinalState, Success: false},
				fmt.Errorf("KERNL DISPATCH FAILURE: agent %s for bead %s: %w", agentInput.AgentName, deps.BeadID, err)
		}
		if !res.Success {
			reason := res.GateFailureReason
			if reason == "" {
				reason = fmt.Sprintf("agent exited non-zero (exit code %s)", formatExitCode(res.ExitCode))
			}
			if ledgerErr := AppendStageAttempt(deps.StateDir, epicID, BuildStageAttemptRecord(StageAttemptInput{
				AgentID:           agentInput.AgentName,
				Dialect:           attemptDialect,
				ConfiguredModel:   agentInput.Model,
				Pool:              agentInput.Pool,
				BeadID:            deps.BeadID,
				Stage:             activeState,
				SessionID:         res.SessionID,
				StartedAt:         startTime,
				Duration:          time.Since(startTime),
				ExitCode:          res.ExitCode,
				BaseSHA:           baseSHA,
				CommitSHA:         headSHA.HeadSHA(deps.Worktree),
				Worktree:          deps.Worktree,
				GatePassed:        false,
				GateFailureReason: reason,
				Failure:           res.Failure,
				FollowUpCount:     res.FollowUpCount,
				Nudged:            res.Nudged,
				Usage:             res.Usage,
				Turns:             res.Turns,
			})); ledgerErr != nil {
				slog.Error("DRIVE_TRACE attempt ledger write failed", "bead", deps.BeadID, "err", ledgerErr)
			}
			slog.Info("DRIVE_TRACE return agent-not-success", "bead", deps.BeadID, "iter", i, "resFinalState", res.FinalState, "reason", reason)
			return RunBeadResult{FinalState: res.FinalState, Success: false, GateFailureReason: reason}, nil
		}
		duration := time.Since(startTime)
		slog.Info("DRIVE_TRACE post-spawn ok", "bead", deps.BeadID, "iter", i, "resFinalState", res.FinalState)

		// Re-fetch the bead so description-based exit gates (e.g. shipment's
		// pr_url marker) see what the agent just wrote, not the stale snapshot
		// from the top of this iteration.
		gateDesc := ""
		freshBead, ferr := deps.Backend.Get(deps.BeadID, deps.RepoPath)
		if ferr == nil && freshBead != nil {
			gateDesc = freshBead.Description
		}
		commitSHA := headSHA.HeadSHA(deps.Worktree)

		// The fork gate is checked BEFORE the exit gate is ever evaluated,
		// and skips it entirely when a handover applies - see the
		// subprocess flow's identical check above and handleForkGate's own
		// doc comment for why this ordering is load-bearing.
		forkHandled, forkErr := handleForkGate(ctx, forkGateAttemptContext{
			Deps:        deps,
			WF:          wf,
			Bead:        freshBead,
			ActiveState: activeState,
			ArtifactDir: artifactDir,
			EpicID:      epicID,
			CallsUsed:   forkGateCalls,
			LedgerInput: StageAttemptInput{
				AgentID:         agentInput.AgentName,
				Dialect:         attemptDialect,
				ConfiguredModel: agentInput.Model,
				Pool:            agentInput.Pool,
				BeadID:          deps.BeadID,
				Stage:           activeState,
				SessionID:       res.SessionID,
				StartedAt:       startTime,
				Duration:        duration,
				ExitCode:        res.ExitCode,
				BaseSHA:         baseSHA,
				CommitSHA:       commitSHA,
				Worktree:        deps.Worktree,
				FollowUpCount:   res.FollowUpCount,
				Nudged:          res.Nudged,
				Usage:           res.Usage,
				Turns:           res.Turns,
			},
		})
		if forkErr != nil {
			return RunBeadResult{FinalState: activeState, Success: false}, forkErr
		}
		if forkHandled.Applied {
			if forkHandled.Reenter {
				forkGateCalls++
				prevState = bead.State
				continue
			}
			return forkHandled.Result, nil
		}

		gateCtx := backend.ExitGateContext{
			FromState:       activeState,
			WorktreePath:    deps.Worktree,
			ArtifactDir:     artifactDir,
			BeadID:          deps.BeadID,
			BeadDescription: gateDesc,
			BaseSHA:         baseSHA,
		}
		gatePassed, gateReason, gateEvidence := backend.EvaluateExitGateWithEvidence(wf, gateCtx)
		if err := AppendStageAttempt(deps.StateDir, epicID, BuildStageAttemptRecord(StageAttemptInput{
			AgentID:           agentInput.AgentName,
			Dialect:           attemptDialect,
			ConfiguredModel:   agentInput.Model,
			Pool:              agentInput.Pool,
			BeadID:            deps.BeadID,
			Stage:             activeState,
			SessionID:         res.SessionID,
			StartedAt:         startTime,
			Duration:          duration,
			ExitCode:          res.ExitCode,
			BaseSHA:           baseSHA,
			CommitSHA:         commitSHA,
			Worktree:          deps.Worktree,
			GatePassed:        gatePassed,
			GateFailureReason: gateReason,
			GateEvidence:      gateEvidence,
			ReviewVerdict:     reviewVerdictForGate(wf, gateCtx),
			CausedBy:          takeRework(),
			FollowUpCount:     res.FollowUpCount,
			Nudged:            res.Nudged,
			Usage:             res.Usage,
			Turns:             res.Turns,
		})); err != nil {
			slog.Error("DRIVE_TRACE attempt ledger write failed", "bead", deps.BeadID, "err", err)
		}
		if gatePassed {
			// Mirrors a decision_record gate's record into the graph before
			// the bead is allowed to move on - a no-op for every other gate
			// type. Run before the state transition, not after: a bead must
			// not advance past a stage whose reasoning failed to persist
			// (see recordDecisionIfGateType).
			if err := recordDecisionIfGateType(ctx, wf, gateCtx, deps, bead, epicID, epicTitle); err != nil {
				return RunBeadResult{FinalState: activeState, Success: false}, err
			}
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
				if err := deps.Backend.Comment(deps.BeadID, buildStageComment(activeState, agentInput.AgentName, res.SessionID, artifactPath, commitSHA, duration), deps.RepoPath); err != nil {
					slog.Warn("DRIVE_TRACE comment failed", "bead", deps.BeadID, "err", err)
				}
			}
		} else {
			// A reviewer that deliberately rejected the work has somewhere
			// to send it: back to the implementer (today's behavior), or,
			// when it classified the rejection as needing an explicit
			// approved decision, to the same DA a proactive fork handover
			// reaches. Every other gate failure still blocks. Both dispatch
			// paths (the subprocess flow above, and this native flow) call
			// the SAME handleGateFailure rather than each inlining this
			// decision - see that function's own doc comment for why.
			handled, handleErr := handleGateFailure(ctx, gateFailureContext{
				Deps:          deps,
				WF:            wf,
				Bead:          freshBead,
				ActiveState:   activeState,
				ArtifactDir:   artifactDir,
				EpicID:        epicID,
				GateReason:    gateReason,
				ReviewRewinds: reviewRewinds,
				ForkGateCalls: forkGateCalls,
			})
			if handleErr != nil {
				return RunBeadResult{FinalState: activeState, Success: false}, handleErr
			}
			if handled.Reenter {
				if handled.ReviewRewinds > reviewRewinds {
					pendingRework = reworkArtifact(gateReason)
				}
				reviewRewinds = handled.ReviewRewinds
				forkGateCalls = handled.ForkGateCalls
				prevState = bead.State
				continue
			}
			return handled.Result, nil
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

// escapesRoot reports whether dir, once joined from caller-supplied
// components, no longer resolves beneath root - the shared belt-and-braces
// check for every path kernl builds from tracker-owned ids (resolveArtifactDir,
// resolveAttemptLedgerPath in attempt_ledger.go).
func escapesRoot(root, dir string) bool {
	rel, err := filepath.Rel(root, dir)
	return err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// formatExitCode renders an exit code for a log message or a ledger
// GateFailureReason, without ever fabricating one: nil (no process ever
// exited to report a code) renders distinctly from any real numeric value,
// including 0.
func formatExitCode(code *int) string {
	if code == nil {
		return "none (no process exited)"
	}
	return strconv.Itoa(*code)
}

// resolveArtifactDir is where exit-gate artifacts (plan.md, review verdicts,
// ...) live for one bead - deliberately outside the worktree, so a stage's
// own `git add <files>` can never sweep kernl's control files into the
// target repository's commits. PR #40 on archeion published
// .kernl/<bead>/*.md in a diff because these used to live inside the
// worktree instead.
func resolveArtifactDir(stateDir, epicID, beadID string) (string, error) {
	dir, err := ArtifactDirPath(stateDir, epicID, beadID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: creating artifact dir %s for bead %s: %w", dir, beadID, err)
	}
	return dir, nil
}

// ArtifactDirPath is the same location without creating it. Exported because a
// reader outside this package - the CLI checking what shipment published -
// has to find an artifact a stage already wrote, and re-deriving the layout at
// that call site is how two places end up disagreeing about where it is.
func ArtifactDirPath(stateDir, epicID, beadID string) (string, error) {
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
	if escapesRoot(runRoot, dir) {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: artifact dir %s for bead %s escapes %s - Fix: epic/parent id %q and bead id %q must resolve to a path beneath it", dir, beadID, runRoot, epicID, beadID)
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

// stateFromStaleLabel recovers the workflow state a bead was active in
// before something set its status to "blocked" - the blocking transition
// itself only ever writes State (see the two "blocked" branches above), so
// the wf:state:* label a prior claim step set is never overwritten and
// still names the stage whose gate actually failed. Returns "" when no such
// label exists (a bead blocked some other way, e.g. by hand).
func stateFromStaleLabel(labels []string) string {
	for _, l := range labels {
		if strings.HasPrefix(l, "wf:state:") {
			return strings.TrimPrefix(l, "wf:state:")
		}
	}
	return ""
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
