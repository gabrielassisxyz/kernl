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
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/logging"
	"github.com/gabrielassisxyz/kernl/internal/preflight"
)

func runServe(configPath string) error {
	logLevel := os.Getenv("KERNL_LOG_LEVEL")
	logging.Init(logLevel)

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

	port := strconv.Itoa(cfg.Server.Port)
	if port == "0" {
		port = "8080"
	}

	a, err := app.NewApp(cfg)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: creating app: %w", err)
	}

	handler := api.NewRouter(a)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		fmt.Printf("kernl serving — API http://localhost:%s\n", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "error", err)
		os.Exit(1)
	}

	fmt.Println("kernl stopped")
	return nil
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
