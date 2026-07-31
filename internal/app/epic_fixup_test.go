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
// findExistingFixupChild (epic_fixup.go) actually exercises.
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
		EpicID: epicID,
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

	// The cap is derived from the real child (List by Parent, checking the
	// label), never from the epic's own description - a description is
	// mutable prose a later edit can erase (see findExistingFixupChild's
	// own doc comment).
	existing, err := findExistingFixupChild(be, "/tmp/repo", "ep-fixup")
	if err != nil {
		t.Fatalf("findExistingFixupChild: %v", err)
	}
	if existing == nil || existing.ID != res.FixupBeadID {
		t.Errorf("findExistingFixupChild = %+v, want the bead just created (%s)", existing, res.FixupBeadID)
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

// TestDriveEpicIntegrationTail_SecondRejectionEscalatesEvenWhenDeclaredFixup
// is the cap: proves a fix-up cannot spawn a fix-up. Without this cap, a
// run spends the day repairing itself and nobody finds out until evening -
// the exact failure mode §7 names.
//
// Inversion: reading this test against DecideFixupAction with the
// "epicAlreadyFixedUp" check removed (or reordered after the Kind switch)
// makes it fail at "expected exactly one Create call (the first one),
// got 2" - a second, real fix-up bead gets created instead of an
// escalation. Confirmed by hand: removing that check's early return lets
// the classification fall through to FixupActionCreateBead because the new
// rejection legitimately declares "fixup" on its own.
func TestDriveEpicIntegrationTail_SecondRejectionEscalatesEvenWhenDeclaredFixup(t *testing.T) {
	be := newFixupFakeBackend()
	deps, artifactDir := epicIntegrationTailDeps(t, be, "ep-cap")
	writeReviewArtifact(t, artifactDir, fixupRejectionRecord)

	first, err := DriveEpicIntegrationTail(context.Background(), deps)
	if err != nil || first.FixupBeadID == "" {
		t.Fatalf("first rejection must create the fix-up bead: res=%+v err=%v", first, err)
	}
	if len(be.created) != 1 {
		t.Fatalf("expected exactly one Create call (the first one), got %d", len(be.created))
	}

	// A later run: the fix-up bead's own worker cycle completed (the
	// executor always drives every child to a terminal state before this
	// tail runs again - see DriveEpicIntegrationTail's own doc comment on
	// alreadyFixedUp), its branch got merged, the epic is back at
	// integration_review, and integration_review rejects again - and this
	// time it ALSO declares "fixup", the same clean declaration as the
	// first time.
	fixup, _ := be.Get(first.FixupBeadID, "")
	fixup.State = "awaiting_integration"
	be.put(fixup)
	epicBead, _ := be.Get("ep-cap", "")
	epicBead.State = "integration_review"
	be.put(epicBead)
	writeReviewArtifact(t, artifactDir, fixupRejectionRecord)

	second, err := DriveEpicIntegrationTail(context.Background(), deps)
	if err == nil {
		t.Fatal("a second rejection on an already-fixed-up epic must escalate, not create a second fix-up bead")
	}
	if !second.Escalated || second.FixupBeadID != "" {
		t.Errorf("got %+v", second)
	}
	if len(be.created) != 1 {
		t.Errorf("expected exactly one Create call (the first one), got %d - a fix-up spawned a fix-up", len(be.created))
	}
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
