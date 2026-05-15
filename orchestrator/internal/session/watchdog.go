package session

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const (
	DefaultWatchdogTimeout = 10 * time.Minute
	DefaultKillDelay       = 5 * time.Second
)

type Watchdog struct {
	timeout   time.Duration
	killDelay time.Duration
	mu        sync.Mutex
	armedAt   time.Time
	timer     *time.Timer
	pid       int
	onKill    func(pid int)
	lastEvent string
	disabled  bool
	stopped   bool
	cancel    context.CancelFunc
}

func NewWatchdog(timeout time.Duration, onKill func(pid int)) *Watchdog {
	disabled := false
	if timeout < 0 {
		disabled = true
		timeout = 0
	}
	if timeout == 0 && !disabled {
		timeout = DefaultWatchdogTimeout
	}
	return &Watchdog{
		timeout:   timeout,
		onKill:    onKill,
		disabled:  disabled,
		killDelay: DefaultKillDelay,
	}
}

func (w *Watchdog) SetPID(pid int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pid = pid
}

func (w *Watchdog) SetLastEventType(evtType string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastEvent = evtType
}

func (w *Watchdog) Start(ctx context.Context, runtime *SessionRuntime) context.Context {
	if w.disabled {
		return ctx
	}

	ctx, cancel := context.WithCancel(ctx)
	w.mu.Lock()
	w.cancel = cancel
	w.stopped = false
	w.armedAt = time.Now()
	w.timer = time.AfterFunc(w.timeout, func() { w.fire(cancel) })
	w.mu.Unlock()

	return ctx
}

func (w *Watchdog) fire(cancel context.CancelFunc) {
	w.mu.Lock()
	pid := w.pid
	lastEventType := w.lastEvent
	armedAt := w.armedAt
	timeout := w.timeout
	w.timer = nil
	stopped := w.stopped
	w.mu.Unlock()

	if stopped {
		return
	}

	msSinceLastEvent := time.Since(armedAt).Milliseconds()

	slog.Warn("[terminal-manager] [watchdog] timeout_fired",
		"pid", pid,
		"timeoutMs", timeout.Milliseconds(),
		"msSinceLastStdout", msSinceLastEvent,
		"lastEventType", fmtIfNil(lastEventType),
		"reason", "timeout",
	)

	TerminateProcessGroup(pid, "watchdog_timeout", w.killDelay)

	if w.onKill != nil {
		w.onKill(pid)
	}
	cancel()
}

func (w *Watchdog) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.disabled || w.stopped {
		return
	}
	if w.timer != nil {
		w.timer.Stop()
	}
	w.armedAt = time.Now()
	w.timer = time.AfterFunc(w.timeout, func() {
		w.mu.Lock()
		cancel := w.cancel
		w.mu.Unlock()
		if cancel != nil {
			w.fire(cancel)
		}
	})
}

func (w *Watchdog) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stopped = true
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
}

func fmtIfNil(s string) string {
	if s == "" {
		return "null"
	}
	return s
}

func TerminateProcessGroup(pid int, reason string, killDelay time.Duration) {
	if pid <= 0 {
		slog.Warn("[terminal-manager] [terminate-process-group] skipping: no valid pid",
			"pid", pid, "reason", reason, "delayMs", killDelay.Milliseconds())
		return
	}

	slog.Warn("[terminal-manager] [terminate-process-group]",
		"pid", pid,
		"reason", reason,
		"signal", "SIGTERM",
		"delayMs", killDelay.Milliseconds(),
	)

	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		slog.Debug("[terminal-manager] [terminate-process-group] SIGTERM to process group failed, falling back to direct kill",
			"pid", pid, "error", err)
		proc, _ := os.FindProcess(pid)
		_ = proc.Signal(syscall.SIGTERM)
	}

	go func() {
		time.Sleep(killDelay)
		slog.Warn("[terminal-manager] [terminate-process-group]",
			"pid", pid,
			"reason", reason,
			"signal", "SIGKILL",
			"delayMs", killDelay.Milliseconds(),
		)

		if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
			slog.Debug("[terminal-manager] [terminate-process-group] SIGKILL to process group failed, falling back to direct kill",
				"pid", pid, "error", err)
			proc, _ := os.FindProcess(pid)
			_ = proc.Signal(syscall.SIGKILL)
		}
	}()
}

func KillProcessGroup(pid int) {
	if pid <= 0 {
		return
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		proc, _ := os.FindProcess(pid)
		_ = proc.Signal(syscall.SIGKILL)
	}
}

func TerminateCmd(cmd *exec.Cmd, reason string, killDelay time.Duration) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	TerminateProcessGroup(cmd.Process.Pid, reason, killDelay)
}