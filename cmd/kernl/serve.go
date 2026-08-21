package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/api"
	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/inbox"
	"github.com/gabrielassisxyz/kernl/internal/preflight"
	"github.com/gabrielassisxyz/kernl/internal/sweep"
	"github.com/gabrielassisxyz/kernl/internal/vault"
)

type sweepRunner interface {
	Tick() error
}

var sweeperFactory = defaultSweeperFactory

// repoSweepRunner is one registered repository's sweeper, named by its repo
// path so a composite tick can report exactly which repository failed. A
// repository whose backend never resolved (bad memoryManager, unresolvable
// tracker) carries buildErr instead of a sw: Tick() returns that same error
// every time, so a repository that never came up is reported every tick
// alongside one that fails while running, instead of disappearing after the
// one warning logged at startup.
type repoSweepRunner struct {
	repoPath string
	sw       sweepRunner
	buildErr error
}

func (r *repoSweepRunner) Tick() error {
	if r.buildErr != nil {
		return r.buildErr
	}
	return r.sw.Tick()
}

// compositeSweeper sweeps every registered repository per tick, in place of
// the single sweeper defaultSweeperFactory used to build from
// cfg.Registry.Repos[0] alone. internal/sweep.Sweeper's contract stays one
// repository per sweeper; this drives N of those together.
type compositeSweeper struct {
	repos []*repoSweepRunner
}

// Tick sweeps every repository and carries on past a failing one: the
// alternative (abort the whole tick on the first failure) was rejected,
// because a repository whose path was removed fails forever, and aborting
// would turn that one repository's permanent config error into a permanent
// outage of automatic sweeping for every other registered repository too.
//
// A composite with nothing to sweep is refused rather than returned as
// success: a tick that swept zero repositories must not read the same as a
// tick that ran and found nothing to do (the latter returns nil, same as
// today).
func (c *compositeSweeper) Tick() error {
	if len(c.repos) == 0 {
		return fmt.Errorf("KERNL DISPATCH FAILURE: sweep tick had no registered repository to sweep")
	}

	var failedRepos []string
	var errs []error
	for _, r := range c.repos {
		if err := r.Tick(); err != nil {
			// Logged every tick, with no de-duplication or suppression: a
			// repeated error every interval is honest reporting of a
			// repository that is still broken.
			slog.Error("sweep tick failed for repository", "repo", r.repoPath, "error", err)
			failedRepos = append(failedRepos, r.repoPath)
			errs = append(errs, fmt.Errorf("%s: %w", r.repoPath, err))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("KERNL DISPATCH FAILURE: sweep tick failed for %d of %d registered repositories (%s): %w",
		len(failedRepos), len(c.repos), strings.Join(failedRepos, ", "), errors.Join(errs...))
}

func defaultSweeperFactory(cfg *config.Config) (sweepRunner, error) {
	if len(cfg.Registry.Repos) == 0 {
		return nil, nil
	}

	sweepCfg := sweep.Config{
		PRStaleWarnDays:  cfg.Sweep.PRStaleWarnDays,
		FailureThreshold: cfg.Sweep.FailureThreshold,
		BackoffMinutes:   cfg.Sweep.BackoffMinutes,
	}

	runners := make([]*repoSweepRunner, 0, len(cfg.Registry.Repos))
	var buildErrs []error
	for _, repo := range cfg.Registry.Repos {
		sw, err := buildRepoSweeper(cfg, repo.Path, sweepCfg)
		if err != nil {
			buildErrs = append(buildErrs, fmt.Errorf("%s: %w", repo.Path, err))
			runners = append(runners, &repoSweepRunner{repoPath: repo.Path, buildErr: err})
			continue
		}
		runners = append(runners, &repoSweepRunner{repoPath: repo.Path, sw: sw})
	}

	// Every registered repository failing to resolve a backend leaves
	// nothing to sweep at all, which stays a refusal at startup, same as a
	// single misconfigured repo always was. One bad repo among several
	// healthy ones is a different case: it must not take the others down
	// with it, so it is wrapped instead and reported every tick.
	if len(buildErrs) == len(cfg.Registry.Repos) {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: no registered repository resolved a sweeper: %w", errors.Join(buildErrs...))
	}

	return &compositeSweeper{repos: runners}, nil
}

func buildRepoSweeper(cfg *config.Config, repoPath string, sweepCfg sweep.Config) (sweepRunner, error) {
	b, err := backend.AutoRouteFromConfig(cfg, repoPath)
	if err != nil {
		return nil, err
	}
	adapter := &sweepBackendAdapter{b: b, dir: repoPath}
	ghAdapter := &ghCliAdapter{}
	return sweep.New(adapter, ghAdapter, sweepCfg), nil
}

// resolveBindHost decides which interface the server binds. KERNL_HOST wins so
// a container can open the bind without editing the mounted config; otherwise
// the configured host applies, and the fallback is loopback. Defaulting to
// loopback is the point: the API has no authentication, so "reachable from the
// network" has to be something a person chose, not something they inherited.
func resolveBindHost(configured, env string) string {
	if h := strings.TrimSpace(env); h != "" {
		return h
	}
	if h := strings.TrimSpace(configured); h != "" {
		return h
	}
	return "127.0.0.1"
}

// bannerConfigPath resolves the config file to an absolute path for the
// startup banner. Nothing else in a running server says which config it read,
// and the default is the bare relative "kernl.yaml" - meaningless from
// anywhere but the working directory the process happened to have. On a host
// keeping more than one config, an edit can then land for days in a file the
// server never opens. An empty path prints nothing: a "config:" label with
// nothing after it answers the question worse than silence.
func bannerConfigPath(configPath string) string {
	if strings.TrimSpace(configPath) == "" {
		return ""
	}
	abs, err := filepath.Abs(configPath)
	if err != nil {
		// Still worth naming. The job is to identify the file, and the raw
		// value identifies it better than silence does.
		return configPath
	}
	return abs
}

// printGraphLockHint writes the multi-line way out of a refused lock to
// stderr, following the precedent the busy-port failure below already sets:
// the error value stays one line, because that is what a log line wants,
// while a human staring at a dead process needs the next command to type.
//
// The pid is presented as what the lock file recorded, never as a claim about
// who holds the lock right now - the owner can exit and a third process can
// take over between the flock that failed and the read of that file.
func printGraphLockHint(locked app.GraphLockedError) {
	fmt.Fprintf(os.Stderr, "\nAnother kernl serve owns %s.\n", locked.Path)
	if locked.PID > 0 {
		fmt.Fprintf(os.Stderr, "The lock file last recorded pid %d:\n    ps -p %d -o pid,lstart,cmd\n",
			locked.PID, locked.PID)
	}
	fmt.Fprintf(os.Stderr,
		"Stop that process, or run this instance against its own copy of the vault -\n"+
			"see AGENTS.md section 10, \"running a second instance\".\n\n")
}

func runServe(configPath string, port int, noOrchestrator bool) error {
	if noOrchestrator {
		slog.Info("starting in GUI-only mode (orchestrator disabled): bd is not required")
	}
	report := preflight.Run(preflight.Deps{
		LookPath:     preflight.LookPath,
		ConfigPath:   configPath,
		GoVersion:    runtime.Version(),
		Orchestrator: !noOrchestrator,
	})
	if !report.AllOK() {
		printReport(report)
	}
	if report.RequiredFailed() {
		return fmt.Errorf("KERNL DISPATCH FAILURE: preflight checks failed - fix the issues above and retry")
	}

	cfg, err := loadCLIConfig(configPath)
	if err != nil {
		return err
	}

	// Before NewApp, which opens and migrates the graph database: a server
	// that is going to be refused must not have touched the database first.
	// The lock lives here rather than in NewApp because bookmark, plan, repo
	// select and epic run all build an App too, and none of them may be
	// refused while a server is up - one process DRIVES the database, it is
	// not the only one allowed to touch it.
	releaseGraphLock, err := app.AcquireGraphLock(cfg)
	if err != nil {
		var locked app.GraphLockedError
		if errors.As(err, &locked) {
			printGraphLockHint(locked)
		}
		return err
	}
	// Deferred immediately, so the early returns below (a busy port, a vault
	// service that fails to start) cannot leak the lock. The two os.Exit(1)
	// calls further down skip this, which is fine and deliberate: the kernel
	// drops the flock when the process dies.
	defer func() { _ = releaseGraphLock() }()

	srvPort := cfg.Server.Port
	if port > 0 {
		srvPort = port
	} else if srvPort == 0 {
		srvPort = 8080
	}
	portStr := strconv.Itoa(srvPort)

	a, err := app.NewApp(cfg)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: creating app: %w", err)
	}
	a.ConfigPath = configPath

	handler := api.NewRouter(a)

	host := resolveBindHost(cfg.Server.Host, os.Getenv("KERNL_HOST"))

	srv := &http.Server{
		Addr:         net.JoinHostPort(host, portStr),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Bind BEFORE announcing. ListenAndServe binds and serves in one call, so
	// printing "serving" ahead of it announced a success that had not happened
	// yet: a port already in use printed the banner and then died. Take the
	// listener first, and the banner can only be true.
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		// The hint goes to stderr rather than into the error value: an error
		// string is one line, but a human staring at a dead process needs the
		// next command to type.
		fmt.Fprintf(os.Stderr,
			"\nAnother kernl is probably already listening on %s. Find it with:\n"+
				"    ss -ltnp 'sport = :%s'\n"+
				"then stop it, or serve on another port with --port.\n\n", portStr, portStr)
		return fmt.Errorf("cannot listen on port %s: %w", portStr, err)
	}

	go func() {
		fmt.Printf("kernl serving - API http://%s\n", srv.Addr)
		if host == "0.0.0.0" || host == "::" {
			fmt.Printf("  reachable from the network, and the API has no authentication.\n")
		}
		if p := bannerConfigPath(configPath); p != "" {
			fmt.Printf("  config: %s\n", p)
		}
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	if !noOrchestrator && cfg.Sweep.AutoIntervalSeconds > 0 {
		sw, err := sweeperFactory(cfg)
		if err != nil {
			slog.Warn("sweep auto-tick disabled", "error", err)
		} else if sw != nil {
			go startAutoTick(ctx, sw, time.Duration(cfg.Sweep.AutoIntervalSeconds)*time.Second)
		}
	}

	// Vault watcher lifecycle - started only when vault.root is configured.
	vault.ApplyDefaults(&cfg.Vault)
	var vaultSvc *vault.Service
	if cfg.Vault.Enabled() {
		if err := vault.Validate(cfg.Vault); err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: vault config: %w", err)
		}
		// Reuse the App's graph handle so the watcher writes into the same
		// database the HTTP API serves (single source of truth, one handle per
		// process). App owns the handle and closes it on shutdown.
		vaultSvc = vault.New(a.Graph, cfg.Vault)
		if err := vaultSvc.Start(ctx); err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: vault service start: %w", err)
		}
	}

	// Inbox DA pre-classifier - assigns a suggested action to each pending
	// capture. Started only when an LLM provider is configured (same gate as the
	// chat API), so it never spends tokens unless the user opts in.
	if cfg.LLM.IsSet() {
		llm, lerr := chat.NewProviderFromConfig(chat.LLMProviderConfig{
			Provider: cfg.LLM.Provider,
			APIKey:   cfg.LLM.APIKey,
			Model:    cfg.LLM.Model,
			Endpoint: cfg.LLM.Endpoint,
		})
		if lerr != nil {
			slog.Warn("inbox classifier disabled", "error", lerr)
		} else {
			// The loop always starts (given an LLM); it gates each tick on the
			// live switch App exposes, so the toggle can pause it at runtime.
			go inbox.NewClassifier(a.Graph, llm, inbox.ClassifierOptions{
				AutoPrep:   cfg.Inbox.AutoPrep,
				VaultRoot:  cfg.Vault.Root,
				DASubdir:   cfg.Inbox.DASubdir,
				LinkBudget: cfg.Planning.LinkBudgetOrDefault(),
			}).Run(ctx, a.AutoClassifyEnabled)
			slog.Info("inbox classifier started", "autoPrep", cfg.Inbox.AutoPrep, "autoClassify", a.AutoClassifyEnabled())
		}
	} else {
		slog.Warn("inbox classifier disabled: no llm provider configured (set llm.provider in kernl.yaml); DA chat, ingest, and note AI are also unavailable")
	}

	<-ctx.Done()

	if vaultSvc != nil {
		vaultSvc.Stop()
	}

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
		os.Exit(1)
	}

	fmt.Println("kernl stopped")
	return nil
}

func startAutoTick(ctx context.Context, sw sweepRunner, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = sw.Tick()
		}
	}
}
