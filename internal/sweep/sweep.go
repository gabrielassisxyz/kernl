package sweep

import (
	"fmt"
	"log"
	"sync"
	"time"
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
	// the dry-run preview, the no-pr_url skip, and the close receipt. It
	// exists so a CLI caller can route this to stdout (where the rest of
	// the CLI's user-facing output goes) while internal/sweep stays free
	// of a direct os.Stdout write and so hermetic tests can assert on it.
	// Nil falls back to the package logger, matching WarnHook's precedent.
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
		s.report(fmt.Sprintf("WARN sweep: epic %s in awaiting_pr_review without pr_url - skipping", e.ID))
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
	}
}

func (s *Sweeper) closeAll(e Epic, reason string) {
	if s.cfg.DryRun {
		s.report(fmt.Sprintf("sweep[dry-run]: would close epic %s and %d children", e.ID, len(e.Children)))
		return
	}

	// Track what actually closed, not what was attempted: a receipt that
	// counts a failed child as closed is worse than no receipt at all.
	closedChildren := 0
	for _, c := range e.Children {
		if err := s.b.Close(c, reason); err != nil {
			log.Printf("WARN sweep: failed to close child %s: %v", c, err)
			continue
		}
		closedChildren++
	}

	epicClosed := true
	if err := s.b.Close(e.ID, reason); err != nil {
		log.Printf("WARN sweep: failed to close epic %s: %v", e.ID, err)
		epicClosed = false
	}

	s.report(closeReceipt(e.ID, closedChildren, len(e.Children), epicClosed, reason))
}

// closeReceipt renders the outcome of closeAll with the same detail the
// dry-run preview gives (epic, child count) plus the reason that justified
// the close. It reports actual counts, since this is the only path a
// previous run had zero output on success and the operator could not tell
// a full sweep from a no-op without checking the tracker directly.
func closeReceipt(epicID string, closedChildren, totalChildren int, epicClosed bool, reason string) string {
	if !epicClosed {
		return fmt.Sprintf("sweep: epic %s NOT closed (see WARN above), %d/%d children closed - reason: %s",
			epicID, closedChildren, totalChildren, reason)
	}
	if closedChildren < totalChildren {
		return fmt.Sprintf("sweep: closed epic %s, %d/%d children closed (%d failed, see WARN above) - reason: %s",
			epicID, closedChildren, totalChildren, totalChildren-closedChildren, reason)
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
