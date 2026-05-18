package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// prettyTerminalHandler prints human-readable bead orchestration lines to the
// terminal while delegating JSON structured output to a secondary writer
// (typically a rotating file).
type prettyTerminalHandler struct {
	w           io.Writer
	level       slog.Leveler
	jsonHandler slog.Handler
}

func NewPrettyTerminalHandler(w io.Writer, jsonOut io.Writer, level slog.Leveler) slog.Handler {
	return &prettyTerminalHandler{
		w:           w,
		level:       level,
		jsonHandler: slog.NewJSONHandler(jsonOut, &slog.HandlerOptions{Level: level}),
	}
}

func (h *prettyTerminalHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *prettyTerminalHandler) Handle(_ context.Context, r slog.Record) error {
	// Always emit JSON to the secondary writer (disk, rotation, etc.)
	_ = h.jsonHandler.Handle(context.Background(), r)

	if r.Level < h.level.Level() {
		return nil
	}

	msg := r.Message

	// Suppress noisy HTTP request logs in the terminal
	if strings.Contains(msg, "request method=") ||
		strings.Contains(msg, "slow request route=") ||
		strings.Contains(msg, "agent log files opened") {
		return nil
	}

	// Pretty-print DRIVE_TRACE bead orchestration lines
	if strings.Contains(msg, "DRIVE_TRACE") {
		line := h.formatBeadLine(r)
		if line != "" {
			_, _ = h.w.Write([]byte(line + "\n"))
		}
		return nil
	}

	// Everything else stays JSON (or could be filtered further)
	// For now, silent for non-DRIVE_TRACE to keep terminal clean
	return nil
}

func (h *prettyTerminalHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &prettyTerminalHandler{
		w:           h.w,
		level:       h.level,
		jsonHandler: h.jsonHandler.WithAttrs(attrs),
	}
}

func (h *prettyTerminalHandler) WithGroup(name string) slog.Handler {
	return &prettyTerminalHandler{
		w:           h.w,
		level:       h.level,
		jsonHandler: h.jsonHandler.WithGroup(name),
	}
}

func (h *prettyTerminalHandler) formatBeadLine(r slog.Record) string {
	attrs := make(map[string]string)
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()
		return true
	})

	bead := attrs["bead"]
	if bead == "" {
		bead = attrs["beadID"]
	}
	if bead == "" {
		return ""
	}

	state := attrs["state"]
	iter := attrs["iter"]
	profile := attrs["profile"]
	claimable := attrs["claimable"]
	agent := attrs["agent"]
	owner := attrs["owner"]
	from := attrs["from"]
	fromState := attrs["fromState"]
	to := attrs["to"]
	toState := attrs["toState"]
	resFinalState := attrs["resFinalState"]
	errStr := attrs["err"]

	timestamp := r.Time.Format("15:04")

	switch {
	case strings.Contains(r.Message, "iter top"):
		return color("gray", "[%s] %s → loop %s | profile=%s | state=%s", timestamp, bead, iter, profile, state)

	case strings.Contains(r.Message, "pre-claim"):
		return color("gray", "[%s] %s → claim check | owner=%s | claimable=%s", timestamp, bead, owner, claimable)

	case strings.Contains(r.Message, "claimed"):
		fromS := firstNonEmpty(from, fromState)
		toS := firstNonEmpty(to, toState)
		return color("green", "[%s] %s → claimed: %s → %s", timestamp, bead, fromS, toS)

	case strings.Contains(r.Message, "spawn"):
		return color("cyan", "[%s] %s → spawn agent=%s", timestamp, bead, agent)

	case strings.Contains(r.Message, "post-spawn ok"):
		return color("cyan", "[%s] %s ← agent done | next=%s", timestamp, bead, resFinalState)

	case strings.Contains(r.Message, "return terminal"):
		return color("green", "[%s] %s ✓ TERMINAL (state=%s)", timestamp, bead, state)

	case strings.Contains(r.Message, "return human-gate"):
		return color("yellow", "[%s] %s ⚡ HUMAN GATE (state=%s, owner=%s)", timestamp, bead, state, owner)

	case strings.Contains(r.Message, "return blocked"):
		return color("red", "[%s] %s ✗ BLOCKED", timestamp, bead)

	case strings.Contains(r.Message, "return agent-err"):
		return color("red", "[%s] %s ✗ AGENT ERROR: %s", timestamp, bead, errStr)

	case strings.Contains(r.Message, "return agent-not-success"):
		return color("red", "[%s] %s ✗ AGENT FAILED | finalState=%s", timestamp, bead, resFinalState)

	default:
		return ""
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return "?"
}

// ANSI color helpers — no-op if stdout is not a tty
var colorEnabled = os.Getenv("FORCE_COLOR") != "" || func() bool {
	stat, _ := os.Stdout.Stat()
	return stat.Mode()&os.ModeCharDevice != 0
}()

func color(name string, format string, args ...any) string {
	if !colorEnabled {
		return sprintSafe(format, args...)
	}
	codes := map[string]string{
		"gray":   "\x1b[38;5;240m",
		"green":  "\x1b[32m",
		"cyan":   "\x1b[36m",
		"yellow": "\x1b[33m",
		"red":    "\x1b[31m",
		"reset":  "\x1b[0m",
	}
	return codes[name] + sprintSafe(format, args...) + codes["reset"]
}

func sprintSafe(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

// PrettyWriter returns a writer for the JSON sidecar (creates directory if needed).
// Callers pass this as jsonOut to NewPrettyTerminalHandler.
func PrettyInit(levelStr string) {
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
}
