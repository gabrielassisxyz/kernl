package orchestration

import (
	"strings"

	"github.com/gastownhall/foolery/internal/backend"
)

func ValidNextStates(wf *backend.WorkflowDescriptor, currentState string, rawKnoState ...string) []string {
	normalized := normalizeForTransitions(currentState)
	if normalized == "" {
		return nil
	}

	statesSet := make(map[string]bool, len(wf.States))
	for _, s := range wf.States {
		statesSet[s] = true
	}
	if statesSet["implementation"] && normalized == "impl" {
		normalized = "implementation"
	}

	effectiveState := normalized
	if len(rawKnoState) > 0 {
		rawNormalized := normalizeForTransitions(rawKnoState[0])
		if rawNormalized != "" && statesSet["implementation"] && rawNormalized == "impl" {
			rawNormalized = "implementation"
		}
		if rawNormalized != "" && rawNormalized != normalized {
			effectiveState = rawNormalized
		}
	}

	nextStates := make(map[string]bool)
	for _, t := range wf.Transitions {
		if t.From == effectiveState || t.From == "*" {
			if statesSet[t.To] || t.To == "deferred" || t.To == "abandoned" || t.To == "shipped" {
				nextStates[t.To] = true
			}
		}
	}

	delete(nextStates, normalized)
	if len(rawKnoState) > 0 {
		rawNormalized := normalizeForTransitions(rawKnoState[0])
		if rawNormalized != "" {
			if statesSet["implementation"] && rawNormalized == "impl" {
				rawNormalized = "implementation"
			}
			delete(nextStates, rawNormalized)
		}
	}

	result := make([]string, 0, len(nextStates))
	for s := range nextStates {
		result = append(result, s)
	}
	return result
}

func normalizeForTransitions(state string) string {
	normalized := strings.TrimSpace(strings.ToLower(state))
	if normalized == "" {
		return ""
	}
	return normalized
}