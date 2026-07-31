// Package review is the fixed vocabulary an integration_review rejection
// must use, the counterpart to internal/merge (the merge_outcome vocabulary
// the integration and shipment stages already use). The prompt that asks the
// reviewer to declare a Kind and the code that later parses it both import
// this package instead of each hardcoding the same two literal strings, so
// the two can never drift apart the way a hand-typed enum in two files
// eventually does.
//
// See local/artifacts/orchestrator-autonomy-decision-model.md §7: the
// reviewer's own test is "can the acceptance criteria be written without
// choosing anything?" - Fixup when yes, Decision when no. A declaration this
// package cannot parse (missing, or any other token) is deliberately not a
// third Kind value here - it is the caller's job to treat "did not parse" as
// ambiguous and escalate, per §7's own rule that an ambiguous case must never
// be guessed into either bucket.
package review

// Kind is the reviewer's own classification of a rejection.
type Kind string

const (
	// KindFixup means the defect and its correct behavior are both already
	// known - the acceptance criteria can be written without choosing
	// anything - so the correction runs as an ordinary bead, with normal
	// autonomy.
	KindFixup Kind = "fixup"
	// KindDecision means answering the defect requires choosing something
	// nothing already determined - so it is a gate for the operator, not a
	// bead any pool may pick up.
	KindDecision Kind = "decision"
)

// All returns the full enum - used by the integration_review prompt template
// to render the literal list, the same way merge.All() does for
// merge_outcome.
func All() []Kind {
	return []Kind{KindFixup, KindDecision}
}

// Parse validates s against the fixed vocabulary. It returns ok=false for
// anything else, including empty - the caller decides what "not a real Kind"
// means (see this package's own doc comment: that decision belongs to
// internal/app, not here).
func Parse(s string) (kind Kind, ok bool) {
	switch Kind(s) {
	case KindFixup, KindDecision:
		return Kind(s), true
	}
	return "", false
}
