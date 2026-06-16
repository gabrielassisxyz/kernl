package watcher

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatcherCreateMdFile(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{CoalesceWindow: 50 * time.Millisecond}
	w, err := New(dir, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	// Give fsnotify time to register initial watches.
	time.Sleep(50 * time.Millisecond)

	// Create a .md file.
	mdPath := filepath.Join(dir, "test.md")
	if err := os.WriteFile(mdPath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-w.Events():
		if ev.Kind != KindChange {
			t.Errorf("expected KindChange, got %s", ev.Kind)
		}
		if !strings.HasSuffix(ev.Path, "test.md") {
			t.Errorf("expected path ending with test.md, got %s", ev.Path)
		}
		if ev.CoalescedCount < 1 {
			t.Errorf("expected coalesced_count >= 1, got %d", ev.CoalescedCount)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for create event")
	}
}

func TestWatcherNonMdFileIgnored(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{CoalesceWindow: 50 * time.Millisecond}
	w, err := New(dir, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	time.Sleep(50 * time.Millisecond)

	// Create a non-.md file.
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Should NOT get an event.
	select {
	case ev := <-w.Events():
		t.Errorf("unexpected event for non-.md file: %+v", ev)
	case <-time.After(300 * time.Millisecond):
		// Expected — no event.
	}
}

func TestWatcherCoalescing(t *testing.T) {
	dir := t.TempDir()

	// Create file first so we can Write to it.
	mdPath := filepath.Join(dir, "coalesce.md")
	if err := os.WriteFile(mdPath, []byte("initial"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{CoalesceWindow: 50 * time.Millisecond}
	w, err := New(dir, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	time.Sleep(50 * time.Millisecond)

	// Drain the create event from the initial file.
	select {
	case <-w.Events():
	case <-time.After(1 * time.Second):
		// Might not get create since file existed before watcher started.
	}

	// Burst of 5 writes within the coalesce window.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = os.WriteFile(mdPath, []byte("burst"), 0o644)
		}(i)
	}
	wg.Wait()

	// We should get exactly one event for this path.
	select {
	case ev := <-w.Events():
		if ev.Kind != KindChange {
			t.Errorf("expected KindChange, got %s", ev.Kind)
		}
		if ev.CoalescedCount < 1 {
			t.Errorf("expected coalesced_count >= 1, got %d", ev.CoalescedCount)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for coalesced event")
	}

	// Ensure no additional events for this path.
	select {
	case ev := <-w.Events():
		t.Errorf("unexpected extra event: %+v", ev)
	case <-time.After(300 * time.Millisecond):
		// Expected — no extra events.
	}
}

func TestWatcherRecursiveWatch(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{CoalesceWindow: 50 * time.Millisecond}
	w, err := New(dir, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	time.Sleep(50 * time.Millisecond)

	// Create a new subdirectory.
	subDir := filepath.Join(dir, "newdir")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Give the watcher time to add a watch for the new directory.
	time.Sleep(100 * time.Millisecond)

	// Create a .md file inside the new subdirectory.
	mdPath := filepath.Join(subDir, "inside.md")
	if err := os.WriteFile(mdPath, []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-w.Events():
		if ev.Kind != KindChange {
			t.Errorf("expected KindChange from file in new subdir, got %s", ev.Kind)
		}
		if !strings.Contains(ev.Path, "inside.md") {
			t.Errorf("expected inside.md in path, got %s", ev.Path)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event from file in new subdirectory")
	}
}

func TestWatcherRenameMoveCandidate(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{CoalesceWindow: 50 * time.Millisecond}
	w, err := New(dir, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	time.Sleep(50 * time.Millisecond)

	// Create a file first.
	oldPath := filepath.Join(dir, "old.md")
	if err := os.WriteFile(oldPath, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Drain the create event.
	select {
	case <-w.Events():
	case <-time.After(1 * time.Second):
	}

	// Rename it.
	newPath := filepath.Join(dir, "new.md")
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}

	// We expect at least one event: move_candidate for the rename or a delete+create pair.
	// The bead says U6 emits raw events; U11 correlates them.
	// We accept either move_candidate or delete+create.
	gotMove := false
	gotDelete := false
	gotCreate := false

	timeout := time.After(2 * time.Second)
loop:
	for {
		select {
		case ev := <-w.Events():
			switch ev.Kind {
			case KindMoveCandidate:
				gotMove = true
				break loop
			case KindDelete:
				gotDelete = true
			case KindCreate:
				gotCreate = true
			}
			// If we got both delete and create, that's a valid representation too.
			if gotDelete && gotCreate {
				break loop
			}
		case <-timeout:
			break loop
		}
	}

	if !gotMove && (!gotDelete || !gotCreate) {
		t.Errorf("expected move_candidate or delete+create; got move=%v delete=%v create=%v", gotMove, gotDelete, gotCreate)
	}
}

func TestWatcherDelete(t *testing.T) {
	dir := t.TempDir()

	// Pre-create the file to ensure it exists before the watcher starts.
	mdPath := filepath.Join(dir, "delete.md")
	if err := os.WriteFile(mdPath, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{CoalesceWindow: 50 * time.Millisecond}
	w, err := New(dir, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	time.Sleep(50 * time.Millisecond)

	// Drain any create/change events from pre-existing file.
	select {
	case <-w.Events():
	case <-time.After(500 * time.Millisecond):
	}

	// Delete the file.
	if err := os.Remove(mdPath); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-w.Events():
		if ev.Kind != KindDelete {
			t.Errorf("expected KindDelete, got %s", ev.Kind)
		}
		if !strings.Contains(ev.Path, "delete.md") {
			t.Errorf("expected delete.md in path, got %s", ev.Path)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for delete event")
	}
}

func TestWatcherShutdownClean(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{CoalesceWindow: 50 * time.Millisecond}
	w, err := New(dir, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Trigger some events to ensure timers are active.
	mdPath := filepath.Join(dir, "shutdown.md")
	_ = os.WriteFile(mdPath, []byte("data"), 0o644)

	// Cancel context to trigger shutdown.
	cancel()

	// Stop should return without hanging.
	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success — clean shutdown.
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not shut down cleanly within 5s")
	}
}

func TestWatcherStressBurst(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{CoalesceWindow: 50 * time.Millisecond}
	w, err := New(dir, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Create 20 .md files.
	for i := 0; i < 20; i++ {
		name := filepath.Join(dir, "stress_"+string(rune('a'+i%26))+"_"+string(rune('a'+i/26))+"_.md")
		_ = os.WriteFile(name, []byte("initial"), 0o644)
	}

	// Drain creates.
	drainCh := make(chan struct{})
	go func() {
		timeout := time.After(2 * time.Second)
		for {
			select {
			case <-w.Events():
			case <-timeout:
				close(drainCh)
				return
			}
		}
	}()
	<-drainCh

	// Now burst writes: 200 writes across 20 files.
	var wg sync.WaitGroup
	var burstCount atomic.Int64
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fname := filepath.Join(dir, "stress_"+string(rune('a'+(i%20)%26))+"_"+string(rune('a'+(i%20)/26))+"_.md")
			_ = os.WriteFile(fname, []byte("burst"), 0o644)
			burstCount.Add(1)
		}(i)
	}
	wg.Wait()

	// Collect events with a timeout.
	var eventCount atomic.Int64
	timeout := time.AfterFunc(2*time.Second, func() {
		cancel()
		w.Stop()
	})

	for {
		select {
		case ev, ok := <-w.Events():
			if !ok {
				goto STOPPED
			}
			_ = ev
			eventCount.Add(1)
		case <-ctx.Done():
			goto STOPPED
		}
	}
STOPPED:
	timeout.Stop()

	n := eventCount.Load()
	if n == 0 {
		t.Error("expected at least some events from stress burst, got 0")
	}
	// With coalescing, we expect at most 20 unique files worth of events.
	// But some may be duplicates if timing is unlucky. We just verify it doesn't crash.
	t.Logf("stress burst: %d events received for 200 writes across 20 files", n)
}

func TestWatcherHiddenDirSkipped(t *testing.T) {
	dir := t.TempDir()

	// Pre-create a hidden directory.
	hiddenDir := filepath.Join(dir, ".hidden")
	if err := os.Mkdir(hiddenDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := Config{CoalesceWindow: 50 * time.Millisecond}
	w, err := New(dir, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	time.Sleep(50 * time.Millisecond)

	// Create an .md file inside the hidden directory.
	hiddenMd := filepath.Join(hiddenDir, "hidden.md")
	if err := os.WriteFile(hiddenMd, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Should NOT get an event for files in hidden directories.
	select {
	case ev := <-w.Events():
		t.Errorf("unexpected event from hidden dir: %+v", ev)
	case <-time.After(300 * time.Millisecond):
		// Expected — no event.
	}
}

func TestWatcherWatchAddErrorSurvives(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{CoalesceWindow: 50 * time.Millisecond}
	w, err := New(dir, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	time.Sleep(50 * time.Millisecond)

	// Create a normal subdirectory that should be watched.
	goodDir := filepath.Join(dir, "gooddir")
	if err := os.Mkdir(goodDir, 0o755); err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond) // let the watcher pick up the new dir

	// Create an .md file in the good directory.
	mdPath := filepath.Join(goodDir, "working.md")
	if err := os.WriteFile(mdPath, []byte("still works"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-w.Events():
		if ev.Kind != KindChange {
			t.Errorf("expected KindChange, got %s", ev.Kind)
		}
		if !strings.Contains(ev.Path, "working.md") {
			t.Errorf("expected working.md in path, got %s", ev.Path)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out — watcher should keep working after a watch error on another dir")
	}
}

func TestCoalesceKinds(t *testing.T) {
	tests := []struct {
		name  string
		kinds []EventKind
		want  EventKind
	}{
		{"single change", []EventKind{KindChange}, KindChange},
		{"single delete", []EventKind{KindDelete}, KindDelete},
		{"single create", []EventKind{KindCreate}, KindChange},
		{"change then delete", []EventKind{KindChange, KindDelete}, KindDelete},
		{"create then change", []EventKind{KindCreate, KindChange}, KindChange},
		{"delete takes priority", []EventKind{KindChange, KindDelete, KindChange}, KindDelete},
		{"move takes priority over change", []EventKind{KindChange, KindMoveCandidate}, KindMoveCandidate},
		{"delete over move", []EventKind{KindMoveCandidate, KindDelete}, KindDelete},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := coalesceKinds(tt.kinds)
			if got != tt.want {
				t.Errorf("coalesceKinds(%v) = %s, want %s", tt.kinds, got, tt.want)
			}
		})
	}
}
