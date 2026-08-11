package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
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

// fakeDecisionRecord is a minimal decision record satisfying the
// decision_record exit gate's required fields (see
// backend.ParseDecisionRecordDocument). Only workerArtifactDriver writes
// this: worker's "implementation" state carries the decision_record gate
// alongside commit_marker, but epic's "integration" deliberately does not
// (it is a merge stage, not an implementer's stage - see
// canonicalImplementationExitGates in internal/backend/state_machine.go), so
// artifactDriver below must never write one.
const fakeDecisionRecord = `{"decisions":[{"decision":"Use the fake artifact driver's canned content.","optionsConsidered":"1. Write a real decision.\n2. Write a minimal but complete fake one.","tradeOffs":"A real decision is more realistic but couples this test to prose that has nothing to do with what it verifies.","rationale":"Option 2 wins: the gate only checks structure, not content."}]}`

// commitRealChange writes relPath into worktree and commits it. The
// commit_marker gate now requires the tree to actually differ from base, not
// merely that a commit exists - `git commit --allow-empty` produces a commit
// with an identical tree and must not satisfy it. Every driver below that
// simulates a stage the gate is supposed to let through commits a real file
// through this helper instead of `--allow-empty`, so these tests keep
// exercising what a genuine implementer/integrator does.
func commitRealChange(worktree, relPath, content, message string) error {
	if err := os.WriteFile(filepath.Join(worktree, relPath), []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", relPath, err)
	}
	if out, err := exec.Command("git", "-C", worktree, "add", relPath).CombinedOutput(); err != nil {
		return fmt.Errorf("git add %s: %w: %s", relPath, err, out)
	}
	if out, err := exec.Command("git", "-C", worktree, "commit", "-m", message).CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %w: %s", err, out)
	}
	return nil
}

// workerArtifactDriver simulates a worker child agent that produces each
// stage's exit-gate output: a "stage: implementation" marker commit plus a
// decision record, then a PASS verdict artifact for implementation_review.
// Artifacts are written to the same directory kernl itself resolves
// (StateDir/run/<epic>/<bead>/, not the worktree), so the driver mirrors what
// applyOpencodePermissions would tell the agent to do.
type workerArtifactDriver struct {
	be       *epicFakeBackend
	beadID   string
	worktree string
	stateDir string
}

func (d *workerArtifactDriver) RunBead(_ context.Context, _ RunBeadInput) (RunBeadResult, error) {
	bd, _ := d.be.Get(d.beadID, "")
	switch bd.State {
	case "implementation":
		if err := commitRealChange(d.worktree, "work.txt", "did the work\n", "stage: implementation: did the work"); err != nil {
			return RunBeadResult{Success: false}, fmt.Errorf("implementation commit: %w", err)
		}
		dir := filepath.Join(d.stateDir, "run", d.beadID, d.beadID)
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, "decision-record.json"), []byte(fakeDecisionRecord), 0o644)
	case "implementation_review":
		dir := filepath.Join(d.stateDir, "run", d.beadID, d.beadID)
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

	stateDir := t.TempDir()
	cfg := newDriveTestConfig()
	cfg.Vault = config.VaultConfig{Root: t.TempDir()}
	graphPath, err := graphDBFilePath(cfg)
	if err != nil {
		t.Fatalf("graphDBFilePath: %v", err)
	}
	// The worker profile's implementation stage carries a decision_record
	// exit gate, so this drive writes a real Decision node - which now needs
	// the run it belongs to to already exist.
	runID := seedRunAtPath(t, graphPath, "Child", []BeadRef{
		{ID: "kernl-c1", Title: "Child", TrackerKind: "bd", RepoPath: "/repo"},
	})

	res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand: "bd",
		StateDir:       stateDir,
		VerifyCommand:  "bin/ci",
		Backend:        be,
		Driver:         &workerArtifactDriver{be: be, beadID: "kernl-c1", worktree: worktree, stateDir: stateDir},
		Config:         cfg,
		BeadID:         "kernl-c1",
		RepoPath:       t.TempDir(),
		Worktree:       worktree,
		RunID:          runID,
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
		TrackerCommand: "bd",
		StateDir:       t.TempDir(),
		VerifyCommand:  "bin/ci",
		Backend:        be,
		Driver:         &silentDriver{},
		Config:         newDriveTestConfig(),
		BeadID:         "kernl-c2",
		RepoPath:       t.TempDir(),
		Worktree:       worktree,
	})
	if res.Success || res.FinalState != "blocked" {
		t.Fatalf("worker should block when implementation leaves no marker commit; got %+v", res)
	}
}

// markerOnlyDriver simulates a worker child that produces the
// commit_marker's own commit but never a decision record - the case
// commit_marker alone cannot distinguish. Its only use is
// TestDriveWorker_BlocksWhenImplementationSkipsDecisionRecord, which needs a
// driver that clears commit_marker while still failing decision_record;
// workerArtifactDriver cannot be reused for that because it always writes
// both together.
type markerOnlyDriver struct {
	worktree   string
	dispatches int
}

func (d *markerOnlyDriver) RunBead(_ context.Context, _ RunBeadInput) (RunBeadResult, error) {
	d.dispatches++
	// Only the first dispatch commits. A gate failure is now retried inside
	// the run, so this driver is asked again, and an agent asked to redo work
	// it already committed has nothing to add - git refuses an empty commit,
	// which would fail this run for a reason that is about the fake rather
	// than about the gate.
	if d.dispatches == 1 {
		if err := commitRealChange(d.worktree, "work.txt", "did the work\n", "stage: implementation: did the work"); err != nil {
			return RunBeadResult{Success: false}, fmt.Errorf("implementation commit: %w", err)
		}
	}
	return RunBeadResult{FinalState: "ok", Success: true, SessionID: "ses"}, nil
}

// TestDriveWorker_BlocksWhenImplementationSkipsDecisionRecord proves the
// decision_record gate on worker's "implementation" state is load-bearing on
// its own, not merely riding along with commit_marker: a driver that writes
// the marker commit but never a decision record must still block. Without
// this test, an evaluation bypass that always reported decision_record as
// passing would go unnoticed - see TestDriveWorker_StopsAtAwaitingIntegration,
// whose driver always writes both artifacts and so cannot tell that gate
// evaluation matters at all.
func TestDriveWorker_BlocksWhenImplementationSkipsDecisionRecord(t *testing.T) {
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
		ID: "kernl-c3", Type: "task", Title: "No Decision Record Child", State: "ready_for_implementation",
		ProfileID: "worker",
	})

	driver := &markerOnlyDriver{worktree: worktree}
	res, _ := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand: "bd",
		StateDir:       t.TempDir(),
		VerifyCommand:  "bin/ci",
		Backend:        be,
		Driver:         driver,
		Config:         newDriveTestConfig(),
		BeadID:         "kernl-c3",
		RepoPath:       t.TempDir(),
		Worktree:       worktree,
	})
	if res.Success || res.FinalState != "blocked" {
		t.Fatalf("worker should block when implementation writes a marker commit but no decision record; got %+v", res)
	}
	// The block is now reached through the retry budget rather than on the
	// first failure, and spending it is bounded: one dispatch plus its
	// retries, never a loop.
	if want := 1 + mechanicalBlockRetryLimit; driver.dispatches != want {
		t.Fatalf("a stage that keeps failing its gate must be dispatched %d times, not %d", want, driver.dispatches)
	}
}

// artifactDriver simulates an agent that produces each epic stage's exit-gate
// artifact, keyed on the bead's current (already-advanced) state. The verdict
// is written to the same artifact directory kernl itself resolves
// (StateDir/run/<epic>/<bead>/, not the worktree).
type artifactDriver struct {
	be       *epicFakeBackend
	epicID   string
	worktree string
	stateDir string
}

func (d *artifactDriver) RunBead(_ context.Context, _ RunBeadInput) (RunBeadResult, error) {
	bd, _ := d.be.Get(d.epicID, "")
	switch bd.State {
	case "integration":
		// No decision-record write here: epic's "integration" does not carry
		// a decision_record gate (see fakeDecisionRecord's doc comment
		// above), so this driver must not manufacture one - doing so would
		// make TestDriveEpic_ReachesAwaitingPRReview pass even if gate
		// evaluation for that state were silently disabled.
		if err := commitRealChange(d.worktree, "merged.txt", "merged children\n", "stage: integration: merged children"); err != nil {
			return RunBeadResult{Success: false}, fmt.Errorf("integration commit: %w", err)
		}
	case "integration_review":
		dir := filepath.Join(d.stateDir, "run", d.epicID, d.epicID)
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

	stateDir := t.TempDir()
	res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand: "bd",
		StateDir:       stateDir,
		VerifyCommand:  "bin/ci",
		Backend:        be,
		Driver:         &artifactDriver{be: be, epicID: "kernl-e1", worktree: worktree, stateDir: stateDir},
		Config:         newDriveTestConfig(),
		BeadID:         "kernl-e1",
		RepoPath:       t.TempDir(),
		Worktree:       worktree,
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
	stateDir := t.TempDir()
	drv := &artifactDriver{be: be, epicID: "kernl-e2", worktree: worktree, stateDir: stateDir}
	noPR := &noPRDriver{artifactDriver: drv}

	res, _ := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand: "bd",
		StateDir:       stateDir,
		VerifyCommand:  "bin/ci",
		Backend:        be,
		Driver:         noPR,
		Config:         newDriveTestConfig(),
		BeadID:         "kernl-e2",
		RepoPath:       t.TempDir(),
		Worktree:       worktree,
	})
	if res.Success || res.FinalState != "blocked" {
		t.Fatalf("epic should block when shipment skips pr_url; got %+v", res)
	}
}

// TestDriveEpic_BlocksOnIntegrationConflict proves the integration commit_marker
// gate stops the epic when the merge agent fails to leave any new commit
// (e.g. an unresolved merge conflict where it bailed).
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
		TrackerCommand: "bd",
		StateDir:       t.TempDir(),
		VerifyCommand:  "bin/ci",
		Backend:        be,
		Driver:         &silentDriver{},
		Config:         newDriveTestConfig(),
		BeadID:         "kernl-e3",
		RepoPath:       t.TempDir(),
		Worktree:       worktree,
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
		// exit zero but do nothing - no pr_url written
		return RunBeadResult{FinalState: "ok", Success: true}, nil
	}
	return d.artifactDriver.RunBead(ctx, in)
}

// TestDriveEpic_DryRunStopsBeforeShipmentButCommitsForReal pins the boundary
// cmd/kernl/epic.go's driveEpic draws for --dry-run: StopBeforeState is
// "shipment", not "integration". Integration and integration_review are not
// previewed, they are actually dispatched, so this asserts the real integration
// commit lands in the worktree and the review artifact is actually written -
// the exact fact an operator has to know before trusting --dry-run not to
// consume the epic's one-shot merge stage (see driveEpic's own doc comment on
// epicAlreadyInTail for why that stage cannot be re-entered once it has run).
// Shipment itself is asserted never to run: no pr_url is ever written.
func TestDriveEpic_DryRunStopsBeforeShipmentButCommitsForReal(t *testing.T) {
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
	commitCount := func() int {
		out, err := exec.Command("git", "-C", worktree, "rev-list", "--count", "HEAD").CombinedOutput()
		if err != nil {
			t.Fatalf("git rev-list: %v: %s", err, out)
		}
		var n int
		if _, err := fmt.Sscanf(string(out), "%d", &n); err != nil {
			t.Fatalf("parsing commit count %q: %v", out, err)
		}
		return n
	}
	before := commitCount()

	be := newEpicFakeBackend()
	be.put(&backend.Bead{
		ID: "kernl-e4", Type: "epic", Title: "Test Epic 4", State: "ready_for_integration",
		ProfileID: "epic",
	})

	stateDir := t.TempDir()
	res, err := DriveBeadToTerminal(context.Background(), DriveBeadDeps{
		TrackerCommand:  "bd",
		StateDir:        stateDir,
		VerifyCommand:   "bin/ci",
		Backend:         be,
		Driver:          &artifactDriver{be: be, epicID: "kernl-e4", worktree: worktree, stateDir: stateDir},
		Config:          newDriveTestConfig(),
		BeadID:          "kernl-e4",
		RepoPath:        t.TempDir(),
		Worktree:        worktree,
		StopBeforeState: "shipment",
	})
	if err != nil {
		t.Fatalf("DriveBeadToTerminal: %v", err)
	}
	if !res.Success || res.FinalState != "ready_for_shipment" {
		t.Fatalf("dry-run epic drive = %+v; want success at ready_for_shipment (stopped before shipment runs)", res)
	}
	final, _ := be.Get("kernl-e4", "")
	if final.State != "ready_for_shipment" {
		t.Errorf("epic bead state = %q, want ready_for_shipment", final.State)
	}

	// The whole point of this test: integration is not a preview, it committed
	// for real.
	if after := commitCount(); after != before+1 {
		t.Errorf("worktree has %d commits after dry-run, want %d (base + one real integration merge commit)", after, before+1)
	}
	reviewArtifact := filepath.Join(stateDir, "run", "kernl-e4", "kernl-e4", "integration-review.md")
	if _, err := os.Stat(reviewArtifact); err != nil {
		t.Errorf("integration_review did not run for real under dry-run (no artifact at %s): %v", reviewArtifact, err)
	}

	// And shipment genuinely never ran: no pr_url was ever written.
	if strings.Contains(final.Description, "pr_url") {
		t.Errorf("epic description = %q, dry-run must never reach shipment", final.Description)
	}
}
