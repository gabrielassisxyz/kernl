package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"

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

type RunBeadInput struct {
	BeadID   string
	RepoPath string
	AgentID  string
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

	dialect := adapter.ResolveDialect(input.AgentID)
	r := session.NewSessionRuntimeWithCapabilities(input.BeadID, input.RepoPath, string(dialect), true)

	proc, stdout, stderr, err := d.spawn(ctx, input.AgentID, nil, input.RepoPath, nil)
	if err != nil {
		return RunBeadResult{}, fmt.Errorf("KERNL DISPATCH FAILURE: spawn agent %s: %w", input.AgentID, err)
	}

	sessionID := fmt.Sprintf("%s-%s", input.BeadID, input.AgentID)
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

	r.Dispose()
	w.stop()

	finalBead, err := d.backend.Get(input.BeadID, input.RepoPath)
	finalState := "unknown"
	if err == nil && finalBead != nil {
		finalState = finalBead.State
	}

	return RunBeadResult{
		SessionID:  sessionID,
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
