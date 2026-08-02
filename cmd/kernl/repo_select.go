package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/app"
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
	// Getwd failing is not fatal here: it just leaves cwd empty, so the
	// working directory cannot answer and resolveRepoEntry falls through to
	// naming the repo explicitly or refusing.
	cwd, _ := os.Getwd()
	entry, err := resolveRepoEntry(cfg, requested, cwd)
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
//
// cwd is the caller's working directory, used only when requested is empty
// and more than one repo is registered: standing inside a registered repo is
// an explicit, checkable fact, so it answers the question before the refusal
// does. It is a parameter rather than a call to os.Getwd() here so this stays
// a pure function of its inputs and the tests stay hermetic (AGENTS.md SS4);
// real callers pass os.Getwd()'s result, and a failed Getwd just means an
// empty cwd, which falls through to the refusal like no match would.
func resolveRepoEntry(cfg *config.Config, requested string, cwd string) (config.RepoEntry, error) {
	repos := cfg.Registry.Repos
	if len(repos) == 0 {
		return config.RepoEntry{}, fmt.Errorf("KERNL DISPATCH FAILURE: no repos registered - Fix: add a repo to registry.repos in kernl.yaml")
	}

	if requested == "" {
		if len(repos) == 1 {
			return repos[0], nil
		}
		if entry, ok := repoAtOrAboveWorkingDir(repos, cwd); ok {
			return entry, nil
		}
		return config.RepoEntry{}, fmt.Errorf(
			"KERNL DISPATCH FAILURE: %d repos are registered and none was named, and %s, so which one this is for is a guess - Fix: pass --repo <path|name>, one of: %s",
			len(repos), workingDirTriedMessage(cwd), strings.Join(repoPaths(repos), ", "))
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
	clean := filepath.Clean(configured)
	return clean == filepath.Clean(requested) || filepath.Base(clean) == requested
}

func repoPaths(repos []config.RepoEntry) []string {
	paths := make([]string, len(repos))
	for i, repo := range repos {
		paths[i] = repo.Path
	}
	return paths
}

// repoAtOrAboveWorkingDir finds the registered repo whose configured path is
// cwd itself or an ancestor of it, walking upward so a repo nested inside
// another registered repo (the deepest match) wins over the coarser one. A
// tie at the same level - two registered repos sharing a path - is refused
// rather than picked, same as an ambiguous --repo name.
func repoAtOrAboveWorkingDir(repos []config.RepoEntry, cwd string) (config.RepoEntry, bool) {
	if cwd == "" {
		return config.RepoEntry{}, false
	}
	for dir := filepath.Clean(cwd); ; {
		if entry, ok, ambiguous := repoWithPath(repos, dir); ambiguous {
			return config.RepoEntry{}, false
		} else if ok {
			return entry, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return config.RepoEntry{}, false
		}
		dir = parent
	}
}

// repoWithPath reports the single repo configured at path, and separately
// flags whether more than one repo shares it, so the caller can tell "no
// match, keep climbing" apart from "matched, but ambiguous, stop climbing".
func repoWithPath(repos []config.RepoEntry, path string) (entry config.RepoEntry, ok bool, ambiguous bool) {
	for _, repo := range repos {
		if filepath.Clean(repo.Path) != path {
			continue
		}
		if ok {
			return config.RepoEntry{}, false, true
		}
		entry, ok = repo, true
	}
	return entry, ok, false
}

// workingDirTriedMessage says what the cwd-based lookup found, so the
// refusal names the working directory as something that was tried and
// failed rather than a step the operator cannot see happened at all.
func workingDirTriedMessage(cwd string) string {
	if cwd == "" {
		return "the working directory could not be determined"
	}
	return fmt.Sprintf("the working directory (%s) matched none of them", cwd)
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
