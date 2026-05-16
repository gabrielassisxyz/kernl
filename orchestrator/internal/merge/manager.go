package merge

// TriggerRouter defines the boundary that the epic Executor uses to
// interact with the MergeManager.  Concrete orchestration lives in the
// Manager implementation; the Executor only sees this slim interface.
type TriggerRouter interface {
	// TryTrigger is called after every child bead reaches awaiting_integration.
	// It no-ops until all children have reached that state.
	TryTrigger(epicID string)

	// RouteOutcome is called once the epic has finished all children and the
	// merger agent has completed; it decides the final epic-level transition.
	RouteOutcome(epicID string)
}

// Manager implements TriggerRouter.
// Concrete merge orchestration (single-flight mutex, bd polling, merger
// agent spawning, PR creation, etc.) is added in later beads.
type Manager struct{}

// NewManager creates a no-op Manager ready for incremental enrichment.
func NewManager() *Manager {
	return &Manager{}
}

// TryTrigger is a no-op until merger orchestration is wired.
func (m *Manager) TryTrigger(epicID string) {}

// RouteOutcome is a no-op until merger orchestration is wired.
func (m *Manager) RouteOutcome(epicID string) {}
