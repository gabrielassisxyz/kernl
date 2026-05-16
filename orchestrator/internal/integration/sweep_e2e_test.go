//go:build integration

package integration

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/sweep"
)

// e2eBackend is a static-list backend that records every Close() call.
// ListEpicsAwaitingPRReview always returns the same slice (simulating a real
// backend that pre-filters to only the still-open children), so the sweeper's
// own mergedCache is what prevents redundant gh.View calls on re-listing.
type e2eBackend struct {
	mu     sync.Mutex
	epics  []sweep.Epic
	closed map[string]bool
}

func (b *e2eBackend) ListEpicsAwaitingPRReview() ([]sweep.Epic, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]sweep.Epic, len(b.epics))
	copy(out, b.epics)
	return out, nil
}

func (b *e2eBackend) Close(id, reason string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed[id] = true
	return nil
}

func (b *e2eBackend) isClosed(id string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed[id]
}

// e2eGH is a configurable GH fake with per-URL call counting and
// a switchable error map for circuit-breaker phase.
type e2eGH struct {
	mu        sync.Mutex
	responses map[string]sweep.PRState
	errs      map[string]error
	calls     map[string]int
}

func (g *e2eGH) View(prURL string) (sweep.PRState, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls[prURL]++
	if e, ok := g.errs[prURL]; ok {
		return sweep.PRState{}, e
	}
	return g.responses[prURL], nil
}

func (g *e2eGH) callCount(prURL string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls[prURL]
}

func (g *e2eGH) setErr(prURL string, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err == nil {
		delete(g.errs, prURL)
	} else {
		g.errs[prURL] = err
	}
}

// TestSweepE2E runs four sequential phases against the same Sweeper instance
// so that mergedCache and circuit-breaker state carry across phases.
//
//  1. Merged PR closes epic + its open (awaiting_integration) children;
//     OPEN PR leaves the second epic untouched.
//  2. Cache hit: second Tick does not call gh.View for the already-merged epic.
//  3. Circuit breaker: 3 consecutive gh failures open the breaker so the 4th
//     Tick skips gh.View entirely for that epic.
func TestSweepE2E(t *testing.T) {
	const (
		epicID1 = "epic-1"
		epicID2 = "epic-2"
		pr1     = "https://github.com/org/repo/pull/1"
		pr2     = "https://github.com/org/repo/pull/2"
	)

	// Step 1: fixture.
	// Each epic has 3 children already in "closed" state — these are NOT
	// included in Children (the backend omits them). Only the 2 children
	// still in "awaiting_integration" appear in Children.
	epic1Open := []string{"e1-o1", "e1-o2"}
	epic2Open := []string{"e2-o1", "e2-o2"}

	b := &e2eBackend{
		closed: make(map[string]bool),
		epics: []sweep.Epic{
			{ID: epicID1, PRURL: pr1, Children: epic1Open},
			{ID: epicID2, PRURL: pr2, Children: epic2Open},
		},
	}

	// Step 2: GH mock — epic-1 PR is MERGED, epic-2 PR is OPEN.
	gh := &e2eGH{
		calls: make(map[string]int),
		responses: map[string]sweep.PRState{
			pr1: {State: "MERGED", MergedAt: time.Now()},
			pr2: {State: "OPEN", CreatedAt: time.Now()},
		},
		errs: make(map[string]error),
	}

	s := sweep.New(b, gh, sweep.Config{
		FailureThreshold: 3,
		BackoffMinutes:   []int{5, 15, 60},
	})

	// Step 3: first Tick.
	if err := s.Tick(); err != nil {
		t.Fatalf("Tick 1: %v", err)
	}

	// epic-1 closed (PR MERGED).
	if !b.isClosed(epicID1) {
		t.Error("epic-1: want closed after MERGED PR")
	}
	for _, id := range epic1Open {
		if !b.isClosed(id) {
			t.Errorf("open child %s of epic-1: want closed (MERGED PR)", id)
		}
	}

	// epic-2 unchanged (PR OPEN).
	if b.isClosed(epicID2) {
		t.Error("epic-2: want unchanged (PR OPEN)")
	}
	for _, id := range epic2Open {
		if b.isClosed(id) {
			t.Errorf("open child %s of epic-2: want NOT closed (PR OPEN)", id)
		}
	}

	// The pre-closed children (e.g. e1-c1..e1-c3) were never included in
	// Children, so Close() must never have been called for them.
	for _, id := range []string{"e1-c1", "e1-c2", "e1-c3", "e2-c1", "e2-c2", "e2-c3"} {
		if b.isClosed(id) {
			t.Errorf("pre-closed child %s: sweep must not call Close() for it", id)
		}
	}
	if gh.callCount(pr1) != 1 {
		t.Errorf("pr1 GH calls after Tick 1: want 1, got %d", gh.callCount(pr1))
	}

	// Step 4: second Tick — backend still lists epic-1 (static list), but
	// its PRURL is cached as merged so gh.View must not be called again.
	if err := s.Tick(); err != nil {
		t.Fatalf("Tick 2: %v", err)
	}
	if gh.callCount(pr1) != 1 {
		t.Errorf("pr1 GH calls after Tick 2: want 1 (cache hit), got %d", gh.callCount(pr1))
	}
	if gh.callCount(pr2) != 2 {
		t.Errorf("pr2 GH calls after Tick 2: want 2 (no cache), got %d", gh.callCount(pr2))
	}

	// Step 5: circuit breaker — force 3 consecutive GH failures for epic-2.
	gh.setErr(pr2, errors.New("simulated gh failure"))
	priorPR2Calls := gh.callCount(pr2)

	for i := 0; i < 3; i++ {
		_ = s.Tick()
	}
	if gh.callCount(pr2) != priorPR2Calls+3 {
		t.Errorf("pr2 GH calls after 3 failures: want %d, got %d",
			priorPR2Calls+3, gh.callCount(pr2))
	}

	// 4th Tick — breaker is open; gh.View must not be called for epic-2.
	_ = s.Tick()
	if gh.callCount(pr2) != priorPR2Calls+3 {
		t.Errorf("pr2 GH calls after breaker Tick: want %d (breaker blocked 4th call), got %d",
			priorPR2Calls+3, gh.callCount(pr2))
	}
}
