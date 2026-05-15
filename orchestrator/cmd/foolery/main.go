package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gastownhall/foolery/internal/api"
	"github.com/gastownhall/foolery/internal/config"
	"github.com/gastownhall/foolery/internal/logging"
)

func main() {
	logLevel := os.Getenv("FOOLERY_LOG_LEVEL")
	logging.Init(logLevel)

	cfg, err := config.Load("foolery.yaml")
	if err != nil {
		slog.Error("FOOLERY DISPATCH FAILURE: failed to load config", "error", err)
		os.Exit(1)
	}

	port := strconv.Itoa(cfg.Server.Port)
	if port == "0" {
		port = "8080"
	}

	handler := api.NewRouter(cfg)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("foolery starting", "addr", srv.Addr)
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

	fmt.Println("foolery stopped")
}