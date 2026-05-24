package main

import (
	"fmt"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/epic"
)

func runEpicAbort(a *app.App, args []string, out func(string)) error {
	if len(args) == 0 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic abort requires an epic ID — run: kernl epic abort <epic-id>")
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

	return nil
}
