package approvals

import "sync"

type ApprovalRegistry struct {
	mu        sync.RWMutex
	approvals map[string]*ApprovalRequest
}

func NewApprovalRegistry() *ApprovalRegistry {
	return &ApprovalRegistry{
		approvals: make(map[string]*ApprovalRequest),
	}
}

func (r *ApprovalRegistry) Register(approval *ApprovalRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.approvals[approval.ID] = approval
}

func (r *ApprovalRegistry) List(filter ApprovalFilter) []*ApprovalRequest {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*ApprovalRequest
	for _, a := range r.approvals {
		if filter.ActiveOnly && a.Status != "pending" {
			continue
		}
		if filter.RepoPath != "" && a.RepoPath != filter.RepoPath {
			continue
		}
		if filter.Status != "" && a.Status != filter.Status {
			continue
		}
		result = append(result, a)
	}
	return result
}

func (r *ApprovalRegistry) ApplyAction(id, action string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	a, ok := r.approvals[id]
	if !ok {
		return nil
	}
	a.Status = action
	return nil
}

func (r *ApprovalRegistry) DetachSession(sessionID, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, a := range r.approvals {
		if a.SessionID == sessionID && a.Actionable {
			a.Status = "manual_required"
			a.Actionable = false
		}
	}
}
