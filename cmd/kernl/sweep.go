package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/sweep"
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
	dryRun := flags.dryRun || !flags.yes
	if !flags.dryRun && !flags.yes {
		fmt.Fprintln(os.Stderr, "sweep: dry-run (no epics will be closed) — add --yes to close merged epics, or --dry-run to silence this notice")
	}

	b := backend.NewBdCliBackend(repoPath)
	adapter := &sweepBackendAdapter{b: b, dir: repoPath}
	ghAdapter := &ghCliAdapter{}

	cfg := sweep.Config{
		DryRun:           dryRun,
		FailureThreshold: flags.failureThreshold,
		BackoffMinutes:   flags.backoffMinutes,
		PRStaleWarnDays:  flags.staleWarnDays,
	}

	s := sweep.New(adapter, ghAdapter, cfg)
	return s.Tick()
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
					return f, usagef("KERNL DISPATCH FAILURE: --backoff-minutes needs comma-separated integers, got %q — example: --backoff-minutes 5,15,60", v)
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
			return f, usagef("KERNL DISPATCH FAILURE: unknown sweep flag %q%s — valid: %s",
				args[i], didYouMean(args[i], sweepFlagNames), strings.Join(sweepFlagNames, ", "))
		}
	}
	return f, nil
}

func sweepFlagValue(args []string, i int) (string, int, error) {
	if i+1 >= len(args) {
		return "", i, usagef("KERNL DISPATCH FAILURE: %s requires a value — run: kernl sweep --help", args[i])
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
	b   *backend.BdCliBackend
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
		prURL := ""
		if eb.Metadata != nil {
			if u, ok := eb.Metadata["pr_url"].(string); ok {
				prURL = u
			}
		}
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
