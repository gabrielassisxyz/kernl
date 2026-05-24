package chat

// ScopeResult is the result of scope derivation.
type ScopeResult struct {
	NodeIDs []string
}

// DeriveScope returns the node IDs that are in scope.
// If explicitScopeNodeID is non-empty, it is the sole scope.
// If explicitScopeNodeID is empty but currentNodeID is non-empty, currentNodeID is the scope.
// If both are empty, scope is empty.
func DeriveScope(currentNodeID, explicitScopeNodeID string) ScopeResult {
	if explicitScopeNodeID != "" {
		return ScopeResult{NodeIDs: []string{explicitScopeNodeID}}
	}
	if currentNodeID != "" {
		return ScopeResult{NodeIDs: []string{currentNodeID}}
	}
	return ScopeResult{NodeIDs: []string{}}
}
