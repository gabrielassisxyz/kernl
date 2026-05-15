package retake

import "strings"

// RetakeTargetState is the canonical state used when reopening a beat for
// regression investigation via ReTake.
const RetakeTargetState = "ready_for_implementation"

var retakeSourceStates = map[string]bool{
	"shipped":  true,
	"closed":   true,
	"done":     true,
	"approved": true,
}

// IsRetakeSourceState returns true if the given state is a valid source for a
// retake operation. Case and whitespace are normalized. Empty strings and
// abandoned states are rejected.
func IsRetakeSourceState(state string) bool {
	normalized := strings.TrimSpace(strings.ToLower(state))
	if normalized == "" {
		return false
	}
	return retakeSourceStates[normalized]
}

// RetakeTerminal is a minimal representation of a running terminal session
// used by retake scoping logic.
type RetakeTerminal struct {
	SessionID string
	BeatID    string
	BeatTitle string
	RepoPath  string
	Status    string
	StartedAt string
}

// RepoScopedBeatKey builds a repo-scoped key for a beat to disambiguate
// duplicate beat IDs across different repositories.
func RepoScopedBeatKey(beatID, repoPath string) string {
	return repoPath + "::" + beatID
}

// BuildRetakeShippingIndex creates a map from repo-scoped beat key to session
// ID for all running terminals. This is used to detect ancestor sessions
// during retake.
func BuildRetakeShippingIndex(terminals []RetakeTerminal) map[string]string {
	acc := make(map[string]string, len(terminals))
	for _, t := range terminals {
		if t.Status != "running" {
			continue
		}
		key := RepoScopedBeatKey(t.BeatID, t.RepoPath)
		acc[key] = t.SessionID
	}
	return acc
}

// FindRunningTerminalForBeat returns the first running terminal matching the
// given beat ID within the specified repo scope.
func FindRunningTerminalForBeat(terminals []RetakeTerminal, beatID, repoPath string) *RetakeTerminal {
	target := RepoScopedBeatKey(beatID, repoPath)
	for i := range terminals {
		t := &terminals[i]
		if t.Status == "running" && RepoScopedBeatKey(t.BeatID, t.RepoPath) == target {
			return t
		}
	}
	return nil
}

// RetakeBeat is a minimal beat representation used by retake parent indexing.
type RetakeBeat struct {
	ID       string
	Parent   string
	RepoPath string
}

// BuildRetakeParentIndex creates a map from repo-scoped beat key to its
// repo-scoped parent key (or empty string if no parent).
func BuildRetakeParentIndex(beats []RetakeBeat) map[string]string {
	m := make(map[string]string, len(beats))
	for _, b := range beats {
		key := RepoScopedBeatKey(b.ID, b.RepoPath)
		if b.Parent != "" {
			m[key] = RepoScopedBeatKey(b.Parent, b.RepoPath)
		} else {
			m[key] = ""
		}
	}
	return m
}

// HasRollingAncestor walks the parent chain of the given beat and returns
// true if any ancestor has a running session in the shipping index.
func HasRollingAncestor(beat RetakeBeat, parentByBeatID map[string]string, shippingIndex map[string]string) bool {
	visited := make(map[string]bool)
	current := RepoScopedBeatKey(beat.ID, beat.RepoPath)
	for {
		if visited[current] {
			return false // cycle detected
		}
		visited[current] = true

		parent, ok := parentByBeatID[current]
		if !ok || parent == "" {
			return false
		}

		if _, rolling := shippingIndex[parent]; rolling {
			return true
		}

		current = parent
	}
}
