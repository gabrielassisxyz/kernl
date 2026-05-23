// Package merge provides the typed outcome enum the epic integration and
// shipment stage prompts use to report results (success, merge_conflict,
// push_failed, pr_create_failed, pr_already_exists).
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
