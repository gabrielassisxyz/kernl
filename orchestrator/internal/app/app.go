package app

import (
	"context"
	"fmt"
	"io"
	"os/exec"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/epic"
	"github.com/gabrielassisxyz/kernl/internal/session"
	"github.com/gabrielassisxyz/kernl/internal/terminal"
)

type App struct {
	Backend    backend.BackendPort
	Terminal   *terminal.TerminalManager
	SCM        *session.SessionConnectionManager
	Driver     *SessionDriver
	Config     *config.Config
	EpicEvents *epic.EpicEventHub
}

func NewApp(cfg *config.Config) (*App, error) {
	if len(cfg.Registry.Repos) == 0 {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: no repos registered in config registry — at least one repo path is required — Fix: add a repo to registry.repos in kernl.yaml")
	}

	repoPath := cfg.Registry.Repos[0].Path
	be := backend.NewBdCliBackend(repoPath)

	tm := terminal.NewTerminalManager(
		terminal.WithMaxSessions(cfg.Orchestrator.MaxConcurrentBeads),
	)

	provider := &terminalSessionProvider{tm: tm}
	scm := session.NewSessionConnectionManager(provider, nil)

	spawn := execSpawnFunc
	driver := NewSessionDriver(DriverDeps{
		Backend: be,
		Spawn:   spawn,
		SCM:     scm,
	})

	return &App{
		Backend:    be,
		Terminal:   tm,
		SCM:        scm,
		Driver:     driver,
		Config:     cfg,
		EpicEvents: epic.NewEpicEventHub(),
	}, nil
}

func execSpawnFunc(ctx context.Context, cmd string, args []string, cwd string, env []string) (Process, io.Reader, io.Reader, error) {
	c := exec.CommandContext(ctx, cmd, args...)
	if cwd != "" {
		c.Dir = cwd
	}
	if len(env) > 0 {
		c.Env = env
	}
	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := c.StderrPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("stderr pipe: %w", err)
	}
	if err := c.Start(); err != nil {
		return nil, nil, nil, fmt.Errorf("start command: %w", err)
	}
	return &osProcess{cmd: c}, stdout, stderr, nil
}

type osProcess struct {
	cmd *exec.Cmd
}

func (p *osProcess) Wait() error { return p.cmd.Wait() }
func (p *osProcess) Kill() error { return p.cmd.Process.Kill() }

type terminalSessionProvider struct {
	tm *terminal.TerminalManager
}

func (p *terminalSessionProvider) GetSessionEntry(id string) (session.SessionInfo, bool) {
	entry, ok := p.tm.GetSession(id)
	if !ok {
		return session.SessionInfo{}, false
	}
	return session.SessionInfo{
		ID:        entry.Session.ID,
		BeadID:    entry.Session.BeadID,
		BeadTitle: entry.Session.BeadTitle,
		RepoPath:  entry.Session.RepoPath,
		Status:    string(entry.Session.Status),
	}, true
}

func (p *terminalSessionProvider) ListSessionIDs() []session.SessionInfo {
	sessions := p.tm.ListSessions()
	result := make([]session.SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		result = append(result, session.SessionInfo{
			ID:        s.ID,
			BeadID:    s.BeadID,
			BeadTitle: s.BeadTitle,
			RepoPath:  s.RepoPath,
			Status:    string(s.Status),
		})
	}
	return result
}

func (p *terminalSessionProvider) PushEvent(id string, evt session.TerminalEvent) {
	p.tm.PushEvent(id, evt)
}
