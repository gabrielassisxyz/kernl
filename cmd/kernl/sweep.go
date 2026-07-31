package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/sweep"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

func runSweep(configPath string, args []string) error {
	flags, err := parseSweepFlags(args)
	if err != nil {
		return err
	}

	repoPath := flags.repo
	if repoPath == "" {
		repoPath = "."
	}

	// Closing epics is a tracker mutation: it only happens with an explicit
	// --yes. A bare `kernl sweep` previews as dry-run and says how to apply.
	//
	// This notice is part of the same account of "what this run did" as the
	// dry-run preview and the close receipt below it, both of which go to
	// stdout through ReportHook - it stayed on stderr only because it is
	// printed before the Sweeper exists to route it. Splitting one run's
	// narrative across two streams means a caller piping stdout alone (the
	// documented way to watch what sweep did) misses the line explaining why
	// nothing else follows.
	dryRun := flags.dryRun || !flags.yes
	if !flags.dryRun && !flags.yes {
		fmt.Println("sweep: dry-run (no epics will be closed) - add --yes to close merged epics, or --dry-run to silence this notice")
	}

	// Sweep reads and closes beads, so it needs the repository's own tracker.
	// It used to construct bd unconditionally, which against a br repository
	// opened a Dolt store that is not there and swept nothing.
	appCfg, err := loadCLIConfig(configPath)
	if err != nil {
		return err
	}
	b, err := backend.AutoRouteFromConfig(appCfg, repoPath)
	if err != nil {
		return err
	}
	adapter := &sweepBackendAdapter{b: b, dir: repoPath}
	ghAdapter := &ghCliAdapter{}

	cfg := sweep.Config{
		DryRun:           dryRun,
		FailureThreshold: flags.failureThreshold,
		BackoffMinutes:   flags.backoffMinutes,
		PRStaleWarnDays:  flags.staleWarnDays,
		// Without this, closeAll's success and the no-pr_url skip only ever
		// reached the log package (stderr), so a --yes run that closed
		// epics printed nothing an operator watching stdout would see.
		ReportHook: func(msg string) { fmt.Println(msg) },
	}

	s := sweep.New(adapter, ghAdapter, cfg)
	if err := s.Tick(); err != nil {
		if strings.Contains(err.Error(), "no beads project") {
			return wrapLoud("sweep", fmt.Errorf("%w - Fix: run from a repo with a .beads project, or pass --repo <path>", err))
		}
		return wrapLoud("sweep", err)
	}
	return nil
}

type sweepFlags struct {
	dryRun           bool
	yes              bool
	repo             string
	failureThreshold int
	backoffMinutes   []int
	staleWarnDays    int
}

var sweepFlagNames = []string{
	"--dry-run", "--yes", "--repo", "--failure-threshold",
	"--backoff-minutes", "--stale-warn-days",
}

// parseSweepFlags fails loud on anything it does not understand: a swallowed
// typo used to silently run a LIVE sweep with default settings.
func parseSweepFlags(args []string) (sweepFlags, error) {
	f := sweepFlags{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run":
			f.dryRun = true
		case "--yes":
			f.yes = true
		case "--repo":
			v, next, err := sweepFlagValue(args, i)
			if err != nil {
				return f, err
			}
			f.repo, i = v, next
		case "--failure-threshold":
			v, next, err := sweepIntValue(args, i)
			if err != nil {
				return f, err
			}
			f.failureThreshold, i = v, next
		case "--backoff-minutes":
			v, next, err := sweepFlagValue(args, i)
			if err != nil {
				return f, err
			}
			i = next
			for _, s := range strings.Split(v, ",") {
				n, err := strconv.Atoi(strings.TrimSpace(s))
				if err != nil {
					return f, usagef("KERNL DISPATCH FAILURE: --backoff-minutes needs comma-separated integers, got %q - example: --backoff-minutes 5,15,60", v)
				}
				f.backoffMinutes = append(f.backoffMinutes, n)
			}
		case "--stale-warn-days":
			v, next, err := sweepIntValue(args, i)
			if err != nil {
				return f, err
			}
			f.staleWarnDays, i = v, next
		default:
			return f, usagef("KERNL DISPATCH FAILURE: unknown sweep flag %q%s - valid: %s",
				args[i], didYouMean(args[i], sweepFlagNames), strings.Join(sweepFlagNames, ", "))
		}
	}
	return f, nil
}

func sweepFlagValue(args []string, i int) (string, int, error) {
	if i+1 >= len(args) {
		return "", i, usagef("KERNL DISPATCH FAILURE: %s requires a value - run: kernl sweep --help", args[i])
	}
	return args[i+1], i + 1, nil
}

func sweepIntValue(args []string, i int) (int, int, error) {
	v, next, err := sweepFlagValue(args, i)
	if err != nil {
		return 0, i, err
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, i, usagef("KERNL DISPATCH FAILURE: %s needs an integer, got %q", args[i], v)
	}
	return n, next, nil
}

type sweepBackendAdapter struct {
	b   backend.BackendPort
	dir string
}

func (a *sweepBackendAdapter) ListEpicsAwaitingPRReview() ([]sweep.Epic, error) {
	epicBeads, err := a.b.List(&backend.BeadListFilters{
		State: "awaiting_pr_review",
		Type:  "epic",
	}, a.dir)
	if err != nil {
		return nil, err
	}

	var out []sweep.Epic
	for _, eb := range epicBeads {
		children, err := a.b.List(&backend.BeadListFilters{Parent: eb.ID}, a.dir)
		if err != nil {
			return nil, err
		}
		// The description is where a shipped epic's pr_url actually lives:
		// the shipment prompt tells the agent to write "pr_url: <url>" there,
		// the shipment exit gate refuses to advance without it
		// (description_contains "pr_url:"), and the containment check reads it
		// back the same way (see epic.go's refuseUnallowedPullRequest).
		//
		// This used to read eb.Metadata["pr_url"], a key nothing in this
		// repository has ever written: Metadata is only ever decoded back out
		// of the tracker, and neither bd nor br exposes a way to set it. So
		// the lookup could not succeed even after a fully automatic run, and
		// every epic was skipped with "in awaiting_pr_review without pr_url".
		// That is why sweep had never closed anything - not, as it appeared,
		// because nothing had merged yet.
		prURL := workflow.GetPRURL(eb.Description)
		childIDs := make([]string, 0, len(children))
		for _, c := range children {
			childIDs = append(childIDs, c.ID)
		}
		out = append(out, sweep.Epic{ID: eb.ID, PRURL: prURL, Children: childIDs})
	}
	return out, nil
}

func (a *sweepBackendAdapter) Close(id, reason string) error {
	_, err := a.b.Close(id, reason, a.dir)
	return err
}

type ghCliAdapter struct{}

func (g *ghCliAdapter) View(prURL string) (sweep.PRState, error) {
	cmd := exec.Command("gh", "pr", "view", prURL, "--json", "state,mergedAt,createdAt")
	out, err := cmd.Output()
	if err != nil {
		return sweep.PRState{}, fmt.Errorf("gh pr view: %w", err)
	}

	var raw struct {
		State     string `json:"state"`
		MergedAt  string `json:"mergedAt"`
		CreatedAt string `json:"createdAt"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return sweep.PRState{}, fmt.Errorf("gh pr view parse: %w", err)
	}

	s := sweep.PRState{State: raw.State}
	if raw.MergedAt != "" {
		if t, err := time.Parse(time.RFC3339, raw.MergedAt); err == nil {
			s.MergedAt = t
		}
	}
	if raw.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, raw.CreatedAt); err == nil {
			s.CreatedAt = t
		}
	}
	return s, nil
}
