// Package merge provides the typed outcome enum and the MergeManager that
// routes epic-level transitions based on the merger agent's reported outcome.
// Spec: docs/2026-05-15-kernl-workflow-brainstorm-spec.md §5.3-§5.4.
package merge

import "fmt"

type Outcome string

const (
	OutcomeSuccess         Outcome = "success"
	OutcomeMergeConflict   Outcome = "merge_conflict"
	OutcomePushFailed      Outcome = "push_failed"
	OutcomePRCreateFailed  Outcome = "pr_create_failed"
	OutcomePRAlreadyExists Outcome = "pr_already_exists"
)

// All returns the full enum — used by the merger prompt template to render the literal list.
func All() []Outcome {
	return []Outcome{
		OutcomeSuccess,
		OutcomeMergeConflict,
		OutcomePushFailed,
		OutcomePRCreateFailed,
		OutcomePRAlreadyExists,
	}
}

func ParseOutcome(s string) (Outcome, error) {
	switch Outcome(s) {
	case OutcomeSuccess, OutcomeMergeConflict, OutcomePushFailed, OutcomePRCreateFailed, OutcomePRAlreadyExists:
		return Outcome(s), nil
	}
	return "", fmt.Errorf("unknown merge_outcome: %q", s)
}
