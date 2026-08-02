package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

// appForSelectedRepo builds the application around the repository this
// invocation names, so it gets that repository's tracker.
//
// The order matters and is the whole point: the backend is chosen once, at
// construction, from a memory manager that belongs to one repository. Building
// the app first and resolving --repo afterwards changed which path was passed
// to each call and never which implementation received it - so `--repo
// <a br repository>` ran bd against it and found no database.
//
// It parses --repo without consuming it; the verb's own takeRepoFlag strips it
// and resolves the same entry again, which is idempotent.
func appForSelectedRepo(cfg *config.Config, verb string, args []string) (*app.App, error) {
	requested, _, err := takeRepoFlag(verb, args)
	if err != nil {
		return nil, err
	}
	entry, err := resolveRepoEntry(cfg, requested)
	if err != nil {
		return nil, err
	}
	a, err := app.NewAppForRepo(cfg, entry.Path)
	if err != nil {
		return nil, wrapLoud("creating app", err)
	}
	return a, nil
}

// resolveRepoEntry picks which registered repository an orchestrator verb acts
// on.
//
// registry.repos[0] used to win, silently. With two repositories registered
// that leaves one of them unreachable with nothing said about it, and it makes
// the destination of a run depend on the order of a YAML list - including the
// destination of the one stage that publishes.
//
// requested matches a repository by its configured path or by the last element
// of that path, which is the name an operator would type.
func resolveRepoEntry(cfg *config.Config, requested string) (config.RepoEntry, error) {
	repos := cfg.Registry.Repos
	if len(repos) == 0 {
		return config.RepoEntry{}, fmt.Errorf("KERNL DISPATCH FAILURE: no repos registered - Fix: add a repo to registry.repos in kernl.yaml")
	}

	if requested == "" {
		if len(repos) == 1 {
			return repos[0], nil
		}
		return config.RepoEntry{}, fmt.Errorf(
			"KERNL DISPATCH FAILURE: %d repos are registered and none was named, so which one this is for is a guess - Fix: pass --repo <path|name>, one of: %s",
			len(repos), strings.Join(repoPaths(repos), ", "))
	}

	var matches []config.RepoEntry
	for _, repo := range repos {
		if repoMatches(repo.Path, requested) {
			matches = append(matches, repo)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return config.RepoEntry{}, fmt.Errorf(
			"KERNL DISPATCH FAILURE: --repo %q matches no registered repo - Fix: pass a path or its last element, one of: %s",
			requested, strings.Join(repoPaths(repos), ", "))
	default:
		return config.RepoEntry{}, fmt.Errorf(
			"KERNL DISPATCH FAILURE: --repo %q matches %d registered repos - Fix: pass the full path instead, one of: %s",
			requested, len(matches), strings.Join(repoPaths(matches), ", "))
	}
}

func repoMatches(configured, requested string) bool {
	// The path comparison itself is shared with internal/backend's
	// AutoRouteFromConfig via backend.SameRepoPath, so a trailing slash or an
	// unclean path resolves the same registry entry from either caller. The
	// bare-name fallback stays local: it is only correct for an operator
	// typing --repo, not for AutoRouteFromConfig's internal path lookups (see
	// the WHY comment there).
	return backend.SameRepoPath(configured, requested) || filepath.Base(filepath.Clean(configured)) == requested
}

func repoPaths(repos []config.RepoEntry) []string {
	paths := make([]string, len(repos))
	for i, repo := range repos {
		paths[i] = repo.Path
	}
	return paths
}

// takeRepoFlag pulls --repo (and --repo=<value>) out of args so each verb can
// parse the rest with its own rules. It is shared rather than repeated because
// a verb that forgets it silently falls back to the first registered repo,
// which is exactly the behavior being removed.
func takeRepoFlag(verb string, args []string) (string, []string, error) {
	var requested string
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case strings.HasPrefix(arg, "--repo="):
			requested = strings.TrimPrefix(arg, "--repo=")
		case arg == "--repo":
			if i+1 >= len(args) {
				return "", nil, usagef("KERNL DISPATCH FAILURE: --repo requires a path or name - run: kernl %s --repo <path|name>", verb)
			}
			requested = args[i+1]
			i++
		default:
			rest = append(rest, arg)
			continue
		}
		if requested == "" {
			return "", nil, usagef("KERNL DISPATCH FAILURE: --repo requires a path or name - run: kernl %s --repo <path|name>", verb)
		}
	}
	return requested, rest, nil
}
