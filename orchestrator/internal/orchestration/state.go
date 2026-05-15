package orchestration

import (
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

func ValidNextStates(wf *backend.WorkflowDescriptor, currentState string, rawKnoState ...string) []string {
	normalized := normalizeForTransitions(currentState)
	if normalized == "" {
		return nil
	}

	statesSet := buildNormalizedStateSet(wf)

	legacyState := normalized
	if normalized == "impl" && statesSet["implementation"] {
		legacyState = "implementation"
	}
	if mapped, ok := applyLegacyAliases(normalized, statesSet); ok {
		normalized = mapped
	}

	effectiveState := normalized
	effectiveLegacy := legacyState
	if len(rawKnoState) > 0 {
		rawNormalized := normalizeForTransitions(rawKnoState[0])
		rawLegacy := rawNormalized
		if rawNormalized == "impl" && statesSet["implementation"] {
			rawLegacy = "implementation"
			rawNormalized = "implementation"
		}
		if mapped, ok := applyLegacyAliases(rawNormalized, statesSet); ok {
			rawNormalized = mapped
		}
		if rawNormalized != "" && rawNormalized != normalized {
			effectiveState = rawNormalized
			effectiveLegacy = rawLegacy
		}
	}

	nextStates := make(map[string]bool)
	for _, t := range wf.Transitions {
		fromMatch := t.From == effectiveLegacy || t.From == effectiveState || t.From == "*"
		if fromMatch {
			toState := t.To
			if mapped, ok := legacyToWorkflowState[toState]; ok {
				toState = mapped
			}
			if statesSet[toState] || toState == string(workflow.StatusClosed) {
				nextStates[toState] = true
			}
		}
	}

	delete(nextStates, normalized)
	if len(rawKnoState) > 0 {
		rawNormalized := normalizeForTransitions(rawKnoState[0])
		if rawNormalized == "impl" && statesSet["implementation"] {
			rawNormalized = "implementation"
		}
		if mapped, ok := applyLegacyAliases(rawNormalized, statesSet); ok {
			rawNormalized = mapped
		}
		if rawNormalized != "" {
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

func buildNormalizedStateSet(wf *backend.WorkflowDescriptor) map[string]bool {
	statesSet := make(map[string]bool, len(wf.States)*2)
	for _, s := range wf.States {
		statesSet[s] = true
		if mapped, ok := legacyToWorkflowState[s]; ok {
			statesSet[mapped] = true
		}
	}
	return statesSet
}

func applyLegacyAliases(state string, statesSet map[string]bool) (string, bool) {
	if mapped, ok := legacyToWorkflowState[state]; ok && statesSet[mapped] {
		return mapped, true
	}
	return state, false
}