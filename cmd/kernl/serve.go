package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
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

func defaultSweeperFactory(cfg *config.Config) (sweepRunner, error) {
	if len(cfg.Registry.Repos) == 0 {
		return nil, nil
	}
	repoPath := cfg.Registry.Repos[0].Path
	b := backend.NewBdCliBackend(repoPath)
	adapter := &sweepBackendAdapter{b: b, dir: repoPath}
	ghAdapter := &ghCliAdapter{}
	sweepCfg := sweep.Config{
		PRStaleWarnDays:  cfg.Sweep.PRStaleWarnDays,
		FailureThreshold: cfg.Sweep.FailureThreshold,
		BackoffMinutes:   cfg.Sweep.BackoffMinutes,
	}
	return sweep.New(adapter, ghAdapter, sweepCfg), nil
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
		return fmt.Errorf("KERNL DISPATCH FAILURE: preflight checks failed — fix the issues above and retry")
	}

	cfg, err := loadCLIConfig(configPath)
	if err != nil {
		return err
	}

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

	srv := &http.Server{
		Addr:         ":" + portStr,
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
		fmt.Printf("kernl serving — API http://localhost:%s\n", portStr)
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

	// Vault watcher lifecycle — started only when vault.root is configured.
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

	// Inbox DA pre-classifier — assigns a suggested action to each pending
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
				AutoPrep:  cfg.Inbox.AutoPrep,
				VaultRoot: cfg.Vault.Root,
				DASubdir:  cfg.Inbox.DASubdir,
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
