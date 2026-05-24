package main

import (
	"context"
	"fmt"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/epic"
)

// runEpicMerge (re-)runs ONLY the epic-level integration stages: it drives the
// epic bead through integration -> integration_review -> shipment, ending at
// awaiting_pr_review. Use it to recover a blocked epic after fixing the cause,
// or to (re)trigger integration once every child is at awaiting_integration.
// It does not run the children — `kernl epic run` does that and then invokes
// the same epic drive automatically.
func runEpicMerge(a *app.App, args []string, out func(string)) error {
	if len(args) == 0 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic merge requires an epic ID — run: kernl epic merge <epic-id>")
	}
	if len(a.Config.Registry.Repos) == 0 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: no repos registered — Fix: add a repo to registry.repos in kernl.yaml")
	}
	epicID := args[0]
	repoPath := a.Config.Registry.Repos[0].Path

	ep, err := epic.LoadEpic(a.Backend, epicID, repoPath)
	if err != nil {
		return err
	}

	wm := epic.NewWorktreeManager(a.Config.Orchestrator.WorktreeRoot, repoPath, execGitRun, nil)
	if _, err := wm.EnsureEpicBranch(epicID); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: cannot ensure epic branch for %s: %w", epicID, err)
	}
	epicWorktree, err := wm.AddEpicWorktree(epicID)
	if err != nil {
		return err
	}

	return driveEpic(context.Background(), a, ep, epicID, repoPath, epicWorktree, out)
}
