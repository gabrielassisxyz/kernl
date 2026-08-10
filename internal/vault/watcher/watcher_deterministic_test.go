package watcher

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// fakeTimer records what the watcher scheduled without any real clock. Stop
// reports whether it prevented the callback, matching *time.Timer, because the
// watcher's WaitGroup accounting depends on that exact answer.
type fakeTimer struct {
	sched   *fakeScheduler
	fn      func()
	fired   bool
	stopped bool
}

func (t *fakeTimer) Stop() bool {
	t.sched.mu.Lock()
	defer t.sched.mu.Unlock()
	if t.fired || t.stopped {
		return false
	}
	t.stopped = true
	return true
}

type fakeScheduler struct {
	mu     sync.Mutex
	timers []*fakeTimer
}

func (s *fakeScheduler) AfterFunc(_ time.Duration, f func()) stoppableTimer {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := &fakeTimer{sched: s, fn: f}
	s.timers = append(s.timers, t)
	return t
}

// fireDue runs every callback that has neither fired nor been stopped, which is
// what the passage of the coalesce window means here. Callbacks run outside the
// scheduler lock: they take the watcher's own mutex, and holding both would
// invert the order handleEvent uses.
func (s *fakeScheduler) fireDue() int {
	s.mu.Lock()
	var due []*fakeTimer
	for _, t := range s.timers {
		if !t.fired && !t.stopped {
			t.fired = true
			due = append(due, t)
		}
	}
	s.mu.Unlock()

	for _, t := range due {
		t.fn()
	}
	return len(due)
}

func (s *fakeScheduler) scheduled() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.timers)
}

// startInjected builds a watcher whose clock and raw events are the test's, and
// returns the raw-event channel to feed it.
func startInjected(t *testing.T, sched *fakeScheduler) (*Watcher, chan fsnotify.Event) {
	t.Helper()
	raw := make(chan fsnotify.Event, 64)
	errs := make(chan error, 1)
	w := newWithSources(t.TempDir(), Config{CoalesceWindow: time.Hour}, sched, raw, errs)
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return w, raw
}

// feed delivers raw events and waits until the watcher has scheduled the
// coalescer they imply, so the test never races the read loop. want is the
// number of timers expected to exist by then.
func feed(t *testing.T, sched *fakeScheduler, raw chan fsnotify.Event, want int, events ...fsnotify.Event) {
	t.Helper()
	for _, ev := range events {
		raw <- ev
	}
	deadline := time.Now().Add(5 * time.Second)
	for sched.scheduled() < want {
		if time.Now().After(deadline) {
			t.Fatalf("watcher scheduled %d coalescers, expected %d", sched.scheduled(), want)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestCoalescingEmitsOneEventForABurst replaces a version that wrote five files
// and slept: it asserted CoalescedCount >= 1, which any single event satisfies,
// and it depended on inotify choosing to collapse the writes. Here the burst is
// exact and the window passes on command.
func TestCoalescingEmitsOneEventForABurst(t *testing.T) {
	sched := &fakeScheduler{}
	w, raw := startInjected(t, sched)
	defer w.Stop()

	path := "/vault/coalesce.md"
	burst := make([]fsnotify.Event, 5)
	for i := range burst {
		burst[i] = fsnotify.Event{Name: path, Op: fsnotify.Write}
	}
	// Five events for one path reschedule the same coalescer five times.
	feed(t, sched, raw, 5, burst...)

	if fired := sched.fireDue(); fired != 1 {
		t.Fatalf("expected exactly one live coalescer after a burst, %d fired", fired)
	}

	select {
	case ev := <-w.Events():
		if ev.Kind != KindChange {
			t.Errorf("expected %s, got %s", KindChange, ev.Kind)
		}
		if ev.CoalescedCount != 5 {
			t.Errorf("expected all 5 raw events coalesced into one, got count %d", ev.CoalescedCount)
		}
		if ev.Path != path {
			t.Errorf("expected path %s, got %s", path, ev.Path)
		}
	default:
		t.Fatal("expected one coalesced event, channel was empty")
	}

	select {
	case ev := <-w.Events():
		t.Fatalf("expected no second event, got %+v", ev)
	default:
	}
}

// TestCoalescingKeepsTheMostSignificantKind pins that a delete inside the window
// wins over the writes around it - the caller acts on the file being gone, and a
// "change" for a path that no longer exists sends the reconciler looking for it.
func TestCoalescingKeepsTheMostSignificantKind(t *testing.T) {
	sched := &fakeScheduler{}
	w, raw := startInjected(t, sched)
	defer w.Stop()

	path := "/vault/gone.md"
	feed(t, sched, raw, 3,
		fsnotify.Event{Name: path, Op: fsnotify.Write},
		fsnotify.Event{Name: path, Op: fsnotify.Remove},
		fsnotify.Event{Name: path, Op: fsnotify.Write},
	)
	sched.fireDue()

	select {
	case ev := <-w.Events():
		if ev.Kind != KindDelete {
			t.Errorf("expected %s to win the window, got %s", KindDelete, ev.Kind)
		}
	default:
		t.Fatal("expected a coalesced event")
	}
}

// TestCoalescingKeepsPathsIndependent guards against one path's window
// swallowing another's, which a single shared timer would do.
func TestCoalescingKeepsPathsIndependent(t *testing.T) {
	sched := &fakeScheduler{}
	w, raw := startInjected(t, sched)
	defer w.Stop()

	feed(t, sched, raw, 2,
		fsnotify.Event{Name: "/vault/a.md", Op: fsnotify.Write},
		fsnotify.Event{Name: "/vault/b.md", Op: fsnotify.Write},
	)
	if fired := sched.fireDue(); fired != 2 {
		t.Fatalf("expected one coalescer per path, %d fired", fired)
	}

	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case ev := <-w.Events():
			seen[ev.Path] = true
		default:
			t.Fatalf("expected 2 events, got %d", len(seen))
		}
	}
	if !seen["/vault/a.md"] || !seen["/vault/b.md"] {
		t.Errorf("expected an event for each path, got %v", seen)
	}
}

// TestStopClosesTheEventsChannel is the contract Stop could not offer while the
// coalescer callbacks were unaccounted for: it used to leave the channel open
// forever, so a consumer ranging over it never learned the watcher had stopped.
func TestStopClosesTheEventsChannel(t *testing.T) {
	sched := &fakeScheduler{}
	w, _ := startInjected(t, sched)

	w.Stop()

	select {
	case _, ok := <-w.Events():
		if ok {
			t.Fatal("expected the events channel to be closed after Stop")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("events channel was still open two seconds after Stop returned")
	}
}

// TestStopReleasesCancelledCoalescerTokens is the arithmetic check. Each
// scheduled coalescer holds a WaitGroup token; a rescheduled path cancels the
// previous timer, whose token must be released by whoever won. Get it wrong in
// one direction and Stop hangs; in the other it panics with a negative counter.
func TestStopReleasesCancelledCoalescerTokens(t *testing.T) {
	sched := &fakeScheduler{}
	w, raw := startInjected(t, sched)

	burst := make([]fsnotify.Event, 20)
	for i := range burst {
		burst[i] = fsnotify.Event{Name: "/vault/churn.md", Op: fsnotify.Write}
	}
	feed(t, sched, raw, 20, burst...)

	done := make(chan struct{})
	go func() { w.Stop(); close(done) }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop hung: a cancelled coalescer's WaitGroup token was never released")
	}
}

// TestStopWithACoalescerAlreadyDue runs Stop against a callback that is firing
// at the same moment. Under -race this is the interleaving that made closing the
// channel look impossible.
func TestStopWithACoalescerAlreadyDue(t *testing.T) {
	sched := &fakeScheduler{}
	w, raw := startInjected(t, sched)

	feed(t, sched, raw, 1, fsnotify.Event{Name: "/vault/due.md", Op: fsnotify.Write})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); sched.fireDue() }()
	go func() { defer wg.Done(); w.Stop() }()

	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()

	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop and a due coalescer deadlocked")
	}
}

// TestStopDoesNotHangOnAFullBuffer: nothing drains the events channel after
// Stop, so a coalescer holding a full buffer would pin Stop forever. Dropping
// the event is the deliberate trade.
func TestStopDoesNotHangOnAFullBuffer(t *testing.T) {
	sched := &fakeScheduler{}
	w, raw := startInjected(t, sched)

	// Fill the buffer, one distinct path per slot.
	const capacity = 256
	events := make([]fsnotify.Event, capacity)
	for i := range events {
		events[i] = fsnotify.Event{Name: "/vault/f" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".md", Op: fsnotify.Write}
	}
	feed(t, sched, raw, capacity, events...)
	sched.fireDue()

	// One more, left pending: its send will find the buffer full.
	feed(t, sched, raw, capacity+1, fsnotify.Event{Name: "/vault/overflow.md", Op: fsnotify.Write})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); sched.fireDue() }()

	done := make(chan struct{})
	go func() { w.Stop(); close(done) }()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop hung on a coalescer blocked sending into a full buffer")
	}
	wg.Wait()
}

// TestStopIsIdempotent - the vault service calls Stop from its own shutdown and
// tests call it from a defer, so a second call must not panic on a closed
// channel.
func TestStopIsIdempotent(t *testing.T) {
	sched := &fakeScheduler{}
	w, _ := startInjected(t, sched)

	w.Stop()
	w.Stop()
}

// TestEventsAfterStopAreNotScheduled: the read loop is cancelled by Stop, but a
// raw event already in flight must not schedule a coalescer nobody waits for.
func TestEventsAfterStopAreNotScheduled(t *testing.T) {
	sched := &fakeScheduler{}
	w, _ := startInjected(t, sched)
	w.Stop()

	before := sched.scheduled()
	w.handleEvent(fsnotify.Event{Name: "/vault/late.md", Op: fsnotify.Write})
	if after := sched.scheduled(); after != before {
		t.Errorf("expected no coalescer scheduled after Stop, went from %d to %d", before, after)
	}
}
