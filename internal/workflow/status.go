// Package workflow defines the kernl issue lifecycle.
// Spec: docs/2026-05-15-kernl-workflow-brainstorm-spec.md §4.
package workflow

type IssueStatus string

const (
	StatusOpen                IssueStatus = "open"
	StatusInProgress          IssueStatus = "in_progress"
	StatusAwaitingIntegration IssueStatus = "awaiting_integration"
	StatusAwaitingPRReview    IssueStatus = "awaiting_pr_review"
	StatusBlocked             IssueStatus = "blocked"
	StatusClosed              IssueStatus = "closed"
)

// KernlCustomStatuses is the exact list registered via EnsureCustomStatuses.
// Order is significant: it is the value passed to `bd config set status.custom`.
var KernlCustomStatuses = []string{
	string(StatusAwaitingIntegration),
	string(StatusAwaitingPRReview),
}

func (s IssueStatus) IsTerminal() bool          { return s == StatusClosed }
func (s IssueStatus) IsClaimableByWorker() bool { return s == StatusOpen }
func (s IssueStatus) HaltsEpic() bool           { return s == StatusBlocked }
func (s IssueStatus) IsCustom() bool {
	return s == StatusAwaitingIntegration || s == StatusAwaitingPRReview
}

type AgentState string

const (
	AgentSpawning AgentState = "spawning"
	AgentWorking  AgentState = "working"
	AgentDone     AgentState = "done"
	AgentStuck    AgentState = "stuck"
	AgentFailed   AgentState = "failed"
)

// IsValidCombination encodes the truth table from spec §4.6 (TT5=A).
// Returns false for combinations that runtime must never produce.
func IsValidCombination(s IssueStatus, a AgentState) bool {
	switch s {
	case StatusOpen:
		return false
	case StatusInProgress:
		return true
	case StatusAwaitingIntegration, StatusAwaitingPRReview, StatusClosed:
		return a == AgentDone
	case StatusBlocked:
		return a == AgentStuck || a == AgentFailed
	}
	return false
}
