package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/epic"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

func runEpicAbort(a *app.App, args []string, out func(string)) error {
	var epicID string
	var yes, dryRun bool
	for _, arg := range args {
		switch arg {
		case "--yes":
			yes = true
		case "--dry-run":
			dryRun = true
		default:
			if strings.HasPrefix(arg, "-") {
				return usagef("KERNL DISPATCH FAILURE: unknown epic abort flag %q%s — valid: --yes, --dry-run",
					arg, didYouMean(arg, []string{"--yes", "--dry-run"}))
			}
			epicID = arg
		}
	}
	if epicID == "" {
		return usagef("KERNL DISPATCH FAILURE: epic abort requires an epic ID — run: kernl epic abort <epic-id>")
	}
	if len(a.Config.Registry.Repos) == 0 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: no repos registered — Fix: add a repo to registry.repos in kernl.yaml")
	}
	repoPath := a.Config.Registry.Repos[0].Path

	ep, err := epic.LoadEpic(a.Backend, epicID, repoPath)
	if err != nil {
		return err
	}

	if dryRun {
		out(fmt.Sprintf("dry-run: abort would close epic %s and %d child bead(s), remove their worktrees and purge agent state\n", epicID, len(ep.Children)))
		for _, child := range ep.Children {
			out(fmt.Sprintf("dry-run: bead %s would be closed (aborted)\n", child.ID))
		}
		return nil
	}
	if !yes {
		return usagef("KERNL DISPATCH FAILURE: epic abort is destructive — it closes epic %s and %d child bead(s), removes worktrees and purges agent state. Re-run with --yes to proceed, or --dry-run to preview", epicID, len(ep.Children))
	}

	for _, child := range ep.Children {
		_, cerr := a.Backend.Close(child.ID, "aborted", repoPath)
		if cerr != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: cannot close child %s — %w — Fix: verify backend is reachable and bead is not already terminal", child.ID, cerr)
		}
		out(fmt.Sprintf("bead %s → closed (aborted)\n", child.ID))
	}

	_, eerr := a.Backend.Close(epicID, "aborted", repoPath)
	if eerr != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: cannot close epic %s — %w — Fix: verify backend is reachable and epic is not already terminal", epicID, eerr)
	}
	out(fmt.Sprintf("epic %s → closed (aborted)\n", epicID))

	// Cleanup filesystem artifacts.
	childIDs := make([]string, 0, len(ep.Children))
	for _, child := range ep.Children {
		childIDs = append(childIDs, child.ID)
	}

	wm := epic.NewWorktreeManager(a.Config.Orchestrator.WorktreeRoot, repoPath, nil, nil)
	if cerr := wm.CleanupEpic(epicID, childIDs); cerr != nil {
		return cerr
	}
	out(fmt.Sprintf("worktrees for epic %s cleaned\n", epicID))

	agentStateDir := filepath.Join(os.Getenv("HOME"), ".kernl", "agentstate")
	store, serr := workflow.NewAgentStateStore(agentStateDir)
	if serr != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: creating AgentStateStore: %w", serr)
	}
	for _, child := range ep.Children {
		if err := store.Purge(child.ID); err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: cannot purge agent state for %s: %w", child.ID, err)
		}
	}
	if err := store.Purge(epicID); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: cannot purge agent state for epic %s: %w", epicID, err)
	}
	out(fmt.Sprintf("agent state for epic %s purged\n", epicID))

	return nil
}
