package session

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatchdog_DefaultTimeout(t *testing.T) {
	w := NewWatchdog(0, nil)
	if w.timeout != DefaultWatchdogTimeout {
		t.Errorf("expected default timeout %v, got %v", DefaultWatchdogTimeout, w.timeout)
	}
}

func TestWatchdog_CustomTimeout(t *testing.T) {
	w := NewWatchdog(5*time.Minute, nil)
	if w.timeout != 5*time.Minute {
		t.Errorf("expected 5m timeout, got %v", w.timeout)
	}
}

func TestWatchdog_DisabledWhenNegative(t *testing.T) {
	w := NewWatchdog(-1, nil)
	if !w.disabled {
		t.Error("negative timeout should disable watchdog")
	}
}

func TestWatchdog_DisabledStartNoOp(t *testing.T) {
	w := NewWatchdog(-1, nil)
	ctx := context.Background()
	resultCtx := w.Start(ctx, nil)
	if resultCtx != ctx {
		t.Error("disabled watchdog should return the original context")
	}
}

func TestWatchdog_DisabledResetNoOp(t *testing.T) {
	w := NewWatchdog(-1, nil)
	w.Reset()
	if !w.disabled {
		t.Error("disabled watchdog should remain disabled after reset")
	}
}

func TestWatchdog_FiresAfterTimeout(t *testing.T) {
	var killed atomic.Int32
	w := NewWatchdog(100*time.Millisecond, func(pid int) {
		killed.Add(1)
	})
	w.SetPID(0)

	ctx := context.Background()
	w.Start(ctx, nil)
	defer w.Stop()

	time.Sleep(300 * time.Millisecond)

	if killed.Load() != 1 {
		t.Errorf("expected watchdog to fire once, got %d", killed.Load())
	}
}

func TestWatchdog_ResetPreventsTimeout(t *testing.T) {
	var killed atomic.Int32
	w := NewWatchdog(200*time.Millisecond, func(pid int) {
		killed.Add(1)
	})
	w.SetPID(0)

	ctx := context.Background()
	w.Start(ctx, nil)
	defer w.Stop()

	for i := 0; i < 5; i++ {
		time.Sleep(100 * time.Millisecond)
		w.Reset()
	}

	if killed.Load() != 0 {
		t.Errorf("watchdog should not have fired with resets, got %d", killed.Load())
	}
}

func TestWatchdog_StopCancelsWatchdog(t *testing.T) {
	var killed atomic.Int32
	w := NewWatchdog(100*time.Millisecond, func(pid int) {
		killed.Add(1)
	})
	w.SetPID(0)

	ctx := context.Background()
	w.Start(ctx, nil)
	w.Stop()

	time.Sleep(300 * time.Millisecond)

	if killed.Load() != 0 {
		t.Error("stopped watchdog should not fire")
	}
}

func TestWatchdog_SetLastEventType(t *testing.T) {
	w := NewWatchdog(100*time.Millisecond, nil)
	w.SetLastEventType("result")

	w.mu.Lock()
	evt := w.lastEvent
	w.mu.Unlock()

	if evt != "result" {
		t.Errorf("expected lastEventType 'result', got %q", evt)
	}
}

func TestWatchdog_FiresEvenAfterResultObserved(t *testing.T) {
	var killed atomic.Int32
	w := NewWatchdog(100*time.Millisecond, func(pid int) {
		killed.Add(1)
	})
	w.SetPID(0)

	runtime := NewSessionRuntime("bead-1", "/repo")
	runtime.MarkResultObserved("turn_ended")

	ctx := context.Background()
	w.Start(ctx, runtime)
	defer w.Stop()

	time.Sleep(300 * time.Millisecond)

	if killed.Load() != 1 {
		t.Errorf("watchdog must fire even after resultObserved, got %d firings", killed.Load())
	}
}

func TestWatchdog_ResetDuringWatchdogGoroutine(t *testing.T) {
	var killed atomic.Int32
	w := NewWatchdog(150*time.Millisecond, func(pid int) {
		killed.Add(1)
	})
	w.SetPID(0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w.Start(ctx, nil)

	w.Reset()

	w.mu.Lock()
	armedAt := w.armedAt
	w.mu.Unlock()

	if armedAt.IsZero() {
		t.Error("armedAt should be set after reset")
	}
}

func TestWatchdog_SetPID(t *testing.T) {
	w := NewWatchdog(100*time.Millisecond, nil)
	w.SetPID(12345)

	w.mu.Lock()
	pid := w.pid
	w.mu.Unlock()

	if pid != 12345 {
		t.Errorf("expected pid 12345, got %d", pid)
	}
}

func TestTerminateProcessGroup_InvalidPID(t *testing.T) {
	TerminateProcessGroup(0, "test", DefaultKillDelay)
	TerminateProcessGroup(-1, "test", DefaultKillDelay)
}

func TestKillProcessGroup_InvalidPID(t *testing.T) {
	KillProcessGroup(0)
	KillProcessGroup(-1)
}

func TestFmtIfNil(t *testing.T) {
	if fmtIfNil("") != "null" {
		t.Error("empty string should return 'null'")
	}
	if fmtIfNil("result") != "result" {
		t.Error("non-empty string should be returned as-is")
	}
}

func TestWatchdog_ConcurrentResetAndStop(t *testing.T) {
	w := NewWatchdog(50*time.Millisecond, func(pid int) {})

	ctx := context.Background()
	w.Start(ctx, nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.Reset()
		}()
	}
	wg.Wait()

	w.Stop()
}

func TestWatchdog_MultipleStopsIdempotent(t *testing.T) {
	w := NewWatchdog(50*time.Millisecond, func(pid int) {})

	ctx := context.Background()
	w.Start(ctx, nil)

	w.Stop()
	w.Stop()
	w.Stop()
}

func TestWatchdog_KillDelay(t *testing.T) {
	w := NewWatchdog(100*time.Millisecond, nil)
	if w.killDelay != DefaultKillDelay {
		t.Errorf("expected default kill delay %v, got %v", DefaultKillDelay, w.killDelay)
	}
}

func TestWatchdog_ContextCancellation(t *testing.T) {
	var killed atomic.Int32
	w := NewWatchdog(500*time.Millisecond, func(pid int) {
		killed.Add(1)
	})

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx, nil)

	cancel()
	time.Sleep(100 * time.Millisecond)

	if killed.Load() != 0 {
		t.Error("context cancellation should prevent watchdog from firing")
	}
}
