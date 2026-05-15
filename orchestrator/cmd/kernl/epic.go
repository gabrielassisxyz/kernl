package main

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

func runEpic(configPath string, args []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: loading config %s: %w", configPath, err)
	}

	a, err := app.NewApp(cfg)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: creating app: %w", err)
	}

	return runEpicWithApp(a, args)
}

func runEpicWithApp(a *app.App, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic requires a subcommand — try: kernl epic list")
	}

	switch args[0] {
	case "list":
		return runEpicList(a, os.Stdout)
	default:
		return fmt.Errorf("KERNL DISPATCH FAILURE: unknown epic subcommand %q — try: kernl epic list", args[0])
	}
}

func runEpicList(a *app.App, w io.Writer) error {
	if len(a.Config.Registry.Repos) == 0 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: no repos registered — Fix: add a repo to registry.repos in kernl.yaml")
	}
	repoPath := a.Config.Registry.Repos[0].Path

	epics, err := a.Backend.List(&backend.BeadListFilters{Type: "epic"}, repoPath)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: listing epics: %w", err)
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTITLE\tCHILDREN\tSTATE")

	for _, epic := range epics {
		children, err := a.Backend.List(&backend.BeadListFilters{Parent: epic.ID}, repoPath)
		if err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: listing children for epic %s: %w", epic.ID, err)
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", epic.ID, epic.Title, len(children), epic.State)
	}

	return tw.Flush()
}
