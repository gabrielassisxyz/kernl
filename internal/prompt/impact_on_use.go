package prompt

import "fmt"

// ImpactOnUseInput carries what RenderImpactOnUse needs to write field 4 of
// a decision record - impact on using the tool - the one field the record's
// implementer never writes (see internal/app/run_report.go's ImpactComposer
// and nodes.Decision.ImpactOnUse's own doc comment on why). OptionsConsidered
// and TradeOffs come from splitting the implementer's Decision.Body back
// apart (decision_record.go's SplitDecisionBody), not from a second field on
// the node.
type ImpactOnUseInput struct {
	DecisionTitle     string
	DecisionContext   string
	OptionsConsidered string
	TradeOffs         string
	Outcome           string
	RepoPath          string
	BeadTitle         string
}

// RenderImpactOnUse returns a prompt asking the LLM to write field 4 of a
// decision record: what the decision changes for someone using the
// software, not what changed in the code. The other four fields are handed
// over as already-decided facts, not questions to re-answer - the composer's
// job is strictly the translation, never a second opinion on the choice
// itself.
func RenderImpactOnUse(in ImpactOnUseInput) string {
	return fmt.Sprintf(`You are writing the fifth field of a decision record for a software repository: "impact on using the tool".

The first four fields were already written by the implementer, at the moment the decision was made, from inside one function. Your job is different: you are looking at the whole run, not one function, so say what this decision changes for someone USING the software - not what changed in the code, and not a restatement of the rationale. If the decision has no user-visible effect at all, say so plainly in one sentence rather than inventing significance it does not have.

Repository: %s
Bead: %s

Decision: %s

What was being decided:
%s

Options considered:
%s

Trade-offs:
%s

Why the chosen option won:
%s

Respond with 2-4 sentences of plain prose. No headings, no bullet points, no restating the question.`,
		in.RepoPath, in.BeadTitle, in.DecisionTitle, in.DecisionContext, in.OptionsConsidered, in.TradeOffs, in.Outcome)
}
