package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

type fakeSweepRunner struct {
	ticks int64
}

func (f *fakeSweepRunner) Tick() error {
	atomic.AddInt64(&f.ticks, 1)
	return nil
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

func TestResolveBindHostDefaultsToLoopback(t *testing.T) {
	// The API has no authentication, so the default must never be an address
	// other machines can reach — an unconfigured kernl is a private kernl.
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
