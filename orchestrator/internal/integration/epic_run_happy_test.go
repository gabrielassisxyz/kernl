package integration

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/epic"
	"github.com/gabrielassisxyz/kernl/internal/sweep"
)

// --- fakeBeadBackend ---
type fakeBeadBackend struct {
	mu    sync.Mutex
	beads map[string]backend.Bead
}

func newFakeBeadBackend() *fakeBeadBackend {
	return &fakeBeadBackend{beads: make(map[string]backend.Bead)}
}

func (f *fakeBeadBackend) add(b backend.Bead) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.beads[b.ID] = b
}

func (f *fakeBeadBackend) get(id string) (backend.Bead, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.beads[id]
	return b, ok
}

func (f *fakeBeadBackend) setState(id, state string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.beads[id]
	if !ok {
		return
	}
	b.State = state
	f.beads[id] = b
}

// --- BackendPort implementation ---

func (f *fakeBeadBackend) Get(id string, _ string) (*backend.Bead, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.beads[id]
	if !ok {
		return nil, nil
	}
	cp := b
	return &cp, nil
}

func (f *fakeBeadBackend) List(filters *backend.BeadListFilters, _ string) ([]backend.Bead, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []backend.Bead
	for _, b := range f.beads {
		if filters != nil && filters.Parent != "" && b.ParentID != filters.Parent {
			continue
		}
		result = append(result, b)
	}
	return result, nil
}

func (f *fakeBeadBackend) Update(id string, input backend.UpdateBeadInput, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.beads[id]
	if !ok {
		return fmt.Errorf("KERNL DISPATCH FAILURE: bead %s not found — Fix: verify bead ID", id)
	}
	if input.State != "" {
		b.State = input.State
	}
	if input.Description != "" {
		if b.Description != "" {
			b.Description += "\n" + input.Description
		} else {
			b.Description = input.Description
		}
	}
	f.beads[id] = b
	return nil
}

func (f *fakeBeadBackend) Close(id string, reason string, _ string) (*backend.TerminalState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.beads[id]
	if !ok {
		return nil, nil
	}
	b.State = "closed"
	f.beads[id] = b
	return &backend.TerminalState{State: "closed", Reason: reason}, nil
}

// --- Remaining BackendPort methods (no-ops / stubs) ---

func (f *fakeBeadBackend) ListReady(_ *backend.BeadListFilters, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (f *fakeBeadBackend) Create(_ backend.CreateBeadInput, _ string) (*backend.Bead, error) {
	return nil, nil
}
func (f *fakeBeadBackend) Delete(_ string, _ string) error { return nil }
func (f *fakeBeadBackend) MarkTerminal(_ string, _ string, _ string, _ string) error {
	return nil
}
func (f *fakeBeadBackend) Reopen(_ string, _ string, _ string) error { return nil }
func (f *fakeBeadBackend) Rewind(_ string, _ string, _ string, _ string) error {
	return nil
}
func (f *fakeBeadBackend) Search(_ string, _ *backend.BeadListFilters, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (f *fakeBeadBackend) Query(_ string, _ *backend.BeadQueryOptions, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (f *fakeBeadBackend) AddDependency(_ string, _ string, _ string) error { return nil }
func (f *fakeBeadBackend) RemoveDependency(_ string, _ string, _ string) error {
	return nil
}
func (f *fakeBeadBackend) ListDependencies(_ string, _ string, _ *backend.DependencyListOptions) ([]backend.BeadDependency, error) {
	return nil, nil
}
func (f *fakeBeadBackend) BuildTakePrompt(_ string, _ *backend.TakePromptOptions, _ string) (*backend.TakePromptResult, error) {
	return nil, nil
}
func (f *fakeBeadBackend) BuildPollPrompt(_ *backend.PollPromptOptions, _ string) (*backend.PollPromptResult, error) {
	return nil, nil
}
func (f *fakeBeadBackend) ListWorkflows(_ string) ([]backend.WorkflowDescriptor, error) {
	return nil, nil
}
func (f *fakeBeadBackend) Capabilities() backend.BackendCapabilities {
	return backend.BackendCapabilities{}
}

// --- sweepBeBackend implements sweep.Backend ---
type sweepBeBackend struct {
	b *fakeBeadBackend
}

func (s *sweepBeBackend) ListEpicsAwaitingPRReview() ([]sweep.Epic, error) {
	s.b.mu.Lock()
	defer s.b.mu.Unlock()
	var result []sweep.Epic
	for id, b := range s.b.beads {
		if b.Type != "epic" || b.State != "awaiting_pr_review" {
			continue
		}
		url := extractPRURL(b.Description)
		if url == "" {
			continue
		}
		children := make([]string, 0)
		for _, c := range s.b.beads {
			if c.ParentID == id {
				children = append(children, c.ID)
			}
		}
		result = append(result, sweep.Epic{ID: id, PRURL: url, Children: children})
	}
	return result, nil
}

func (s *sweepBeBackend) Close(id, reason string) error {
	s.b.mu.Lock()
	defer s.b.mu.Unlock()
	b, ok := s.b.beads[id]
	if !ok {
		return fmt.Errorf("bead %s not found", id)
	}
	b.State = "closed"
	s.b.beads[id] = b
	return nil
}

func extractPRURL(desc string) string {
	for _, line := range strings.Split(desc, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "pr_url:") {
			return strings.TrimSpace(strings.TrimPrefix(t, "pr_url:"))
		}
	}
	return ""
}

// --- fakeGHView implements sweep.GH ---
type fakeGHView struct {
	mu   sync.Mutex
	seq  []sweep.PRState
	idx  int
	cnt  map[string]int
}

func newFakeGHView() *fakeGHView {
	return &fakeGHView{seq: make([]sweep.PRState, 0), cnt: make(map[string]int)}
}

func (g *fakeGHView) addResponse(st sweep.PRState) {
	g.seq = append(g.seq, st)
}

func (g *fakeGHView) View(prURL string) (sweep.PRState, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cnt[prURL]++
	if g.idx >= len(g.seq) {
		return g.seq[len(g.seq)-1], nil
	}
	st := g.seq[g.idx]
	g.idx++
	return st, nil
}

func (g *fakeGHView) callCount(prURL string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.cnt[prURL]
}

// --- epicRunMerge implements merge.TriggerRouter ---
type epicRunMerge struct {
	b      *fakeBeadBackend
	id     string
	prURL  string
	path   string
	mu     sync.Mutex
	done   int
	total  int
	merged bool
}

func newEpicRunMerge(b *fakeBeadBackend, epicID, prURL, repoPath string, childCount int) *epicRunMerge {
	return &epicRunMerge{
		b:     b,
		id:    epicID,
		prURL: prURL,
		path:  repoPath,
		total: childCount,
	}
}

func (e *epicRunMerge) TryTrigger(string) {
	e.mu.Lock()
	e.done++
	e.mu.Unlock()
}

func (e *epicRunMerge) RouteOutcome(string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.merged {
		return
	}
	e.merged = true
	_ = e.b.Update(e.id, backend.UpdateBeadInput{
		State:       "awaiting_pr_review",
		Description: fmt.Sprintf("merge_outcome: success\npr_url: %s", e.prURL),
	}, e.path)
}

// ------ The Test ------

func TestEpicRunHappyPath(t *testing.T) {
	repoPath := t.TempDir()

	// ---- Step 1: Build the fixture ----
	be := newFakeBeadBackend()
	be.add(backend.Bead{ID: "kernl-abc", Type: "epic", State: "open"})
	be.add(backend.Bead{ID: "kernl-c1", Type: "bead", State: "open", ParentID: "kernl-abc"})
	be.add(backend.Bead{ID: "kernl-c2", Type: "bead", State: "open", ParentID: "kernl-abc"})
	be.add(backend.Bead{ID: "kernl-c3", Type: "bead", State: "open", ParentID: "kernl-abc"})

	prURL := "https://x/pr/1"

	// ---- Step 2: Mock the GH adapter ----
	gh := newFakeGHView()
	gh.addResponse(sweep.PRState{State: "OPEN", CreatedAt: time.Now()})
	gh.addResponse(sweep.PRState{State: "MERGED", MergedAt: time.Now()})

	// ---- Step 3: Mock the agent dispatcher + merger ----
	mm := newEpicRunMerge(be, "kernl-abc", prURL, repoPath, 3)

	runBead := func(ctx context.Context, in epic.RunInput) (epic.RunResult, error) {
		return epic.RunResult{FinalState: "done", Success: true}, nil
	}

	// Load and execute
	ep, err := epic.LoadEpic(be, "kernl-abc", repoPath)
	if err != nil {
		t.Fatalf("LoadEpic: %v", err)
	}

	wm := epic.NewWorktreeManager(t.TempDir())
	ex := epic.NewExecutor(epic.ExecutorDeps{
		Epic:          ep,
		RunBead:       runBead,
		Worktree:      wm,
		MaxConcurrent: 5,
		MergeManager:  mm,
	})

	if err := ex.Run(context.Background()); err != nil {
		t.Fatalf("executor.Run: %v", err)
	}
	if ex.State() != epic.EpicCompleted {
		t.Fatalf("epic state = %v, want completed", ex.State())
	}

	// ---- Step 4: Assert end state after merge ----
	epicBead, ok := be.get("kernl-abc")
	if !ok {
		t.Fatal("epic bead not found after run")
	}
	if epicBead.State != "awaiting_pr_review" {
		t.Fatalf("epic state = %q, want awaiting_pr_review", epicBead.State)
	}
	if pr := extractPRURL(epicBead.Description); pr != prURL {
		t.Fatalf("epic description missing pr_url: %s\nwant pr_url: %s", epicBead.Description, prURL)
	}
	if epicBead.State != "awaiting_pr_review" {
		t.Fatalf("epic state = %q, want awaiting_pr_review", epicBead.State)
	}

	// Children have "open" state (executor doesn't mutate backend state)
	for _, cid := range []string{"kernl-c1", "kernl-c2", "kernl-c3"} {
		cb, _ := be.get(cid)
		if cb.State != "open" {
			t.Fatalf("child %s state = %q, want open (executor does not close in backend)", cid, cb.State)
		}
	}

	// Set up the sweeper with our fake Backend + GH
	swept := &sweepBeBackend{b: be}
	s := sweep.New(swept, gh, sweep.Config{})

	// ---- Tick 1: PR still OPEN → epic stays awaiting_pr_review ----
	if err := s.Tick(); err != nil {
		t.Fatalf("first sweep.Tick: %v", err)
	}
	if gh.callCount(prURL) != 1 {
		t.Fatalf("gh.View call count = %d, want 1 after first tick", gh.callCount(prURL))
	}
	epicBead, _ = be.get("kernl-abc")
	if epicBead.State != "awaiting_pr_review" {
		t.Fatalf("after first tick: epic state = %q, want awaiting_pr_review (PR was OPEN)", epicBead.State)
	}

	// ---- Tick 2: PR MERGED → epic + children closed ----
	if err := s.Tick(); err != nil {
		t.Fatalf("second sweep.Tick: %v", err)
	}
	if gh.callCount(prURL) != 2 {
		t.Fatalf("gh.View call count = %d, want 2 after second tick", gh.callCount(prURL))
	}

	epicBead, _ = be.get("kernl-abc")
	if epicBead.State != "closed" {
		t.Fatalf("after second tick: epic state = %q, want closed (PR was MERGED)", epicBead.State)
	}

	for _, cid := range []string{"kernl-c1", "kernl-c2", "kernl-c3"} {
		cb, _ := be.get(cid)
		if cb.State != "closed" {
			t.Fatalf("child %s state = %q, want closed", cid, cb.State)
		}
	}
}
