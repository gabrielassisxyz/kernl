package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/dispatch"
	"github.com/gabrielassisxyz/kernl/internal/shipment"
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

	a, err := app.NewApp(cfg)
	if err != nil {
		return wrapLoud("creating app", err)
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
	if len(args) == 0 {
		return usagef("KERNL DISPATCH FAILURE: bead run requires a bead ID - run: kernl bead run <bead-id>")
	}

	beadID := args[0]

	repoEntry, err := resolveRepoEntry(a.Config, repoFlag)
	if err != nil {
		return err
	}
	repoPath := repoEntry.Path

	if err := refuseUnverifiedShipment(a, beadID, repoEntry); err != nil {
		return err
	}

	input, err := app.ResolveAgentForBead(a.Config, a.Backend, beadID, repoPath)
	if err != nil {
		return err
	}
	input.BeadID = beadID
	input.RepoPath = repoPath

	fmt.Printf("bead %s → implementing\n", beadID)
	fmt.Printf("agent %s spawned (cmd: %s args: %v)\n", input.AgentName, input.Command, input.Args)

	res, err := a.Driver.RunBead(context.Background(), input)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: running bead %s: %w", beadID, err)
	}

	fmt.Printf("bead %s → done\n", beadID)

	if !res.Success {
		return fmt.Errorf("KERNL DISPATCH FAILURE: bead %s exited with error, final state %s", beadID, res.FinalState)
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
	if _, err := shipment.ResolveDestination(repoEntry.Path, repoEntry.Shipment.Remote, repoEntry.Shipment.AllowedRemotes, nil); err != nil {
		return err
	}
	return nil
}
