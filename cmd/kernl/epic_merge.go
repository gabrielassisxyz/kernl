package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
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
	var stopBeforeShipment bool
	var positional []string
	for _, arg := range args {
		switch arg {
		case "--stop-before-shipment", "--dry-run":
			// --dry-run is the accepted alias - see runEpicRun's flag parsing
			// for why the flag was renamed.
			stopBeforeShipment = true
		default:
			if strings.HasPrefix(arg, "-") {
				return usagef("KERNL DISPATCH FAILURE: unknown epic merge flag %q%s - valid: --stop-before-shipment, --dry-run, --repo",
					arg, didYouMean(arg, []string{"--stop-before-shipment", "--dry-run", "--repo"}))
			}
			positional = append(positional, arg)
		}
	}
	if len(positional) == 0 {
		return usagef("KERNL DISPATCH FAILURE: epic merge requires an epic ID - run: kernl epic merge <epic-id>")
	}
	epicID := positional[0]
	cwd, _ := os.Getwd()
	repoEntry, err := resolveRepoEntry(a.Config, repoFlag, cwd)
	if err != nil {
		return err
	}
	repoPath := repoEntry.Path

	// epic merge drives the same integration -> shipment tail as epic run, so
	// it needs the same verified destination. Containing only one of the two
	// entry points would leave the other publishing wherever it liked.
	plan, err := resolveShipmentPlan(repoEntry, stopBeforeShipment, out)
	if err != nil {
		return err
	}
	if stopBeforeShipment {
		// Unlike epic run, epic merge never touches the children - it only
		// drives the epic bead itself, so integration is the whole effect,
		// not the smaller half of it.
		out("--stop-before-shipment: integration and integration_review still run for real and commit onto the epic branch (epic merge does not run children) - only the push and pull request are withheld, and that merge cannot be redone\n")
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

	// Which tracker this repository uses, and how an agent standing in a
	// worktree reaches it, are facts about the repository too - and the one
	// the stage prompts state in prose.
	trackerManager, err := backend.ResolveMemoryManager(repoPath, repoEntry.MemoryManager)
	if err != nil {
		return err
	}
	trackerCommand, err := backend.TrackerInvocation(trackerManager, repoPath)
	if err != nil {
		return err
	}
	out(fmt.Sprintf("tracker: %s\n", trackerCommand))

	wm := epic.NewWorktreeManager(a.Config.Orchestrator.WorktreeRoot, repoPath, baseBranch, execGitRun, nil)
	if _, err := wm.EnsureEpicBranch(epicID); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: cannot ensure epic branch for %s: %w", epicID, err)
	}
	epicWorktree, err := wm.AddEpicWorktree(epicID)
	if err != nil {
		return err
	}

	if a.StateDir == "" {
		return fmt.Errorf("KERNL DISPATCH FAILURE: no state directory for epic %s, so kernl has nowhere of its own to keep agent state - Fix: set App.StateDir (app.DefaultStateDir() outside tests)", epicID)
	}
	agentStateDir := filepath.Join(a.StateDir, "agentstate")
	stateStore, err := workflow.NewAgentStateStore(agentStateDir)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: creating AgentStateStore: %w", err)
	}

	return driveEpic(context.Background(), epicDrive{
		App: a, Epic: ep, EpicID: epicID, RepoPath: repoPath,
		BaseBranch: baseBranch, VerifyCommand: verifyCommand, TrackerCommand: trackerCommand, Worktree: epicWorktree,
		StateStore: stateStore, Shipment: plan, Out: out,
	})
}
