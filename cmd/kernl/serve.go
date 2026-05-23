package main

import (
	"context"
	"fmt"
	"log/slog"
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
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
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

func runServe(configPath string, port int) error {
	report := preflight.Run(preflight.Deps{
		LookPath:   preflight.LookPath,
		ConfigPath: configPath,
		GoVersion:  runtime.Version(),
	})
	if !report.AllOK() {
		printReport(report)
		if hardCheckFailed(report) {
			return fmt.Errorf("KERNL DISPATCH FAILURE: preflight checks failed — fix the issues above and retry")
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: loading config %s: %w", configPath, err)
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

	go func() {
		fmt.Printf("kernl serving — API http://localhost:%s\n", portStr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	if cfg.Sweep.AutoIntervalSeconds > 0 {
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
		vaultDBPath := cfg.Vault.Root + "/.kernl-graph.db"
		g, err := graph.Open(ctx, graph.Config{Path: vaultDBPath})
		if err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: open vault graph: %w", err)
		}
		defer g.Close()

		vaultSvc = vault.New(g, cfg.Vault)
		if err := vaultSvc.Start(ctx); err != nil {
			return fmt.Errorf("KERNL DISPATCH FAILURE: vault service start: %w", err)
		}
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

func hardCheckFailed(r *preflight.Report) bool {
	names := []string{"bd", "opencode", "go"}
	for _, name := range names {
		c := r.Check(name)
		if c != nil && !c.OK {
			return true
		}
	}
	return false
}
