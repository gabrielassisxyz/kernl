//go:build integration

package integration

import (
	"errors"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/sweep"
)

type sweepFixtureHarness struct {
	*Harness
	be *backend.BdCliBackend
}

func newSweepHarness(t *testing.T) *sweepFixtureHarness {
	t.Helper()
	h := newHarnessWithFixture(t, "beads-sweep")
	be := backend.NewBdCliBackend(h.RepoPath)

	for _, row := range []struct{ id, status string }{
		{"epic-1", "awaiting_pr_review"},
		{"c1d", "awaiting_integration"},
		{"c1e", "awaiting_integration"},
		{"epic-2", "awaiting_pr_review"},
		{"c2d", "awaiting_integration"},
		{"c2e", "awaiting_integration"},
	} {
		cmd := exec.Command("bd", "-C", h.RepoPath, "update", row.id, "--status", row.status)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("bd update %s --status %s: %v\n%s", row.id, row.status, err, out)
		}
	}

	return &sweepFixtureHarness{Harness: h, be: be}
}

type sweepAdapter struct {
	b   *backend.BdCliBackend
	dir string
}

func (a *sweepAdapter) ListEpicsAwaitingPRReview() ([]sweep.Epic, error) {
	epicBeads, err := a.b.List(&backend.BeadListFilters{
		State: "awaiting_pr_review",
		Type:  "epic",
	}, a.dir)
	if err != nil {
		return nil, err
	}

	var out []sweep.Epic
	for _, eb := range epicBeads {
		children, err := a.b.List(&backend.BeadListFilters{Parent: eb.ID}, a.dir)
		if err != nil {
			return nil, err
		}
		prURL := ""
		if eb.Metadata != nil {
			if u, ok := eb.Metadata["pr_url"].(string); ok {
				prURL = u
			}
		}
		childIDs := make([]string, 0, len(children))
		for _, c := range children {
			childIDs = append(childIDs, c.ID)
		}
		out = append(out, sweep.Epic{ID: eb.ID, PRURL: prURL, Children: childIDs})
	}
	return out, nil
}

func (a *sweepAdapter) Close(id, reason string) error {
	_, err := a.b.Close(id, reason, a.dir)
	return err
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

func beadState(t *testing.T, be *backend.BdCliBackend, dir, id string) string {
	t.Helper()
	b, err := be.Get(id, dir)
	if err != nil {
		t.Fatalf("beadState(%s): %v", id, err)
	}
	if b == nil {
		t.Fatalf("beadState(%s): bead not found", id)
	}
	return b.State
}

func TestSweep_MergedClosesChildrenAndEpic_OpenEpicUnchanged(t *testing.T) {
	h := newSweepHarness(t)

	mergedAt := time.Now()
	g := &fakeGH{
		responses: map[string]sweep.PRState{
			"https://github.com/test/pr/1": {State: "MERGED", MergedAt: mergedAt},
			"https://github.com/test/pr/2": {State: "OPEN", CreatedAt: time.Now()},
		},
		calls: map[string]int{},
	}

	adapter := &sweepAdapter{b: h.be, dir: h.RepoPath}
	s := sweep.New(adapter, g, sweep.Config{})

	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}

	if st := beadState(t, h.be, h.RepoPath, "epic-1"); st != "closed" {
		t.Errorf("epic-1 state = %q, want closed", st)
	}
	if st := beadState(t, h.be, h.RepoPath, "c1d"); st != "closed" {
		t.Errorf("c1d state = %q, want closed", st)
	}
	if st := beadState(t, h.be, h.RepoPath, "c1e"); st != "closed" {
		t.Errorf("c1e state = %q, want closed", st)
	}
	if st := beadState(t, h.be, h.RepoPath, "c1a"); st != "closed" {
		t.Errorf("c1a was already closed, state = %q, want closed", st)
	}

	if st := beadState(t, h.be, h.RepoPath, "epic-2"); st != "awaiting_pr_review" {
		t.Errorf("epic-2 state = %q, want awaiting_pr_review", st)
	}
	if st := beadState(t, h.be, h.RepoPath, "c2d"); st != "awaiting_integration" {
		t.Errorf("c2d state = %q, want awaiting_integration", st)
	}
	if st := beadState(t, h.be, h.RepoPath, "c2e"); st != "awaiting_integration" {
		t.Errorf("c2e state = %q, want awaiting_integration", st)
	}

	if g.calls["https://github.com/test/pr/1"] != 1 {
		t.Errorf("epic-1 gh calls = %d, want 1", g.calls["https://github.com/test/pr/1"])
	}
	if g.calls["https://github.com/test/pr/2"] != 1 {
		t.Errorf("epic-2 gh calls = %d, want 1", g.calls["https://github.com/test/pr/2"])
	}
}

func TestSweep_CacheHit_NoSecondGHCall(t *testing.T) {
	h := newSweepHarness(t)

	g := &fakeGH{
		responses: map[string]sweep.PRState{
			"https://github.com/test/pr/1": {State: "MERGED", MergedAt: time.Now()},
			"https://github.com/test/pr/2": {State: "OPEN", CreatedAt: time.Now()},
		},
		calls: map[string]int{},
	}

	adapter := &sweepAdapter{b: h.be, dir: h.RepoPath}
	s := sweep.New(adapter, g, sweep.Config{})

	_ = s.Tick()
	callsAfterFirst := g.calls["https://github.com/test/pr/1"]

	_ = s.Tick()

	if g.calls["https://github.com/test/pr/1"] != callsAfterFirst {
		t.Errorf("cache miss: epic-1 had %d gh calls after second tick, want %d",
			g.calls["https://github.com/test/pr/1"], callsAfterFirst)
	}
}

func TestSweep_CircuitBreaker_OpensAfter3Fails(t *testing.T) {
	h := newSweepHarness(t)

	g := &fakeGH{
		responses: map[string]sweep.PRState{
			"https://github.com/test/pr/1": {State: "MERGED", MergedAt: time.Now()},
		},
		errs: map[string]error{
			"https://github.com/test/pr/2": errors.New("network error"),
		},
		calls: map[string]int{},
	}

	adapter := &sweepAdapter{b: h.be, dir: h.RepoPath}
	s := sweep.New(adapter, g, sweep.Config{
		FailureThreshold: 3,
		BackoffMinutes:   []int{5, 15, 60},
	})

	for i := 0; i < 3; i++ {
		_ = s.Tick()
	}
	callsAfter3Fails := g.calls["https://github.com/test/pr/2"]

	_ = s.Tick()

	if g.calls["https://github.com/test/pr/2"] > callsAfter3Fails {
		t.Errorf("circuit breaker: epic-2 had %d gh calls after 4 ticks, want max %d",
			g.calls["https://github.com/test/pr/2"], callsAfter3Fails)
	}
}
