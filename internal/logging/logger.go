package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
)

type ctxKey struct{}

func Init(levelStr string) {
	var level slog.Leveler
	switch levelStr {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// When running in an interactive terminal, use the human-readable
	// pretty handler that filters HTTP noise and shows bead orchestration
	// in color.  In non-interactive mode (pipe, file, systemd) fall back
	// to JSON so logs remain machine-parseable.
	if isInteractiveTerminal() {
		var jsonOut io.Writer
		logDir := os.Getenv("KERNL_JSON_LOG_DIR")
		if logDir == "" {
			logDir = os.Getenv("HOME") + "/.kernl/logs"
		}
		if err := os.MkdirAll(logDir, 0o755); err == nil {
			f, err := os.OpenFile(
				logDir+"/kernl.orchestrator.jsonl",
				os.O_CREATE|os.O_WRONLY|os.O_APPEND,
				0o644,
			)
			if err == nil {
				jsonOut = f
			}
		}
		if jsonOut == nil {
			jsonOut = io.Discard
		}
		handler := NewPrettyTerminalHandler(os.Stderr, jsonOut, level)
		slog.SetDefault(slog.New(handler))
		return
	}

	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))
}

func isInteractiveTerminal() bool {
	stat, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return stat.Mode()&os.ModeCharDevice != 0
}

func CorrelationID(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKey{}).(string); ok {
		return v
	}
	return ""
}

func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

func GenerateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}