package sweep_test

import (
	"errors"
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
	if err, ok := f.failIDs[id]; ok && err != nil {
		return err
	}
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

func TestSweep_PartialCloseFailure_ReportsActualCount(t *testing.T) {
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
	// c1 failed to close, but c2 and the epic itself must still be attempted
	// and closed - one failing child does not block the rest.
	if len(b.closed) != 2 {
		t.Fatalf("expected c2 and epic e1 closed despite c1 failing, got %v", b.closed)
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
