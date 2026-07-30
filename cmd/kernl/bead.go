package main

import (
	"context"
	"fmt"
	"os"
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

// resolveBeadShipmentDestination settles where bead run may publish before
// any dispatch, exactly like epic run's resolveShipmentPlan does. It is a
// variable, mirroring epicGitRunner (cmd/kernl/epic.go), so a hermetic test
// can stub the git remote lookup shipment.ResolveDestination performs
// underneath without shelling out to a real git process; production has no
// path to override it.
var resolveBeadShipmentDestination = func(repoPath string, cfg config.ShipmentConfig) (shipment.Destination, error) {
	return shipment.ResolveDestination(repoPath, cfg.Remote, cfg.AllowedRemotes, nil)
}

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
// against for --autonomous), and a second positional must not be silently
// dropped: `kernl bead run kb-1 kb-2` dispatches exactly one bead, so a
// second bead ID is a usage error, not a hint the CLI ignores.
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
		default:
			return "", false, usagef("KERNL DISPATCH FAILURE: bead run takes exactly one bead ID, got unexpected extra argument %q - run: kernl bead run <bead-id>", arg)
		}
	}
	if beadID == "" {
		return "", false, usagef("KERNL DISPATCH FAILURE: bead run requires a bead ID - run: kernl bead run <bead-id>")
	}
	return beadID, dryRun, nil
}

// runBeadDispatch drives a single, standalone bead through the same
// machinery epic run uses for each of its children: a real per-bead worktree
// from epic.WorktreeManager, then app.DriveBeadToTerminal for the stage
// prompt, per-dialect argv and exit-gate advance. `bead run` used to skip all
// of this and call a.Driver.RunBead directly, which spawned the agent with
// no prompt at all.
//
// "Standalone" is load-bearing: a bead that belongs to an epic needs the
// epic branch, dependency merges, integration/shipment BuildPrompt overrides
// and shipment plan that only epic run assembles. Rebuilding that here would
// be a second, divergent copy of epic run's orchestration; refuseEpicManagedBead
// below refuses those beads instead, naming the command that already handles
// them, rather than silently driving them through a lesser path.
//
// driver is threaded through explicitly, rather than read off App inside this
// function, so a hermetic test can hand it a fake BeadDriver and observe
// exactly what gets assembled, without spawning a real agent process.
func runBeadDispatch(a *app.App, driver app.BeadDriver, beadID string, repoEntry config.RepoEntry, dryRun bool) (app.RunBeadResult, error) {
	repoPath := repoEntry.Path

	bead, err := a.Backend.Get(beadID, repoPath)
	if err != nil || bead == nil {
		return app.RunBeadResult{}, fmt.Errorf("KERNL DISPATCH FAILURE: bead %s not found in repo %s: %w", beadID, repoPath, err)
	}

	// --dry-run must stop before the stage is entered, not spawn the agent
	// and ask it to hold back - an agent told not to publish can still decide
	// that publishing is what the instruction meant. Returning here, before
	// any of the checks or filesystem work below, is what containment means
	// for a single bead: nothing is created, removed or resolved, so there is
	// nothing for a dry run to have side effects on. wm.Add in particular
	// auto-cleans an existing worktree path, so reaching it even once on the
	// --dry-run path would discard whatever a previous real run left there.
	if dryRun {
		return app.RunBeadResult{FinalState: bead.State, Success: true}, nil
	}

	if err := refuseEpicManagedBead(bead); err != nil {
		return app.RunBeadResult{}, err
	}

	// Resolved unconditionally, regardless of the bead's current state:
	// DriveBeadToTerminal may advance this bead all the way through its own
	// shipment stage in this same call, and refuseUnverifiedShipment (called
	// by the CLI entry point before this function) only catches a bead that
	// is ALREADY at the shipment state - not one that will reach it a few
	// stages from now. A repository with no declared shipment destination
	// must refuse before any work starts, the same as epic run does at
	// epic.go's resolveShipmentPlan, not partway through a run that already
	// dispatched real agent work.
	if _, err := resolveBeadShipmentDestination(repoPath, repoEntry.Shipment); err != nil {
		return app.RunBeadResult{}, err
	}

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
	// sibling dependency branches to merge in (refuseEpicManagedBead above
	// already refused a bead that has either).
	worktreePath := wm.Path(beadID, beadID)
	if info, statErr := os.Stat(worktreePath); statErr == nil && info.IsDir() {
		return app.RunBeadResult{}, fmt.Errorf("KERNL DISPATCH FAILURE: a worktree for bead %s already exists at %s - bead run does not resume a bead into an existing worktree, and Add would force-recreate it, discarding whatever is there - Fix: remove %s by hand if it is safe to discard, or if this bead is mid-workflow after a previous run, continue it manually in that worktree", beadID, worktreePath, worktreePath)
	}
	worktree, err := wm.Add(beadID, beadID, nil)
	if err != nil {
		return app.RunBeadResult{}, err
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
		VerifyCommand:   verifyCommand,
		TrackerCommand:  trackerCommand,
		Log: func(stage int, state string) {
			fmt.Printf("bead %s [stage %d] %s\n", beadID, stage, state)
		},
	})
}

// refuseEpicManagedBead is the root-cause guard behind findings 3 and 5: a
// bead that belongs to an epic, declares dependencies on other beads, or runs
// the "epic" workflow profile needs the epic branch, dependency merges and
// integration/shipment prompts that only epic run assembles. bead run has no
// epic DAG to draw any of that from, so rather than build a second, lesser
// copy of that machinery it refuses and names the command that already has
// it.
func refuseEpicManagedBead(bead *backend.Bead) error {
	if bead.ParentID != "" {
		return fmt.Errorf("KERNL DISPATCH FAILURE: bead %s is a child of epic %s - bead run has no epic branch or dependency graph to drive it correctly - Fix: run `kernl epic run %s`, which drives every child of the epic together", bead.ID, bead.ParentID, bead.ParentID)
	}
	if len(bead.Dependencies) > 0 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: bead %s declares %d dependencies - bead run has no epic DAG to merge them from before dispatch - Fix: run the owning epic with `kernl epic run <epic-id>`", bead.ID, len(bead.Dependencies))
	}
	wf := backend.ResolveWorkflow(bead)
	if wf.ID == "epic" {
		return fmt.Errorf("KERNL DISPATCH FAILURE: bead %s runs the epic workflow profile - its integration and shipment stages need the epic-specific prompt and verified destination epic run builds, not the generic one bead run would give it - Fix: run `kernl epic run %s`", bead.ID, bead.ID)
	}
	return nil
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
	if _, err := resolveBeadShipmentDestination(repoEntry.Path, repoEntry.Shipment); err != nil {
		return err
	}
	return nil
}
