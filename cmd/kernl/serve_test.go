package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

type fakeSweepRunner struct {
	ticks int64
	err   error
}

func (f *fakeSweepRunner) Tick() error {
	atomic.AddInt64(&f.ticks, 1)
	return f.err
}

func (f *fakeSweepRunner) count() int64 {
	return atomic.LoadInt64(&f.ticks)
}

func TestStartAutoTick_TicksAndCancels(t *testing.T) {
	fake := &fakeSweepRunner{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		startAutoTick(ctx, fake, 50*time.Millisecond)
	}()

	time.Sleep(150 * time.Millisecond)
	c := fake.count()
	if c < 2 {
		t.Fatalf("expected >=2 ticks, got %d", c)
	}

	cancel()
	wg.Wait()
}

func TestServeDispatchesAutoTick(t *testing.T) {
	fake := &fakeSweepRunner{}

	origFactory := sweeperFactory
	sweeperFactory = func(_ *config.Config) (sweepRunner, error) {
		return fake, nil
	}
	t.Cleanup(func() { sweeperFactory = origFactory })

	origServe := serveFn
	serveFn = func(configPath string, port int, noOrch bool) error {
		_ = port
		_ = noOrch
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go startAutoTick(ctx, fake, 50*time.Millisecond)

		time.Sleep(150 * time.Millisecond)
		c := fake.count()
		if c < 2 {
			t.Fatalf("expected >=2 auto-ticks, got %d", c)
		}
		cancel()
		return nil
	}
	t.Cleanup(func() { serveFn = origServe })

	if err := Dispatch([]string{"serve"}); err != nil {
		t.Fatalf("dispatch serve failed: %v", err)
	}
}

// A registered repository used to mean index 0 of cfg.Registry.Repos: the
// second and third repositories were never swept, however long the server
// ran. compositeSweeper exists so every registered repository gets a tick.
func TestCompositeSweeperTicksEveryRegisteredRepo(t *testing.T) {
	a := &fakeSweepRunner{}
	b := &fakeSweepRunner{}
	c := &fakeSweepRunner{}
	cs := &compositeSweeper{repos: []*repoSweepRunner{
		{repoPath: "repo-a", sw: a},
		{repoPath: "repo-b", sw: b},
		{repoPath: "repo-c", sw: c},
	}}

	if err := cs.Tick(); err != nil {
		t.Fatalf("Tick() with all repos healthy: %v", err)
	}
	if a.count() != 1 || b.count() != 1 || c.count() != 1 {
		t.Fatalf("expected every repo ticked once, got a=%d b=%d c=%d", a.count(), b.count(), c.count())
	}
}

// The maintainer's decision: a repository failing mid-tick (tracker
// unreachable, gh unauthenticated, path gone) must not stop the others. The
// alternative (abort the whole tick) makes one repository's permanent config
// error a permanent global outage for every other repository too.
func TestCompositeSweeperCarriesOnPastOneFailure(t *testing.T) {
	ok1 := &fakeSweepRunner{}
	failing := &fakeSweepRunner{err: errors.New("tracker unreachable")}
	ok2 := &fakeSweepRunner{}
	cs := &compositeSweeper{repos: []*repoSweepRunner{
		{repoPath: "repo-a", sw: ok1},
		{repoPath: "repo-b", sw: failing},
		{repoPath: "repo-c", sw: ok2},
	}}

	err := cs.Tick()
	if err == nil {
		t.Fatal("expected an aggregated error naming the failing repository")
	}
	if !strings.Contains(err.Error(), "repo-b") {
		t.Fatalf("error must name the failing repository, got: %v", err)
	}
	if ok1.count() != 1 || ok2.count() != 1 {
		t.Fatalf("healthy repos must still be ticked, got a=%d c=%d", ok1.count(), ok2.count())
	}
	if failing.count() != 1 {
		t.Fatalf("failing repo must still be attempted, got %d", failing.count())
	}
}

// A repeated single-error message ("first repo failed") would hide the
// second failure from whoever reads the aggregated error.
func TestCompositeSweeperAggregatedErrorNamesEveryFailure(t *testing.T) {
	failA := &fakeSweepRunner{err: errors.New("gh unauthenticated")}
	ok := &fakeSweepRunner{}
	failC := &fakeSweepRunner{err: errors.New("path gone")}
	cs := &compositeSweeper{repos: []*repoSweepRunner{
		{repoPath: "repo-a", sw: failA},
		{repoPath: "repo-b", sw: ok},
		{repoPath: "repo-c", sw: failC},
	}}

	err := cs.Tick()
	if err == nil {
		t.Fatal("expected an aggregated error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "repo-a") || !strings.Contains(msg, "repo-c") {
		t.Fatalf("error must name every failed repository, not just the first, got: %v", msg)
	}
	if strings.Contains(msg, "repo-b") {
		t.Fatalf("error must not name a repository that succeeded, got: %v", msg)
	}
}

// A registered repository that never resolved a backend (bad memoryManager,
// unresolvable tracker) is not silently dropped from future ticks: it is
// wrapped so its build error is reported every tick, the same as a runtime
// failure, rather than disappearing after the one warning at startup.
func TestCompositeSweeperReportsARepositoryThatNeverBuilt(t *testing.T) {
	cs := &compositeSweeper{repos: []*repoSweepRunner{
		{repoPath: "repo-broken", buildErr: errors.New("no tracker configured")},
	}}

	err := cs.Tick()
	if err == nil {
		t.Fatal("expected the build failure to surface every tick")
	}
	if !strings.Contains(err.Error(), "repo-broken") {
		t.Fatalf("error must name the repository, got: %v", err)
	}
}

// A tick that swept zero repositories (nothing registered) must not read the
// same as a tick that ran and simply found nothing to do: the latter returns
// nil today, so the former must not.
func TestCompositeSweeperEmptyNeverReadsAsFoundNothing(t *testing.T) {
	cs := &compositeSweeper{}

	if err := cs.Tick(); err == nil {
		t.Fatal("a composite with no repositories to sweep must not report success")
	}
}

// cfg.Registry.Repos with zero entries disables auto-sweep entirely (the
// ticker goroutine never starts, per runServe's nil check) rather than
// starting a composite with nothing in it. This preserves that meaning.
func TestDefaultSweeperFactoryZeroRegisteredRepos(t *testing.T) {
	cfg := &config.Config{}

	runner, err := defaultSweeperFactory(cfg)
	if err != nil {
		t.Fatalf("zero registered repos must not be an error, got: %v", err)
	}
	if runner != nil {
		t.Fatalf("zero registered repos must produce no runner, got: %v", runner)
	}
}

// Three registered repositories must each get their own sweeper: the defect
// this fixes was defaultSweeperFactory reading only cfg.Registry.Repos[0].
func TestDefaultSweeperFactoryBuildsOneSweeperPerRepo(t *testing.T) {
	repos := make([]config.RepoEntry, 0, 3)
	for i := 0; i < 3; i++ {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".beads"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".beads", "beads.db"), []byte("sqlite"), 0o644); err != nil {
			t.Fatal(err)
		}
		repos = append(repos, config.RepoEntry{Path: dir, MemoryManager: "br"})
	}
	cfg := &config.Config{Registry: config.RegistryConfig{Repos: repos}}

	runner, err := defaultSweeperFactory(cfg)
	if err != nil {
		t.Fatalf("defaultSweeperFactory: %v", err)
	}
	cs, ok := runner.(*compositeSweeper)
	if !ok {
		t.Fatalf("expected a *compositeSweeper, got %T", runner)
	}
	if len(cs.repos) != 3 {
		t.Fatalf("expected one sweeper per registered repo, got %d", len(cs.repos))
	}
}

func TestResolveBindHostDefaultsToLoopback(t *testing.T) {
	// The API has no authentication, so the default must never be an address
	// other machines can reach - an unconfigured kernl is a private kernl.
	tests := []struct {
		name       string
		configured string
		env        string
		want       string
	}{
		{name: "nothing set", want: "127.0.0.1"},
		{name: "blank config", configured: "   ", want: "127.0.0.1"},
		{name: "config chooses", configured: "0.0.0.0", want: "0.0.0.0"},
		{name: "env overrides config", configured: "127.0.0.1", env: "0.0.0.0", want: "0.0.0.0"},
		{name: "env is trimmed", env: "  192.168.1.5  ", want: "192.168.1.5"},
		{name: "blank env falls back to config", configured: "10.0.0.2", env: "", want: "10.0.0.2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveBindHost(tt.configured, tt.env); got != tt.want {
				t.Errorf("resolveBindHost(%q, %q) = %q, want %q", tt.configured, tt.env, got, tt.want)
			}
		})
	}
}
