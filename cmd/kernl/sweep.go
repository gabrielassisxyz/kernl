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
)

func runSweep(configPath string, args []string) error {
	flags := parseSweepFlags(args)

	repoPath := flags.repo
	if repoPath == "" {
		repoPath = "."
	}

	b := backend.NewBdCliBackend(repoPath)
	adapter := &sweepBackendAdapter{b: b, dir: repoPath}
	ghAdapter := &ghCliAdapter{}

	cfg := sweep.Config{
		DryRun:           flags.dryRun,
		FailureThreshold: flags.failureThreshold,
		BackoffMinutes:   flags.backoffMinutes,
		PRStaleWarnDays:  flags.staleWarnDays,
	}

	s := sweep.New(adapter, ghAdapter, cfg)
	return s.Tick()
}

type sweepFlags struct {
	dryRun           bool
	repo             string
	failureThreshold int
	backoffMinutes   []int
	staleWarnDays    int
}

func parseSweepFlags(args []string) sweepFlags {
	f := sweepFlags{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run":
			f.dryRun = true
		case "--repo":
			if i+1 < len(args) {
				f.repo = args[i+1]
				i++
			}
		case "--failure-threshold":
			if i+1 < len(args) {
				if v, err := strconv.Atoi(args[i+1]); err == nil {
					f.failureThreshold = v
				}
				i++
			}
		case "--backoff-minutes":
			if i+1 < len(args) {
				for _, s := range strings.Split(args[i+1], ",") {
					if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
						f.backoffMinutes = append(f.backoffMinutes, v)
					}
				}
				i++
			}
		case "--stale-warn-days":
			if i+1 < len(args) {
				if v, err := strconv.Atoi(args[i+1]); err == nil {
					f.staleWarnDays = v
				}
				i++
			}
		}
	}
	return f
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
