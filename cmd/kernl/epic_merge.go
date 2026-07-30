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
	repoFlag, args, err := takeRepoFlag("epic merge", args)
	if err != nil {
		return err
	}
	var dryRun bool
	var positional []string
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			dryRun = true
		default:
			if strings.HasPrefix(arg, "-") {
				return usagef("KERNL DISPATCH FAILURE: unknown epic merge flag %q%s - valid: --dry-run, --repo",
					arg, didYouMean(arg, []string{"--dry-run", "--repo"}))
			}
			positional = append(positional, arg)
		}
	}
	if len(positional) == 0 {
		return usagef("KERNL DISPATCH FAILURE: epic merge requires an epic ID - run: kernl epic merge <epic-id>")
	}
	epicID := positional[0]
	repoEntry, err := resolveRepoEntry(a.Config, repoFlag)
	if err != nil {
		return err
	}
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

	verifyCommand, err := epic.ResolveVerifyCommand(repoPath, repoEntry.VerifyCommand)
	if err != nil {
		return err
	}
	out(fmt.Sprintf("verify with: %s\n", verifyCommand))

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

	return driveEpic(context.Background(), epicDrive{
		App: a, Epic: ep, EpicID: epicID, RepoPath: repoPath,
		BaseBranch: baseBranch, VerifyCommand: verifyCommand, Worktree: epicWorktree,
		StateStore: stateStore, Shipment: plan, Out: out,
	})
}
