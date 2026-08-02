package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

func TestDispatchSweepRunsTick(t *testing.T) {
	var ran bool
	sweepFn = func(configPath string, args []string) error { ran = true; return nil }
	t.Cleanup(func() { sweepFn = runSweep })
	if err := Dispatch([]string{"sweep"}); err != nil || !ran {
		t.Fatalf("sweep not dispatched: ran=%v err=%v", ran, err)
	}
}

func TestDispatchSweepPassesArgs(t *testing.T) {
	var captured []string
	sweepFn = func(configPath string, args []string) error {
		captured = args
		return nil
	}
	t.Cleanup(func() { sweepFn = runSweep })
	_ = Dispatch([]string{"sweep", "--dry-run", "--repo", "/tmp/x"})
	if !stringsContain(captured, "--dry-run") {
		t.Fatalf("expected --dry-run in args, got %v", captured)
	}
	if !stringsContain(captured, "--repo") {
		t.Fatalf("expected --repo in args, got %v", captured)
	}
}

func TestSweepFlagsDefault(t *testing.T) {
	f, err := parseSweepFlags(nil)
	if err != nil {
		t.Fatalf("parseSweepFlags(nil): %v", err)
	}
	if f.dryRun {
		t.Fatal("expected dryRun=false by default")
	}
	if f.repo != "" {
		t.Fatalf("expected repo=\"\" by default, got %q", f.repo)
	}
}

func TestSweepFlagsDryRunParsed(t *testing.T) {
	f, err := parseSweepFlags([]string{"--dry-run"})
	if err != nil {
		t.Fatalf("parseSweepFlags: %v", err)
	}
	if !f.dryRun {
		t.Fatal("expected dryRun=true")
	}
}

func TestSweepFlagsRepoParsed(t *testing.T) {
	f, err := parseSweepFlags([]string{"--repo", "/tmp/x"})
	if err != nil {
		t.Fatalf("parseSweepFlags: %v", err)
	}
	if f.repo != "/tmp/x" {
		t.Fatalf("expected repo=/tmp/x, got %q", f.repo)
	}
}

func TestSweepFlagsAllParsed(t *testing.T) {
	f, err := parseSweepFlags([]string{
		"--dry-run",
		"--repo", "/tmp/x",
		"--failure-threshold", "5",
		"--backoff-minutes", "1,2,3",
		"--stale-warn-days", "14",
	})
	if err != nil {
		t.Fatalf("parseSweepFlags: %v", err)
	}
	if !f.dryRun {
		t.Fatal("expected dryRun=true")
	}
	if f.repo != "/tmp/x" {
		t.Fatalf("expected repo=/tmp/x, got %q", f.repo)
	}
	if f.failureThreshold != 5 {
		t.Fatalf("expected failureThreshold=5, got %d", f.failureThreshold)
	}
	if len(f.backoffMinutes) != 3 || f.backoffMinutes[0] != 1 || f.backoffMinutes[1] != 2 || f.backoffMinutes[2] != 3 {
		t.Fatalf("expected backoffMinutes=[1,2,3], got %v", f.backoffMinutes)
	}
	if f.staleWarnDays != 14 {
		t.Fatalf("expected staleWarnDays=14, got %d", f.staleWarnDays)
	}
}

func TestPrintHelpIncludesSweep(t *testing.T) {
	data := captureHelpOutput()
	if !strings.Contains(data, "sweep") {
		t.Fatalf("help output should include 'sweep', got:\n%s", data)
	}
}

func captureHelpOutput() string {
	origHelp := helpFn
	defer func() { helpFn = origHelp }()

	var out strings.Builder
	helpFn = func() error {
		printHelpString(&out)
		return nil
	}
	_ = Dispatch([]string{"--help"})
	return out.String()
}

func printHelpString(out *strings.Builder) {
	out.WriteString("kernl - multi-agent orchestration runner\n\n")
	out.WriteString("Subcommands:\n")
	out.WriteString("  serve        Start the HTTP API server\n")
	out.WriteString("  doctor       Run system checks\n")
	out.WriteString("  epic         Manage epics\n")
	out.WriteString("  bead         Manage individual beads\n")
	out.WriteString("  sweep        Close epics whose PRs are merged\n")
}

func stringsContain(args []string, target string) bool {
	for _, a := range args {
		if a == target {
			return true
		}
	}
	return false
}

func TestSweepFlagsUnknownFlagFailsLoud(t *testing.T) {
	_, err := parseSweepFlags([]string{"--dryrun"})
	if err == nil || !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Fatalf("unknown sweep flag must fail loud, got: %v", err)
	}
	if !strings.Contains(err.Error(), `did you mean "--dry-run"?`) {
		t.Errorf("expected did-you-mean hint, got: %v", err)
	}
	if exitCode(err) != 2 {
		t.Errorf("unknown flag should be usage error, got exit %d", exitCode(err))
	}
}

func TestSweepFlagsInvalidIntFailsLoud(t *testing.T) {
	_, err := parseSweepFlags([]string{"--failure-threshold", "abc"})
	if err == nil || !strings.Contains(err.Error(), `got "abc"`) {
		t.Fatalf("invalid int must fail loud naming the value, got: %v", err)
	}
	_, err = parseSweepFlags([]string{"--backoff-minutes", "5,x,60"})
	if err == nil || !strings.Contains(err.Error(), "comma-separated integers") {
		t.Fatalf("invalid backoff list must fail loud, got: %v", err)
	}
}

func TestSweepFlagsYesParsed(t *testing.T) {
	f, err := parseSweepFlags([]string{"--yes"})
	if err != nil || !f.yes {
		t.Fatalf("expected yes=true, got %+v err=%v", f, err)
	}
}

// Sweep reads and closes beads, so it needs the repository's own tracker. Both
// entry points constructed bd unconditionally, which against a br repository
// opened a Dolt store that is not there and swept nothing while reporting
// success.
func TestSweepUsesTheRepositorysTracker(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".beads", "beads.db"), []byte("sqlite"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Registry: config.RegistryConfig{Repos: []config.RepoEntry{{Path: repo, MemoryManager: "br"}}}}

	runner, err := defaultSweeperFactory(cfg)
	if err != nil {
		t.Fatalf("defaultSweeperFactory: %v", err)
	}
	if runner == nil {
		t.Fatal("a registered repo must produce a sweeper")
	}
}

// A repository whose tracker cannot be named must stop the sweeper rather than
// have one picked for it.
func TestSweepRefusesARepositoryWithNoResolvableTracker(t *testing.T) {
	cfg := &config.Config{Registry: config.RegistryConfig{Repos: []config.RepoEntry{{Path: t.TempDir()}}}}

	if _, err := defaultSweeperFactory(cfg); err == nil {
		t.Fatal("expected a refusal rather than a default tracker")
	}
}

// A typo in --repo used to reach AutoRouteFromConfig unresolved, matched
// nothing, and fell through to sniffing a directory that does not exist -
// which reported "nothing says which tracker it uses" and sent the operator
// to fix a memoryManager that was never broken. resolveSweepRepoPath must
// refuse before any of that, naming the registered paths instead.
func TestResolveSweepRepoPathUnmatchedFailsWithRegistryListing(t *testing.T) {
	cfg := cfgWithRepos("/home/me/archeion")
	_, err := resolveSweepRepoPath(cfg, "/home/me/clarty-data-workflow", "/elsewhere")
	if err == nil {
		t.Fatal("expected a loud failure for an unregistered --repo")
	}
	if strings.Contains(err.Error(), "says which tracker") {
		t.Fatalf("expected the registry-listing error, got the memoryManager one: %v", err)
	}
	if !strings.Contains(err.Error(), "archeion") {
		t.Errorf("error must name the registered repos so the typo is obvious, got: %v", err)
	}
}

// A bare `kernl sweep` with exactly one repo registered runs against that
// repo - the same rule every other verb applies through resolveRepoEntry.
func TestResolveSweepRepoPathBareInvocationUsesTheSoleRepo(t *testing.T) {
	cfg := cfgWithRepos("/home/me/archeion")
	got, err := resolveSweepRepoPath(cfg, "", "/elsewhere")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/home/me/archeion" {
		t.Fatalf("repoPath = %q, want the only registered repo", got)
	}
}

// A bare `kernl sweep` used to default to ".", which never touched the
// registry - so with several repos registered it silently swept whatever
// directory the operator happened to be standing in. It must now refuse and
// name the choice explicitly, like every other verb without --repo.
func TestResolveSweepRepoPathBareInvocationWithMultipleReposFailsLoud(t *testing.T) {
	cfg := cfgWithRepos("/home/me/archeion", "/home/me/daytrace")
	_, err := resolveSweepRepoPath(cfg, "", "/elsewhere")
	if err == nil {
		t.Fatal("expected a loud failure rather than a silent default to \".\"")
	}
	if !strings.Contains(err.Error(), "--repo") {
		t.Errorf("error must say how to disambiguate, got: %v", err)
	}
}

// sweep reaches the registry through the same resolveRepoEntry as every other
// verb, so it inherits the working-directory default too. Pinned here because
// the two changes that produced this behaviour landed separately and neither
// one could see the other: sweep started calling resolveRepoEntry in one, and
// resolveRepoEntry learned about the working directory in the other.
func TestResolveSweepRepoPathBareInvocationResolvesFromWorkingDir(t *testing.T) {
	cfg := cfgWithRepos("/home/me/archeion", "/home/me/daytrace")
	got, err := resolveSweepRepoPath(cfg, "", "/home/me/daytrace/internal/store")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/home/me/daytrace" {
		t.Fatalf("repoPath = %q, want the repo the working directory sits in", got)
	}
}

// No repos registered at all: sweep must say so rather than fall back to "."
// and report a tracker failure over a directory that was never configured.
func TestResolveSweepRepoPathNoReposRegisteredFailsLoud(t *testing.T) {
	cfg := &config.Config{}
	_, err := resolveSweepRepoPath(cfg, "", "/elsewhere")
	if err == nil {
		t.Fatal("expected a loud failure when nothing is registered")
	}
	if !strings.Contains(err.Error(), "no repos registered") {
		t.Errorf("expected the no-repos-registered error, got: %v", err)
	}
}

// sweepEpicBackend is a named fake (AGENTS.md §4) that answers the two List
// calls sweepBackendAdapter makes: the epics awaiting PR review, and each
// epic's children. It embeds testBackend so only those two behaviours have to
// be spelled out here.
type sweepEpicBackend struct {
	*testBackend
	epics []backend.Bead
}

func (b *sweepEpicBackend) List(filters *backend.BeadListFilters, repoPath string) ([]backend.Bead, error) {
	if filters != nil && filters.Parent != "" {
		return nil, nil
	}
	return b.epics, nil
}

// TestSweepReadsPRURLFromTheDescription pins where the pr_url comes from.
// sweep used to read it from Bead.Metadata, a key nothing in this repository
// writes: Metadata is only decoded back out of the tracker, and neither bd nor
// br can set it. The shipment prompt writes "pr_url: <url>" into the
// description and the shipment exit gate refuses to advance without it there,
// so the description is the only source that is ever populated. Reading the
// other one made sweep skip every epic, which looked like "nothing has merged
// yet" rather than a broken contract.
func TestSweepReadsPRURLFromTheDescription(t *testing.T) {
	const url = "https://github.com/owner/repo/pull/41"
	a := &sweepBackendAdapter{
		b: &sweepEpicBackend{
			testBackend: &testBackend{},
			epics: []backend.Bead{{
				ID:          "arch-pun",
				Type:        "epic",
				State:       "awaiting_pr_review",
				Description: "Some epic body.\n\npr_url: " + url + "\nmerge_outcome: success\n",
			}},
		},
		dir: t.TempDir(),
	}

	epics, err := a.ListEpicsAwaitingPRReview()
	if err != nil {
		t.Fatalf("ListEpicsAwaitingPRReview: %v", err)
	}
	if len(epics) != 1 {
		t.Fatalf("expected 1 epic, got %d", len(epics))
	}
	if epics[0].PRURL != url {
		t.Errorf("PRURL = %q, want %q - sweep must read pr_url from the description the shipment gate enforces, not from a metadata key nothing writes", epics[0].PRURL, url)
	}
}

// An epic that genuinely has no pr_url must still come back empty, so sweep's
// own "in awaiting_pr_review without pr_url" warning keeps firing for the real
// case instead of being masked by a fallback.
func TestSweepPRURLEmptyWhenTheDescriptionHasNone(t *testing.T) {
	a := &sweepBackendAdapter{
		b: &sweepEpicBackend{
			testBackend: &testBackend{},
			epics: []backend.Bead{{
				ID:          "arch-none",
				Type:        "epic",
				State:       "awaiting_pr_review",
				Description: "An epic that never shipped.\n",
			}},
		},
		dir: t.TempDir(),
	}

	epics, err := a.ListEpicsAwaitingPRReview()
	if err != nil {
		t.Fatalf("ListEpicsAwaitingPRReview: %v", err)
	}
	if epics[0].PRURL != "" {
		t.Errorf("PRURL = %q, want empty for an epic whose description carries no pr_url", epics[0].PRURL)
	}
}
