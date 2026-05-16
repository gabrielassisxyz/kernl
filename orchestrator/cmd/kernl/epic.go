package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/gabrielassisxyz/kernl/internal/api"
	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/epic"
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

	return runEpicWithApp(a, args, nil)
}

func runEpicWithApp(a *app.App, args []string, out func(string)) error {
	if out == nil {
		out = func(s string) { fmt.Print(s) }
	}
	if len(args) == 0 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic requires a subcommand — try: kernl epic list")
	}

	switch args[0] {
	case "list":
		return runEpicList(a, os.Stdout)
	case "run":
		return runEpicRun(a, args[1:], out)
	case "merge":
		return runEpicMerge(a, args[1:], out)
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

func runEpicRun(a *app.App, args []string, out func(string)) error {
	if len(args) == 0 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic run requires an epic ID — run: kernl epic run <epic-id>")
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

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: starting HTTP listener: %w", err)
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port

	handler := api.NewRouter(a)
	srv := &http.Server{Handler: handler}
	go func() {
		srv.Serve(listener)
	}()
	defer srv.Close()

	out(fmt.Sprintf("GUI em http://localhost:%d\n", actualPort))

	wm := epic.NewWorktreeManager(a.Config.Orchestrator.WorktreeRoot, repoPath, nil, nil)

	ex := epic.NewExecutor(epic.ExecutorDeps{
		Epic: ep,
		RunBead: func(ctx context.Context, in epic.RunInput) (epic.RunResult, error) {
			res, err := a.Driver.RunBead(ctx, app.RunBeadInput{
				BeadID:   in.BeadID,
				RepoPath: repoPath,
				AgentID:  "opencode",
			})
			if err != nil {
				return epic.RunResult{}, err
			}
			return epic.RunResult{FinalState: res.FinalState, Success: res.Success}, nil
		},
		Worktree:      wm,
		MaxConcurrent: a.Config.Orchestrator.MaxConcurrentBeads,
		Emit: func(ev epic.EpicEvent) {
			a.EpicEvents.Publish(ev)
			if ev.Type == epic.BeadStateChanged {
				out(fmt.Sprintf("bead %s → %s\n", ev.BeadID, ev.Detail))
			}
		},
	})

	if err := ex.Run(context.Background()); err != nil {
		out(fmt.Sprintf("epic %s bloqueado — corrija e rode kernl epic run %s de novo para retomar\n", epicID, epicID))
		return err
	}

	metric := ex.Parallelism()
	out(fmt.Sprintf("epic %s concluído — paralelismo realizado: %.1fx (pico %d, max %d)\n", epicID, metric.Realized, metric.Peak, metric.GraphMax))

	return nil
}

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
