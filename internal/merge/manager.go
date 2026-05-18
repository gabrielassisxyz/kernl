package merge

import (
	"log"
	"sync"

	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

type Backend interface {
	ListChildrenAwaitingIntegration(epicID string) ([]string, error)
	CountChildren(epicID string) (int, error)
	UpdateStatus(id string, s workflow.IssueStatus) error
	UpdateState(id string, state string) error
	GetDescription(id string) (string, error)
}

type Dispatcher interface {
	DispatchMerger(epicID string) error
}

type TriggerRouter interface {
	TryTrigger(epicID string) error
	RouteOutcome(epicID string) error
	DispatchMerger(epicID string) error
}

type Manager struct {
	b        Backend
	d        Dispatcher
	inFlight sync.Map
}

func NewManager(b Backend, d Dispatcher) *Manager {
	return &Manager{b: b, d: d}
}

func (m *Manager) TryTrigger(epicID string) error {
	if _, loaded := m.inFlight.LoadOrStore(epicID, true); loaded {
		return nil
	}

	awaiting, err := m.b.ListChildrenAwaitingIntegration(epicID)
	if err != nil {
		m.inFlight.Delete(epicID)
		return err
	}
	total, err := m.b.CountChildren(epicID)
	if err != nil {
		m.inFlight.Delete(epicID)
		return err
	}
	if total == 0 || len(awaiting) < total {
		m.inFlight.Delete(epicID)
		return nil
	}

	if err := m.b.UpdateStatus(epicID, workflow.StatusInProgress); err != nil {
		m.inFlight.Delete(epicID)
		return err
	}
	if err := m.d.DispatchMerger(epicID); err != nil {
		m.inFlight.Delete(epicID)
		return err
	}
	return nil
}

func (m *Manager) DispatchMerger(epicID string) error {
	return m.d.DispatchMerger(epicID)
}

func (m *Manager) RouteOutcome(epicID string) error {
	defer m.inFlight.Delete(epicID)

	desc, err := m.b.GetDescription(epicID)
	if err != nil {
		return err
	}
	outcomeStr := workflow.GetMergeOutcome(desc)
	if outcomeStr == "" {
		log.Printf("ERROR merge: epic %s — merger agent did not report outcome", epicID)
		return m.b.UpdateStatus(epicID, workflow.StatusBlocked)
	}
	outcome, err := ParseOutcome(outcomeStr)
	if err != nil {
		log.Printf("ERROR merge: epic %s — %v", epicID, err)
		return m.b.UpdateStatus(epicID, workflow.StatusBlocked)
	}

	switch outcome {
	case OutcomeSuccess:
		return m.b.UpdateState(epicID, "ready_for_integration")
	case OutcomePRAlreadyExists:
		// Legacy: if the merger somehow already had a PR, skip to awaiting_pr_review.
		children, err := m.b.ListChildrenAwaitingIntegration(epicID)
		if err != nil {
			return err
		}
		for _, c := range children {
			if err := m.b.UpdateStatus(c, workflow.StatusClosed); err != nil {
				return err
			}
		}
		return m.b.UpdateStatus(epicID, workflow.StatusAwaitingPRReview)
	case OutcomeMergeConflict, OutcomePushFailed, OutcomePRCreateFailed:
		return m.b.UpdateStatus(epicID, workflow.StatusBlocked)
	}
	return m.b.UpdateStatus(epicID, workflow.StatusBlocked)
}
