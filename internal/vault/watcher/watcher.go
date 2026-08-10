// Package watcher provides recursive filesystem watching for .md files
// using fsnotify with per-path event coalescing.
package watcher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// EventKind represents the type of a filesystem change event.
type EventKind string

const (
	KindCreate        EventKind = "create"
	KindChange        EventKind = "change"
	KindDelete        EventKind = "delete"
	KindMoveCandidate EventKind = "move_candidate"
)

// ChangeEvent is a normalized filesystem event emitted by the Watcher.
type ChangeEvent struct {
	Kind           EventKind `json:"kind"`
	Path           string    `json:"path"`
	CoalescedCount int       `json:"coalesced_count"`
	Timestamp      time.Time `json:"ts"`
}

// Config configures the Watcher.
type Config struct {
	// CoalesceWindow is the quiet period after the last event for a path
	// before the coalesced event is emitted. Default 300ms.
	CoalesceWindow time.Duration
}

// DefaultCoalesceWindow is the default debounce window.
const DefaultCoalesceWindow = 300 * time.Millisecond

// stoppableTimer is the part of *time.Timer this package uses. Stop reports
// whether it prevented the callback from running, which is load-bearing here:
// it is how Stop knows whether a scheduled coalescer will ever decrement the
// WaitGroup, or whether it must do so on its behalf.
type stoppableTimer interface {
	Stop() bool
}

// timerScheduler is the clock boundary. Production schedules on real time;
// tests inject a fake they advance by hand, because a coalesce window proven
// by sleeping is a test that passes on an idle laptop and fails on a loaded
// CI runner - and an injected Now() alone would not help, since what has to
// be controlled is when the callback FIRES, not what time it reports.
type timerScheduler interface {
	AfterFunc(d time.Duration, f func()) stoppableTimer
}

type realScheduler struct{}

func (realScheduler) AfterFunc(d time.Duration, f func()) stoppableTimer {
	return time.AfterFunc(d, f)
}

// Watcher watches a root directory recursively for .md file changes
// and emits coalesced ChangeEvents on a channel.
type Watcher struct {
	root   string
	events chan ChangeEvent
	cfg    Config
	// fsw is nil when the raw event source was injected, which is how the
	// coalescing tests run without touching the filesystem's notify API.
	fsw       *fsnotify.Watcher
	sched     timerScheduler
	rawEvents <-chan fsnotify.Event
	rawErrors <-chan error
	cancel    context.CancelFunc
	// shuttingDown is closed by Stop before it waits, so a coalescer already
	// past its Stop() check abandons its send instead of blocking forever on a
	// full buffer nobody is draining any more.
	shuttingDown chan struct{}
	wg           sync.WaitGroup
	mu           sync.Mutex
	closed       bool
	dirs         map[string]struct{} // tracked directories
	timers       map[string]stoppableTimer
	pending      map[string][]EventKind // pending events per path (latest kind wins)
}

// New creates a new Watcher for the given root directory.
// The caller must call Start() to begin watching, and Stop() to shut down.
func New(root string, cfg Config) (*Watcher, error) {
	if cfg.CoalesceWindow <= 0 {
		cfg.CoalesceWindow = DefaultCoalesceWindow
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve watcher root: %w", err)
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}

	w := &Watcher{
		root:         absRoot,
		events:       make(chan ChangeEvent, 256),
		cfg:          cfg,
		fsw:          fsw,
		sched:        realScheduler{},
		rawEvents:    fsw.Events,
		rawErrors:    fsw.Errors,
		shuttingDown: make(chan struct{}),
		dirs:         make(map[string]struct{}),
		timers:       make(map[string]stoppableTimer),
		pending:      make(map[string][]EventKind),
	}

	return w, nil
}

// newWithSources builds a Watcher whose clock and raw event source are supplied
// by the caller and whose fsnotify watcher is absent. Only tests use it: the
// coalescing and shutdown invariants are about this package's own bookkeeping,
// and driving them through real inotify delivery is what made the original test
// assert a timing window by sleeping.
func newWithSources(root string, cfg Config, sched timerScheduler, raw <-chan fsnotify.Event, rawErrs <-chan error) *Watcher {
	if cfg.CoalesceWindow <= 0 {
		cfg.CoalesceWindow = DefaultCoalesceWindow
	}
	return &Watcher{
		root:         root,
		events:       make(chan ChangeEvent, 256),
		cfg:          cfg,
		sched:        sched,
		rawEvents:    raw,
		rawErrors:    rawErrs,
		shuttingDown: make(chan struct{}),
		dirs:         make(map[string]struct{}),
		timers:       make(map[string]stoppableTimer),
		pending:      make(map[string][]EventKind),
	}
}

// Events returns the channel on which ChangeEvents are emitted.
func (w *Watcher) Events() <-chan ChangeEvent {
	return w.events
}

// Start begins watching the root directory recursively.
// It returns after the initial recursive walk to add watches.
func (w *Watcher) Start(ctx context.Context) error {
	// Add watches for all existing directories recursively. Skipped when the
	// raw source was injected: there is no fsnotify watcher to add them to.
	if w.fsw != nil {
		if err := w.addDirRecursive(w.root); err != nil {
			w.fsw.Close()
			return fmt.Errorf("add watches recursively: %w", err)
		}
	}

	ctx, w.cancel = context.WithCancel(ctx)

	w.wg.Add(1)
	go w.loop(ctx)

	return nil
}

// Stop shuts down the watcher and closes the events channel. It is idempotent,
// and it does not return until every scheduled coalescer has finished, so a
// consumer that sees the channel close knows no further event can arrive.
//
// Every scheduled coalescer holds a WaitGroup token from the moment it is
// scheduled, which is what makes that guarantee available: an unfired timer's
// token is released here (Stop() reporting true means its callback will never
// run), and a fired one releases its own. Waiting on a WaitGroup that only
// covered the read loop - as this used to - left the callbacks unaccounted for,
// which is why closing the channel looked unsafe and was skipped.
func (w *Watcher) Stop() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	pending := w.timers
	w.timers = make(map[string]stoppableTimer)
	w.mu.Unlock()

	if w.cancel != nil {
		w.cancel()
	}

	// Before the wait, so a coalescer that already passed its own closed check
	// abandons its send rather than blocking on a buffer nobody drains.
	close(w.shuttingDown)

	for _, timer := range pending {
		if timer.Stop() {
			w.wg.Done()
		}
	}

	w.wg.Wait()

	if w.fsw != nil {
		w.fsw.Close()
	}
	close(w.events)
}

// addDirRecursive adds a watch for the given directory and all subdirectories.
// Errors for individual directories are logged but don't stop the walk.
func (w *Watcher) addDirRecursive(dir string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		// Skip hidden directories
		if strings.HasPrefix(d.Name(), ".") && path != w.root {
			return filepath.SkipDir
		}
		if err := w.fsw.Add(path); err != nil {
			slog.Warn("watcher: add watch failed",
				"path", path,
				"error", err,
			)
			return nil
		}
		w.mu.Lock()
		w.dirs[path] = struct{}{}
		w.mu.Unlock()
		return nil
	})
}

func (w *Watcher) loop(ctx context.Context) {
	defer w.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.rawEvents:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.rawErrors:
			if !ok {
				return
			}
			slog.Error("watcher: fsnotify error", "error", err)
		}
	}
}

func (w *Watcher) handleEvent(event fsnotify.Event) {
	path := event.Name

	// Only care about .md files and directories (for recursive watch extension).
	isDir := false
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		isDir = true
	}

	if !isDir {
		if filepath.Ext(path) != ".md" {
			return
		}
	}

	// When a new directory is created, extend watches into it.
	if isDir && (event.Has(fsnotify.Create) || event.Has(fsnotify.Rename)) {
		// On some systems, Rename is emitted when a dir is moved in.
		if event.Has(fsnotify.Create) {
			_ = w.addDirRecursive(path)
		}
		return // Directory events for new dirs don't emit ChangeEvents
	}

	// For existing directories, just track; don't emit events.
	if isDir {
		return
	}

	// Map raw ops to a normalized kind.
	kind := w.normalizeKind(event)
	if kind == "" {
		return
	}

	// Coalesce: reset the debounce timer for this path.
	w.mu.Lock()

	// A timer scheduled after Stop has taken the pending set would never be
	// waited for, and its send would race the channel close.
	if w.closed {
		w.mu.Unlock()
		return
	}

	w.pending[path] = append(w.pending[path], kind)
	// Keep only the most recent kind; but for move_candidate, also keep it.
	// Strategy: coalesce all events for this path, emit the "most important" kind.
	// delete/move_candidate > change > create

	// Cancel the existing timer, releasing its token when the cancellation
	// beat its callback - otherwise that callback releases its own.
	if t, ok := w.timers[path]; ok {
		if t.Stop() {
			w.wg.Done()
		}
	}
	w.wg.Add(1)
	w.timers[path] = w.sched.AfterFunc(w.cfg.CoalesceWindow, func() {
		defer w.wg.Done()

		w.mu.Lock()
		kinds := w.pending[path]
		delete(w.pending, path)
		delete(w.timers, path)
		w.mu.Unlock()

		if len(kinds) == 0 {
			return
		}

		finalKind := coalesceKinds(kinds)

		ev := ChangeEvent{
			Kind:           finalKind,
			Path:           path,
			CoalescedCount: len(kinds),
			Timestamp:      time.Now(),
		}

		slog.Info("watcher: event",
			"kind", ev.Kind,
			"path", ev.Path,
			"coalesced_count", ev.CoalescedCount,
			"ts", ev.Timestamp.Format(time.RFC3339Nano),
		)

		select {
		case w.events <- ev:
		case <-w.shuttingDown:
			// Dropping a coalesced event during shutdown is the correct
			// trade: the alternative is Stop hanging on a full buffer.
		}
	})
	w.mu.Unlock()
}

// coalesceKinds picks the most significant kind from a sequence.
// delete and move_candidate take highest priority.
func coalesceKinds(kinds []EventKind) EventKind {
	hasDelete := false
	hasMove := false

	for _, k := range kinds {
		switch k {
		case KindDelete:
			hasDelete = true
		case KindMoveCandidate:
			hasMove = true
			// create is coalesced to change; neither individually matters.
		case KindCreate, KindChange:
			// default priority
		}
	}

	if hasDelete {
		return KindDelete
	}
	if hasMove {
		return KindMoveCandidate
	}
	return KindChange
}

func (w *Watcher) normalizeKind(event fsnotify.Event) EventKind {
	if event.Has(fsnotify.Remove) {
		return KindDelete
	}
	if event.Has(fsnotify.Rename) {
		return KindMoveCandidate
	}
	if event.Has(fsnotify.Create) {
		return KindCreate
	}
	if event.Has(fsnotify.Write) {
		return KindChange
	}
	return ""
}

// ErrWatchFailed is the sentinel for watch-add failures that should not stop the watcher.
var ErrWatchFailed = errors.New("watch add failed for path")
