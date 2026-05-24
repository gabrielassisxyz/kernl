package subprocess

// HandoffRequest defines the standard JSON input passed to escape hatch subprocesses.
// Spec §4.4 / U2
type HandoffRequest struct {
	EpicID         string `json:"epic_id"`
	BeadID         string `json:"bead_id"`
	WorktreePath   string `json:"worktree_path"`
	ContextPayload string `json:"context_payload"`
}

// HandoffResponse defines the standard JSON output expected from escape hatch subprocesses.
// The engine reads this from STDOUT.
type HandoffResponse struct {
	ContextPayload string `json:"context_payload"`
}
