package sweep_test

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/sweep"
)

type epicRow struct {
	ID       string
	PRURL    string
	Children []string
}

type fakeBackend struct {
	mu     sync.Mutex
	epics  []epicRow
	closed []string
}

func (f *fakeBackend) ListEpicsAwaitingPRReview() ([]sweep.Epic, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []sweep.Epic
	for _, e := range f.epics {
		out = append(out, sweep.Epic{ID: e.ID, PRURL: e.PRURL, Children: e.Children})
	}
	return out, nil
}

func (f *fakeBackend) Close(id, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = append(f.closed, id)
	return nil
}

type fakeGH struct {
	mu        sync.Mutex
	responses map[string]sweep.PRState
	errs      map[string]error
	calls     map[string]int
}

func (g *fakeGH) View(prURL string) (sweep.PRState, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls[prURL]++
	if e, ok := g.errs[prURL]; ok && e != nil {
		return sweep.PRState{}, e
	}
	return g.responses[prURL], nil
}

func TestSweep_HappyMerged_ClosesChildrenAndEpic(t *testing.T) {
	b := &fakeBackend{epics: []epicRow{{ID: "e1", PRURL: "https://x/pr/1", Children: []string{"c1", "c2"}}}}
	g := &fakeGH{
		responses: map[string]sweep.PRState{"https://x/pr/1": {State: "MERGED", MergedAt: time.Now()}},
		calls:     map[string]int{},
	}
	s := sweep.New(b, g, sweep.Config{})
	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(b.closed) != 3 {
		t.Fatalf("expected 3 closes (2 children + epic), got %d (%v)", len(b.closed), b.closed)
	}
}

func TestSweep_CacheHit_NoSecondGHCall(t *testing.T) {
	b := &fakeBackend{epics: []epicRow{{ID: "e1", PRURL: "https://x/pr/1"}}}
	g := &fakeGH{
		responses: map[string]sweep.PRState{"https://x/pr/1": {State: "MERGED", MergedAt: time.Now()}},
		calls:     map[string]int{},
	}
	s := sweep.New(b, g, sweep.Config{})
	_ = s.Tick()
	_ = s.Tick()
	if g.calls["https://x/pr/1"] != 1 {
		t.Fatalf("expected 1 gh call (cache hit on 2nd tick), got %d", g.calls["https://x/pr/1"])
	}
}

func TestSweep_CircuitBreaker_OpensAfter3Fails(t *testing.T) {
	b := &fakeBackend{epics: []epicRow{{ID: "e1", PRURL: "https://x/pr/1"}}}
	g := &fakeGH{
		errs:  map[string]error{"https://x/pr/1": errors.New("network")},
		calls: map[string]int{},
	}
	s := sweep.New(b, g, sweep.Config{FailureThreshold: 3, BackoffMinutes: []int{5, 15, 60}})
	for i := 0; i < 3; i++ {
		_ = s.Tick()
	}
	_ = s.Tick()
	if g.calls["https://x/pr/1"] > 3 {
		t.Fatalf("expected breaker to skip gh after 3 fails, got %d calls", g.calls["https://x/pr/1"])
	}
}

func TestSweep_DryRun_NoWrites(t *testing.T) {
	b := &fakeBackend{epics: []epicRow{{ID: "e1", PRURL: "https://x/pr/1", Children: []string{"c1"}}}}
	g := &fakeGH{
		responses: map[string]sweep.PRState{"https://x/pr/1": {State: "MERGED", MergedAt: time.Now()}},
		calls:     map[string]int{},
	}
	s := sweep.New(b, g, sweep.Config{DryRun: true})
	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(b.closed) != 0 {
		t.Fatalf("dry-run wrote: %v", b.closed)
	}
}

func TestSweep_PRStaleWARN_FiresHookAndNoClose(t *testing.T) {
	b := &fakeBackend{epics: []epicRow{{ID: "e1", PRURL: "https://x/pr/1"}}}
	old := time.Now().Add(-10 * 24 * time.Hour)
	g := &fakeGH{
		responses: map[string]sweep.PRState{"https://x/pr/1": {State: "OPEN", CreatedAt: old}},
		calls:     map[string]int{},
	}
	var warns []string
	s := sweep.New(b, g, sweep.Config{
		PRStaleWarnDays: 7,
		WarnHook:        func(msg string) { warns = append(warns, msg) },
	})
	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(b.closed) != 0 {
		t.Fatalf("OPEN PR should not be closed: %v", b.closed)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "open for 10 days") {
		t.Fatalf("expected WARN containing 'open for 10 days', got %v", warns)
	}
}
