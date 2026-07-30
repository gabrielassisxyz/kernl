package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/epic"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

// runEpicMerge (re-)runs ONLY the epic-level integration stages: it drives the
// epic bead through integration -> integration_review -> shipment, ending at
// awaiting_pr_review. Use it to recover a blocked epic after fixing the cause,
// or to (re)trigger integration once every child is at awaiting_integration.
// It does not run the children - `kernl epic run` does that and then invokes
// the same epic drive automatically.
func runEpicMerge(a *app.App, args []string, out func(string)) error {
	var dryRun bool
	var positional []string
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			dryRun = true
		default:
			if strings.HasPrefix(arg, "-") {
				return usagef("KERNL DISPATCH FAILURE: unknown epic merge flag %q%s - valid: --dry-run",
					arg, didYouMean(arg, []string{"--dry-run"}))
			}
			positional = append(positional, arg)
		}
	}
	if len(positional) == 0 {
		return usagef("KERNL DISPATCH FAILURE: epic merge requires an epic ID - run: kernl epic merge <epic-id>")
	}
	if len(a.Config.Registry.Repos) == 0 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: no repos registered - Fix: add a repo to registry.repos in kernl.yaml")
	}
	epicID := positional[0]
	repoEntry := a.Config.Registry.Repos[0]
	repoPath := repoEntry.Path

	// epic merge drives the same integration -> shipment tail as epic run, so
	// it needs the same verified destination. Containing only one of the two
	// entry points would leave the other publishing wherever it liked.
	plan, err := resolveShipmentPlan(repoEntry, dryRun, out)
	if err != nil {
		return err
	}

	ep, err := epic.LoadEpic(a.Backend, epicID, repoPath)
	if err != nil {
		return err
	}

	baseBranch, err := epic.ResolveBaseBranch(repoPath, repoEntry.DefaultBranch, execGitRun)
	if err != nil {
		return err
	}
	out(fmt.Sprintf("base branch: %s\n", baseBranch))

	wm := epic.NewWorktreeManager(a.Config.Orchestrator.WorktreeRoot, repoPath, baseBranch, execGitRun, nil)
	if _, err := wm.EnsureEpicBranch(epicID); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: cannot ensure epic branch for %s: %w", epicID, err)
	}
	epicWorktree, err := wm.AddEpicWorktree(epicID)
	if err != nil {
		return err
	}

	agentStateDir := filepath.Join(os.Getenv("HOME"), ".kernl", "agentstate")
	stateStore, err := workflow.NewAgentStateStore(agentStateDir)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: creating AgentStateStore: %w", err)
	}

	return driveEpic(context.Background(), a, ep, epicID, repoPath, baseBranch, epicWorktree, stateStore, plan, out)
}
