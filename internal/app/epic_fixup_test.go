package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/config"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

// fixupFakeBackend is a named fake (AGENTS.md §4) covering exactly what
// DriveEpicIntegrationTail exercises: Get/Update/Rewind/List to drive the
// epic bead itself and discover its own children, and a real, persisting
// Create - unlike every other fake BackendPort in this package, whose
// Create is a no-op, because this is the one test that actually needs to
// observe what gets created.
type fixupFakeBackend struct {
	mu       sync.Mutex
	beads    map[string]*backend.Bead
	nextID   int
	created  []backend.CreateBeadInput
	rewound  []struct{ id, state, reason string }
	createFn func(backend.CreateBeadInput) (*backend.Bead, error)
	// rewindFailTimes makes the next N Rewind calls fail with a simulated
	// transient error before succeeding - the shape a real tracker error
	// between "bead created" and "epic rewound" takes.
	rewindFailTimes int
}

func newFixupFakeBackend() *fixupFakeBackend {
	return &fixupFakeBackend{beads: make(map[string]*backend.Bead)}
}

func (b *fixupFakeBackend) put(bd *backend.Bead) { b.beads[bd.ID] = bd }

func (b *fixupFakeBackend) Get(id string, _ string) (*backend.Bead, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if bd, ok := b.beads[id]; ok {
		cp := *bd
		return &cp, nil
	}
	return nil, nil
}

func (b *fixupFakeBackend) Update(id string, in backend.UpdateBeadInput, _ string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	bd, ok := b.beads[id]
	if !ok {
		return nil
	}
	if in.State != "" {
		bd.State = in.State
	}
	if in.Description != "" {
		bd.Description = in.Description
	}
	if in.Acceptance != "" {
		bd.Acceptance = in.Acceptance
	}
	return nil
}

func (b *fixupFakeBackend) Create(in backend.CreateBeadInput, repoPath string) (*backend.Bead, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.created = append(b.created, in)
	if b.createFn != nil {
		return b.createFn(in)
	}
	b.nextID++
	bd := &backend.Bead{
		ID:          "fixup-" + strconv.Itoa(b.nextID),
		Title:       in.Title,
		Description: in.Description,
		Acceptance:  in.Acceptance,
		Type:        in.Type,
		Labels:      in.Labels,
		ParentID:    in.ParentID,
		// A fix-up bead is dispatched under the worker profile like any
		// other epic child - ready_for_implementation is its own real
		// starting state, not an arbitrary test value. The cap logic in
		// epic_fixup.go depends on this NOT being a "cycle complete" state
		// until something later moves it there.
		State: "ready_for_implementation",
	}
	b.beads[bd.ID] = bd
	return bd, nil
}

func (b *fixupFakeBackend) Rewind(id, targetState, reason, _ string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.rewindFailTimes > 0 {
		b.rewindFailTimes--
		return errors.New("simulated: database is locked")
	}
	b.rewound = append(b.rewound, struct{ id, state, reason string }{id, targetState, reason})
	if bd, ok := b.beads[id]; ok {
		bd.State = targetState
	}
	return nil
}

// List answers the Parent filter only - the one shape
// surveyFixupChildren (epic_fixup.go) actually exercises.
func (b *fixupFakeBackend) List(filters *backend.BeadListFilters, _ string) ([]backend.Bead, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if filters == nil || filters.Parent == "" {
		return nil, nil
	}
	var out []backend.Bead
	for _, bd := range b.beads {
		if bd.ParentID == filters.Parent {
			out = append(out, *bd)
		}
	}
	return out, nil
}
func (b *fixupFakeBackend) ListReady(_ *backend.BeadListFilters, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *fixupFakeBackend) Delete(_ string, _ string) error { return nil }
func (b *fixupFakeBackend) Close(_ string, _ string, _ string) (*backend.TerminalState, error) {
	return nil, nil
}
func (b *fixupFakeBackend) MarkTerminal(_, _, _, _ string) error { return nil }
func (b *fixupFakeBackend) Reopen(_, _, _ string) error          { return nil }
func (b *fixupFakeBackend) Search(_ string, _ *backend.BeadListFilters, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *fixupFakeBackend) Query(_ string, _ *backend.BeadQueryOptions, _ string) ([]backend.Bead, error) {
	return nil, nil
}
func (b *fixupFakeBackend) AddDependency(_, _, _ string) error    { return nil }
func (b *fixupFakeBackend) RemoveDependency(_, _, _ string) error { return nil }
func (b *fixupFakeBackend) ListDependencies(_, _ string, _ *backend.DependencyListOptions) ([]backend.BeadDependency, error) {
	return nil, nil
}
func (b *fixupFakeBackend) BuildTakePrompt(_ string, _ *backend.TakePromptOptions, _ string) (*backend.TakePromptResult, error) {
	return nil, nil
}
func (b *fixupFakeBackend) BuildPollPrompt(_ *backend.PollPromptOptions, _ string) (*backend.PollPromptResult, error) {
	return nil, nil
}
func (b *fixupFakeBackend) ListWorkflows(_ string) ([]backend.WorkflowDescriptor, error) {
	return nil, nil
}
func (b *fixupFakeBackend) Comment(_ string, _ string, _ string) error { return nil }
func (b *fixupFakeBackend) Capabilities() backend.BackendCapabilities {
	return backend.BackendCapabilities{}
}

// epicIntegrationTailDeps builds a DriveEpicIntegrationTailDeps whose epic
// bead already sits AT "integration_review" (an action state of the epic
// profile, not a queue state - see backend.builtinProfiles) so
// DriveBeadToTerminal dispatches it directly without a preceding
// "integration" stage the test does not need. The review artifact is left
// for each test to write into the returned directory before calling
// DriveEpicIntegrationTail.
func epicIntegrationTailDeps(t *testing.T, be *fixupFakeBackend, epicID string) (DriveEpicIntegrationTailDeps, string) {
	t.Helper()
	stateDir := t.TempDir()
	// wf:state:integration_review mirrors what a real claim step already
	// wrote before this bead ever reached this action state - required so
	// stateFromStaleLabel can recover it once this test simulates the bead
	// being revisited already "blocked" (see
	// TestDriveEpicIntegrationTail_ResumesRewindAfterATransientFailure).
	be.put(&backend.Bead{ID: epicID, Title: "the epic", State: "integration_review", ProfileID: "epic", Labels: []string{"wf:state:integration_review"}})

	artifactDir := filepath.Join(stateDir, "run", epicID, epicID)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatal(err)
	}

	deps := DriveEpicIntegrationTailDeps{
		EpicID:     epicID,
		EpicBranch: "feat/" + epicID,
		BaseBranch: "master",
		// Nothing published, nothing irreversible touched: the ordinary
		// pre-shipment state, where the reversibility question is the oracle's
		// to answer and not a fact's.
		Inspector: fakeInspector{summary: " 2 files changed, 12 insertions(+)"},
		Judge:     cheapJudge(),
		DriveBeadDeps: DriveBeadDeps{
			Backend:        be,
			Driver:         &scriptedDriver{},
			Config:         newDriveTestConfig(),
			StateDir:       stateDir,
			BeadID:         epicID,
			RepoPath:       "/tmp/repo",
			Worktree:       t.TempDir(),
			VerifyCommand:  "bin/ci",
			TrackerCommand: "br --db /tmp/repo/.beads/beads.db",
		},
	}
	return deps, artifactDir
}

func writeReviewArtifact(t *testing.T, artifactDir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(artifactDir, "integration-review.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --- A PASS verdict, and an unrelated failure, must pass straight through -
// this function adds behavior, it does not change either of these. ---

func TestDriveEpicIntegrationTail_PassPassesThrough(t *testing.T) {
	be := newFixupFakeBackend()
	deps, artifactDir := epicIntegrationTailDeps(t, be, "ep-pass")
	writeReviewArtifact(t, artifactDir, "looks good\n\nVERDICT: PASS")

	// A PASS advances the bead past integration_review into shipment, whose
	// own gate (description_contains "pr_url:") this fake driver never
	// satisfies - that failure is real but belongs to a different stage and
	// is not what this test is about; only the fix-up/escalation fields
	// matter here.
	res, err := DriveEpicIntegrationTail(context.Background(), deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.FixupBeadID != "" || res.Escalated {
		t.Errorf("a PASS at integration_review must never be read as a rejection: %+v", res)
	}
	if len(be.created) != 0 {
		t.Error("a PASS must never create a fix-up bead")
	}
}

func TestDriveEpicIntegrationTail_UnrelatedFailurePassesThrough(t *testing.T) {
	be := newFixupFakeBackend()
	deps, _ := epicIntegrationTailDeps(t, be, "ep-nofile")
	// No integration-review.md written at all - the agent crashed or wrote
	// nothing, a genuine stage failure with no rejection to read.

	res, err := DriveEpicIntegrationTail(context.Background(), deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Error("a missing artifact is a genuine failure, not a success")
	}
	if res.FixupBeadID != "" || res.Escalated {
		t.Errorf("a missing artifact must not be read as a rejection: %+v", res)
	}
	if len(be.created) != 0 {
		t.Error("a missing artifact must never create a fix-up bead")
	}
}

// TestDriveEpicIntegrationTail_StaleRejectionFromADifferentStageIsIgnored is
// the concrete scenario a plain "was FinalState blocked" check cannot tell
// apart from a real, current rejection: the epic was already "blocked" on
// entry (a previous run's failure, not something THIS call produced), the
// stale wf:state:* label names "integration" - a DIFFERENT stage - and a
// leftover integration-review.md from an earlier, unrelated
// integration_review pass still sits at the well-known path ending in a
// valid REJECT verdict. Re-reading that file blind would create a fix-up
// bead for a rejection that has nothing to do with the CURRENT failure.
//
// Inversion: reading this test against DriveEpicIntegrationTail with the
// "res.BlockedAtState != \"integration_review\"" guard removed makes it
// fail at "a stale rejection from a different stage must never create a
// fix-up bead" - len(be.created) becomes 1, because the function falls
// straight through to reading the leftover artifact and finding its old
// REJECT verdict. Confirmed by hand.
func TestDriveEpicIntegrationTail_StaleRejectionFromADifferentStageIsIgnored(t *testing.T) {
	be := newFixupFakeBackend()
	deps, artifactDir := epicIntegrationTailDeps(t, be, "ep-stale")
	// A leftover artifact from an earlier, unrelated integration_review
	// pass - still ending in a valid REJECT verdict.
	writeReviewArtifact(t, artifactDir, fixupRejectionRecord)

	// The epic is already blocked on entry, and the stage that actually
	// caused it (per the stale wf:state:* label) is "integration", not
	// integration_review - a commit_marker failure, say, unrelated to any
	// review verdict.
	epicBead, _ := be.Get("ep-stale", "")
	epicBead.State = "blocked"
	epicBead.Labels = []string{"wf:state:integration"}
	be.put(epicBead)

	res, err := DriveEpicIntegrationTail(context.Background(), deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.FixupBeadID != "" || res.Escalated {
		t.Errorf("a stale rejection from a different stage must never be read as a current one: %+v", res)
	}
	if len(be.created) != 0 {
		t.Error("a stale rejection from a different stage must never create a fix-up bead")
	}
}

// --- The classification wiring: fixup creates a bead and pauses; decision
// and ambiguous escalate; the cap always escalates. ---

func TestDriveEpicIntegrationTail_FixupCreatesLinkedBeadAndRewinds(t *testing.T) {
	be := newFixupFakeBackend()
	deps, artifactDir := epicIntegrationTailDeps(t, be, "ep-fixup")
	writeReviewArtifact(t, artifactDir, fixupRejectionRecord)

	res, err := DriveEpicIntegrationTail(context.Background(), deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.FixupBeadID == "" {
		t.Fatal("expected a fix-up bead id")
	}
	if res.Escalated || !res.Success {
		t.Errorf("a fix-up dispatch is a graceful pause, not an escalation or a failure: %+v", res)
	}
	if len(be.created) != 1 {
		t.Fatalf("expected exactly one Create call, got %d", len(be.created))
	}
	created := be.created[0]
	if created.ParentID != "ep-fixup" {
		t.Errorf("ParentID = %q, want the epic id, so the fix-up bead is a real child integration can pick up", created.ParentID)
	}
	if !HasLabel(created.Labels, IntegrationFixupLabel) {
		t.Errorf("labels = %v, want %q so the report can tell this bead's own decisions apart", created.Labels, IntegrationFixupLabel)
	}
	if created.Acceptance == "" {
		t.Error("the fix-up bead must carry the reviewer's own acceptance criteria")
	}

	// The budget is derived from the real children (List by Parent, checking
	// the label), never from the epic's own description - a description is
	// mutable prose a later edit can erase (see surveyFixupChildren's own
	// doc comment).
	history, err := surveyFixupChildren(be, "/tmp/repo", "ep-fixup")
	if err != nil {
		t.Fatalf("surveyFixupChildren: %v", err)
	}
	if history.Pending == nil || history.Pending.ID != res.FixupBeadID {
		t.Errorf("pending fix-up = %+v, want the bead just created (%s)", history.Pending, res.FixupBeadID)
	}
	if history.Spent != 0 {
		t.Errorf("spent = %d, want 0 - a bead that has not run yet has spent no round", history.Spent)
	}

	epicBead, _ := be.Get("ep-fixup", "")
	if epicBead.State != "ready_for_integration" {
		t.Errorf("epic state = %q, want ready_for_integration so a later `epic run` re-attempts integration", epicBead.State)
	}
	if len(be.rewound) != 1 || be.rewound[0].state != "ready_for_integration" {
		t.Errorf("rewound = %+v", be.rewound)
	}
}

func TestDriveEpicIntegrationTail_DecisionEscalates(t *testing.T) {
	be := newFixupFakeBackend()
	deps, artifactDir := epicIntegrationTailDeps(t, be, "ep-decision")
	writeReviewArtifact(t, artifactDir, decisionRejectionRecord)

	res, err := DriveEpicIntegrationTail(context.Background(), deps)
	if err == nil {
		t.Fatal("a decision rejection must escalate with an error, not report success")
	}
	if !res.Escalated || res.FixupBeadID != "" {
		t.Errorf("got %+v", res)
	}
	if len(be.created) != 0 {
		t.Error("a decision must never create a fix-up bead")
	}
	if !strings.Contains(res.EscalationReason, "Which slug rule wins") {
		t.Errorf("EscalationReason = %q, want the reviewer's own question", res.EscalationReason)
	}
}

func TestDriveEpicIntegrationTail_AmbiguousEscalates(t *testing.T) {
	be := newFixupFakeBackend()
	deps, artifactDir := epicIntegrationTailDeps(t, be, "ep-ambiguous")
	writeReviewArtifact(t, artifactDir, "some prose with no classification at all\n\nVERDICT: REJECT")

	res, err := DriveEpicIntegrationTail(context.Background(), deps)
	if err == nil {
		t.Fatal("an unparseable rejection must escalate, never be guessed into a fix-up")
	}
	if !res.Escalated || res.FixupBeadID != "" {
		t.Errorf("got %+v", res)
	}
	if len(be.created) != 0 {
		t.Error("an ambiguous rejection must never create a fix-up bead")
	}
}

// TestDriveEpicIntegrationTail_SecondCheapRejectionKeepsGoing is the rule
// this gate replaced the one-fix-up cap with: what decides whether a human is
// woken up is how expensive the change would be to undo, not whether an
// automatic budget ran out.
//
// The cap's own justification was that "a reviewer that rejects the same work
// twice is describing something the loop is not going to fix by running
// again". That premise did not hold in practice: the two rejections that
// motivated this were different defects, each pass fixed exactly what it was
// told to, and the escalation cost a day for nothing. Nothing is published at
// this point, so another round is a branch operation, and the run continues.
//
// Inversion: reading this test against a DecideFixupAction that still
// escalates once a single round has been spent makes it fail at "expected a
// second fix-up bead" - len(be.created) stays 1. Confirmed by hand.
func TestDriveEpicIntegrationTail_SecondCheapRejectionKeepsGoing(t *testing.T) {
	be := newFixupFakeBackend()
	deps, artifactDir := epicIntegrationTailDeps(t, be, "ep-second")
	writeReviewArtifact(t, artifactDir, fixupRejectionRecord)

	first, err := DriveEpicIntegrationTail(context.Background(), deps)
	if err != nil || first.FixupBeadID == "" {
		t.Fatalf("first rejection must create the fix-up bead: res=%+v err=%v", first, err)
	}

	// A later run: the first fix-up bead completed its own worker cycle, its
	// branch got merged, the epic is back at integration_review, and the
	// reviewer rejects again - a different defect, declared cleanly as a
	// fix-up.
	completeFixupCycle(t, be, first.FixupBeadID, "ep-second")
	writeReviewArtifact(t, artifactDir, fixupRejectionRecord)

	second, err := DriveEpicIntegrationTail(context.Background(), deps)
	if err != nil {
		t.Fatalf("a second rejection that is cheap to reverse must not escalate: %v", err)
	}
	if second.Escalated || second.FixupBeadID == "" {
		t.Fatalf("got %+v, want a second fix-up bead and no escalation", second)
	}
	if len(be.created) != 2 {
		t.Errorf("expected a second fix-up bead, got %d Create calls", len(be.created))
	}
	if second.ReversibilityCause != GateCheapToReverse {
		t.Errorf("cause = %q, want %q recorded on the path that continued", second.ReversibilityCause, GateCheapToReverse)
	}
	if second.ReversibilityReason == "" {
		t.Error("a gate that kept going without saying why is the failure this replaced, in the other direction")
	}
}

// The budget is what keeps "cheap to reverse" from being an unbounded loop: a
// reviewer can always find something, and a run that repairs itself all day
// while nobody finds out is the failure the old cap aimed at, even though the
// cap itself was set at the wrong number.
func TestDriveEpicIntegrationTail_ExhaustedBudgetEscalates(t *testing.T) {
	be := newFixupFakeBackend()
	deps, artifactDir := epicIntegrationTailDeps(t, be, "ep-budget")

	for round := 0; round < config.DefaultFixupBudget; round++ {
		writeReviewArtifact(t, artifactDir, fixupRejectionRecord)
		res, err := DriveEpicIntegrationTail(context.Background(), deps)
		if err != nil || res.FixupBeadID == "" {
			t.Fatalf("round %d must still be inside the budget: res=%+v err=%v", round, res, err)
		}
		completeFixupCycle(t, be, res.FixupBeadID, "ep-budget")
	}

	writeReviewArtifact(t, artifactDir, fixupRejectionRecord)
	spent, err := DriveEpicIntegrationTail(context.Background(), deps)
	if err == nil {
		t.Fatal("a rejection past the fix-up budget must escalate")
	}
	if !spent.Escalated || spent.ReversibilityCause != GateBudgetExhausted {
		t.Errorf("got %+v, want an escalation caused by %q", spent, GateBudgetExhausted)
	}
	if len(be.created) != config.DefaultFixupBudget {
		t.Errorf("expected exactly %d fix-up beads, got %d", config.DefaultFixupBudget, len(be.created))
	}
}

// A branch that already exists outside this machine is not something a branch
// operation can take back, and no opinion is needed to establish that.
func TestDriveEpicIntegrationTail_PublishedBranchEscalates(t *testing.T) {
	be := newFixupFakeBackend()
	deps, artifactDir := epicIntegrationTailDeps(t, be, "ep-published")
	deps.Inspector = fakeInspector{refs: []string{"refs/remotes/origin/feat/ep-published"}, summary: "1 file changed"}
	judge := cheapJudge()
	deps.Judge = judge
	writeReviewArtifact(t, artifactDir, fixupRejectionRecord)

	res, err := DriveEpicIntegrationTail(context.Background(), deps)
	if err == nil {
		t.Fatal("a rejection on a published branch must escalate")
	}
	if res.ReversibilityCause != GatePublished {
		t.Errorf("cause = %q, want %q", res.ReversibilityCause, GatePublished)
	}
	if judge.calls != 0 {
		t.Errorf("the oracle was asked %d times about a branch that is already published", judge.calls)
	}
	if len(be.created) != 0 {
		t.Error("no fix-up bead may be created on the escalating path")
	}
}

// Git failing to answer must never read as "nothing published, nothing
// irreversible touched": those are the two answers that let a run carry on
// alone.
func TestDriveEpicIntegrationTail_UnmeasurableFactsEscalate(t *testing.T) {
	be := newFixupFakeBackend()
	deps, artifactDir := epicIntegrationTailDeps(t, be, "ep-nogit")
	deps.Inspector = fakeInspector{err: errors.New("not a git repository")}
	writeReviewArtifact(t, artifactDir, fixupRejectionRecord)

	res, err := DriveEpicIntegrationTail(context.Background(), deps)
	if err == nil {
		t.Fatal("facts that could not be measured must escalate")
	}
	if res.ReversibilityCause != GateReversibilityUnknown {
		t.Errorf("cause = %q, want %q", res.ReversibilityCause, GateReversibilityUnknown)
	}
	if len(be.created) != 0 {
		t.Error("no fix-up bead may be created while the facts are unknown")
	}
}

// completeFixupCycle moves a fix-up bead through its own worker cycle and puts
// the epic back at integration_review: the state a later `epic run` finds once
// the executor has driven every child to a terminal state.
func completeFixupCycle(t *testing.T, be *fixupFakeBackend, fixupID, epicID string) {
	t.Helper()
	fixup, _ := be.Get(fixupID, "")
	fixup.State = "awaiting_integration"
	be.put(fixup)
	epicBead, _ := be.Get(epicID, "")
	epicBead.State = "integration_review"
	be.put(epicBead)
}

// TestDriveEpicIntegrationTail_ResumesRewindAfterATransientFailure is the
// concrete scenario of a fix-up bead created but the epic's own rewind
// failing transiently: Create succeeds, Rewind does not. The error message
// this produces says retrying is safe - this test proves it actually is,
// by calling DriveEpicIntegrationTail again with nothing else changed and
// checking that it resumes (reuses the already-created bead and retries
// only the rewind) rather than either creating a second fix-up bead or
// escalating as though the cap had already been used.
//
// Inversion: reading this test against DriveEpicIntegrationTail with
// alreadyFixedUp computed as "existingFixup != nil" (no cycle-completion
// check) makes the second call's own assertions fail: err becomes non-nil
// ("a second rejection ... must escalate" would fire) and
// second.FixupBeadID stays empty, because a freshly-created fix-up bead
// sitting at ready_for_implementation would be read as "the cap was
// already used" instead of "the previous rewind never completed".
// Confirmed by hand.
func TestDriveEpicIntegrationTail_ResumesRewindAfterATransientFailure(t *testing.T) {
	be := newFixupFakeBackend()
	deps, artifactDir := epicIntegrationTailDeps(t, be, "ep-rewind-retry")
	writeReviewArtifact(t, artifactDir, fixupRejectionRecord)
	be.rewindFailTimes = 1

	first, err := DriveEpicIntegrationTail(context.Background(), deps)
	if err == nil {
		t.Fatal("expected the simulated rewind failure to surface")
	}
	if first.FixupBeadID != "" {
		t.Errorf("a failed rewind must not report a fix-up id as if it succeeded, got %+v", first)
	}
	if len(be.created) != 1 {
		t.Fatalf("expected exactly one Create call, got %d", len(be.created))
	}
	epicBead, _ := be.Get("ep-rewind-retry", "")
	if epicBead.State == "ready_for_integration" {
		t.Fatal("the epic must still be blocked - the rewind that would have moved it never succeeded")
	}

	// Retry, with nothing else changed: same epic bead (still blocked, per
	// the check above), same rejection artifact, and the fix-up bead
	// created a moment ago still sitting at ready_for_implementation
	// (nothing has driven it - this test calls DriveEpicIntegrationTail
	// directly, bypassing the executor that would normally do so before
	// this tail ever runs again).
	second, err := DriveEpicIntegrationTail(context.Background(), deps)
	if err != nil {
		t.Fatalf("the retry must succeed once the simulated tracker issue clears: %v", err)
	}
	if second.Escalated {
		t.Error("a pending (not yet cycle-complete) fix-up child must never be read as the cap already having been used")
	}
	if second.FixupBeadID == "" {
		t.Fatal("expected the retry to report the (reused) fix-up bead id")
	}
	if len(be.created) != 1 {
		t.Errorf("expected still exactly one Create call after the retry, got %d - the retry created a duplicate fix-up bead", len(be.created))
	}
	if len(be.rewound) != 1 {
		t.Errorf("expected exactly one recorded (successful) rewind, got %d", len(be.rewound))
	}
}

// TestDriveEpicIntegrationTail_ResumesFromPartialCreateFailure is the other
// partial-failure boundary: br create's own internal acceptance/notes
// follow-up can fail even though the issue itself was created (see
// backend.CreatePartialError). This proves the caller resumes from the
// bead that error carries - filling in the acceptance criteria and
// finishing the rewind - instead of discarding it and reporting a plain
// failure that would make a caller's retry create a duplicate.
func TestDriveEpicIntegrationTail_ResumesFromPartialCreateFailure(t *testing.T) {
	be := newFixupFakeBackend()
	deps, artifactDir := epicIntegrationTailDeps(t, be, "ep-partial-create")
	writeReviewArtifact(t, artifactDir, fixupRejectionRecord)

	be.createFn = func(in backend.CreateBeadInput) (*backend.Bead, error) {
		partial := &backend.Bead{
			ID:       "fixup-partial-1",
			Title:    in.Title,
			Type:     in.Type,
			Labels:   in.Labels,
			ParentID: in.ParentID,
			State:    "ready_for_implementation",
			// Acceptance deliberately left unset: the simulated failure is
			// br create's own second, internal call (writing
			// acceptance/notes), which is what did not happen here.
		}
		be.beads[partial.ID] = partial
		return nil, backend.NewCreatePartialError(partial, errors.New("simulated: database is locked"))
	}

	res, err := DriveEpicIntegrationTail(context.Background(), deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.FixupBeadID != "fixup-partial-1" {
		t.Errorf("FixupBeadID = %q, want the bead the partial failure carried", res.FixupBeadID)
	}
	fixup, _ := be.Get("fixup-partial-1", "")
	if fixup.Acceptance == "" {
		t.Error("the fix-up bead must have its acceptance criteria filled in even though Create's own internal write failed")
	}
	if len(be.rewound) != 1 {
		t.Errorf("expected exactly one rewind, got %d", len(be.rewound))
	}
}
