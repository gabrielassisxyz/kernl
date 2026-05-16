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

// MergeDispatchPort is the boundary used by the epic merge subcommand to
// manually re-dispatch a merger agent for a blocked epic.
type MergeDispatchPort interface {
	TriggerRouter

	// DispatchMerger manually dispatches a merger agent for the given epic,
	// skipping the normal trigger check since the caller already verified
	// that conditions are met.
	DispatchMerger(epicID string) error
}

// Manager implements TriggerRouter and MergeDispatchPort.
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

// DispatchMerger is a no-op until merger orchestration is wired.
func (m *Manager) DispatchMerger(epicID string) error { return nil }
