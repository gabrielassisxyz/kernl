package main

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

func cfgWithRepos(paths ...string) *config.Config {
	repos := make([]config.RepoEntry, len(paths))
	for i, p := range paths {
		repos[i] = config.RepoEntry{Path: p}
	}
	return &config.Config{Registry: config.RegistryConfig{Repos: repos}}
}

func TestResolveRepoEntryWithOneRepoNeedsNoFlag(t *testing.T) {
	got, err := resolveRepoEntry(cfgWithRepos("/home/me/archeion"), "", "/somewhere/else")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Path != "/home/me/archeion" {
		t.Fatalf("repo = %q, want the only registered one", got.Path)
	}
}

// The finding itself: with two repos registered, the first used to win and the
// second was unreachable with nothing said about it.
func TestResolveRepoEntryRefusesToGuessBetweenRepos(t *testing.T) {
	_, err := resolveRepoEntry(cfgWithRepos("/home/me/archeion", "/home/me/daytrace"), "", "/home/me/somewhere-else")
	if err == nil {
		t.Fatal("expected a loud failure rather than a silent pick of the first repo")
	}
	for _, want := range []string{"--repo", "/home/me/archeion", "/home/me/daytrace", "working directory"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must mention %q so the operator can act on it, got: %v", want, err)
		}
	}
}

// The working directory answers the question a bare --repo cannot, before the
// refusal fires: standing inside a registered repo is an explicit, checkable
// fact, not a guess.
func TestResolveRepoEntryFromWorkingDirExactMatch(t *testing.T) {
	cfg := cfgWithRepos("/home/me/archeion", "/home/me/daytrace")

	got, err := resolveRepoEntry(cfg, "", "/home/me/daytrace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Path != "/home/me/daytrace" {
		t.Fatalf("repo = %q, want the one matching the working directory", got.Path)
	}
}

func TestResolveRepoEntryFromWorkingDirSubdirectory(t *testing.T) {
	cfg := cfgWithRepos("/home/me/archeion", "/home/me/daytrace")

	got, err := resolveRepoEntry(cfg, "", "/home/me/daytrace/internal/backend")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Path != "/home/me/daytrace" {
		t.Fatalf("repo = %q, want the ancestor repo containing the working directory", got.Path)
	}
}

// A repo registered inside another registered repo is the more specific
// answer, so the deepest match wins over the coarser ancestor.
func TestResolveRepoEntryFromWorkingDirDeepestNestedMatchWins(t *testing.T) {
	cfg := cfgWithRepos("/home/me/monorepo", "/home/me/monorepo/services/api")

	got, err := resolveRepoEntry(cfg, "", "/home/me/monorepo/services/api/handlers")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Path != "/home/me/monorepo/services/api" {
		t.Fatalf("repo = %q, want the deepest registered ancestor", got.Path)
	}
}

// An explicit --repo still wins over the working directory: naming the repo
// is always more specific than inferring it.
func TestResolveRepoEntryExplicitFlagWinsOverWorkingDir(t *testing.T) {
	cfg := cfgWithRepos("/home/me/archeion", "/home/me/daytrace")

	got, err := resolveRepoEntry(cfg, "archeion", "/home/me/daytrace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Path != "/home/me/archeion" {
		t.Fatalf("repo = %q, want the explicitly requested one, not the working directory match", got.Path)
	}
}

// A working directory that sits outside every registered repo still refuses,
// and the message names the working directory as the thing that was tried.
func TestResolveRepoEntryWorkingDirNoMatchStillRefuses(t *testing.T) {
	cfg := cfgWithRepos("/home/me/archeion", "/home/me/daytrace")

	_, err := resolveRepoEntry(cfg, "", "/somewhere/unregistered")
	if err == nil {
		t.Fatal("expected a loud failure when the working directory matches no registered repo")
	}
	for _, want := range []string{"working directory", "/somewhere/unregistered", "--repo"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must mention %q so the operator can act on it, got: %v", want, err)
		}
	}
}

func TestResolveRepoEntryMatchesPathOrName(t *testing.T) {
	cfg := cfgWithRepos("/home/me/archeion", "/home/me/daytrace")

	for _, requested := range []string{"/home/me/daytrace", "/home/me/daytrace/", "daytrace"} {
		got, err := resolveRepoEntry(cfg, requested, "")
		if err != nil {
			t.Fatalf("--repo %q: unexpected error: %v", requested, err)
		}
		if got.Path != "/home/me/daytrace" {
			t.Errorf("--repo %q resolved to %q", requested, got.Path)
		}
	}
}

func TestResolveRepoEntryRejectsUnknownAndAmbiguous(t *testing.T) {
	t.Run("unknown", func(t *testing.T) {
		_, err := resolveRepoEntry(cfgWithRepos("/home/me/archeion"), "nope", "")
		if err == nil {
			t.Fatal("expected a loud failure for a repo that is not registered")
		}
	})

	t.Run("ambiguous name", func(t *testing.T) {
		_, err := resolveRepoEntry(cfgWithRepos("/a/archeion", "/b/archeion"), "archeion", "")
		if err == nil {
			t.Fatal("expected a loud failure when a name matches two repos")
		}
		if !strings.Contains(err.Error(), "full path") {
			t.Fatalf("error must say how to disambiguate, got: %v", err)
		}
	})
}

func TestTakeRepoFlag(t *testing.T) {
	t.Run("separate value", func(t *testing.T) {
		got, rest, err := takeRepoFlag("epic run", []string{"--dry-run", "--repo", "archeion", "e1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "archeion" {
			t.Errorf("repo = %q", got)
		}
		if strings.Join(rest, " ") != "--dry-run e1" {
			t.Errorf("rest = %v, want the other args untouched and in order", rest)
		}
	})

	t.Run("inline value", func(t *testing.T) {
		got, rest, err := takeRepoFlag("epic run", []string{"--repo=archeion", "e1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "archeion" || strings.Join(rest, " ") != "e1" {
			t.Errorf("repo = %q, rest = %v", got, rest)
		}
	})

	t.Run("missing value", func(t *testing.T) {
		if _, _, err := takeRepoFlag("epic run", []string{"--repo"}); err == nil {
			t.Fatal("expected a usage error when --repo has no value")
		}
		if _, _, err := takeRepoFlag("epic run", []string{"--repo="}); err == nil {
			t.Fatal("expected a usage error when --repo= has an empty value")
		}
	})
}
