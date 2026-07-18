package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/api"
	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/dispatch"
	"github.com/gabrielassisxyz/kernl/internal/epic"
	"github.com/gabrielassisxyz/kernl/internal/prompt"
	"github.com/gabrielassisxyz/kernl/internal/runstate"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

// execGitRun shells out to `git -C <dir> <args...>` and returns stdout.
// Used by WorktreeManager so each bead gets a real isolated git worktree
// (not just an empty mkdir'd directory, which leaves agents nothing to
// edit and was the cause of multiple "stuck at state" failures during
// the kernl-npp MVP run on 2026-05-17).
func execGitRun(dir string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", dir}, args...)
	out, err := exec.Command("git", cmdArgs...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w: %s", strings.Join(cmdArgs, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

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
		return usagef("KERNL DISPATCH FAILURE: epic requires a subcommand — try: kernl epic list")
	}

	switch args[0] {
	case "list":
		return runEpicList(a, os.Stdout)
	case "run":
		return runEpicRun(a, args[1:], out)
	case "merge":
		return runEpicMerge(a, args[1:], out)
	case "abort":
		return runEpicAbort(a, args[1:], out)
	default:
		return usagef("KERNL DISPATCH FAILURE: unknown epic subcommand %q%s — valid: list, run, merge, abort. Run: kernl epic --help",
			args[0], didYouMean(args[0], []string{"list", "run", "merge", "abort"}))
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
	var workflowPath string
	var workflowFlagSeen bool
	var autonomous bool
	var interactive bool
	var remainingArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--workflow=") {
			workflowFlagSeen = true
			workflowPath = strings.TrimPrefix(arg, "--workflow=")
			if workflowPath == "" {
				return fmt.Errorf("KERNL DISPATCH FAILURE: --workflow flag requires a path")
			}
		} else if arg == "--workflow" {
			workflowFlagSeen = true
			if i+1 < len(args) {
				workflowPath = args[i+1]
				if workflowPath == "" {
					return fmt.Errorf("KERNL DISPATCH FAILURE: --workflow flag requires a path")
				}
				i++
			} else {
				return fmt.Errorf("KERNL DISPATCH FAILURE: --workflow flag requires a path")
			}
		} else if arg == "--autonomous" {
			autonomous = true
		} else if arg == "--interactive" {
			interactive = true
		} else if strings.HasPrefix(arg, "-") {
			// A mistyped flag must not silently become the epic ID (it used
			// to swallow --autonomous typos and run non-autonomous).
			return usagef("KERNL DISPATCH FAILURE: unknown epic run flag %q%s — valid: --workflow, --autonomous, --interactive",
				arg, didYouMean(arg, []string{"--workflow", "--autonomous", "--interactive"}))
		} else {
			remainingArgs = append(remainingArgs, arg)
		}
	}

	if workflowFlagSeen && workflowPath == "" {
		return fmt.Errorf("KERNL DISPATCH FAILURE: --workflow flag requires a path")
	}

	if len(remainingArgs) == 0 {
		return usagef("KERNL DISPATCH FAILURE: epic run requires an epic ID — run: kernl epic run <epic-id>")
	}
	if len(a.Config.Registry.Repos) == 0 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: no repos registered — Fix: add a repo to registry.repos in kernl.yaml")
	}
	epicID := remainingArgs[0]
	repoPath := a.Config.Registry.Repos[0].Path

	// U1: Config and CLI flags for autonomous mode
	if !autonomous && !interactive {
		autoCfg, _ := dispatch.LoadAutonomousConfig("kernl.yaml")
		if autoCfg {
			autonomous = true
		}
	}

	var customProfileID string
	if workflowPath != "" {
		desc, err := backend.LoadWorkflowYAML(workflowPath)
		if err != nil {
			return err
		}
		backend.RegisterWorkflow(desc)
		customProfileID = desc.ID
	} else if autonomous {
		// U2: DA workflow inference
		epicBead, err := a.Backend.Get(epicID, repoPath)
		if err != nil {
			return err
		}
		res, err := dispatch.InferWorkflow(context.Background(), a.Config.LLM, epicBead)
		if err == nil && res != nil {
			customProfileID = res.ShapeID
			out(fmt.Sprintf("Inferred workflow shape: %s (Reason: %s)\n", res.ShapeID, res.Rationale))

			// U3: CLI confirmation prompting
			if interactive {
				out(fmt.Sprintf("Proceed with shape '%s'? [Y/n] ", res.ShapeID))
				var confirm string
				_, _ = fmt.Scanln(&confirm)
				if confirm != "" && strings.ToLower(confirm) != "y" {
					return fmt.Errorf("aborted by user")
				}
			}
		}
	}

	ep, err := epic.LoadEpic(a.Backend, epicID, repoPath)
	if err != nil {
		return err
	}

	// Persist autonomous label
	if autonomous {
		epicBead, _ := a.Backend.Get(epicID, repoPath)
		if epicBead != nil {
			newLabels := dispatch.SetEpicAutonomous(epicBead)
			_ = a.Backend.Update(epicID, backend.UpdateBeadInput{SetLabels: newLabels}, repoPath)
		}
	}

	beadPort := a.Config.Server.Port
	if beadPort == 0 {
		beadPort = 8080
	}
	beadListenAddr := fmt.Sprintf(":%d", beadPort)
	listener, err := net.Listen("tcp", beadListenAddr)
	if err != nil {
		listener, err = net.Listen("tcp", ":0")
		if err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: starting HTTP listener: %w", err)
		}
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port

	handler := api.NewRouter(a)
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(listener) }()
	defer srv.Close()

	// Open the run-state store so we can plan resume actions based on
	// previous execution state.
	rs, err := runstate.Open(a.Config.Orchestrator.RunStatePath)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: open runstate %s: %w", a.Config.Orchestrator.RunStatePath, err)
	}
	defer rs.Close()

	// Construct ONE AgentStateStore
	agentStateDir := filepath.Join(os.Getenv("HOME"), ".kernl", "agentstate")
	stateStore, err := workflow.NewAgentStateStore(agentStateDir)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: creating AgentStateStore: %w", err)
	}

	resumePlan := epic.PlanResume(a.Backend, rs, ep, repoPath)
	for _, child := range ep.Children {
		action := resumePlan.Action(child.ID)
		switch action {
		case epic.ResumeSkip:
			out(fmt.Sprintf("bead %s [skip] already at terminal / human-gate state\n", child.ID))
		case epic.ResumeSession:
			out(fmt.Sprintf("bead %s [resume] session %s\n", child.ID, resumePlan.SessionID(child.ID)))
		case epic.ResumeError:
			out(fmt.Sprintf("bead %s [error] %s\n", child.ID, resumePlan.Detail(child.ID)))
		}
	}

	out(fmt.Sprintf("GUI em http://localhost:%d/?epic=%s\n", actualPort, epicID))

	// Only wire real git execution when the repo path is actually a git
	// repo -- hermetic tests use t.TempDir() which is not a git repo, and
	// the worktree manager already has a no-git mkdir-only fallback for
	// that case.
	var gitRunForWM func(dir string, args ...string) (string, error)
	if _, err := execGitRun(repoPath, "rev-parse", "--git-dir"); err == nil {
		gitRunForWM = execGitRun
	}
	// Wire updateDesc so worktree creation stores the path in runstate.
	wtUpdateDesc := func(beadID string, fn func(string) string) error {
		// Not used for epic branch; runstate tracks worktrees separately.
		return nil
	}
	wm := epic.NewWorktreeManager(a.Config.Orchestrator.WorktreeRoot, repoPath, gitRunForWM, wtUpdateDesc)
	if gitRunForWM != nil {
		if _, err := wm.EnsureEpicBranch(epicID); err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: cannot ensure epic branch for %s: %w", epicID, err)
		}
	}

	doneSet := resumePlan.DoneSet()
	// Collect session IDs for beads that have a recorded session.
	sessionResumes := make(map[string]string)
	for _, child := range ep.Children {
		if resumePlan.Action(child.ID) == epic.ResumeSession {
			sessionResumes[child.ID] = resumePlan.SessionID(child.ID)
		}
	}
	ex := epic.NewExecutorWithDoneSet(epic.ExecutorDeps{
		Epic: ep,
		RunBead: func(ctx context.Context, in epic.RunInput) (epic.RunResult, error) {
			// Persist worktree path in runstate before dispatching so
			// future runs know a worktree existed for this bead.
			_ = rs.SetWorktree(epicID, in.BeadID, in.Worktree)
			// Epic children run the worker profile: implement + review, then
			// STOP at awaiting_integration handing the branch to the epic.
			if err := ensureWorkerEntry(a.Backend, in.BeadID, repoPath, customProfileID); err != nil {
				return epic.RunResult{}, err
			}
			res, err := app.DriveBeadToTerminal(ctx, app.DriveBeadDeps{
				Backend:         a.Backend,
				Driver:          a.Driver,
				Config:          a.Config,
				BeadID:          in.BeadID,
				RepoPath:        repoPath,
				Worktree:        in.Worktree,
				SessionID:       in.SessionID,
				AgentStateStore: stateStore,
				Log: func(stage int, state string) {
					ts := time.Now().Format("15:04:05")
					out(fmt.Sprintf("[%s] bead %s [stage %d] %s\n", ts, in.BeadID, stage, state))
					a.EpicEvents.Publish(epic.EpicEvent{
						Type:   epic.BeadStateChanged,
						EpicID: ep.ID,
						BeadID: in.BeadID,
						Detail: state,
						Time:   time.Now().Unix(),
					})
				},
			})
			if err != nil {
				return epic.RunResult{SessionID: res.SessionID, FinalState: res.FinalState, Success: res.Success}, err
			}
			return epic.RunResult{SessionID: res.SessionID, FinalState: res.FinalState, Success: res.Success}, nil
		},
		Worktree:       wm,
		GetWorktree:    rs.Worktree,
		SessionResumes: sessionResumes,
		MaxConcurrent:  a.Config.Orchestrator.MaxConcurrentBeads,
		Emit: func(ev epic.EpicEvent) {
			a.EpicEvents.Publish(ev)
			if ev.Type == epic.BeadStateChanged {
				ts := time.Now().Format("15:04:05")
				out(fmt.Sprintf("[%s] bead %s \u2192 %s\n", ts, ev.BeadID, ev.Detail))
			}
		},
	}, doneSet)

	if err := ex.Run(context.Background()); err != nil {
		out(fmt.Sprintf("epic %s bloqueado — corrija e rode kernl epic run %s de novo para retomar\n", epicID, epicID))
		return err
	}

	metric := ex.Parallelism()
	out(fmt.Sprintf("epic %s concluído — paralelismo realizado: %.1fx (pico %d, max %d)\n", epicID, metric.Realized, metric.Peak, metric.GraphMax))

	// All children reached awaiting_integration. Drive the epic bead itself
	// through integration -> integration_review -> shipment -> awaiting_pr_review.
	epicWorktree, werr := wm.AddEpicWorktree(epicID)
	if werr != nil {
		return werr
	}
	_ = rs.SetWorktree(epicID, epicID, epicWorktree)
	if err := driveEpic(context.Background(), a, ep, epicID, repoPath, epicWorktree, stateStore, out); err != nil {
		out(fmt.Sprintf("epic %s bloqueado na integração — corrija e rode kernl epic run %s de novo para retomar\n", epicID, epicID))
		return err
	}

	return nil
}

// ensureWorkerEntry puts a freshly-created epic child (bd status "open") onto
// the worker profile and its initial workflow state so DriveBeadToTerminal can
// claim it. Children already mid-workflow (resume) are left untouched.
func ensureWorkerEntry(be backend.BackendPort, beadID, repoPath string, profileID string) error {
	bead, err := be.Get(beadID, repoPath)
	if err != nil || bead == nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: child %s not found in repo %s: %w", beadID, repoPath, err)
	}
	if bead.State != "open" {
		return nil
	}
	if profileID == "" {
		profileID = "worker"
	}
	labels := setWFLabel(bead.Labels, "wf:profile:", profileID)
	labels = setWFLabel(labels, "wf:state:", "ready_for_implementation")
	return be.Update(beadID, backend.UpdateBeadInput{State: "ready_for_implementation", SetLabels: labels}, repoPath)
}

// setWFLabel replaces any existing label with the given prefix by prefix+value.
func setWFLabel(labels []string, prefix, value string) []string {
	out := make([]string, 0, len(labels)+1)
	for _, l := range labels {
		if !strings.HasPrefix(l, prefix) {
			out = append(out, l)
		}
	}
	return append(out, prefix+value)
}

// driveEpic puts the epic bead on the epic profile and drives it through
// integration -> integration_review -> shipment, ending at awaiting_pr_review.
// The BuildPrompt override injects epic-specific integration/shipment prompts.
func driveEpic(ctx context.Context, a *app.App, ep *epic.Epic, epicID, repoPath, epicWorktree string, stateStore *workflow.AgentStateStore, out func(string)) error {
	epicBead, err := a.Backend.Get(epicID, repoPath)
	if err != nil || epicBead == nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic %s not found in repo %s: %w", epicID, repoPath, err)
	}
	labels := setWFLabel(epicBead.Labels, "wf:profile:", "epic")
	labels = setWFLabel(labels, "wf:state:", "ready_for_integration")
	if err := a.Backend.Update(epicID, backend.UpdateBeadInput{State: "ready_for_integration", SetLabels: labels}, repoPath); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: cannot set epic %s to ready_for_integration: %w", epicID, err)
	}

	res, err := app.DriveBeadToTerminal(ctx, app.DriveBeadDeps{
		Backend:         a.Backend,
		Driver:          a.Driver,
		Config:          a.Config,
		BeadID:          epicID,
		RepoPath:        repoPath,
		Worktree:        epicWorktree,
		AgentStateStore: stateStore,
		Log: func(stage int, state string) {
			ts := time.Now().Format("15:04:05")
			out(fmt.Sprintf("[%s] epic %s [stage %d] %s\n", ts, epicID, stage, state))
			a.EpicEvents.Publish(epic.EpicEvent{Type: epic.BeadStateChanged, EpicID: ep.ID, BeadID: epicID, Detail: state, Time: time.Now().Unix()})
		},
		BuildPrompt: func(bead *backend.Bead, activeState string, wf backend.WorkflowDescriptor, rp, wt string) string {
			switch activeState {
			case "integration":
				children, _ := a.Backend.List(&backend.BeadListFilters{Parent: epicID}, rp)
				cs := make([]prompt.Child, 0, len(children))
				for _, c := range children {
					cs = append(cs, prompt.Child{ID: c.ID, Branch: "kernl/" + c.ID})
				}
				s, perr := prompt.RenderIntegration(prompt.IntegrationInput{
					EpicID: epicID, EpicTitle: bead.Title,
					EpicBranch: "feat/" + epicID, BaseBranch: "master", Children: cs,
				})
				if perr != nil {
					return app.BuildBeadStagePrompt(bead, activeState, wf.Stages, rp, wt)
				}
				return s
			case "shipment":
				s, perr := prompt.RenderShipment(prompt.ShipmentInput{
					EpicID: epicID, EpicTitle: bead.Title,
					EpicBranch: "feat/" + epicID, BaseBranch: "master",
				})
				if perr != nil {
					return app.BuildBeadStagePrompt(bead, activeState, wf.Stages, rp, wt)
				}
				return s
			default:
				return app.BuildBeadStagePrompt(bead, activeState, wf.Stages, rp, wt)
			}
		},
	})
	if err != nil {
		return err
	}
	if !res.Success {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic %s integration stopped at %q", epicID, res.FinalState)
	}
	out(fmt.Sprintf("epic %s → %s\n", epicID, res.FinalState))
	return nil
}
