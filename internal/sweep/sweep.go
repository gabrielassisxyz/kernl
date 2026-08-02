package sweep

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

type Epic struct {
	ID       string
	PRURL    string
	Children []string
}

type PRState struct {
	State     string
	MergedAt  time.Time
	CreatedAt time.Time
}

type Backend interface {
	ListEpicsAwaitingPRReview() ([]Epic, error)
	Close(id, reason string) error
}

type GH interface {
	View(prURL string) (PRState, error)
}

type Config struct {
	DryRun           bool
	FailureThreshold int
	BackoffMinutes   []int
	PRStaleWarnDays  int
	WarnHook         func(msg string)
	// ReportHook receives the human-facing account of what the sweep did:
	// the dry-run preview, the no-pr_url skip, the not-yet-merged notice,
	// and the close receipt. It exists so a CLI caller can route this to
	// stdout (where the rest of the CLI's user-facing output goes) while
	// internal/sweep stays free of a direct os.Stdout write and so hermetic
	// tests can assert on it. Nil falls back to the package logger, matching
	// WarnHook's precedent.
	ReportHook func(msg string)
}

type breaker struct {
	failures  int
	openUntil time.Time
}

type Sweeper struct {
	b           Backend
	gh          GH
	cfg         Config
	mu          sync.Mutex
	mergedCache map[string]bool
	breakers    map[string]*breaker
}

func New(b Backend, gh GH, cfg Config) *Sweeper {
	if cfg.FailureThreshold == 0 {
		cfg.FailureThreshold = 3
	}
	if len(cfg.BackoffMinutes) == 0 {
		cfg.BackoffMinutes = []int{5, 15, 60}
	}
	return &Sweeper{
		b:           b,
		gh:          gh,
		cfg:         cfg,
		mergedCache: map[string]bool{},
		breakers:    map[string]*breaker{},
	}
}

func (s *Sweeper) Tick() error {
	epics, err := s.b.ListEpicsAwaitingPRReview()
	if err != nil {
		return err
	}
	if len(epics) == 0 {
		return nil
	}
	for _, e := range epics {
		s.processEpic(e)
	}
	return nil
}

func (s *Sweeper) processEpic(e Epic) {
	if e.PRURL == "" {
		// Not a WARN: "WARN" is what the stderr-only messages in this file
		// use, and this line goes to ReportHook (stdout in the CLI) so it
		// can explain an otherwise-empty run. Keeping the WARN prefix on a
		// stdout line would make the prefix stop meaning "check stderr".
		s.report(fmt.Sprintf("sweep: epic %s has no pr_url yet - skipping until the shipment stage writes one", e.ID))
		return
	}
	s.mu.Lock()
	if s.mergedCache[e.PRURL] {
		s.mu.Unlock()
		s.closeAll(e, "merged via PR (cached)")
		return
	}
	br := s.breakers[e.ID]
	if br != nil && time.Now().Before(br.openUntil) {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	state, err := s.gh.View(e.PRURL)
	if err != nil {
		s.recordFailure(e.ID)
		log.Printf("WARN sweep: gh pr view failed for epic %s: %v", e.ID, err)
		return
	}
	s.recordSuccess(e.ID)

	if s.cfg.PRStaleWarnDays > 0 && state.State == "OPEN" {
		if days := int(time.Since(state.CreatedAt).Hours() / 24); days > s.cfg.PRStaleWarnDays {
			msg := fmt.Sprintf("WARN sweep: PR %s open for %d days (epic %s)", e.PRURL, days, e.ID)
			if s.cfg.WarnHook != nil {
				s.cfg.WarnHook(msg)
			} else {
				log.Println(msg)
			}
		}
	}

	if state.State == "MERGED" {
		s.mu.Lock()
		s.mergedCache[e.PRURL] = true
		s.mu.Unlock()
		s.closeAll(e, "merged via PR "+e.PRURL+" at "+state.MergedAt.UTC().Format(time.RFC3339))
		return
	}

	// Neither closeAll's receipt nor the no-pr_url skip fires here, so
	// without this line a correctly-declining sweep (PR still open) and an
	// empty sweep (nothing to check) both print nothing - an operator
	// watching stdout cannot tell them apart. Same stdout/ReportHook
	// treatment as the skip above: this is an account of what sweep found,
	// not a WARN.
	s.report(fmt.Sprintf("sweep: epic %s not closed - PR %s is %s", e.ID, e.PRURL, state.State))
}

func (s *Sweeper) closeAll(e Epic, reason string) {
	if s.cfg.DryRun {
		s.report(fmt.Sprintf("sweep[dry-run]: would close epic %s and %d children", e.ID, len(e.Children)))
		return
	}

	// Track what actually closed, not what was attempted: a receipt that
	// counts a failed child as closed is worse than no receipt at all. The
	// failing ids are collected (not just counted) so the receipt names
	// them directly instead of pointing at a WARN on a different stream -
	// the receipt goes through ReportHook (stdout in the CLI), the WARN
	// stays on log (stderr), and "see above" is not true across streams.
	//
	// A close that reports backend.ErrAlreadyClosed reached the desired end
	// state before this run touched it - a re-run after a partial failure
	// hits this on every bead the previous run did manage to close. That is
	// not a failure, but it is also not something this run gets credit for,
	// so it is tracked apart from both the ordinary closes and the failures.
	var failedChildren []failedChild
	var alreadyClosedChildren []string
	for _, c := range e.Children {
		err := s.b.Close(c, reason)
		switch {
		case err == nil:
		case errors.Is(err, backend.ErrAlreadyClosed):
			alreadyClosedChildren = append(alreadyClosedChildren, c)
		default:
			log.Printf("WARN sweep: failed to close child %s: %v", c, err)
			failedChildren = append(failedChildren, failedChild{ID: c, Cause: err})
		}
	}

	var epicErr error
	epicAlreadyClosed := false
	if err := s.b.Close(e.ID, reason); err != nil {
		if errors.Is(err, backend.ErrAlreadyClosed) {
			epicAlreadyClosed = true
		} else {
			log.Printf("WARN sweep: failed to close epic %s: %v", e.ID, err)
			epicErr = err
		}
	}

	closedChildren := len(e.Children) - len(failedChildren)
	s.report(closeReceipt(e.ID, closedChildren, len(e.Children), failedChildren, alreadyClosedChildren, epicErr, epicAlreadyClosed, reason))
}

// failedChild pairs a child that genuinely refused to close with the reason
// it gave. The id alone was enough while every failure was equally opaque,
// but a close now fails for reasons that differ in what the operator has to
// do next - an unreachable tracker is retried, an epic holding open children
// is not - and the receipt is the only account of the run that reaches
// stdout.
type failedChild struct {
	ID    string
	Cause error
}

// describeFailedChildren renders each failure as "id (cause)" so the receipt
// carries the reason itself rather than pointing at the WARN that also
// logged it. The two land on different streams (stdout vs stderr) with no
// ordering between them, which is the same argument closeReceipt's own doc
// comment already makes for naming the ids inline.
func describeFailedChildren(failures []failedChild) []string {
	out := make([]string, 0, len(failures))
	for _, f := range failures {
		if f.Cause == nil {
			out = append(out, f.ID)
			continue
		}
		out = append(out, fmt.Sprintf("%s (%v)", f.ID, f.Cause))
	}
	return out
}

// closeReceipt renders the outcome of closeAll with the same detail the
// dry-run preview gives (epic, child count) plus the reason that justified
// the close. It reports actual counts and names the specific children that
// failed and the epic's own close error inline, rather than referring the
// reader to "the WARN above": the receipt and the WARN land on different
// streams (stdout vs stderr) with no guaranteed ordering between them, so a
// cross-stream position reference can point at a line the reader never
// sees.
func closeReceipt(epicID string, closedChildren, totalChildren int, failedChildren []failedChild, alreadyClosedChildren []string, epicErr error, epicAlreadyClosed bool, reason string) string {
	childDetail := fmt.Sprintf("%d/%d children closed", closedChildren, totalChildren)
	if len(alreadyClosedChildren) > 0 {
		childDetail += fmt.Sprintf(" (already closed: %s)", strings.Join(alreadyClosedChildren, ", "))
	}
	if len(failedChildren) > 0 {
		childDetail += fmt.Sprintf(" (failed: %s)", strings.Join(describeFailedChildren(failedChildren), ", "))
	}

	if epicErr != nil {
		return fmt.Sprintf("sweep: epic %s NOT closed (%v), %s - reason: %s", epicID, epicErr, childDetail, reason)
	}
	// An epic already closed before this run reached it (a re-run after a
	// partial failure, most often) did not get closed BY this run, so the
	// receipt says so instead of claiming a close that did not happen here.
	if epicAlreadyClosed {
		return fmt.Sprintf("sweep: epic %s already closed, %s - reason: %s", epicID, childDetail, reason)
	}
	if len(failedChildren) > 0 || len(alreadyClosedChildren) > 0 {
		return fmt.Sprintf("sweep: closed epic %s, %s - reason: %s", epicID, childDetail, reason)
	}
	return fmt.Sprintf("sweep: closed epic %s and %d children - reason: %s", epicID, totalChildren, reason)
}

// report sends a human-facing message through ReportHook when the caller
// wired one (e.g. the CLI, to land it on stdout), falling back to the
// package logger otherwise so a caller that never sets ReportHook keeps
// today's behavior.
func (s *Sweeper) report(msg string) {
	if s.cfg.ReportHook != nil {
		s.cfg.ReportHook(msg)
		return
	}
	log.Println(msg)
}

func (s *Sweeper) recordFailure(epicID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	br := s.breakers[epicID]
	if br == nil {
		br = &breaker{}
		s.breakers[epicID] = br
	}
	br.failures++
	if br.failures >= s.cfg.FailureThreshold {
		idx := br.failures - s.cfg.FailureThreshold
		if idx >= len(s.cfg.BackoffMinutes) {
			idx = len(s.cfg.BackoffMinutes) - 1
		}
		br.openUntil = time.Now().Add(time.Duration(s.cfg.BackoffMinutes[idx]) * time.Minute)
	}
}

func (s *Sweeper) recordSuccess(epicID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.breakers, epicID)
}
