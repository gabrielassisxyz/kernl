package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/epic"
	"github.com/gabrielassisxyz/kernl/internal/merge"
	"github.com/gabrielassisxyz/kernl/internal/session"
	"github.com/gabrielassisxyz/kernl/internal/terminal"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

type App struct {
	Backend       backend.BackendPort
	Terminal      *terminal.TerminalManager
	SCM           *session.SessionConnectionManager
	Driver        *SessionDriver
	Config        *config.Config
	EpicEvents    *epic.EpicEventHub
	MergeManager  merge.TriggerRouter
	NudgeRegistry *session.NudgeRegistry
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
	nudges := session.NewNudgeRegistry()

	spawn := execSpawnFunc
	driver := NewSessionDriver(DriverDeps{
		Backend:       be,
		Spawn:         spawn,
		SCM:           scm,
		NudgeRegistry: nudges,
	})

	return &App{
		Backend:       be,
		Terminal:      tm,
		SCM:           scm,
		Driver:        driver,
		Config:        cfg,
		EpicEvents:    epic.NewEpicEventHub(),
		NudgeRegistry: nudges,
	}, nil
}

func execSpawnFunc(ctx context.Context, cmd string, args []string, cwd string, env []string) (Process, io.Reader, io.Reader, error) {
	c := exec.CommandContext(ctx, cmd, args...)
	if cwd != "" {
		c.Dir = cwd
	}
	if len(env) > 0 {
		// Layer caller overrides ON TOP of the inherited environment so
		// PATH / HOME / etc. survive — otherwise an agent with just
		// OPENCODE_CONFIG=... and nothing else can't find /usr/bin/git,
		// the bd binary, or the user's home dir.
		c.Env = append(os.Environ(), env...)
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

func (a *App) EpicMerge(epicID string) error {
	if len(a.Config.Registry.Repos) == 0 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: no repos registered — Fix: add a repo to registry.repos in kernl.yaml")
	}
	repoPath := a.Config.Registry.Repos[0].Path

	bead, err := a.Backend.Get(epicID, repoPath)
	if err != nil || bead == nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic %s not found in repo %s — Fix: verify the bead ID exists", epicID, repoPath)
	}

	if bead.State == "in_progress" {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic %s already in_progress — Fix: the merge dispatch is already in flight, wait for the merger agent to complete", epicID)
	}

	if bead.State != "blocked" {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic %s is %q, expected blocked — Fix: the epic must be blocked due to a merge issue before manual re-dispatch", epicID, bead.State)
	}

	hasConflictAt := workflow.GetMergeConflictAt(bead.Description) != ""
	outcomeStr := workflow.GetMergeOutcome(bead.Description)

	hasRecoverableOutcome := false
	if outcomeStr != "" {
		o, err := merge.ParseOutcome(outcomeStr)
		if err == nil && (o == merge.OutcomePushFailed || o == merge.OutcomePRCreateFailed) {
			hasRecoverableOutcome = true
		}
	}

	if !hasConflictAt && !hasRecoverableOutcome {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic %s has no recovery signal — neither merge_conflict_at nor a recoverable merge_outcome (push_failed, pr_create_failed) found in description — Fix: the epic must have a merge conflict or failed merge outcome before re-dispatch", epicID)
	}

	children, err := a.Backend.List(&backend.BeadListFilters{Parent: epicID}, repoPath)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: listing children for epic %s — %w — Fix: verify bd backend is reachable", epicID, err)
	}
	for _, child := range children {
		if child.State != "awaiting_integration" && child.State != "closed" {
			return fmt.Errorf("KERNL DISPATCH FAILURE: child %s is %q, must be awaiting_integration or closed — Fix: ensure all children have completed their work before re-dispatching the merge", child.ID, child.State)
		}
	}

	desc := workflow.RemoveMergeConflictAt(bead.Description)
	desc = workflow.RemoveMergeOutcome(desc)

	if err := a.Backend.Update(epicID, backend.UpdateBeadInput{
		State:       "in_progress",
		Description: desc,
	}, repoPath); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: cannot re-open epic %s — %w — Fix: verify bd backend is reachable", epicID, err)
	}

	if a.MergeManager != nil {
		if err := a.MergeManager.DispatchMerger(epicID); err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: dispatching merger for epic %s — %w", epicID, err)
		}
	}

	return nil
}
