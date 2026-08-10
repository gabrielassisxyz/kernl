package sweep_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/sweep"
)

type epicRow struct {
	ID       string
	PRURL    string
	Children []string
}

type fakeBackend struct {
	mu      sync.Mutex
	epics   []epicRow
	closed  []string
	failIDs map[string]error
	// blockedBy simulates br's own ordering guard: Close(id) refuses with a
	// typed backend.CloseRefusedError until blockedBy[id] shows up closed -
	// either pre-existing (via failIDs[blocker] = backend.ErrAlreadyClosed)
	// or closed earlier in this same run. It is what lets a fake stand in
	// for a real dependency chain across closeAll's internal passes.
	blockedBy map[string]string
	// closeCalls counts Close attempts per id, so a test can assert a call
	// was skipped entirely (the epic while a child is unresolved) or was not
	// retried past the pass that already gave up on it (zero progress).
	closeCalls map[string]int
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
	if f.closeCalls == nil {
		f.closeCalls = map[string]int{}
	}
	f.closeCalls[id]++
	if err, ok := f.failIDs[id]; ok && err != nil {
		return err
	}
	if blocker, ok := f.blockedBy[id]; ok && !f.resolvedLocked(blocker) {
		return &backend.CloseRefusedError{ID: id, Kind: backend.CloseRefusalOpenDependents, Reason: fmt.Sprintf("blocked by %s", blocker)}
	}
	f.closed = append(f.closed, id)
	return nil
}

// resolvedLocked reports whether id has reached the closed state this run
// can observe - already closed before this run started, or closed by an
// earlier pass of it. Callers must hold f.mu.
func (f *fakeBackend) resolvedLocked(id string) bool {
	if err, ok := f.failIDs[id]; ok && errors.Is(err, backend.ErrAlreadyClosed) {
		return true
	}
	for _, c := range f.closed {
		if c == id {
			return true
		}
	}
	return false
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

func TestSweep_HappyMerged_ReportsWhatClosed(t *testing.T) {
	b := &fakeBackend{epics: []epicRow{{ID: "e1", PRURL: "https://x/pr/1", Children: []string{"c1", "c2"}}}}
	g := &fakeGH{
		responses: map[string]sweep.PRState{"https://x/pr/1": {State: "MERGED", MergedAt: time.Now()}},
		calls:     map[string]int{},
	}
	var reports []string
	s := sweep.New(b, g, sweep.Config{ReportHook: func(msg string) { reports = append(reports, msg) }})
	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected exactly 1 report, got %d: %v", len(reports), reports)
	}
	msg := reports[0]
	if !strings.Contains(msg, "e1") {
		t.Fatalf("receipt missing epic id: %q", msg)
	}
	if !strings.Contains(msg, "2 children") {
		t.Fatalf("receipt missing child count: %q", msg)
	}
	if !strings.Contains(msg, "merged via PR https://x/pr/1") {
		t.Fatalf("receipt missing reason: %q", msg)
	}
}

func TestSweep_PartialCloseFailure_SkipsEpicAndReportsActualCount(t *testing.T) {
	b := &fakeBackend{
		epics:   []epicRow{{ID: "e1", PRURL: "https://x/pr/1", Children: []string{"c1", "c2"}}},
		failIDs: map[string]error{"c1": errors.New("tracker unavailable")},
	}
	g := &fakeGH{
		responses: map[string]sweep.PRState{"https://x/pr/1": {State: "MERGED", MergedAt: time.Now()}},
		calls:     map[string]int{},
	}
	var reports []string
	s := sweep.New(b, g, sweep.Config{ReportHook: func(msg string) { reports = append(reports, msg) }})
	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}
	// c1 fails permanently but must not block c2 - one failing child does
	// not stop the rest. The epic itself, though, must stay open: closing it
	// while c1 remains unresolved would guarantee a failing close (and
	// br's --force hint) on every later sweep tick.
	if len(b.closed) != 1 || b.closed[0] != "c2" {
		t.Fatalf("expected only c2 closed (c1 failed, epic skipped while unresolved), got %v", b.closed)
	}
	if len(reports) != 1 {
		t.Fatalf("expected exactly 1 report, got %d: %v", len(reports), reports)
	}
	if !strings.Contains(reports[0], "1/2 children closed") {
		t.Fatalf("receipt should reflect only 1 of 2 children actually closed, got %q", reports[0])
	}
	// The receipt must name which child failed, not point at a WARN on a
	// different stream (the WARN is stderr, the receipt is stdout in the
	// CLI, and nothing orders one before the other).
	if !strings.Contains(reports[0], "failed: c1") {
		t.Fatalf("receipt should name the failed child id directly, got %q", reports[0])
	}
	if strings.Contains(reports[0], "WARN above") || strings.Contains(reports[0], "see WARN") {
		t.Fatalf("receipt should not refer to a WARN on another stream, got %q", reports[0])
	}
	if strings.Contains(reports[0], "closed epic e1") {
		t.Fatalf("epic close must be skipped while a child is unresolved, got %q", reports[0])
	}
}

func TestSweep_EpicCloseFailure_ReceiptNamesTheError(t *testing.T) {
	b := &fakeBackend{
		epics:   []epicRow{{ID: "e1", PRURL: "https://x/pr/1", Children: []string{"c1"}}},
		failIDs: map[string]error{"e1": errors.New("tracker locked")},
	}
	g := &fakeGH{
		responses: map[string]sweep.PRState{"https://x/pr/1": {State: "MERGED", MergedAt: time.Now()}},
		calls:     map[string]int{},
	}
	var reports []string
	s := sweep.New(b, g, sweep.Config{ReportHook: func(msg string) { reports = append(reports, msg) }})
	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected exactly 1 report, got %d: %v", len(reports), reports)
	}
	// The epic's own close error must be readable from the receipt alone,
	// since the WARN carrying it lands on a different stream.
	if !strings.Contains(reports[0], "e1") || !strings.Contains(reports[0], "tracker locked") {
		t.Fatalf("receipt should embed the epic close error, got %q", reports[0])
	}
}

func TestSweep_MissingPRURL_ReportsSkipThroughHook(t *testing.T) {
	b := &fakeBackend{epics: []epicRow{{ID: "e1", PRURL: ""}}}
	g := &fakeGH{responses: map[string]sweep.PRState{}, calls: map[string]int{}}
	var reports []string
	s := sweep.New(b, g, sweep.Config{ReportHook: func(msg string) { reports = append(reports, msg) }})
	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || !strings.Contains(reports[0], "e1") {
		t.Fatalf("expected a skip report naming epic e1, got %v", reports)
	}
	// This line goes to stdout via ReportHook. A "WARN" prefix is reserved
	// for the stderr-only log lines in this package; keeping it here would
	// make the prefix stop meaning "this is on stderr".
	if strings.Contains(reports[0], "WARN") {
		t.Fatalf("skip report on stdout should not carry the WARN prefix reserved for stderr, got %q", reports[0])
	}
}

func TestSweep_OpenPR_ReportsNotClosedThroughHook(t *testing.T) {
	b := &fakeBackend{epics: []epicRow{{ID: "e1", PRURL: "https://x/pr/1"}}}
	g := &fakeGH{
		responses: map[string]sweep.PRState{"https://x/pr/1": {State: "OPEN", CreatedAt: time.Now()}},
		calls:     map[string]int{},
	}
	var reports []string
	s := sweep.New(b, g, sweep.Config{ReportHook: func(msg string) { reports = append(reports, msg) }})
	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(b.closed) != 0 {
		t.Fatalf("OPEN PR should not be closed: %v", b.closed)
	}
	// Without this line, "no epic is waiting" and "one is waiting on an open
	// PR" both print nothing - the defect this test guards against.
	if len(reports) != 1 {
		t.Fatalf("expected exactly 1 report, got %d: %v", len(reports), reports)
	}
	if !strings.Contains(reports[0], "e1") {
		t.Fatalf("report missing epic id: %q", reports[0])
	}
	if !strings.Contains(reports[0], "OPEN") {
		t.Fatalf("report missing PR state: %q", reports[0])
	}
}

func TestSweep_DryRun_ReportsPreviewThroughHook(t *testing.T) {
	b := &fakeBackend{epics: []epicRow{{ID: "e1", PRURL: "https://x/pr/1", Children: []string{"c1"}}}}
	g := &fakeGH{
		responses: map[string]sweep.PRState{"https://x/pr/1": {State: "MERGED", MergedAt: time.Now()}},
		calls:     map[string]int{},
	}
	var reports []string
	s := sweep.New(b, g, sweep.Config{DryRun: true, ReportHook: func(msg string) { reports = append(reports, msg) }})
	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(b.closed) != 0 {
		t.Fatalf("dry-run wrote: %v", b.closed)
	}
	if len(reports) != 1 || !strings.Contains(reports[0], "would close epic e1") {
		t.Fatalf("expected a dry-run preview report, got %v", reports)
	}
}

// A child that was already closed before this sweep ran (a re-run after a
// previous partial failure, most often) reached the desired end state
// without this run's help. It must still count toward the closed total -
// the epic's children did all end up closed - but the receipt must not
// claim this run closed it, so an operator can tell a genuine close apart
// from a no-op one.
func TestSweep_ChildAlreadyClosed_CountsAsClosedAndReceiptSaysSo(t *testing.T) {
	b := &fakeBackend{
		epics:   []epicRow{{ID: "e1", PRURL: "https://x/pr/1", Children: []string{"c1", "c2"}}},
		failIDs: map[string]error{"c1": backend.ErrAlreadyClosed},
	}
	g := &fakeGH{
		responses: map[string]sweep.PRState{"https://x/pr/1": {State: "MERGED", MergedAt: time.Now()}},
		calls:     map[string]int{},
	}
	var reports []string
	s := sweep.New(b, g, sweep.Config{ReportHook: func(msg string) { reports = append(reports, msg) }})
	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected exactly 1 report, got %d: %v", len(reports), reports)
	}
	msg := reports[0]
	if !strings.Contains(msg, "2/2 children closed") {
		t.Fatalf("an already-closed child reached the desired end state and must count toward the total, got %q", msg)
	}
	if !strings.Contains(msg, "already closed: c1") {
		t.Fatalf("receipt should name c1 as already closed rather than claim this run closed it, got %q", msg)
	}
	if strings.Contains(msg, "failed: c1") {
		t.Fatalf("an already-closed child is not a failure, got %q", msg)
	}
}

// A close refused for a reason other than "already closed" (br collapses
// both under the same NOTHING_TO_DO code) is a genuine failure and must stay
// one - the fake backend here returns an ordinary error, standing in for
// whatever real cause the tracker gave, and closeAll must not treat it as
// reaching the desired end state.
func TestSweep_ChildRefusedForOtherReason_StaysAFailure(t *testing.T) {
	b := &fakeBackend{
		epics:   []epicRow{{ID: "e1", PRURL: "https://x/pr/1", Children: []string{"c1"}}},
		failIDs: map[string]error{"c1": errors.New("epic has 1/1 open children (use --force to close anyway)")},
	}
	g := &fakeGH{
		responses: map[string]sweep.PRState{"https://x/pr/1": {State: "MERGED", MergedAt: time.Now()}},
		calls:     map[string]int{},
	}
	var reports []string
	s := sweep.New(b, g, sweep.Config{ReportHook: func(msg string) { reports = append(reports, msg) }})
	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected exactly 1 report, got %d: %v", len(reports), reports)
	}
	msg := reports[0]
	if !strings.Contains(msg, "0/1 children closed") {
		t.Fatalf("a genuinely refused close must not count toward the closed total, got %q", msg)
	}
	if !strings.Contains(msg, "failed: c1") || !strings.Contains(msg, "open children") {
		t.Fatalf("receipt must name the real cause of the failure, got %q", msg)
	}
	if strings.Contains(msg, "already closed") {
		t.Fatalf("a real refusal must not be reported as already closed, got %q", msg)
	}
}

// The epic's own close gets the same already-closed treatment as a child's:
// a re-run that finds the epic itself already closed (from a run that
// succeeded despite an earlier partial failure) must not claim to have
// closed it again.
func TestSweep_EpicAlreadyClosed_ReceiptSaysSo(t *testing.T) {
	b := &fakeBackend{
		epics:   []epicRow{{ID: "e1", PRURL: "https://x/pr/1", Children: []string{"c1"}}},
		failIDs: map[string]error{"e1": backend.ErrAlreadyClosed},
	}
	g := &fakeGH{
		responses: map[string]sweep.PRState{"https://x/pr/1": {State: "MERGED", MergedAt: time.Now()}},
		calls:     map[string]int{},
	}
	var reports []string
	s := sweep.New(b, g, sweep.Config{ReportHook: func(msg string) { reports = append(reports, msg) }})
	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected exactly 1 report, got %d: %v", len(reports), reports)
	}
	msg := reports[0]
	if !strings.Contains(msg, "e1 already closed") {
		t.Fatalf("receipt should say the epic was already closed rather than claim this run closed it, got %q", msg)
	}
	if strings.Contains(msg, "NOT closed") {
		t.Fatalf("an already-closed epic is not a failure, got %q", msg)
	}
}

// The bug this fixes: a three-deep chain used to take one sweep TICK per
// level, because closeAll made exactly one pass and never revisited a child
// that refused because its own blocker was still open. Given in reverse
// dependency order - the worst case, since c3's blocker (c2) has not been
// touched yet when c3 is attempted, and neither has c2's (c1) - convergence
// needs three internal passes, exactly what the pass cap (len(e.Children))
// allows, and it all happens inside this one run instead of three ticks.
func TestSweep_ThreeDeepChainReversedOrder_ConvergesInOneRun(t *testing.T) {
	b := &fakeBackend{
		epics: []epicRow{{ID: "e1", PRURL: "https://x/pr/1", Children: []string{"c3", "c2", "c1"}}},
		blockedBy: map[string]string{
			"c3": "c2",
			"c2": "c1",
		},
	}
	mergedAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	g := &fakeGH{
		responses: map[string]sweep.PRState{"https://x/pr/1": {State: "MERGED", MergedAt: mergedAt}},
		calls:     map[string]int{},
	}
	var reports []string
	s := sweep.New(b, g, sweep.Config{ReportHook: func(msg string) { reports = append(reports, msg) }})
	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(b.closed) != 4 {
		t.Fatalf("expected all 3 children and the epic closed in one run, got %v", b.closed)
	}
	want := "sweep: closed epic e1 and 3 children - reason: merged via PR https://x/pr/1 at 2026-08-10T12:00:00Z"
	if len(reports) != 1 || reports[0] != want {
		t.Fatalf("receipt mismatch:\n got:  %v\n want: [%q]", reports, want)
	}
}

// A pass that closes nothing must not spend the rest of the pass budget
// re-asking the same question: none of these children's blockers is ever
// satisfiable within this run, so a second or third attempt cannot succeed
// either.
func TestSweep_ZeroProgressFirstPass_StopsRetryingImmediately(t *testing.T) {
	b := &fakeBackend{
		epics: []epicRow{{ID: "e1", PRURL: "https://x/pr/1", Children: []string{"c1", "c2", "c3"}}},
		blockedBy: map[string]string{
			"c1": "never-closes",
			"c2": "never-closes",
			"c3": "never-closes",
		},
	}
	g := &fakeGH{
		responses: map[string]sweep.PRState{"https://x/pr/1": {State: "MERGED", MergedAt: time.Now()}},
		calls:     map[string]int{},
	}
	s := sweep.New(b, g, sweep.Config{})
	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"c1", "c2", "c3"} {
		if b.closeCalls[id] != 1 {
			t.Errorf("expected exactly 1 attempt for %s (zero progress on pass 1 must stop the loop), got %d", id, b.closeCalls[id])
		}
	}
}

// A chain and an unrelated permanent failure inside the same epic must not
// interfere with each other: the chain still converges, and the permanent
// failure still fails, without either changing the other's outcome.
func TestSweep_ChainedAndIndependentChildrenMixed_PartialConvergence(t *testing.T) {
	b := &fakeBackend{
		epics:     []epicRow{{ID: "e1", PRURL: "https://x/pr/1", Children: []string{"c2", "c1", "c3", "c4"}}},
		blockedBy: map[string]string{"c2": "c1"},
		failIDs:   map[string]error{"c4": errors.New("tracker unavailable")},
	}
	g := &fakeGH{
		responses: map[string]sweep.PRState{"https://x/pr/1": {State: "MERGED", MergedAt: time.Now()}},
		calls:     map[string]int{},
	}
	var reports []string
	s := sweep.New(b, g, sweep.Config{ReportHook: func(msg string) { reports = append(reports, msg) }})
	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}
	closedSet := map[string]bool{}
	for _, c := range b.closed {
		closedSet[c] = true
	}
	for _, want := range []string{"c1", "c2", "c3"} {
		if !closedSet[want] {
			t.Errorf("expected %s closed, got %v", want, b.closed)
		}
	}
	if closedSet["e1"] {
		t.Errorf("epic must stay open while c4 is unresolved, got %v", b.closed)
	}
	if len(reports) != 1 {
		t.Fatalf("expected exactly 1 report, got %d: %v", len(reports), reports)
	}
	if !strings.Contains(reports[0], "3/4 children closed") {
		t.Fatalf("expected 3/4 children closed, got %q", reports[0])
	}
	if !strings.Contains(reports[0], "failed: c4") {
		t.Fatalf("expected c4 named as the failure, got %q", reports[0])
	}
}

// closeAll must not attempt the epic's own close at all while a child
// remains unresolved - not merely fail to report it as closed. Attempting it
// anyway would guarantee a failing close, and with it br's own --force hint,
// on every tick that does not fully converge.
func TestSweep_EpicCloseSkipped_WhileChildUnresolved(t *testing.T) {
	b := &fakeBackend{
		epics:   []epicRow{{ID: "e1", PRURL: "https://x/pr/1", Children: []string{"c1"}}},
		failIDs: map[string]error{"c1": errors.New("tracker unavailable")},
	}
	g := &fakeGH{
		responses: map[string]sweep.PRState{"https://x/pr/1": {State: "MERGED", MergedAt: time.Now()}},
		calls:     map[string]int{},
	}
	s := sweep.New(b, g, sweep.Config{})
	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}
	if b.closeCalls["e1"] != 0 {
		t.Fatalf("epic close must not be attempted while a child is unresolved, got %d attempts", b.closeCalls["e1"])
	}
}

// A typed close refusal (BrCliBackend's real return for an ordering
// constraint - see internal/backend/close_refusal.go) is redacted at its
// source: its Reason never carries br's own "--force" suggestion. Sweep does
// not need to know the refusal is typed to keep that redaction intact - it
// only has to embed the error's own text, which is exactly what the receipt
// already does for every failure.
func TestSweep_TypedOrderingRefusal_ReceiptOmitsForceHint(t *testing.T) {
	b := &fakeBackend{
		epics: []epicRow{{ID: "e1", PRURL: "https://x/pr/1", Children: []string{"c1"}}},
		// c1 is blocked by a bead outside this epic's own children, so this
		// run has no way to close it - the refusal never resolves.
		blockedBy: map[string]string{"c1": "blocker-not-in-epic"},
	}
	g := &fakeGH{
		responses: map[string]sweep.PRState{"https://x/pr/1": {State: "MERGED", MergedAt: time.Now()}},
		calls:     map[string]int{},
	}
	var reports []string
	s := sweep.New(b, g, sweep.Config{ReportHook: func(msg string) { reports = append(reports, msg) }})
	if err := s.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected exactly 1 report, got %d: %v", len(reports), reports)
	}
	msg := reports[0]
	if !strings.Contains(msg, "blocked by blocker-not-in-epic") {
		t.Fatalf("receipt should name the real ordering cause, got %q", msg)
	}
	if strings.Contains(msg, "--force") {
		t.Fatalf("a typed ordering refusal must never surface br's --force suggestion, got %q", msg)
	}
}

// A permanently unresolved epic must not regenerate the same close attempts
// (and WARN logs) on every sweep tick forever. This exercises the cached
// "PR already merged" path specifically, because that path used to call
// closeAll on every tick with no breaker gate at all - the mergedCache hit
// returned before the breaker check ever ran.
func TestSweep_PermanentCloseFailure_BreakerStopsRetryingCachedEpic(t *testing.T) {
	b := &fakeBackend{
		epics:   []epicRow{{ID: "e1", PRURL: "https://x/pr/1", Children: []string{"c1"}}},
		failIDs: map[string]error{"c1": errors.New("tracker down")},
	}
	g := &fakeGH{
		responses: map[string]sweep.PRState{"https://x/pr/1": {State: "MERGED", MergedAt: time.Now()}},
		calls:     map[string]int{},
	}
	s := sweep.New(b, g, sweep.Config{FailureThreshold: 3, BackoffMinutes: []int{5, 15, 60}})
	for i := 0; i < 4; i++ {
		if err := s.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if b.closeCalls["c1"] != 3 {
		t.Fatalf("expected exactly 3 close attempts (the breaker opens on the 3rd failure and must skip the 4th tick), got %d", b.closeCalls["c1"])
	}
}
