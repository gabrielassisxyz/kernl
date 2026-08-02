package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	"github.com/gabrielassisxyz/kernl/internal/shipment"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

// epicGitRunner hands back the git executor for a repository, and refuses a
// path that is not one.
//
// The refusal used to be a silent downgrade: a failed `rev-parse` left the
// worktree manager with no git executor, which skipped base-branch resolution,
// skipped the epic branch, and created each bead's "worktree" as an empty
// mkdir'd directory. A run against a mistyped registry.repos[].path therefore
// dispatched agents into empty folders and reported nothing wrong - the exact
// shape of failure the fail-loud rule exists for, on the one config value that
// decides which repository the whole run acts on.
//
// It is a variable because the no-git worktree mode is real, but only as a
// test fixture. Overriding this is how a test asks for it; production has no
// path to it.
var epicGitRunner = func(repoPath string) (epic.GitRunner, error) {
	if _, err := execGitRun(repoPath, "rev-parse", "--git-dir"); err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: %s is not a git repository, so there is nothing to cut branches or worktrees from - %w - Fix: correct registry.repos[].path in kernl.yaml, or run `git init` there", repoPath, err)
	}
	return execGitRun, nil
}

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

// warnLostWorktreeRecord reports a runstate write that did not land, and does
// not stop the run over it.
//
// The row's only reader is PlanResume, which uses it to notice a worktree that
// has since vanished from disk. Losing it costs a diagnosis on the next resume -
// the bead looks like it never had a worktree and is dispatched fresh - and
// costs nothing to the run in progress, so failing the epic here would do more
// damage than the lost row. What is not acceptable is the silence: the write
// used to be discarded outright, which is why two epics contending for one
// runstate.db degraded resume with nothing anywhere saying so.
func warnLostWorktreeRecord(err error, epicID, beadID string) {
	if err == nil {
		return
	}
	slog.Warn("runstate: worktree path not recorded, resume will not detect a missing worktree for this bead",
		"epic", epicID,
		"bead", beadID,
		"error", err,
	)
}

func runEpic(v verbContext, args []string) error {
	// Usage validation precedes config/app setup: `kernl epic staus` must be
	// diagnosed as a typo, not fail on whatever config problem comes first.
	if err := validateEpicSubcommand(args); err != nil {
		return err
	}

	// events and sessions are read-only views the API already serves; they are
	// dispatched before the app is built because inspecting an epic's history
	// should not require the orchestrator toolchain the other subcommands need.
	if args[0] == "events" || args[0] == "sessions" {
		return runEpicAPI(v, args)
	}

	cfg, err := loadCLIConfig(v.configPath)
	if err != nil {
		return err
	}

	// The repository is resolved before the app is built, because the tracker
	// is a property of the repository and the backend is chosen at
	// construction. Resolving it afterwards only ever changed the path handed
	// to each call, never the implementation receiving it.
	a, err := appForSelectedRepo(cfg, "epic "+args[0], args[1:])
	if err != nil {
		return err
	}

	return runEpicWithApp(a, v.configPath, args, nil)
}

func validateEpicSubcommand(args []string) error {
	if len(args) == 0 {
		return usagef("KERNL DISPATCH FAILURE: epic requires a subcommand - try: kernl epic list")
	}
	switch args[0] {
	case "list", "run", "merge", "abort", "events", "sessions":
		return nil
	default:
		return usagef("KERNL DISPATCH FAILURE: unknown epic subcommand %q%s - valid: list, run, merge, abort, events, sessions. Run: kernl epic --help",
			args[0], didYouMean(args[0], []string{"list", "run", "merge", "abort", "events", "sessions"}))
	}
}

func runEpicWithApp(a *app.App, configPath string, args []string, out func(string)) error {
	if out == nil {
		out = func(s string) { fmt.Print(s) }
	}
	if len(args) == 0 {
		return usagef("KERNL DISPATCH FAILURE: epic requires a subcommand - try: kernl epic list")
	}

	switch args[0] {
	case "list":
		return runEpicList(a, os.Stdout, args[1:])
	case "run":
		return runEpicRun(a, configPath, args[1:], out)
	case "merge":
		return runEpicMerge(a, args[1:], out)
	case "abort":
		return runEpicAbort(a, args[1:], out)
	default:
		return usagef("KERNL DISPATCH FAILURE: unknown epic subcommand %q%s - valid: list, run, merge, abort. Run: kernl epic --help",
			args[0], didYouMean(args[0], []string{"list", "run", "merge", "abort"}))
	}
}

func runEpicList(a *app.App, w io.Writer, args []string) error {
	repoFlag, args, err := takeRepoFlag("epic list", args)
	if err != nil {
		return err
	}
	var asJSON bool
	for _, arg := range args {
		switch arg {
		case "--json":
			asJSON = true
		default:
			return usagef("KERNL DISPATCH FAILURE: unknown epic list flag %q%s - valid: --json, --repo",
				arg, didYouMean(arg, []string{"--json", "--repo"}))
		}
	}
	cwd, _ := os.Getwd()
	repoEntry, err := resolveRepoEntry(a.Config, repoFlag, cwd)
	if err != nil {
		return err
	}
	repoPath := repoEntry.Path

	epics, err := a.Backend.List(&backend.BeadListFilters{Type: "epic"}, repoPath)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: listing epics: %w", err)
	}

	rows := make([]epicListRow, 0, len(epics))
	for _, e := range epics {
		children, err := a.Backend.List(&backend.BeadListFilters{Parent: e.ID}, repoPath)
		if err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: listing children for epic %s: %w", e.ID, err)
		}
		rows = append(rows, epicListRow{ID: e.ID, Title: e.Title, Children: len(children), State: e.State})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })

	if asJSON {
		enc := json.NewEncoder(w)
		return enc.Encode(epicListOutput{Epics: rows})
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTITLE\tCHILDREN\tSTATE")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", r.ID, r.Title, r.Children, r.State)
	}
	return tw.Flush()
}

// epicListOutput is the machine contract for `kernl epic list --json`.
// Keys are camelCase, matching the REST API convention.
type epicListOutput struct {
	Epics []epicListRow `json:"epics"`
}

type epicListRow struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Children int    `json:"children"`
	State    string `json:"state"`
}

func runEpicRun(a *app.App, configPath string, args []string, out func(string)) (err error) {
	repoFlag, args, err := takeRepoFlag("epic run", args)
	if err != nil {
		return err
	}
	var workflowPath string
	var workflowFlagSeen bool
	var autonomous bool
	var interactive bool
	var stopBeforeShipment bool
	var remainingArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--workflow=") {
			workflowFlagSeen = true
			workflowPath = strings.TrimPrefix(arg, "--workflow=")
			if workflowPath == "" {
				return usagef("KERNL DISPATCH FAILURE: --workflow flag requires a path - run: kernl epic run --workflow <path> <epic-id>")
			}
		} else if arg == "--workflow" {
			workflowFlagSeen = true
			if i+1 < len(args) {
				workflowPath = args[i+1]
				if workflowPath == "" {
					return usagef("KERNL DISPATCH FAILURE: --workflow flag requires a path - run: kernl epic run --workflow <path> <epic-id>")
				}
				i++
			} else {
				return usagef("KERNL DISPATCH FAILURE: --workflow flag requires a path - run: kernl epic run --workflow <path> <epic-id>")
			}
		} else if arg == "--autonomous" {
			autonomous = true
		} else if arg == "--interactive" {
			interactive = true
		} else if arg == "--stop-before-shipment" || arg == "--dry-run" {
			// --dry-run is the accepted alias: this is what the flag used to be
			// called before it was renamed. The old name promised a run that
			// does nothing, but children, integration, and integration_review
			// all dispatch for real under it - only shipment is withheld. The
			// new name says what actually happens; the old one keeps working
			// so nothing already depending on it breaks.
			stopBeforeShipment = true
		} else if strings.HasPrefix(arg, "-") {
			// A mistyped flag must not silently become the epic ID (it used
			// to swallow --autonomous typos and run non-autonomous).
			return usagef("KERNL DISPATCH FAILURE: unknown epic run flag %q%s - valid: --workflow, --autonomous, --interactive, --stop-before-shipment, --dry-run, --repo",
				arg, didYouMean(arg, []string{"--workflow", "--autonomous", "--interactive", "--stop-before-shipment", "--dry-run", "--repo"}))
		} else {
			remainingArgs = append(remainingArgs, arg)
		}
	}

	if workflowFlagSeen && workflowPath == "" {
		return usagef("KERNL DISPATCH FAILURE: --workflow flag requires a path - run: kernl epic run --workflow <path> <epic-id>")
	}

	if len(remainingArgs) == 0 {
		return usagef("KERNL DISPATCH FAILURE: epic run requires an epic ID - run: kernl epic run <epic-id>")
	}
	epicID := remainingArgs[0]
	cwd, _ := os.Getwd()
	repoEntry, err := resolveRepoEntry(a.Config, repoFlag, cwd)
	if err != nil {
		return err
	}
	repoPath := repoEntry.Path

	// U1: Config and CLI flags for autonomous mode. The lookup honors the
	// global --config flag (it used to hardcode "kernl.yaml", silently
	// ignoring non-default configs) and surfaces parse failures instead of
	// discarding them.
	if !autonomous && !interactive {
		autoCfg, err := dispatch.LoadAutonomousConfig(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot read orchestrator.autonomous from %s: %v\n", configPath, err)
		}
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
				if err := confirmShape(os.Stdin, out, res.ShapeID); err != nil {
					return err
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
	if a.StateDir == "" {
		return fmt.Errorf("KERNL DISPATCH FAILURE: no state directory for epic %s, so kernl has nowhere of its own to keep agent state - Fix: set App.StateDir (app.DefaultStateDir() outside tests)", epicID)
	}
	agentStateDir := filepath.Join(a.StateDir, "agentstate")
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

	out(fmt.Sprintf("GUI at http://localhost:%d/?epic=%s\n", actualPort, epicID))

	// Only wire real git execution when the repo path is actually a git
	// repo -- hermetic tests use t.TempDir() which is not a git repo, and
	// the worktree manager already has a no-git mkdir-only fallback for
	// that case.
	gitRunForWM, err := epicGitRunner(repoPath)
	if err != nil {
		return err
	}
	// Wire updateDesc so worktree creation stores the path in runstate.
	wtUpdateDesc := func(beadID string, fn func(string) string) error {
		// Not used for epic branch; runstate tracks worktrees separately.
		return nil
	}
	// The base branch is a fact about the target repository, not a constant.
	// It is resolved once here and handed to everything downstream that has to
	// name it: the worktree manager cuts branches from it and the integration
	// and shipment prompts tell the agent what to merge onto.
	var baseBranch string
	if gitRunForWM != nil {
		resolved, berr := epic.ResolveBaseBranch(repoPath, repoEntry.DefaultBranch, gitRunForWM)
		if berr != nil {
			return berr
		}
		baseBranch = resolved
		out(fmt.Sprintf("base branch: %s\n", baseBranch))
	}

	// How this repository is verified is its own fact too, and a repository
	// that cannot state one gets no work dispatched into it: an agent with no
	// check to run declares itself done having proved nothing.
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

	wm := epic.NewWorktreeManager(a.Config.Orchestrator.WorktreeRoot, repoPath, baseBranch, gitRunForWM, wtUpdateDesc)
	if gitRunForWM != nil {
		if _, err := wm.EnsureEpicBranch(epicID); err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: cannot ensure epic branch for %s: %w", epicID, err)
		}
	}

	// Settle the publish destination before the executor spawns anything, so a
	// run that would be refused at shipment is refused now rather than after
	// every child has been implemented and reviewed. It sits after argument and
	// workflow validation on purpose: a flag typo must still report itself as a
	// flag typo.
	plan, err := resolveShipmentPlan(repoEntry, stopBeforeShipment, out)
	if err != nil {
		return err
	}
	if stopBeforeShipment {
		// epic run's biggest effect under this flag is not the epic's own
		// integration merge - it is every child: real implementation, real
		// review, real commits on each kernl/<beadID> branch, real writes to
		// the target repository's own tracker, real agent cost. Naming only
		// the epic-level merge here would leave the larger effect invisible
		// to whoever is reading the terminal instead of --help.
		out("--stop-before-shipment: children still implement and get reviewed for real (commits, tracker writes, agent cost); integration and integration_review then run for real too and commit onto the epic branch - only the push and pull request are withheld, and that merge cannot be redone\n")
	}

	// The run record opens here, after every read-only validation above and
	// after the shipment destination is settled: a run refused for a flag
	// typo or an undeclared remote must never leave a "running" run node
	// behind for a dispatch that in fact never started. It closes via the
	// defer immediately below, on every path out of this function from here
	// on, including --stop-before-shipment (it only stops shipment, not
	// dispatch itself - see resolveShipmentPlan).
	epicBead, err := a.Backend.Get(epicID, repoPath)
	if err != nil || epicBead == nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic %s not found in repo %s while opening its workflow run record: %w", epicID, repoPath, err)
	}
	runBeads := make([]app.BeadRef, 0, len(ep.Children)+1)
	runBeads = append(runBeads, app.BeadRef{ID: epicID, Title: epicBead.Title})
	for _, child := range ep.Children {
		// IsFixup must be read here, from the child's own labels, and
		// carried into StartWorkflowRun: that call's own bead_reference
		// write is the first one for this run, and it is the only chance -
		// a later one (recordDecisionIfGateType, once this child's
		// implementation stage runs) is a no-op against an id that already
		// exists.
		runBeads = append(runBeads, app.BeadRef{ID: child.ID, Title: child.Title, IsFixup: app.HasLabel(child.Labels, app.IntegrationFixupLabel)})
	}
	runWorkflowName := customProfileID
	if runWorkflowName == "" {
		runWorkflowName = "worker"
	}
	runStartedAt := time.Now()
	runID, err := app.StartWorkflowRun(context.Background(), a.Graph, app.StartWorkflowRunInput{
		EntryPoint:     "epic run",
		Title:          epicBead.Title,
		WorkflowName:   runWorkflowName,
		Beads:          runBeads,
		RepoPath:       repoPath,
		BaseBranch:     baseBranch,
		VerifyCommand:  verifyCommand,
		TrackerCommand: trackerCommand,
		DryRun:         stopBeforeShipment,
		StartedAt:      runStartedAt,
	})
	if err != nil {
		return err
	}
	// The oracle is whichever LLM kernl.yaml configures - nil, deliberately,
	// when none is: ComposeRunReport's own doc comment on why that must
	// never fail the close is the reason a missing llm.provider is not
	// checked here. A configured-but-broken llm.agent DOES stop the run, and
	// stops it now rather than after the work: see NewImpactComposer.
	impactComposer, err := app.NewImpactComposer(a.Config)
	if err != nil {
		return err
	}
	// The reversibility gate asks that same actor a different question, once
	// an integration review rejects. Resolved here, with everything else the
	// run needs, so a broken llm.agent is refused before any work is done
	// rather than at the moment a rejection needs judging.
	reversibilityOracle, err := app.NewOracle(a.Config)
	if err != nil {
		return err
	}
	// The fork gate asks a different actor a different question, mid-stage,
	// whenever an implementer hands one over. Resolved here, with everything
	// else the run needs, so a configured-but-broken da.agent/da.workDir is
	// refused before any work is done rather than at the moment a fork is
	// handed over - the same reason NewOracle is resolved at this point
	// rather than lazily.
	forkDA, err := app.NewDA(a.Config)
	if err != nil {
		return err
	}
	defer func() {
		status, failure := "completed", ""
		if err != nil {
			status, failure = "failed", err.Error()
		}
		finishedAt := time.Now()

		// Composed and written before the run record closes, and on every
		// path out of this function including failure - a report of what
		// failed is worth more than no report, and the run's own status is
		// one of the facts its header states.
		reportPath, reportErr := app.ComposeRunReport(context.Background(), app.ComposeRunReportInput{
			Graph:       a.Graph,
			Composer:    impactComposer,
			RunID:       runID,
			Status:      status,
			FinishedAt:  finishedAt,
			Beads:       beadRunOutcomes(a.Backend, repoPath, runBeads),
			StateDir:    a.StateDir,
			EpicID:      epicID,
			ContextDocs: repoEntry.ContextDocs,
		})
		if reportErr != nil {
			fmt.Fprintf(os.Stderr, "warning: KERNL DISPATCH: composing the run report for epic %s failed: %v\n", epicID, reportErr)
		} else {
			out(fmt.Sprintf("run report: %s\n", reportPath))
		}

		closeErr := app.CloseWorkflowRun(context.Background(), a.Graph, runID, app.CloseWorkflowRunInput{
			Status:     status,
			FinishedAt: finishedAt,
			Failure:    failure,
		})
		switch {
		case closeErr == nil:
		case err != nil:
			err = fmt.Errorf("%w - additionally, closing workflow run %s failed: %v", err, runID, closeErr)
		default:
			// The dispatch itself succeeded; only the run record failed to
			// close. Reporting that as a plain success would leave a run stuck
			// at "running" with no report ever composed for it, and nothing
			// short of reading this error would reveal that happened.
			err = fmt.Errorf("KERNL DISPATCH FAILURE: epic %s completed successfully but its workflow run record %s failed to close: %w - Fix: the run node in the graph is stuck at status \"running\"; investigate the graph db directly", epicID, runID, closeErr)
		}

		// A report that could not be written is escalated only when the run
		// would otherwise report plain success. An unresolved field 4 is
		// swallowed by design (ComposeRunReport's doc comment says why); a
		// report file that does not exist at all is not the same thing,
		// because the operator judges a run by reading one. When the run
		// already failed, the stderr warning above is the record and
		// escalating would only overwrite a more specific failure with a
		// vaguer one.
		if reportErr != nil && err == nil {
			err = fmt.Errorf("KERNL DISPATCH FAILURE: epic %s completed successfully but its run report could not be written: %w", epicID, reportErr)
		}
	}()

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
			warnLostWorktreeRecord(rs.SetWorktree(epicID, in.BeadID, in.Worktree), epicID, in.BeadID)
			// Epic children run the worker profile: implement + review, then
			// STOP at awaiting_integration handing the branch to the epic.
			if err := ensureWorkerEntry(a.Backend, in.BeadID, repoPath, customProfileID); err != nil {
				return epic.RunResult{}, err
			}
			res, err := app.DriveBeadToTerminal(ctx, app.DriveBeadDeps{
				Backend:         a.Backend,
				Driver:          a.Driver,
				Config:          a.Config,
				StateDir:        a.StateDir,
				BeadID:          in.BeadID,
				RepoPath:        repoPath,
				Worktree:        in.Worktree,
				SessionID:       in.SessionID,
				AgentStateStore: stateStore,
				VerifyCommand:   verifyCommand,
				TrackerCommand:  trackerCommand,
				RunID:           runID,
				DA:              forkDA,
				ContextDocs:     repoEntry.ContextDocs,
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
		out(fmt.Sprintf("epic %s blocked - fix the cause and re-run kernl epic run %s to resume\n", epicID, epicID))
		return err
	}

	metric := ex.Parallelism()
	out(fmt.Sprintf("epic %s complete - realized parallelism: %.1fx (peak %d, max %d)\n", epicID, metric.Realized, metric.Peak, metric.GraphMax))

	// All children reached awaiting_integration. Drive the epic bead itself
	// through integration -> integration_review -> shipment -> awaiting_pr_review.
	epicWorktree, werr := wm.AddEpicWorktree(epicID)
	if werr != nil {
		return werr
	}
	warnLostWorktreeRecord(rs.SetWorktree(epicID, epicID, epicWorktree), epicID, epicID)
	if err := driveEpic(context.Background(), epicDrive{
		App: a, Epic: ep, EpicID: epicID, RepoPath: repoPath,
		BaseBranch: baseBranch, VerifyCommand: verifyCommand, TrackerCommand: trackerCommand, Worktree: epicWorktree, RunID: runID,
		StateStore: stateStore, Shipment: plan, Out: out,
		IrreversibleSurfaces: repoEntry.IrreversibleSurfaces,
		// The same actor the run report's oracle is, asked a different
		// question: nothing else in kernl has an opinion worth having about
		// what a change costs to undo.
		Judge:       app.OracleReversibilityJudge{Oracle: reversibilityOracle},
		DA:          forkDA,
		ContextDocs: repoEntry.ContextDocs,
	}); err != nil {
		out(fmt.Sprintf("epic %s blocked at integration - fix the cause and re-run kernl epic run %s to resume\n", epicID, epicID))
		return err
	}

	return nil
}

// confirmShape asks for an explicit go-ahead on the inferred workflow shape.
// Enter or y confirms; anything else aborts. Crucially, EOF (no TTY, closed
// stdin) ABORTS instead of auto-confirming - a prompt that cannot be answered
// must never default to yes.
func confirmShape(in io.Reader, out func(string), shapeID string) error {
	out(fmt.Sprintf("Proceed with shape '%s'? [Y/n] ", shapeID))
	line, err := bufio.NewReader(in).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	if err != nil && answer == "" {
		return fmt.Errorf("KERNL DISPATCH FAILURE: --interactive needs an answer but stdin closed before one arrived - drop --interactive to accept the inferred shape without a prompt in non-TTY contexts")
	}
	if answer == "" || answer == "y" {
		return nil
	}
	return fmt.Errorf("KERNL DISPATCH FAILURE: aborted at shape confirmation (answered %q) - re-run kernl epic run and answer y, or use --autonomous", answer)
}

// beadRunOutcomes resolves each run bead's current tracker state for the run
// report's header and summary. It is best-effort, not fail-loud, on purpose:
// ComposeRunReport's own doc comment already establishes that composing the
// report must never fail the run, and a bead whose state cannot be re-read
// at the very end of a run (the tracker briefly unreachable, a bead deleted
// mid-run) is exactly that same class of problem, not a reason to lose the
// report entirely.
func beadRunOutcomes(be backend.BackendPort, repoPath string, runBeads []app.BeadRef) []app.BeadRunOutcome {
	out := make([]app.BeadRunOutcome, 0, len(runBeads))
	for _, ref := range runBeads {
		state := "unknown"
		if bead, err := be.Get(ref.ID, repoPath); err == nil && bead != nil {
			state = bead.State
		}
		out = append(out, app.BeadRunOutcome{ID: ref.ID, Title: ref.Title, FinalState: state})
	}
	return out
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

// shipmentPlan carries the verified answer to "where does this run publish?"
// through the epic driver: the destination checked before dispatch, the
// allow-list the reported pull request is checked against afterwards, and
// whether shipment runs at all.
type shipmentPlan struct {
	Destination shipment.Destination
	Allowed     []string
	DryRun      bool
	// TextCommand is registry.repos[].prTextCommand: what this repository runs
	// over a pull request's title and body to say the prose is publishable.
	// Empty means it declares none, which publishes as before.
	TextCommand string
}

// resolveShipmentPlan settles where a run may publish before a single agent is
// spawned. A run that would be refused at shipment is refused now, rather than
// after the agent work that precedes it; --stop-before-shipment (--dry-run) is
// how the rest of the pipeline is exercised while that configuration is still
// missing.
//
// It prints nothing for the stop-before-shipment case itself: what still runs
// for real differs between epic run (which dispatches every child) and epic
// merge (which does not), so each caller states its own scope right after
// calling this, rather than this shared function guessing at a scope it does
// not have.
func resolveShipmentPlan(repoEntry config.RepoEntry, stopBeforeShipment bool, out func(string)) (shipmentPlan, error) {
	plan := shipmentPlan{Allowed: repoEntry.Shipment.AllowedRemotes, DryRun: stopBeforeShipment, TextCommand: repoEntry.PRTextCommand}
	if stopBeforeShipment {
		return plan, nil
	}
	dest, err := shipment.ResolveDestination(repoEntry.Path, repoEntry.Shipment.Remote, repoEntry.Shipment.AllowedRemotes, nil)
	if err != nil {
		return shipmentPlan{}, err
	}
	plan.Destination = dest
	out(fmt.Sprintf("shipment destination: %s (%s)\n", dest.RemoteName, dest.RemoteURL))
	return plan, nil
}

// epicDrive is everything driving the epic bead through its tail needs.
type epicDrive struct {
	App            *app.App
	Epic           *epic.Epic
	EpicID         string
	RepoPath       string
	BaseBranch     string
	VerifyCommand  string
	TrackerCommand string
	Worktree       string
	StateStore     *workflow.AgentStateStore
	Shipment       shipmentPlan
	Out            func(string)
	// IrreversibleSurfaces is what this repository declares it cannot cheaply
	// undo (registry.repos[].irreversibleSurfaces). An integration rejection
	// on a branch that touched one goes to the operator instead of another
	// automatic fix-up round.
	IrreversibleSurfaces []string
	// Judge answers how expensive an integration rejection would be to
	// reverse. Nil when no oracle is configured, which escalates rather than
	// guessing.
	Judge app.ReversibilityJudge
	// RunID is the same workflow run the children ran under: integration and
	// shipment are stages of that one run, not a second one, so a decision
	// recorded here belongs to the same report as a child's.
	RunID string
	// DA and ContextDocs are threaded into the epic bead's own DriveBeadDeps
	// for the same reason every DriveBeadDeps construction site carries them
	// (see DriveBeadDeps.DA): today this is a no-op for the epic profile's
	// own stages (integration/integration_review/shipment never declare a
	// decision_record exit gate, so the fork gate never arms here), but a
	// DriveBeadDeps built without them would silently disagree with every
	// other call site about whether this actor exists at all.
	DA          app.DA
	ContextDocs []string
}

// buildIntegrationChildren pairs each FINISHED child bead with its own
// artifact directory, which sits beside the epic's under the same run root
// (<StateDir>/run/<epicID>/<id> - see resolveArtifactDir in
// internal/app/drive_bead.go). Before exit-gate artifacts moved outside the
// worktree, a child's plan and review verdicts arrived for free once its
// branch was merged; now that they never travel in a commit, the
// integration agent has no way to read a sibling's artifacts unless the
// path is named explicitly here, alongside the branch it already names.
//
// "Finished" is awaiting_integration and nothing else, because that state is
// the profile's own way of saying "done, handed to the epic". Every child used
// to be listed regardless of state, which names a kernl/<id> branch for beads
// that have none: one deferred by the operator, one closed without ever being
// implemented. The integration agent is then told to merge a ref that does not
// exist, and the prompt's only escape hatch is merge_conflict, which that is
// not - so it improvises. The worse half is a child closed as INVALID whose
// branch still carries commits: listing it merges work somebody deliberately
// threw away.
func buildIntegrationChildren(children []backend.Bead, epicArtifactDir string) []prompt.Child {
	runRoot := filepath.Dir(epicArtifactDir)
	cs := make([]prompt.Child, 0, len(children))
	for _, c := range children {
		if c.State != string(workflow.StatusAwaitingIntegration) {
			continue
		}
		cs = append(cs, prompt.Child{ID: c.ID, Branch: "kernl/" + c.ID, ArtifactDir: filepath.Join(runRoot, c.ID)})
	}
	return cs
}

// buildReviewedChildren pairs each MERGED child with the criteria it was
// implemented against, which is what the integration reviewer grades the merge
// by. It carries none of the branch and artifact paths buildIntegrationChildren
// does: by review time every listed branch is already merged, so naming them
// again would describe work that no longer exists to do.
//
// The filter is the same one buildIntegrationChildren applies, and it has to
// be: the prompt introduces this list as "children merged into this branch",
// so listing a child that was not merged states something untrue and the
// reviewer grades against it. A real epic was rejected for exactly that -
// two children the operator had deliberately deferred were listed as merged,
// were correctly found absent from the branch, and the review concluded the
// epic was unfinished. Integration was taught this in one place and the
// review was left listing everything, which is how one fix covered half a
// defect.
func buildReviewedChildren(children []backend.Bead) []prompt.ReviewedChild {
	cs := make([]prompt.ReviewedChild, 0, len(children))
	for _, c := range children {
		if c.State != string(workflow.StatusAwaitingIntegration) {
			continue
		}
		cs = append(cs, prompt.ReviewedChild{ID: c.ID, Title: c.Title, Acceptance: c.Acceptance})
	}
	return cs
}

// driveEpic puts the epic bead on the epic profile and drives it through
// integration -> integration_review -> shipment, ending at awaiting_pr_review.
// The BuildPrompt override injects epic-specific integration/shipment prompts.
// epicAlreadyInTail reports whether an epic is already somewhere inside the
// epic profile's own pipeline, in which case driveEpic must leave its state
// alone and let DriveBeadToTerminal resume from it.
//
// Writing the entry state unconditionally made the epic tail unresumable.
// Once integration has already merged everything there is nothing left to
// merge, so no NEW commit can exist, and the commit_marker gate refuses an
// ancestor on purpose (it was hardened for exactly that). An epic rewound to
// ready_for_integration therefore fails that gate forever, parks at blocked,
// and cannot be rescued by hand either: a manual advance to
// ready_for_shipment was observed being overwritten by the next run seconds
// later. The message the run prints on failure - "re-run kernl epic run <id>
// to resume" - was false for every epic past integration.
//
// "blocked" is deliberately NOT treated as being in the tail. It is not one
// of the profile's own states, and rewinding a blocked epic to the entry is
// the existing way an operator unsticks one; taking that away is a separate
// decision from fixing the rewind, and it is not this change's to take. What
// this does buy a blocked epic is that a deliberate advance now sticks.
func epicAlreadyInTail(state string) bool {
	for _, s := range backend.BuiltinProfileDescriptor("epic").States {
		if s == state {
			return state != "ready_for_integration"
		}
	}
	return false
}

func driveEpic(ctx context.Context, d epicDrive) error {
	// Named locals for the body below, which predates the struct. The struct
	// exists for the call sites: five of these are strings, and a positional
	// list of five adjacent strings is one transposition away from driving the
	// wrong repository with the right branch name.
	a, ep, epicID, repoPath := d.App, d.Epic, d.EpicID, d.RepoPath
	baseBranch, epicWorktree := d.BaseBranch, d.Worktree
	stateStore, plan, out := d.StateStore, d.Shipment, d.Out

	epicBead, err := a.Backend.Get(epicID, repoPath)
	if err != nil || epicBead == nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic %s not found in repo %s: %w", epicID, repoPath, err)
	}
	labels := setWFLabel(epicBead.Labels, "wf:profile:", "epic")
	if !epicAlreadyInTail(epicBead.State) {
		labels = setWFLabel(labels, "wf:state:", "ready_for_integration")
		if err := a.Backend.Update(epicID, backend.UpdateBeadInput{State: "ready_for_integration", SetLabels: labels}, repoPath); err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: cannot set epic %s to ready_for_integration: %w", epicID, err)
		}
	} else if err := a.Backend.Update(epicID, backend.UpdateBeadInput{SetLabels: labels}, repoPath); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: cannot put epic %s on the epic profile: %w", epicID, err)
	}

	stopBefore := ""
	if plan.DryRun {
		stopBefore = "shipment"
	}

	res, err := app.DriveEpicIntegrationTail(ctx, app.DriveEpicIntegrationTailDeps{
		EpicID:               epicID,
		EpicBranch:           "feat/" + epicID,
		BaseBranch:           baseBranch,
		IrreversibleSurfaces: d.IrreversibleSurfaces,
		Judge:                d.Judge,
		DriveBeadDeps: app.DriveBeadDeps{
			Backend:         a.Backend,
			Driver:          a.Driver,
			Config:          a.Config,
			StateDir:        a.StateDir,
			BeadID:          epicID,
			RepoPath:        repoPath,
			Worktree:        epicWorktree,
			AgentStateStore: stateStore,
			StopBeforeState: stopBefore,
			Log: func(stage int, state string) {
				ts := time.Now().Format("15:04:05")
				out(fmt.Sprintf("[%s] epic %s [stage %d] %s\n", ts, epicID, stage, state))
				a.EpicEvents.Publish(epic.EpicEvent{Type: epic.BeadStateChanged, EpicID: ep.ID, BeadID: epicID, Detail: state, Time: time.Now().Unix()})
			},
			VerifyCommand:  d.VerifyCommand,
			TrackerCommand: d.TrackerCommand,
			RunID:          d.RunID,
			DA:             d.DA,
			ContextDocs:    d.ContextDocs,
			BuildPrompt: func(in app.StagePromptInput, wf backend.WorkflowDescriptor) string {
				switch in.State {
				case "integration":
					children, listErr := a.Backend.List(&backend.BeadListFilters{Parent: epicID}, in.RepoPath)
					if listErr != nil {
						// Merging whatever a failed listing returned is the
						// one outcome worse than not merging: an empty list
						// reads as "this epic has no work", and integration
						// would commit nothing and call it done.
						out(fmt.Sprintf("KERNL DISPATCH FAILURE: cannot read epic %s's children to decide what to merge: %v\n", epicID, listErr))
						return ""
					}
					cs := buildIntegrationChildren(children, in.ArtifactDir)
					s, perr := prompt.RenderIntegration(prompt.IntegrationInput{
						EpicID: epicID, EpicTitle: in.Bead.Title,
						EpicBranch: "feat/" + epicID, BaseBranch: baseBranch, Children: cs,
						VerifyCommand: in.VerifyCommand, TrackerCommand: in.TrackerCommand,
					})
					if perr != nil {
						return app.BuildBeadStagePrompt(in)
					}
					return s
				case "integration_review":
					artifactPath := backend.ResolveArtifactPath("<artifact_dir>/integration-review.md", epicID, in.ArtifactDir)
					children, listErr := a.Backend.List(&backend.BeadListFilters{Parent: epicID}, in.RepoPath)
					if listErr != nil {
						// Falling through with no children would tell the
						// reviewer this epic commissioned nothing, which is a
						// different statement from "kernl could not look" - and
						// the reviewer grades against exactly that text.
						out(fmt.Sprintf("KERNL DISPATCH FAILURE: cannot read epic %s's children for the integration review scope: %v\n", epicID, listErr))
						return ""
					}
					s, perr := prompt.RenderIntegrationReview(prompt.IntegrationReviewInput{
						EpicID: epicID, EpicTitle: in.Bead.Title,
						EpicDescription: in.Bead.Description, Children: buildReviewedChildren(children),
						ArtifactPath: artifactPath, VerifyCommand: in.VerifyCommand, TrackerCommand: in.TrackerCommand,
					})
					if perr != nil {
						return app.BuildBeadStagePrompt(in)
					}
					return s
				case "shipment":
					prBodyPath := backend.ResolveArtifactPath("<artifact_dir>/pr-body.md", epicID, in.ArtifactDir)
					s, perr := prompt.RenderShipment(prompt.ShipmentInput{
						EpicID: epicID, EpicTitle: in.Bead.Title,
						EpicBranch: "feat/" + epicID, BaseBranch: baseBranch,
						RemoteName: plan.Destination.RemoteName, RemoteURL: plan.Destination.RemoteURL,
						RepoSlug: plan.Destination.RepoSlug, TrackerCommand: in.TrackerCommand,
						PRBodyPath: prBodyPath, PRTextCommand: plan.TextCommand,
					})
					if perr != nil {
						// Falling back to the generic prompt here would drop the
						// verified destination and hand the agent the ambiguity
						// back. The stage does not run without one.
						out(fmt.Sprintf("KERNL DISPATCH FAILURE: cannot render the shipment prompt for epic %s: %v\n", epicID, perr))
						return ""
					}
					return s
				default:
					return app.BuildBeadStagePrompt(in)
				}
			},
		},
	})
	// A fix-up bead was created and the epic rewound to ready_for_integration
	// - a graceful, expected pause, not a failure: re-running `epic run`
	// discovers the new bead as a real child and, once it reaches
	// awaiting_integration, this function runs again and re-attempts
	// integration. Checked before err: DriveEpicIntegrationTail always
	// returns a nil error alongside a non-empty FixupBeadID.
	if res.FixupBeadID != "" {
		// The judgment that let this continue is printed, not only the fact
		// that it did. A gate that decides on its own to keep a run going and
		// leaves no trace of why is the same problem as one that escalates
		// silently, only harder to notice.
		out(fmt.Sprintf("epic %s: reversibility gate continued [%s]: %s\n", epicID, res.ReversibilityCause, res.ReversibilityReason))
		out(fmt.Sprintf("epic %s: integration review rejected with a fix-up - created bead %s, epic rewound to ready_for_integration; re-run `kernl epic run %s` once it reaches awaiting_integration\n", epicID, res.FixupBeadID, epicID))
		return nil
	}
	if err != nil {
		return err
	}
	if !res.Success {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic %s integration stopped at %q", epicID, res.FinalState)
	}
	if err := verifyPublishedPullRequest(a, epicID, repoPath, plan); err != nil {
		return err
	}
	if err := verifyPublishedPullRequestText(a, epicID, epicBead.Title, repoPath, plan); err != nil {
		return err
	}
	out(fmt.Sprintf("epic %s → %s\n", epicID, res.FinalState))
	return nil
}

// verifyPublishedPullRequestText runs the repository's own prose gate over the
// pull request shipment published.
//
// The shipment prompt already tells the agent to run this before calling gh,
// which is where the failure is actually prevented: kernl cannot intercept the
// text, because the agent composes the body AND opens the pull request. That
// makes the prompt step the gate and this the backstop - an agent that skips
// it publishes prose the repository refuses, and without this the run reports
// plain success while the pull request is already red. That is exactly what
// happened: a pull request whose code passed every check was refused on two
// em-dashes in its body, and nothing in the run had ever looked at the one
// artifact the stage writes entirely by itself.
//
// Unlike verifyPublishedPullRequest, a failure here does NOT block the epic.
// Publishing to a repository the operator never allowed is unsafe and the work
// has to stop; prose the gate refuses is a pull request that exists, is
// awaiting review, and needs its text edited. Marking the epic blocked would
// say the work is broken when it is the writing that is.
func verifyPublishedPullRequestText(a *app.App, epicID, epicTitle, repoPath string, plan shipmentPlan) error {
	if plan.DryRun || strings.TrimSpace(plan.TextCommand) == "" {
		return nil
	}
	artifactDir, err := app.ArtifactDirPath(a.StateDir, epicID, epicID)
	if err != nil {
		return err
	}
	bodyPath := filepath.Join(artifactDir, "pr-body.md")
	body, readErr := os.ReadFile(bodyPath)
	if readErr != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic %s published a pull request, but its body file %s cannot be read, so %s's own text gate could not be applied to what went out: %w - Fix: read the pull request on GitHub and check its text by hand", epicID, bodyPath, repoPath, readErr)
	}
	return shipment.CheckPRText(shipment.PRTextCheckInput{
		RepoPath: repoPath,
		Command:  plan.TextCommand,
		Title:    epicTitle,
		Body:     string(body),
	})
}

// verifyPublishedPullRequest checks, after shipment has run, that the pull
// request it reported lives on an allowed repository.
//
// The pre-dispatch check fixes the destination; this one catches the agent
// routing around it. That is not hypothetical: the run that motivated this code
// was pointed at a clone whose origin was a local path, found the public
// upstream reachable through it, and opened a real pull request there. The
// shipment exit gate already requires a "pr_url:" line, so the evidence is
// sitting in the epic description by the time this runs.
func verifyPublishedPullRequest(a *app.App, epicID, repoPath string, plan shipmentPlan) error {
	if plan.DryRun {
		return nil
	}
	epicBead, err := a.Backend.Get(epicID, repoPath)
	if err != nil || epicBead == nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: cannot re-read epic %s to verify where it published: %w", epicID, err)
	}
	prURL := workflow.GetPRURL(epicBead.Description)
	if prURL == "" {
		// No pull request means shipment did not reach that far; the stage's
		// own outcome reporting owns that case.
		return nil
	}
	checkErr := shipment.CheckPullRequestAllowed(prURL, plan.Allowed)
	if checkErr == nil {
		return nil
	}

	// The drive loop advances the bead as soon as the exit gate passes, so by
	// the time this runs the epic already reads as awaiting_pr_review. Leaving
	// it there would make the tracker say the run succeeded while the CLI says
	// it published somewhere it may not - and the tracker is what the next
	// session reads. Block it, and say so if that itself fails.
	if err := a.Backend.Update(epicID, backend.UpdateBeadInput{State: string(workflow.StatusBlocked)}, repoPath); err != nil {
		return fmt.Errorf("%w - and the epic could not be marked blocked (%v), so its state still reads as a successful run: fix it by hand", checkErr, err)
	}
	return fmt.Errorf("%w - epic %s marked blocked", checkErr, epicID)
}
