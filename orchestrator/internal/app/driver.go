package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
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
	Backend backend.BackendPort
	Spawn   SpawnFunc
	SCM     *session.SessionConnectionManager
}

// RunBeadInput tells the driver which bead to run and which agent to spawn.
//
// Command is the agent CLI binary (e.g. "opencode"). Args are passed after it.
// Env is exported into the spawned process if non-empty.
// AgentName is the logical name of the agent (the key in settings.agents,
// e.g. "deepseek-v4-pro-high") and is used for the session ID and logs —
// distinct from the binary, which is the same `opencode` for every agent
// when going through litellm.
type RunBeadInput struct {
	BeadID    string
	// RepoPath is the canonical bd-tracked repo — passed to every backend call.
	RepoPath  string
	// Cwd is the working directory for the spawned agent process. Defaults to
	// RepoPath when empty. Set this to the bead's isolated worktree so the
	// agent edits files in isolation while bd reads/writes stay on the repo.
	Cwd       string
	Command   string
	Args      []string
	Env       map[string]string
	AgentName string
}

type RunBeadResult struct {
	SessionID  string
	FinalState string
	Success    bool
}

type SessionDriver struct {
	backend backend.BackendPort
	spawn   SpawnFunc
	scm     *session.SessionConnectionManager
}

func NewSessionDriver(deps DriverDeps) *SessionDriver {
	return &SessionDriver{
		backend: deps.Backend,
		spawn:   deps.Spawn,
		scm:     deps.SCM,
	}
}

func (d *SessionDriver) RunBead(ctx context.Context, input RunBeadInput) (RunBeadResult, error) {
	bead, err := d.backend.Get(input.BeadID, input.RepoPath)
	if err != nil || bead == nil {
		return RunBeadResult{}, fmt.Errorf("KERNL DISPATCH FAILURE: bead %s not found in repo %s: %w", input.BeadID, input.RepoPath, err)
	}
	claimedState := bead.State

	if input.Command == "" {
		return RunBeadResult{}, fmt.Errorf("KERNL DISPATCH FAILURE: RunBeadInput.Command empty for bead %s — Fix: resolve an agent from settings.pools before calling RunBead", input.BeadID)
	}

	dialect := adapter.ResolveDialect(input.Command)
	r := session.NewSessionRuntimeWithCapabilities(input.BeadID, input.RepoPath, string(dialect), true)

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
	stdoutLogPath, stderrLogPath, closeLogs := openStageLogs(input.BeadID, input.AgentName)
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

	r.Start(ctx, stdout, stderr)

	_ = claimedState

	w := &sessionPump{
		scm:       d.scm,
		runtime:   r,
		sessionID: sessionID,
		beadID:    input.BeadID,
		repoPath:  input.RepoPath,
		backend:   d.backend,
	}
	w.start()

	exitErr := proc.Wait()
	exitCode := exitCodeFromErr(exitErr)

	capturedSID := r.CapturedSessionID()
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

	return RunBeadResult{
		SessionID:  resultSessionID,
		FinalState: finalState,
		Success:    exitCode == 0,
	}, nil
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

	return terminal.HandleTakeLoopTurnEnded(ctx, deps)
}

func exitCodeFromErr(err error) int {
	if err == nil {
		return 0
	}
	return 1
}

// stageLog wraps a file writer with its filesystem path so callers can both
// write to it and report where it lives.
type stageLog struct {
	path string
	w    io.Writer
}

// discardLog is the fallback used when we cannot open a real log file —
// the agent still runs, we just lose forensic data for this stage.
var discardLog = stageLog{path: "(discarded — log open failed)", w: io.Discard}

// openStageLogs opens per-bead per-agent stdout/stderr log files under
// ~/.kernl/logs/<bead>/<timestamp>-<agent>.{stdout,stderr}.log. Always
// returns usable stageLogs (real files or io.Discard) plus a single
// close func the caller must defer.
func openStageLogs(beadID, agentName string) (stageLog, stageLog, func()) {
	closers := []func() error{}
	closeAll := func() {
		for _, c := range closers {
			_ = c()
		}
	}

	logDir := filepath.Join(os.Getenv("HOME"), ".kernl", "logs", beadID)
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
