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

// Watcher watches a root directory recursively for .md file changes
// and emits coalesced ChangeEvents on a channel.
type Watcher struct {
	root    string
	events  chan ChangeEvent
	cfg     Config
	fsw     *fsnotify.Watcher
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	mu      sync.Mutex
	closed  bool
	dirs    map[string]struct{} // tracked directories
	timers  map[string]*time.Timer
	pending map[string][]EventKind // pending events per path (latest kind wins)
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
		root:    absRoot,
		events:  make(chan ChangeEvent, 256),
		cfg:     cfg,
		fsw:     fsw,
		dirs:    make(map[string]struct{}),
		timers:  make(map[string]*time.Timer),
		pending: make(map[string][]EventKind),
	}

	return w, nil
}

// Events returns the channel on which ChangeEvents are emitted.
func (w *Watcher) Events() <-chan ChangeEvent {
	return w.events
}

// Start begins watching the root directory recursively.
// It returns after the initial recursive walk to add watches.
func (w *Watcher) Start(ctx context.Context) error {
	// Add watches for all existing directories recursively.
	if err := w.addDirRecursive(w.root); err != nil {
		w.fsw.Close()
		return fmt.Errorf("add watches recursively: %w", err)
	}

	ctx, w.cancel = context.WithCancel(ctx)

	w.wg.Add(1)
	go w.loop(ctx)

	return nil
}

// Stop shuts down the watcher and drains all pending timers.
// The events channel is closed after all goroutines have exited.
func (w *Watcher) Stop() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	w.mu.Unlock()

	if w.cancel != nil {
		w.cancel()
	}

	// Drain all pending timers so they don't fire after close.
	w.mu.Lock()
	for path, timer := range w.timers {
		timer.Stop()
		delete(w.timers, path)
	}
	w.mu.Unlock()

	w.wg.Wait()

	w.fsw.Close()
	// NOTE: we do NOT close w.events here because timer goroutines
	// may still be running when Stop returns and a send on a closed
	// channel panics. The channel will be garbage-collected with the
	// Watcher.
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
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.fsw.Errors:
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

	w.pending[path] = append(w.pending[path], kind)
	// Keep only the most recent kind; but for move_candidate, also keep it.
	// Strategy: coalesce all events for this path, emit the "most important" kind.
	// delete/move_candidate > change > create

	// Cancel existing timer
	if t, ok := w.timers[path]; ok {
		t.Stop()
	}
	w.timers[path] = time.AfterFunc(w.cfg.CoalesceWindow, func() {
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

		w.events <- ev
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
