package vault

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
	"github.com/gabrielassisxyz/kernl/internal/vault/watcher"
)

// Service manages the vault watcher lifecycle: cold-start reconciliation,
// live fsnotify event routing, periodic FlushExpired, and optional rescan.
// Construct with New, then call Start(ctx) and Stop().
type Service struct {
	g   *graph.Graph
	cfg config.VaultConfig

	w   *watcher.Watcher
	r   *reconcile.Reconciler

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a Service for the given graph and vault config.
// cfg must already have defaults applied (ApplyDefaults) and be validated
// (Validate). It is a programming error to call New with an empty Root.
func New(g *graph.Graph, cfg config.VaultConfig) *Service {
	return &Service{g: g, cfg: cfg}
}

// Start runs the vault lifecycle:
//  1. Constructs the reconciler and sets its delete window.
//  2. Runs ColdStart (full vault → graph diff) before live watching.
//  3. Constructs and starts the watcher.
//  4. Spawns a goroutine that routes watcher events to the reconciler.
//  5. Spawns a ticker goroutine that calls FlushExpired at moveWindow cadence.
//  6. If RescanIntervalSec > 0, spawns a periodic rescan goroutine.
//
// Start fails fast with a clear error when the vault root is missing or not a
// directory (even though Validate already caught this — belt-and-suspenders).
func (s *Service) Start(ctx context.Context) error {
	if err := Validate(s.cfg); err != nil {
		return fmt.Errorf("vault service start: %w", err)
	}

	root := s.cfg.Root
	coalWindow := coalesceWindow(s.cfg)
	mvWindow := moveWindow(s.cfg)

	slog.Info("vault: starting",
		"root", root,
		"coalesce_window_ms", s.cfg.CoalesceWindowMs,
		"move_window_ms", s.cfg.MoveWindowMs,
		"rescan_interval_sec", s.cfg.RescanIntervalSec,
	)

	// 1. Reconciler.
	s.r = reconcile.New(s.g, root)
	s.r.SetDeleteWindow(mvWindow)

	// 2. Cold-start reconciliation before live watching.
	if err := s.r.ColdStart(ctx); err != nil {
		return fmt.Errorf("vault cold-start: %w", err)
	}
	coldStats := s.r.Stats()
	slog.Info("vault: cold-start complete",
		"events_processed", coldStats.EventsProcessed,
		"notes_created", coldStats.NotesCreated,
		"tombstoned", coldStats.NotesTombstoned,
		"revived", coldStats.NotesRevived,
	)

	// 3. Watcher.
	w, err := watcher.New(root, watcher.Config{CoalesceWindow: coalWindow})
	if err != nil {
		return fmt.Errorf("vault: create watcher: %w", err)
	}
	s.w = w

	innerCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	if err := s.w.Start(innerCtx); err != nil {
		cancel()
		return fmt.Errorf("vault: start watcher: %w", err)
	}

	// 4. Event router goroutine.
	s.wg.Add(1)
	go s.routeEvents(innerCtx)

	// 5. FlushExpired ticker goroutine.
	s.wg.Add(1)
	go s.flushTicker(innerCtx, mvWindow)

	// 6. Optional periodic rescan.
	if s.cfg.RescanIntervalSec > 0 {
		s.wg.Add(1)
		go s.rescanTicker(innerCtx, time.Duration(s.cfg.RescanIntervalSec)*time.Second)
	}

	slog.Info("vault: live watch started", "root", root)
	return nil
}

// Stop shuts down the watcher and waits for all goroutines to exit.
// It is safe to call Stop multiple times.
func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.w != nil {
		s.w.Stop()
	}
	s.wg.Wait()
	slog.Info("vault: stopped")
}

// routeEvents reads from the watcher's event channel and dispatches to the
// reconciler. Handler errors are logged but do not terminate the loop.
//
// On Linux, inotify often coalesces a file create+write into a single KindChange
// event. When OnChange returns graph.ErrNotFound (node not yet in the graph),
// the router retries the path as OnCreate so the note is properly ingested.
func (s *Service) routeEvents(ctx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-s.w.Events():
			if !ok {
				return
			}
			var err error
			switch ev.Kind {
			case watcher.KindCreate, watcher.KindMoveCandidate:
				err = s.r.OnCreate(ctx, ev.Path)
			case watcher.KindChange:
				err = s.r.OnChange(ctx, ev.Path)
				if errors.Is(err, graph.ErrNotFound) {
					// Node not in graph yet — this create+write was coalesced;
					// treat it as a create.
					err = s.r.OnCreate(ctx, ev.Path)
				}
			case watcher.KindDelete:
				err = s.r.OnDelete(ctx, ev.Path)
			}
			if err != nil {
				slog.Error("vault: reconcile event error",
					"kind", ev.Kind,
					"path", ev.Path,
					"error", err,
				)
			}
		}
	}
}

// flushTicker calls FlushExpired at the given cadence to tombstone deletes
// whose move window has elapsed.
func (s *Service) flushTicker(ctx context.Context, cadence time.Duration) {
	defer s.wg.Done()
	t := time.NewTicker(cadence)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n, err := s.r.FlushExpired(ctx)
			if err != nil {
				slog.Error("vault: FlushExpired error", "error", err)
				continue
			}
			if n > 0 {
				slog.Info("vault: tombstoned expired deletes", "count", n)
			}
		}
	}
}

// rescanTicker re-runs ColdStart at the given interval as a safety net for
// missed fsnotify events (e.g. watch-add failures or OS-level dropped events).
func (s *Service) rescanTicker(ctx context.Context, interval time.Duration) {
	defer s.wg.Done()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			slog.Info("vault: periodic rescan starting")
			if err := s.r.ColdStart(ctx); err != nil {
				slog.Error("vault: periodic rescan error", "error", err)
				continue
			}
			stats := s.r.Stats()
			slog.Info("vault: periodic rescan complete",
				"total_events_processed", stats.EventsProcessed,
			)
		}
	}
}
