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
	// The breaker is checked once, ahead of both paths below: a cached
	// "merged" epic used to skip straight to closeAll, bypassing the
	// breaker entirely, so an epic whose children never converge kept
	// regenerating the same close attempts (and WARN logs) on every tick
	// forever instead of backing off like a GH failure does.
	s.mu.Lock()
	br := s.breakers[e.ID]
	breakerOpen := br != nil && time.Now().Before(br.openUntil)
	merged := s.mergedCache[e.PRURL]
	s.mu.Unlock()

	if breakerOpen {
		return
	}
	if merged {
		s.closeAll(e, "merged via PR (cached)")
		return
	}

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

	closedThisRun, alreadyClosedChildren, failedChildren := s.closeChildrenUntilConverged(reason, e.Children)

	var epicErr error
	epicAlreadyClosed, epicClosedThisRun := false, false
	// The epic's own close is attempted only once every child either closed
	// this run or was already closed. Attempting it while any child remains
	// unresolved would guarantee a failing close - and with it br's own
	// "--force" hint - on every tick that does not fully converge.
	if len(failedChildren) == 0 {
		if err := s.b.Close(e.ID, reason); err != nil {
			if errors.Is(err, backend.ErrAlreadyClosed) {
				epicAlreadyClosed = true
			} else {
				log.Printf("WARN sweep: failed to close epic %s: %v", e.ID, err)
				epicErr = err
			}
		} else {
			epicClosedThisRun = true
		}
	}

	closedChildren := len(closedThisRun) + len(alreadyClosedChildren)
	epicSkipped := len(failedChildren) > 0
	s.report(closeReceipt(e.ID, closedChildren, len(e.Children), failedChildren, alreadyClosedChildren, epicErr, epicAlreadyClosed, epicSkipped, reason))

	// Feed the outcome into the same breaker/backoff a GH failure already
	// uses: an epic that made no progress at all (every failure this run
	// repeats the exact one from last time) must not regenerate the same
	// close attempts on every tick forever. Any real progress - a child or
	// the epic itself closing that had not before - resets the breaker,
	// because that is the dependency state changing, not the epic being
	// stuck.
	progressed := len(closedThisRun) > 0 || epicClosedThisRun
	resolved := len(failedChildren) == 0 && (epicClosedThisRun || epicAlreadyClosed)
	if resolved || progressed {
		s.recordSuccess(e.ID)
	} else {
		s.recordFailure(e.ID)
	}
}

// closeChildrenUntilConverged repeats a pass over e.Children until a pass
// closes nothing new, so a dependency chain converges within this one run
// instead of needing one sweep tick per level: a child that refuses because
// its own blocker is still open is retried once that blocker resolves,
// possibly in a later pass of the same run. backend.ErrAlreadyClosed is
// still per-child "no progress from this call", but it removes the child
// from the retry set exactly like a real close does - it reached the
// desired end state, just not by this run's doing.
//
// Passes are capped at len(children): that is the deepest chain this many
// children can form, and it also bounds the case where every pass makes
// some progress but never fully empties the retry set.
func (s *Sweeper) closeChildrenUntilConverged(reason string, children []string) (closedThisRun, alreadyClosedChildren []string, failedChildren []failedChild) {
	remaining := children
	for pass := 0; len(remaining) > 0 && pass < len(children); pass++ {
		var stillOpen []string
		var passFailures []failedChild
		for _, c := range remaining {
			err := s.b.Close(c, reason)
			switch {
			case err == nil:
				closedThisRun = append(closedThisRun, c)
			case errors.Is(err, backend.ErrAlreadyClosed):
				alreadyClosedChildren = append(alreadyClosedChildren, c)
			default:
				log.Printf("WARN sweep: failed to close child %s: %v", c, err)
				stillOpen = append(stillOpen, c)
				passFailures = append(passFailures, failedChild{ID: c, Cause: err})
			}
		}
		failedChildren = passFailures
		if len(stillOpen) == len(remaining) {
			// Zero progress this pass: no later pass can do better, so
			// there is no reason to spend the rest of the cap re-asking the
			// same question.
			return
		}
		remaining = stillOpen
	}
	return
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
func closeReceipt(epicID string, closedChildren, totalChildren int, failedChildren []failedChild, alreadyClosedChildren []string, epicErr error, epicAlreadyClosed, epicSkipped bool, reason string) string {
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
	// epicSkipped means closeAll never attempted the epic's own close at
	// all, because a child was still unresolved after every pass - distinct
	// from epicErr above, where the close was attempted and the tracker
	// itself refused it.
	if epicSkipped {
		return fmt.Sprintf("sweep: epic %s not closed - children unresolved, %s - reason: %s", epicID, childDetail, reason)
	}
	// An epic already closed before this run reached it (a re-run after a
	// partial failure, most often) did not get closed BY this run, so the
	// receipt says so instead of claiming a close that did not happen here.
	if epicAlreadyClosed {
		return fmt.Sprintf("sweep: epic %s already closed, %s - reason: %s", epicID, childDetail, reason)
	}
	if len(alreadyClosedChildren) > 0 {
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
