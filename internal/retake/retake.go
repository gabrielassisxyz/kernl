package retake

import (
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

// RetakeTargetState is the canonical state used when reopening a bead for
// regression investigation via ReTake.
var RetakeTargetState = string(workflow.StatusOpen)

var retakeSourceStates = map[string]bool{
	string(workflow.StatusClosed): true,
}

// IsRetakeSourceState returns true if the given state is a valid source for a
// retake operation. Case and whitespace are normalized. Empty strings and
// abandoned states are rejected.
func IsRetakeSourceState(state string) bool {
	normalized := strings.TrimSpace(strings.ToLower(state))
	if normalized == "" {
		return false
	}
	if mapped, ok := legacyToWorkflow[normalized]; ok {
		normalized = mapped
	}
	return retakeSourceStates[normalized]
}

// legacyToWorkflow maps legacy bead state names to workflow.IssueStatus string
// values for backward compatibility.
var legacyToWorkflow = map[string]string{
	"shipped":   string(workflow.StatusClosed),
	"closed":    string(workflow.StatusClosed),
	"done":      string(workflow.StatusClosed),
	"approved":  string(workflow.StatusClosed),
}

// RetakeTerminal is a minimal representation of a running terminal session
// used by retake scoping logic.
type RetakeTerminal struct {
	SessionID string
	BeadID    string
	BeadTitle string
	RepoPath  string
	Status    string
	StartedAt string
}

// RepoScopedBeadKey builds a repo-scoped key for a bead to disambiguate
// duplicate bead IDs across different repositories.
func RepoScopedBeadKey(beadID, repoPath string) string {
	return repoPath + "::" + beadID
}

// BuildRetakeShippingIndex creates a map from repo-scoped bead key to session
// ID for all running terminals. This is used to detect ancestor sessions
// during retake.
func BuildRetakeShippingIndex(terminals []RetakeTerminal) map[string]string {
	acc := make(map[string]string, len(terminals))
	for _, t := range terminals {
		if t.Status != "running" {
			continue
		}
		key := RepoScopedBeadKey(t.BeadID, t.RepoPath)
		acc[key] = t.SessionID
	}
	return acc
}

// FindRunningTerminalForBead returns the first running terminal matching the
// given bead ID within the specified repo scope.
func FindRunningTerminalForBead(terminals []RetakeTerminal, beadID, repoPath string) *RetakeTerminal {
	target := RepoScopedBeadKey(beadID, repoPath)
	for i := range terminals {
		t := &terminals[i]
		if t.Status == "running" && RepoScopedBeadKey(t.BeadID, t.RepoPath) == target {
			return t
		}
	}
	return nil
}

// RetakeBead is a minimal bead representation used by retake parent indexing.
type RetakeBead struct {
	ID       string
	Parent   string
	RepoPath string
}

// BuildRetakeParentIndex creates a map from repo-scoped bead key to its
// repo-scoped parent key (or empty string if no parent).
func BuildRetakeParentIndex(beads []RetakeBead) map[string]string {
	m := make(map[string]string, len(beads))
	for _, b := range beads {
		key := RepoScopedBeadKey(b.ID, b.RepoPath)
		if b.Parent != "" {
			m[key] = RepoScopedBeadKey(b.Parent, b.RepoPath)
		} else {
			m[key] = ""
		}
	}
	return m
}

// HasRollingAncestor walks the parent chain of the given bead and returns
// true if any ancestor has a running session in the shipping index.
func HasRollingAncestor(bead RetakeBead, parentByBeadID map[string]string, shippingIndex map[string]string) bool {
	visited := make(map[string]bool)
	current := RepoScopedBeadKey(bead.ID, bead.RepoPath)
	for {
		if visited[current] {
			return false // cycle detected
		}
		visited[current] = true

		parent, ok := parentByBeadID[current]
		if !ok || parent == "" {
			return false
		}

		if _, rolling := shippingIndex[parent]; rolling {
			return true
		}

		current = parent
	}
}
