package main

import (
	"fmt"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
)

func runEpicMerge(a *app.App, args []string, out func(string)) error {
	if len(args) == 0 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic merge requires an epic ID — run: kernl epic merge <epic-id>")
	}
	epicID := args[0]

	if err := a.EpicMerge(epicID); err != nil {
		return err
	}
	repoPath := a.Config.Registry.Repos[0].Path

	bead, _ := a.Backend.Get(epicID, repoPath)
	state := "unknown"
	if bead != nil {
		state = bead.State
	}
	out(fmt.Sprintf("epic %s → %s\n", epicID, state))

	children, _ := a.Backend.List(&backend.BeadListFilters{Parent: epicID}, repoPath)
	for _, child := range children {
		out(fmt.Sprintf("child %s → %s\n", child.ID, child.State))
	}

	return nil
}
