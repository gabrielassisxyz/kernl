package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// epicFakeBackend persists state, labels, and description so description-based
// exit gates (shipment's pr_url) can observe what the agent wrote.
type epicFakeBackend struct {
	mu    sync.Mutex
	beads map[string]*backend.Bead
}

func newEpicFakeBackend() *epicFakeBackend {
	return &epicFakeBackend{beads: make(map[string]*backend.Bead)}
}

func (b *epicFakeBackend) put(bd *backend.Bead) { b.beads[bd.ID] = bd }

func (b *epicFakeBackend) Get(id string, _ string) (*backend.Bead, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if bd, ok := b.beads[id]; ok {
		cp := *bd
		cp.Labels = append([]string(nil), bd.Labels...)
		return &cp, nil
	}
	return nil, nil
}

func (b *epicFakeBackend) Update(id string, in backend.UpdateBeadInput, _ string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	bd, ok := b.beads[id]
	if !ok {
		return nil
	}
	if in.State != "" {
		bd.State = in.State
	}
	if in.SetLabels != nil {
		bd.Labels = append([]string(nil), in.SetLabels...)
	}
	if in.Description != "" {
		bd.Description = in.Description
	}
	return nil
}

// unused BackendPort surface
func (b *epicFakeBackend) List(_ *backend.BeadListFilters, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *epicFakeBackend) ListReady(_ *backend.BeadListFilters, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *epicFakeBackend) Create(_ backend.CreateBeadInput, _ string) (*backend.Bead, error) {
	return nil, nil
}
func (b *epicFakeBackend) Delete(_ string, _ string) error { return nil }
func (b *epicFakeBackend) Close(_ string, _ string, _ string) (*backend.TerminalState, error) {
	return nil, nil
}
func (b *epicFakeBackend) MarkTerminal(_, _, _, _ string) error { return nil }
func (b *epicFakeBackend) Reopen(_, _, _ string) error          { return nil }
func (b *epicFakeBackend) Rewind(_, _, _, _ string) error       { return nil }
func (b *epicFakeBackend) Search(_ string, _ *backend.BeadListFilters, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *epicFakeBackend) Query(_ string, _ *backend.BeadQueryOptions, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *epicFakeBackend) AddDependency(_, _, _ string) error    { return nil }
func (b *epicFakeBackend) RemoveDependency(_, _, _ string) error { return nil }
func (b *epicFakeBackend) ListDependencies(_, _ string, _ *backend.DependencyListOptions) ([]backend.BeadDependency, error) {
	return nil, nil
}
func (b *epicFakeBackend) BuildTakePrompt(_ string, _ *backend.TakePromptOptions, _ string) (*backend.TakePromptResult, error) {
	return nil, nil
}
func (b *epicFakeBackend) BuildPollPrompt(_ *backend.PollPromptOptions, _ string) (*backend.PollPromptResult, error) {
	return nil, nil
}
func (b *epicFakeBackend) ListWorkflows(_ string) ([]backend.WorkflowDescriptor, error) {
	return nil, nil
}
func (b *epicFakeBackend) Comment(_ string, _ string, _ string) error { return nil }
func (b *epicFakeBackend) Capabilities() backend.BackendCapabilities {
	return backend.BackendCapabilities{}
}

// workerArtifactDriver simulates a worker child agent that produces each
// stage's exit-gate output: a "stage: implementation" marker commit, then a
// PASS verdict artifact for implementation_review.
type workerArtifactDriver struct {
	be       *epicFakeBackend
	beadID   string
	worktree string
}

func (d *workerArtifactDriver) RunBead(_ context.Context, _ RunBeadInput) (RunBeadResult, error) {
	bd, _ := d.be.Get(d.beadID, "")
	switch bd.State {
	case "implementation":
		cmd := exec.Command("git", "-C", d.worktree, "commit", "--allow-empty", "-m", "stage: implementation: did the work")
		if out, err := cmd.CombinedOutput(); err != nil {
			return RunBeadResult{Success: false}, fmt.Errorf("implementation commit: %v: %s", err, out)
		}
	case "implementation_review":
		dir := filepath.Join(d.worktree, ".kernl", d.beadID)
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, "implementation-review.md"), []byte("code matches the plan\n\nVERDICT: PASS"), 0o644)
	}
	return RunBeadResult{FinalState: "ok", Success: true, SessionID: "ses"}, nil
}

// TestDriveWorker_StopsAtAwaitingIntegration drives a worker-profile child from
// its initial state through the real exit gates and asserts it hands off at
// awaiting_integration only after producing the marker commit and PASS verdict.
func TestDriveWorker_StopsAtAwaitingIntegration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	worktree := t.TempDir()
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"},
		{"commit", "--allow-empty", "-m", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", worktree}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	be := newEpicFakeBackend()
	be.put(&backend.Bead{
		ID: "kernl-c1", Type: "task", Title: "Child", State: "ready_for_implementation",
		ProfileID: "worker",
	})

	res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		Backend:  be,
		Driver:   &workerArtifactDriver{be: be, beadID: "kernl-c1", worktree: worktree},
		Config:   newDriveTestConfig(),
		BeadID:   "kernl-c1",
		RepoPath: t.TempDir(),
		Worktree: worktree,
	})
	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}
	if !res.Success || res.FinalState != "awaiting_integration" {
		t.Fatalf("worker final = %+v; want awaiting_integration/success", res)
	}
}

// TestDriveWorker_BlocksWhenImplementationSkipsCommit reproduces the kernl-gc7j
// failure: a worker child whose agent exits zero but leaves no marker commit
// must block at the implementation gate instead of silently sailing to the
// terminal awaiting_integration with zero work done.
func TestDriveWorker_BlocksWhenImplementationSkipsCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	worktree := t.TempDir()
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"},
		{"commit", "--allow-empty", "-m", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", worktree}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	be := newEpicFakeBackend()
	be.put(&backend.Bead{
		ID: "kernl-c2", Type: "task", Title: "Empty Child", State: "ready_for_implementation",
		ProfileID: "worker",
	})

	res, _ := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		Backend:  be,
		Driver:   &silentDriver{},
		Config:   newDriveTestConfig(),
		BeadID:   "kernl-c2",
		RepoPath: t.TempDir(),
		Worktree: worktree,
	})
	if res.Success || res.FinalState != "blocked" {
		t.Fatalf("worker should block when implementation leaves no marker commit; got %+v", res)
	}
}

// artifactDriver simulates an agent that produces each epic stage's exit-gate
// artifact, keyed on the bead's current (already-advanced) state.
type artifactDriver struct {
	be       *epicFakeBackend
	epicID   string
	worktree string
}

func (d *artifactDriver) RunBead(_ context.Context, _ RunBeadInput) (RunBeadResult, error) {
	bd, _ := d.be.Get(d.epicID, "")
	switch bd.State {
	case "integration":
		cmd := exec.Command("git", "-C", d.worktree, "commit", "--allow-empty", "-m", "stage: integration: merged children")
		if out, err := cmd.CombinedOutput(); err != nil {
			return RunBeadResult{Success: false}, fmt.Errorf("integration commit: %v: %s", err, out)
		}
	case "integration_review":
		dir := filepath.Join(d.worktree, ".kernl", d.epicID)
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, "integration-review.md"), []byte("merge looks coherent\n\nVERDICT: PASS"), 0o644)
	case "shipment":
		_ = d.be.Update(d.epicID, backend.UpdateBeadInput{
			Description: "merge_outcome: success\npr_url: https://github.com/x/pr/1",
		}, "")
	}
	return RunBeadResult{FinalState: "ok", Success: true, SessionID: "ses"}, nil
}

// TestDriveEpic_ReachesAwaitingPRReview drives an epic-profile bead through
// integration -> integration_review -> shipment with real exit gates, asserting
// it lands at awaiting_pr_review only after every gate artifact exists.
func TestDriveEpic_ReachesAwaitingPRReview(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	worktree := t.TempDir()
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"},
		{"commit", "--allow-empty", "-m", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", worktree}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	be := newEpicFakeBackend()
	be.put(&backend.Bead{
		ID: "kernl-e1", Type: "epic", Title: "Test Epic", State: "ready_for_integration",
		ProfileID: "epic",
	})

	res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		Backend:  be,
		Driver:   &artifactDriver{be: be, epicID: "kernl-e1", worktree: worktree},
		Config:   newDriveTestConfig(),
		BeadID:   "kernl-e1",
		RepoPath: t.TempDir(),
		Worktree: worktree,
	})
	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}
	if !res.Success || res.FinalState != "awaiting_pr_review" {
		t.Fatalf("epic final = %+v; want awaiting_pr_review/success", res)
	}
	final, _ := be.Get("kernl-e1", "")
	if final.State != "awaiting_pr_review" {
		t.Errorf("epic bead state = %q, want awaiting_pr_review", final.State)
	}
}

// TestDriveEpic_BlocksWhenShipmentSkipsPR proves the shipment exit gate stops a
// silent agent that exits zero without recording a pr_url.
func TestDriveEpic_BlocksWhenShipmentSkipsPR(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	worktree := t.TempDir()
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"},
		{"commit", "--allow-empty", "-m", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", worktree}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	be := newEpicFakeBackend()
	be.put(&backend.Bead{
		ID: "kernl-e2", Type: "epic", Title: "Test Epic 2", State: "ready_for_integration",
		ProfileID: "epic",
	})

	// Driver creates integration + review artifacts but NEVER writes pr_url.
	drv := &artifactDriver{be: be, epicID: "kernl-e2", worktree: worktree}
	noPR := &noPRDriver{artifactDriver: drv}

	res, _ := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		Backend:  be,
		Driver:   noPR,
		Config:   newDriveTestConfig(),
		BeadID:   "kernl-e2",
		RepoPath: t.TempDir(),
		Worktree: worktree,
	})
	if res.Success || res.FinalState != "blocked" {
		t.Fatalf("epic should block when shipment skips pr_url; got %+v", res)
	}
}

// TestDriveEpic_BlocksOnIntegrationConflict proves the integration commit_marker
// gate stops the epic when the merge agent fails to leave a "stage: integration"
// commit (e.g. an unresolved merge conflict where it bailed).
func TestDriveEpic_BlocksOnIntegrationConflict(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	worktree := t.TempDir()
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"},
		{"commit", "--allow-empty", "-m", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", worktree}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	be := newEpicFakeBackend()
	be.put(&backend.Bead{
		ID: "kernl-e3", Type: "epic", Title: "Test Epic 3", State: "ready_for_integration",
		ProfileID: "epic",
	})

	// Driver exits zero on integration but leaves NO marker commit.
	res, _ := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		Backend:  be,
		Driver:   &silentDriver{},
		Config:   newDriveTestConfig(),
		BeadID:   "kernl-e3",
		RepoPath: t.TempDir(),
		Worktree: worktree,
	})
	if res.Success || res.FinalState != "blocked" {
		t.Fatalf("epic should block when integration leaves no marker commit; got %+v", res)
	}
}

// silentDriver exits zero without producing any stage artifact.
type silentDriver struct{}

func (silentDriver) RunBead(_ context.Context, _ RunBeadInput) (RunBeadResult, error) {
	return RunBeadResult{FinalState: "ok", Success: true}, nil
}

type noPRDriver struct{ *artifactDriver }

func (d *noPRDriver) RunBead(ctx context.Context, in RunBeadInput) (RunBeadResult, error) {
	bd, _ := d.be.Get(d.epicID, "")
	if bd.State == "shipment" {
		// exit zero but do nothing — no pr_url written
		return RunBeadResult{FinalState: "ok", Success: true}, nil
	}
	return d.artifactDriver.RunBead(ctx, in)
}
