package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/adapter"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/session"
	"github.com/gabrielassisxyz/kernl/internal/terminal"
)

type Process interface {
	Wait() error
	Kill() error
}

type SpawnFunc func(ctx context.Context, cmd string, args []string, cwd string, env []string) (Process, io.Reader, io.Reader, error)

type DriverDeps struct {
	Backend       backend.BackendPort
	Spawn         SpawnFunc
	SCM           *session.SessionConnectionManager
	NudgeRegistry *session.NudgeRegistry
	// LogDir is the base directory kernl writes per-bead per-agent stdout/stderr
	// logs under. It is resolved once at the process boundary (app.DefaultStateDir()
	// plus "logs") and passed in rather than derived here, because a function deep
	// in the dispatch loop that resolves its own home directory wrote into the
	// operator's real ~/.kernl/logs from every unit test that reached it.
	LogDir string
}

// RunBeadInput tells the driver which bead to run and which agent to spawn.
//
// Command is the agent CLI binary (e.g. "opencode"). Args are passed after it.
// Env is exported into the spawned process if non-empty.
// AgentName is the logical name of the agent (the key in settings.agents,
// e.g. "deepseek-v4-pro-high") and is used for the session ID and logs  -
// distinct from the binary, which is the same `opencode` for every agent
// when going through litellm.
type RunBeadInput struct {
	BeadID string
	// RepoPath is the canonical bd-tracked repo -- passed to every backend call.
	RepoPath string
	// Cwd is the working directory for the spawned agent process. Defaults to
	// RepoPath when empty. Set this to the bead's isolated worktree so the
	// agent edits files in isolation while bd reads/writes stay on the repo.
	Cwd     string
	Command string
	// Args are the operator's extra flags from settings.agents.<id>.args.
	// The prompt-carrying shape itself is built per-dialect by BuildStageArgs.
	Args []string
	// Model is passed to the CLI in whatever way its dialect expects
	// (--model, -m, -c model="..."), which is why it travels as a value
	// rather than pre-baked into Args.
	Model        string
	ApprovalMode string
	Env          map[string]string
	AgentName    string
	// SessionID is the opencode session to resume via -s. When non-empty
	// the driver passes it through appendOpencodeStageFlags so the agent
	// reconnects to its existing conversation context instead of starting
	// a brand-new session.
	SessionID string
	// TrackerCommand is how this repository's tracker is typed. Recorded on
	// the nudge record because a follow-up prompt tells the agent to inspect
	// and advance the bead, and it cannot name the tracker it does not know.
	TrackerCommand string
	// Pool is the settings.pools key this agent was selected from (e.g.
	// "implementation"). Carried through so the stage-attempt ledger can
	// record which pool dispatched the agent, distinct from AgentName (which
	// agent within the pool actually ran).
	Pool string
}

type RunBeadResult struct {
	SessionID  string
	FinalState string
	Success    bool
	// ExitCode is the spawned process's real exit code, or nil when no
	// process ever exited to report one - a spawn failure, or any error
	// surfaced before proc.Wait() returned an exit status. -1 means the
	// process was terminated by a signal (see exec.ExitError.ExitCode()).
	// The stage-attempt ledger records it as a fact independent of Success,
	// which also folds in the follow-up-refusal case below.
	ExitCode *int
	// Usage is the last terminal token-usage event the dialect's stream
	// reported (codex's turn.completed, claude's result), or nil when the
	// run never emitted one - e.g. it errored before completing a turn.
	Usage *session.TokenUsageCounts
	// FollowUpCount is how many take-loop nudges this run sent before the
	// agent's turn ended for good.
	FollowUpCount int
	// Nudged reports whether at least one follow-up was sent.
	Nudged bool
	// BlockedAtState names the workflow state that was active when
	// FinalState became "blocked" - FinalState itself is only ever the
	// literal string "blocked" in that case, which loses which stage
	// actually failed. A caller that must react to one SPECIFIC stage's
	// own failure (Phase 6's fix-up mechanism reacts only to
	// integration_review, never to a different stage that happens to
	// block while an earlier integration_review's own artifact is still
	// sitting on disk from a prior attempt) keys on this field instead of
	// re-reading a well-known artifact path and hoping it is fresh. Empty
	// for every FinalState other than "blocked".
	BlockedAtState string
	// GateFailureReason is the exit gate's own failure reason for
	// BlockedAtState (backend.EvaluateExitGate's vocabulary:
	// verdict_reject, verdict_not_pass, commit_marker_missing, ...), or the
	// subprocess flow's own "subprocess_<cause>" when the block was not a
	// gate evaluation at all. Empty whenever BlockedAtState is.
	GateFailureReason string
	// Turns is the runtime's own live count of counted turn boundaries
	// (session.SessionRuntime.CountedTurns: pi's turn_end, opencode's
	// step_finish), or nil when this dialect counts none at all - distinct
	// from Usage's dialect-reported num_turns (claude only). The
	// stage-attempt ledger falls back to this when Usage reports none.
	Turns *int64
}

type SessionDriver struct {
	backend backend.BackendPort
	spawn   SpawnFunc
	scm     *session.SessionConnectionManager
	nudges  *session.NudgeRegistry
	logDir  string
}

func NewSessionDriver(deps DriverDeps) *SessionDriver {
	return &SessionDriver{
		backend: deps.Backend,
		spawn:   deps.Spawn,
		scm:     deps.SCM,
		nudges:  deps.NudgeRegistry,
		logDir:  deps.LogDir,
	}
}

// NudgeRegistry exposes the registry to callers (e.g. App.Nudge) that need to
// query spawn context for an existing session.
func (d *SessionDriver) NudgeRegistry() *session.NudgeRegistry { return d.nudges }

func (d *SessionDriver) RunBead(ctx context.Context, input RunBeadInput) (RunBeadResult, error) {
	bead, err := d.backend.Get(input.BeadID, input.RepoPath)
	if err != nil || bead == nil {
		return RunBeadResult{}, fmt.Errorf("KERNL DISPATCH FAILURE: bead %s not found in repo %s: %w", input.BeadID, input.RepoPath, err)
	}
	if input.Command == "" {
		return RunBeadResult{}, fmt.Errorf("KERNL DISPATCH FAILURE: RunBeadInput.Command empty for bead %s - Fix: resolve an agent from settings.pools before calling RunBead", input.BeadID)
	}
	if d.logDir == "" {
		return RunBeadResult{}, fmt.Errorf("KERNL DISPATCH FAILURE: no log directory for bead %s, so kernl has nowhere of its own to write agent stdout/stderr logs - Fix: set DriverDeps.LogDir (app.DefaultStateDir() outside tests)", input.BeadID)
	}

	dialect := adapter.ResolveDialect(input.Command)
	// RunBead always dispatches one-shot: input.Args is built by BuildStageArgs,
	// which calls adapter.BuildPromptModeArgs - the cli-arg/one-shot shape
	// (`-p <prompt>`, `exec <prompt>`, ...) - never the interactive transport
	// (stream-json over stdin, JSON-RPC, HTTP server). Passing interactive=true
	// here would hand out a capability profile promising a delivery channel
	// this invocation never opens.
	r := session.NewSessionRuntimeWithCapabilities(input.BeadID, input.RepoPath, string(dialect), false)
	usageLogger := session.NewCapturingUsageLogger()
	r.SetTokenUsageLogger(usageLogger)

	envSlice := envMapToSlice(input.Env)
	cwd := input.Cwd
	if cwd == "" {
		cwd = input.RepoPath
	}
	proc, stdout, stderr, err := d.spawn(ctx, input.Command, input.Args, cwd, envSlice)
	if err != nil {
		return RunBeadResult{}, fmt.Errorf("KERNL DISPATCH FAILURE: spawn agent %s (%s): %w", input.AgentName, input.Command, err)
	}

	// Tee stdout+stderr to per-bead log files so stuck-state failures
	// always leave forensic breadcrumbs. Best-effort: if the log dir
	// can't be created or files can't be opened, the agent still runs;
	// the logs are diagnostic, not load-bearing.
	stdoutLogPath, stderrLogPath, closeLogs := openStageLogs(d.logDir, input.BeadID, input.AgentName)
	stdout = io.TeeReader(stdout, stdoutLogPath.w)
	stderr = io.TeeReader(stderr, stderrLogPath.w)
	defer closeLogs()
	slog.Info("agent log files opened",
		"bead", input.BeadID, "agent", input.AgentName,
		"stdout", stdoutLogPath.path, "stderr", stderrLogPath.path)

	agentLabel := input.AgentName
	if agentLabel == "" {
		agentLabel = input.Command
	}
	sessionID := fmt.Sprintf("%s-%s", input.BeadID, agentLabel)
	d.scm.Connect(sessionID)

	// Register spawn context so manual /nudge requests can later re-resolve
	// the agent and respawn against the captured opencode session id.
	d.nudges.Upsert(sessionID, session.NudgeRecord{
		BeadID:             input.BeadID,
		RepoPath:           input.RepoPath,
		Cwd:                cwd,
		OpencodeConfigPath: input.Env["OPENCODE_CONFIG"],
		TrackerCommand:     input.TrackerCommand,
		Running:            true,
	})
	defer d.nudges.SetRunning(sessionID, false)

	// Registration BEFORE the readers, and the order is the contract: a
	// reader that consumes the terminal event while onTurnEnded is still nil
	// drops the only signal that fires a follow-up, and no later barrier can
	// replay it - WaitDrained waits for the readers to finish, not for a
	// callback that was never called. The pump is safe to start first
	// because r.events is created by the constructor, not by Start.
	w := &sessionPump{
		scm:       d.scm,
		runtime:   r,
		sessionID: sessionID,
		beadID:    input.BeadID,
		repoPath:  input.RepoPath,
		backend:   d.backend,
	}
	w.start()
	reportRunBeadPhase(phasePumpRegistered)

	r.Start(ctx, stdout, stderr)
	reportRunBeadPhase(phaseReadersStarted)

	// Drain stdout/stderr to EOF before reaping the process. exec.Cmd's
	// StdoutPipe/StderrPipe are closed by Wait() as soon as it sees the
	// process exit; calling Wait() first (as this used to) races that close
	// against the goroutines still reading the final buffered lines, which
	// can silently drop the very event (e.g. claude's "result") that fires
	// onTurnEnded - so a stalled stage becomes indistinguishable from a
	// stage that never needed a nudge.
	r.WaitDrained()

	// Read back after WaitDrained: the reader goroutine that fed the
	// logger has fully finished by this point (see WaitDrained's own
	// comment on why Wait() used to race it), so this read cannot miss the
	// event carrying the run's token totals.
	usage := usageLogger.Usage()

	exitErr := proc.Wait()
	exitCode := exitCodeFromErr(exitErr)
	success := exitCode != nil && *exitCode == 0 && !r.IsError()

	gateFailureReason := ""
	if !success {
		if r.IsError() && r.LastError() != "" {
			gateFailureReason = r.LastError()
		} else if exitCode != nil && *exitCode != 0 {
			gateFailureReason = fmt.Sprintf("agent exited non-zero (exit code %s)", formatExitCode(exitCode))
		}
	}

	capturedSID := r.CapturedSessionID()
	if capturedSID != "" {
		d.nudges.SetOpencodeSessionID(sessionID, capturedSID)
	}
	r.Dispose()
	w.stop()

	finalBead, err := d.backend.Get(input.BeadID, input.RepoPath)
	finalState := "unknown"
	if err == nil && finalBead != nil {
		finalState = finalBead.State
	}

	// Prefer the real opencode session ID captured from the NDJSON stream;
	// fall back to the constructed label so callers always get a non-empty ID.
	resultSessionID := capturedSID
	if resultSessionID == "" {
		resultSessionID = sessionID
	}

	// A follow-up refused for lack of a delivery channel (SupportsFollowUp
	// false) means the stage could not be nudged out of a stuck state. An
	// exit-zero exit code from the agent process does not make that stage
	// successful - report the refusal as the run's own failure so the
	// caller (DriveBeadToTerminal) halts instead of evaluating the exit
	// gate and advancing the bead on a stage nobody actually finished.
	followUpCount, nudged := w.followUpStats()

	// resultTurns is CountedTurns exposed on the result for the ledger
	// writer (internal/app/attempt_ledger.go) - nil for a dialect that
	// counts no turn boundary at all, never a fabricated number.
	var resultTurns *int64
	if countedTurns, counted := r.CountedTurns(); counted {
		resultTurns = &countedTurns
	}

	// Checked before the follow-up refusal below because it is the more
	// specific cause: a dispatch stopped for looping did not merely end
	// badly, it was ended BY kernl, and the exit code it leaves behind
	// names the signal rather than the reason. Without this the ledger
	// records "agent exited non-zero (exit code 143)" for a runaway agent,
	// which is indistinguishable from an operator killing it by hand - the
	// exact ambiguity that made the first one take 39 minutes to notice.
	if exceeded, turns := r.TurnCeilingExceeded(); exceeded {
		return RunBeadResult{
			SessionID:     resultSessionID,
			FinalState:    finalState,
			Success:       false,
			ExitCode:      exitCode,
			Usage:         usage,
			FollowUpCount: followUpCount,
			Nudged:        nudged,
			Turns:         resultTurns,
		}, fmt.Errorf("KERNL DISPATCH FAILURE: bead %s crossed %d turns in one dispatch, over the %d turn limit, and was stopped - an agent this far past the limit is repeating itself rather than progressing - Fix: read the stage's log for the repeated action, and if the work genuinely needs this many turns, split the bead", input.BeadID, turns, session.MaxTurnsPerDispatch)
	}

	if followUpErr := w.followUpError(); followUpErr != nil {
		return RunBeadResult{
			SessionID:     resultSessionID,
			FinalState:    finalState,
			Success:       false,
			ExitCode:      exitCode,
			Usage:         usage,
			FollowUpCount: followUpCount,
			Nudged:        nudged,
			Turns:         resultTurns,
		}, followUpErr
	}

	return RunBeadResult{
		SessionID:         resultSessionID,
		FinalState:        finalState,
		Success:           success,
		ExitCode:          exitCode,
		Usage:             usage,
		FollowUpCount:     followUpCount,
		Nudged:            nudged,
		GateFailureReason: gateFailureReason,
		Turns:             resultTurns,
	}, nil
}

// The two RunBead phases whose ORDER is a correctness invariant rather than
// an implementation detail. They exist because the invariant is otherwise
// untestable: "a reader saw a nil callback" only reproduces when the reader
// goroutine happens to win, so a passing `-count=N` loop is evidence of
// scheduling luck and never of the ordering. Recording the order instead is
// deterministic - the buggy arrangement reports readers first, every run.
const (
	phasePumpRegistered = "pump_registered"
	phaseReadersStarted = "readers_started"
)

// runBeadPhaseHook is nil in production; a test sets it to capture the order
// above. Guarded by runBeadPhaseMu so -race stays quiet when a test that sets
// it runs beside one that does not.
var (
	runBeadPhaseHook func(phase string)
	runBeadPhaseMu   sync.RWMutex
)

func reportRunBeadPhase(phase string) {
	runBeadPhaseMu.RLock()
	hook := runBeadPhaseHook
	runBeadPhaseMu.RUnlock()
	if hook != nil {
		hook(phase)
	}
}

type sessionPump struct {
	scm       *session.SessionConnectionManager
	runtime   *session.SessionRuntime
	sessionID string
	beadID    string
	repoPath  string
	backend   backend.BackendPort

	stopCh chan struct{}
	done   chan struct{}

	mu            sync.Mutex
	followUpErr   error
	followUpCount int
	nudged        bool
}

// followUpError reports the most recent hard failure from the take-loop's
// follow-up gate (set by handleTurnEnded when terminal.HandleTakeLoopTurnEnded
// returns a non-nil error), so RunBead can surface it after the run instead
// of only the boolean handleTurnEnded returns to SetOnTurnEnded - that bool
// already carries a different meaning (whether to keep stdin open) and can't
// also carry "this run must be reported as failed."
func (p *sessionPump) followUpError() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.followUpErr
}

func (p *sessionPump) setFollowUpError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.followUpErr == nil {
		p.followUpErr = err
	}
}

// recordFollowUpSent tracks that HandleTakeLoopTurnEnded actually sent a
// nudge prompt (its "proceed" return is only ever true after SendUserTurn
// succeeded - see terminal.HandleTakeLoopTurnEnded), so RunBead can report
// how many follow-ups a run needed for the stage-attempt ledger.
func (p *sessionPump) recordFollowUpSent() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.followUpCount++
	p.nudged = true
}

// followUpStats reports how many follow-ups this run sent, safe to call
// once the runtime's reader goroutine (the only writer) has stopped.
func (p *sessionPump) followUpStats() (count int, nudged bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.followUpCount, p.nudged
}

func (p *sessionPump) start() {
	p.stopCh = make(chan struct{})
	p.done = make(chan struct{})

	r := p.runtime
	r.SetOnTurnEnded(func(reason string) bool {
		return p.handleTurnEnded(reason)
	})

	go func() {
		defer close(p.done)
		for {
			select {
			case evt, ok := <-r.Events():
				if !ok {
					return
				}
				p.scm.HandleEvent(p.sessionID, evt)
			case <-p.stopCh:
				for {
					select {
					case evt, ok := <-r.Events():
						if !ok {
							return
						}
						p.scm.HandleEvent(p.sessionID, evt)
					default:
						return
					}
				}
			}
		}
	}()
}

func (p *sessionPump) stop() {
	close(p.stopCh)
	<-p.done
}

func (p *sessionPump) handleTurnEnded(reason string) bool {
	bead, err := p.backend.Get(p.beadID, p.repoPath)
	if err != nil || bead == nil {
		slog.Warn("driver: turn-ended bead fetch failed", "beadId", p.beadID, "error", err)
		return false
	}

	ctx := &terminal.TakeLoopContext{
		ID:               p.sessionID,
		BeadID:           p.beadID,
		Bead:             bead,
		RepoPath:         p.repoPath,
		ResolvedRepoPath: p.repoPath,
		Entry: &terminal.SessionEntry{
			Session: &terminal.TerminalSession{ID: p.sessionID},
		},
		PushEvent: func(evt session.TerminalEvent) {
			p.scm.HandleEvent(p.sessionID, evt)
		},
		TakeIteration:    &terminal.IterationCounter{Value: 1},
		FollowUpAttempts: &terminal.FollowUpCounter{},
		Dialect:          p.runtime.Dialect(),
		Capabilities:     p.runtime.Capabilities(),
		TurnFailed:       p.runtime.IsError(),
	}

	deps := terminal.FollowUpDeps{
		GetBead: func(beadID, repoPath string) (*backend.Bead, error) {
			return p.backend.Get(beadID, repoPath)
		},
		SendUserTurn: func(prompt, source string) bool {
			return p.runtime.SendUserTurn(prompt)
		},
		LeaseChecker: &terminal.DefaultLeaseHealthChecker{},
	}

	proceed, err := terminal.HandleTakeLoopTurnEnded(ctx, deps)
	if err != nil {
		p.setFollowUpError(err)
	}
	if proceed {
		p.recordFollowUpSent()
	}
	return proceed
}

// exitCodeFromErr reports the process's real exit status from proc.Wait()'s
// error, never a fabricated stand-in. A nil err means a clean exit (code 0).
// A non-nil err that IS an *exec.ExitError carries the process's own exit
// code (ExitCode() returns -1 when the process was killed by a signal
// rather than exiting normally - still the real, distinct fact, not code 1
// standing in for "something failed"). Any other error (Wait() itself
// failing for a reason unrelated to the child's exit status) returns nil:
// no process exit was ever observed, so there is no code to report.
func exitCodeFromErr(err error) *int {
	if err == nil {
		code := 0
		return &code
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		return &code
	}
	return nil
}

// stageLog wraps a file writer with its filesystem path so callers can both
// write to it and report where it lives.
type stageLog struct {
	path string
	w    io.Writer
}

// discardLog is the fallback used when we cannot open a real log file  -
// the agent still runs, we just lose forensic data for this stage.
var discardLog = stageLog{path: "(discarded - log open failed)", w: io.Discard}

// openStageLogs opens per-bead per-agent stdout/stderr log files under
// <baseLogDir>/<bead>/<timestamp>-<agent>.{stdout,stderr}.log. baseLogDir is
// resolved once at the process boundary (DriverDeps.LogDir) and never
// re-derived here. Always returns usable stageLogs (real files or
// io.Discard) plus a single close func the caller must defer.
func openStageLogs(baseLogDir, beadID, agentName string) (stageLog, stageLog, func()) {
	closers := []func() error{}
	closeAll := func() {
		for _, c := range closers {
			_ = c()
		}
	}

	logDir := filepath.Join(baseLogDir, beadID)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		slog.Warn("agent log dir create failed; logs will be discarded",
			"dir", logDir, "error", err)
		return discardLog, discardLog, closeAll
	}

	ts := time.Now().Format("20060102-150405")
	agent := agentName
	if agent == "" {
		agent = "agent"
	}
	mkLog := func(stream string) stageLog {
		p := filepath.Join(logDir, fmt.Sprintf("%s-%s.%s.log", ts, agent, stream))
		f, err := os.Create(p)
		if err != nil {
			slog.Warn("agent log file create failed; stream discarded",
				"path", p, "error", err)
			return discardLog
		}
		closers = append(closers, f.Close)
		return stageLog{path: p, w: f}
	}
	return mkLog("stdout"), mkLog("stderr"), closeAll
}

// envMapToSlice converts a map[KEY]VALUE to ["KEY=VALUE", ...] for exec.Cmd.
// Returns nil for empty/nil input so SpawnFunc keeps the inherited environment.
func envMapToSlice(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}
