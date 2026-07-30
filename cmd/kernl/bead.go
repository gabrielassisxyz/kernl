package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/dispatch"
	"github.com/gabrielassisxyz/kernl/internal/epic"
	"github.com/gabrielassisxyz/kernl/internal/shipment"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

// beadSubcommands splits the verb in two on a real boundary. `run` drives an
// agent in a local worktree, so it needs the app wired up here and cannot be a
// call to a remote server. Everything else is tracker data the API already
// serves, and is dispatched before any of that setup - building the app
// requires bd, and asking for a bead list should not.
var beadSubcommands = []string{"run", "list", "get", "create", "set", "close", "rollback", "refine-scope", "mark-terminal"}

func runBead(v verbContext, args []string) error {
	if len(args) == 0 {
		return usagef("KERNL DISPATCH FAILURE: bead requires a subcommand - valid: %s. Run: kernl bead --help",
			strings.Join(beadSubcommands, ", "))
	}
	if args[0] != "run" {
		return runBeadAPI(v, args)
	}

	cfg, err := loadCLIConfig(v.configPath)
	if err != nil {
		return err
	}

	// Resolved before construction: the backend is picked once, and which
	// tracker it speaks is a property of the repository this run names.
	a, err := appForSelectedRepo(cfg, "bead run", args[1:])
	if err != nil {
		return err
	}

	return runBeadWithApp(a, args)
}

func runBeadWithApp(a *app.App, args []string) error {
	if len(args) == 0 {
		return usagef("KERNL DISPATCH FAILURE: bead requires a subcommand - run: kernl bead run <bead-id>")
	}

	switch args[0] {
	case "run":
		return runBeadCmd(a, args[1:])
	default:
		return usagef("KERNL DISPATCH FAILURE: unknown bead subcommand %q%s - valid: run. Run: kernl bead run <bead-id>",
			args[0], didYouMean(args[0], []string{"run"}))
	}
}

func runBeadCmd(a *app.App, args []string) error {
	repoFlag, args, err := takeRepoFlag("bead run", args)
	if err != nil {
		return err
	}
	beadID, dryRun, err := parseBeadRunArgs(args)
	if err != nil {
		return err
	}

	repoEntry, err := resolveRepoEntry(a.Config, repoFlag)
	if err != nil {
		return err
	}

	if err := refuseUnverifiedShipment(a, beadID, repoEntry); err != nil {
		return err
	}

	if dryRun {
		fmt.Println("dry-run: the run stops before entering the stage - no agent is dispatched")
	}
	fmt.Printf("bead %s → implementing\n", beadID)

	res, err := runBeadDispatch(a, a.Driver, beadID, repoEntry, dryRun)
	if err != nil {
		return err
	}

	fmt.Printf("bead %s → %s\n", beadID, res.FinalState)

	if !res.Success {
		return fmt.Errorf("KERNL DISPATCH FAILURE: bead %s exited with error, final state %s", beadID, res.FinalState)
	}

	return nil
}

// parseBeadRunArgs splits the positional bead ID from --dry-run. A mistyped
// flag must not silently become the bead ID (the same trap epic run guards
// against for --autonomous).
func parseBeadRunArgs(args []string) (beadID string, dryRun bool, err error) {
	for _, arg := range args {
		switch {
		case arg == "--dry-run":
			dryRun = true
		case strings.HasPrefix(arg, "-"):
			return "", false, usagef("KERNL DISPATCH FAILURE: unknown bead run flag %q%s - valid: --dry-run, --repo",
				arg, didYouMean(arg, []string{"--dry-run", "--repo"}))
		case beadID == "":
			beadID = arg
		}
	}
	if beadID == "" {
		return "", false, usagef("KERNL DISPATCH FAILURE: bead run requires a bead ID - run: kernl bead run <bead-id>")
	}
	return beadID, dryRun, nil
}

// runBeadDispatch drives a single bead through the same machinery epic run
// uses for each of its children: a real per-bead worktree from
// epic.WorktreeManager, then app.DriveBeadToTerminal for the stage prompt,
// per-dialect argv and exit-gate advance. `bead run` used to skip all of this
// and call a.Driver.RunBead directly, which spawned the agent with no prompt
// at all.
//
// driver is threaded through explicitly, rather than read off App inside this
// function, so a hermetic test can hand it a fake BeadDriver and observe
// exactly what gets assembled, without spawning a real agent process.
func runBeadDispatch(a *app.App, driver app.BeadDriver, beadID string, repoEntry config.RepoEntry, dryRun bool) (app.RunBeadResult, error) {
	repoPath := repoEntry.Path

	gitRun, err := epicGitRunner(repoPath)
	if err != nil {
		return app.RunBeadResult{}, err
	}

	// Only wire a base branch when there is a real git executor - the no-git
	// worktree fallback (test fixtures only) never cuts a branch either.
	var baseBranch string
	if gitRun != nil {
		baseBranch, err = epic.ResolveBaseBranch(repoPath, repoEntry.DefaultBranch, gitRun)
		if err != nil {
			return app.RunBeadResult{}, err
		}
	}

	verifyCommand, err := epic.ResolveVerifyCommand(repoPath, repoEntry.VerifyCommand)
	if err != nil {
		return app.RunBeadResult{}, err
	}

	trackerManager, err := backend.ResolveMemoryManager(repoPath, repoEntry.MemoryManager)
	if err != nil {
		return app.RunBeadResult{}, err
	}
	trackerCommand, err := backend.TrackerInvocation(trackerManager, repoPath)
	if err != nil {
		return app.RunBeadResult{}, err
	}

	// Resolved from the app's own StateDir rather than the home directory
	// here: a function that derives its own path writes into the operator's
	// real ~/.kernl from every unit test that reaches it.
	if strings.TrimSpace(a.StateDir) == "" {
		return app.RunBeadResult{}, fmt.Errorf("KERNL DISPATCH FAILURE: no state directory for bead %s, so kernl has nowhere of its own to write run state - Fix: set App.StateDir (app.DefaultStateDir() outside tests)", beadID)
	}
	stateStore, err := workflow.NewAgentStateStore(filepath.Join(a.StateDir, "agentstate"))
	if err != nil {
		return app.RunBeadResult{}, fmt.Errorf("KERNL DISPATCH FAILURE: creating AgentStateStore for bead %s: %w", beadID, err)
	}

	// updateDesc records an epic's branch on the epic bead itself; a
	// standalone bead run has no epic bead to record it on.
	noopUpdateDesc := func(string, func(string) string) error { return nil }
	wm := epic.NewWorktreeManager(a.Config.Orchestrator.WorktreeRoot, repoPath, baseBranch, gitRun, noopUpdateDesc)
	// Grouped under its own ID rather than a parent epic's: bead run dispatches
	// exactly one bead, so there is no epic branch to layer it onto and no
	// sibling dependency branches to merge in.
	worktree, err := wm.Add(beadID, beadID, nil)
	if err != nil {
		return app.RunBeadResult{}, err
	}

	// --dry-run must stop before the stage is entered, not spawn the agent and
	// ask it to hold back - an agent told not to publish can still decide that
	// publishing is what the instruction meant. Stopping the loop as soon as it
	// sees the bead's own current state, before any claim or dispatch, is what
	// containment looks like for a single bead (epic run does the same thing by
	// naming the shipment state explicitly).
	stopBefore := ""
	if dryRun {
		bead, err := a.Backend.Get(beadID, repoPath)
		if err != nil || bead == nil {
			return app.RunBeadResult{}, fmt.Errorf("KERNL DISPATCH FAILURE: bead %s not found in repo %s: %w", beadID, repoPath, err)
		}
		stopBefore = bead.State
	}

	return app.DriveBeadToTerminal(context.Background(), app.DriveBeadDeps{
		Backend:         a.Backend,
		Driver:          driver,
		Config:          a.Config,
		StateDir:        a.StateDir,
		BeadID:          beadID,
		RepoPath:        repoPath,
		Worktree:        worktree,
		AgentStateStore: stateStore,
		StopBeforeState: stopBefore,
		VerifyCommand:   verifyCommand,
		TrackerCommand:  trackerCommand,
		Log: func(stage int, state string) {
			fmt.Printf("bead %s [stage %d] %s\n", beadID, stage, state)
		},
	})
}

// refuseUnverifiedShipment stops `bead run` from dispatching the shipment stage
// unless the repository has declared, and kernl has verified, where it may
// publish.
//
// bead run is a second door into the same stage: it resolves an agent and
// spawns it directly, without the epic driver's pre-dispatch check, its prompt,
// or its --dry-run. Containing one entry point and not the other contains
// nothing, and this is the stage that opened a public pull request nobody asked
// for.
func refuseUnverifiedShipment(a *app.App, beadID string, repoEntry config.RepoEntry) error {
	bead, err := a.Backend.Get(beadID, repoEntry.Path)
	if err != nil || bead == nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: bead %s not found in repo %s: %w", beadID, repoEntry.Path, err)
	}
	wf := backend.ResolveWorkflow(bead)
	if dispatch.DerivePoolKey(&wf, bead.State) != "shipment" {
		return nil
	}
	if _, err := shipment.ResolveDestination(repoEntry.Path, repoEntry.Shipment.Remote, repoEntry.Shipment.AllowedRemotes, nil); err != nil {
		return err
	}
	return nil
}
