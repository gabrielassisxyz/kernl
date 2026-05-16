package integration

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/epic"
	"github.com/gabrielassisxyz/kernl/internal/sweep"
)

// epicRunStore is an in-memory bead store for the happy-path E2E test.
// It is safe for concurrent use (executor runs children in parallel goroutines).
type epicRunStore struct {
	mu    sync.Mutex
	beads map[string]*epicRunBead
}

type epicRunBead struct {
	id       string
	typ      string
	state    string
	prURL    string
	children []string
}

func newEpicRunStore() *epicRunStore {
	s := &epicRunStore{beads: make(map[string]*epicRunBead)}
	s.beads["kernl-abc"] = &epicRunBead{
		id:       "kernl-abc",
		typ:      "epic",
		state:    "open",
		children: []string{"kernl-c1", "kernl-c2", "kernl-c3"},
	}
	for _, cid := range []string{"kernl-c1", "kernl-c2", "kernl-c3"} {
		s.beads[cid] = &epicRunBead{id: cid, typ: "task", state: "open"}
	}
	return s
}

func (s *epicRunStore) getState(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b := s.beads[id]; b != nil {
		return b.state
	}
	return ""
}

func (s *epicRunStore) getPRURL(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b := s.beads[id]; b != nil {
		return b.prURL
	}
	return ""
}

func (s *epicRunStore) setState(id, state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b := s.beads[id]; b != nil {
		b.state = state
	}
}

func (s *epicRunStore) setEpicPR(id, prURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b := s.beads[id]; b != nil {
		b.state = "awaiting_pr_review"
		b.prURL = prURL
	}
}

// fakeSweepBackend implements sweep.Backend against epicRunStore.
type fakeSweepBackend struct{ store *epicRunStore }

func (f *fakeSweepBackend) ListEpicsAwaitingPRReview() ([]sweep.Epic, error) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	var out []sweep.Epic
	for _, b := range f.store.beads {
		if b.typ == "epic" && b.state == "awaiting_pr_review" {
			out = append(out, sweep.Epic{ID: b.id, PRURL: b.prURL, Children: b.children})
		}
	}
	return out, nil
}

func (f *fakeSweepBackend) Close(id, _ string) error {
	f.store.setState(id, "closed")
	return nil
}

// fakeGH implements sweep.GH: returns OPEN on the first View call, MERGED on the second.
type fakeGH struct {
	mu    sync.Mutex
	calls int
}

func (g *fakeGH) View(_ string) (sweep.PRState, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	if g.calls < 2 {
		return sweep.PRState{State: "OPEN", CreatedAt: time.Now()}, nil
	}
	return sweep.PRState{State: "MERGED", MergedAt: time.Now(), CreatedAt: time.Now()}, nil
}

// fakeMergeManager implements merge.TriggerRouter.
// RouteOutcome records the PR URL and transitions the epic to awaiting_pr_review.
type fakeMergeManager struct{ store *epicRunStore }

func (m *fakeMergeManager) TryTrigger(_ string) {}

func (m *fakeMergeManager) RouteOutcome(epicID string) {
	m.store.setEpicPR(epicID, "https://x/pr/1")
}

// fakeWorktree satisfies the worktreeAdder interface used by epic.Executor.
// Each child gets its own sub-directory under base so agents write disjoint files.
type fakeWorktree struct{ base string }

func (w *fakeWorktree) Add(_, beadID string) (string, error) {
	dir := filepath.Join(w.base, beadID)
	return dir, os.MkdirAll(dir, 0o755)
}

// TestEpicRunHappyPath_3Children_MergerPRSweep is the end-to-end happy-path:
//   - 3 disjoint-file children run in parallel
//   - merger transitions epic to awaiting_pr_review with pr_url
//   - sweep.Tick() 1: PR OPEN → no state change
//   - sweep.Tick() 2: PR MERGED → epic + children closed
func TestEpicRunHappyPath_3Children_MergerPRSweep(t *testing.T) {
	store := newEpicRunStore()

	// Build the Epic in-process: no bd CLI required.
	dag, err := epic.NewDAG([]epic.Node{
		{ID: "kernl-c1"},
		{ID: "kernl-c2"},
		{ID: "kernl-c3"},
	})
	if err != nil {
		t.Fatalf("build DAG: %v", err)
	}
	ep := &epic.Epic{
		ID: "kernl-abc",
		Children: []backend.Bead{
			{ID: "kernl-c1", Type: "task", ParentID: "kernl-abc"},
			{ID: "kernl-c2", Type: "task", ParentID: "kernl-abc"},
			{ID: "kernl-c3", Type: "task", ParentID: "kernl-abc"},
		},
		DAG: dag,
	}

	wtBase := t.TempDir()
	mm := &fakeMergeManager{store: store}

	ex := epic.NewExecutor(epic.ExecutorDeps{
		Epic: ep,
		RunBead: func(ctx context.Context, in epic.RunInput) (epic.RunResult, error) {
			// Each child writes to its own file → disjoint changes, no merge conflict.
			f := filepath.Join(in.Worktree, in.BeadID+".txt")
			if err := os.WriteFile(f, []byte("change by "+in.BeadID), 0o644); err != nil {
				return epic.RunResult{}, err
			}
			store.setState(in.BeadID, "closed")
			return epic.RunResult{FinalState: "closed", Success: true}, nil
		},
		Worktree:      &fakeWorktree{base: wtBase},
		MaxConcurrent: 3,
		Emit:          func(epic.EpicEvent) {},
		MergeManager:  mm,
	})

	if err := ex.Run(context.Background()); err != nil {
		t.Fatalf("executor Run: %v", err)
	}

	// Children must be closed after the executor drains all waves.
	for _, cid := range []string{"kernl-c1", "kernl-c2", "kernl-c3"} {
		if got := store.getState(cid); got != "closed" {
			t.Errorf("child %s: state = %q, want closed", cid, got)
		}
	}

	// Executor calls RouteOutcome after all children finish;
	// fakeMergeManager sets epic to awaiting_pr_review with pr_url.
	if got := store.getState("kernl-abc"); got != "awaiting_pr_review" {
		t.Fatalf("epic state = %q, want awaiting_pr_review", got)
	}
	if got := store.getPRURL("kernl-abc"); got != "https://x/pr/1" {
		t.Errorf("epic pr_url = %q, want https://x/pr/1", got)
	}

	sw := sweep.New(&fakeSweepBackend{store: store}, &fakeGH{}, sweep.Config{})

	// Tick 1: gh.View returns OPEN → sweeper takes no action.
	if err := sw.Tick(); err != nil {
		t.Fatalf("sweep Tick 1: %v", err)
	}
	if got := store.getState("kernl-abc"); got != "awaiting_pr_review" {
		t.Errorf("after Tick 1: epic = %q, want awaiting_pr_review", got)
	}

	// Tick 2: gh.View returns MERGED → sweeper closes epic + children.
	if err := sw.Tick(); err != nil {
		t.Fatalf("sweep Tick 2: %v", err)
	}
	if got := store.getState("kernl-abc"); got != "closed" {
		t.Errorf("after Tick 2: epic = %q, want closed", got)
	}
}
