package prompt

import (
	"fmt"
	"strings"
)

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
	// RepositoryContext is what app.AssembleContext read out of the target
	// repository (its README, its ROADMAP, whatever registry.repos[].contextDocs
	// names) - the Oracle's only way to know what this software is FOR, since
	// it has no tools and no working directory inside the repository (see
	// app.CLIImpactComposer's own doc comment on why). Empty is a legitimate
	// value - no llm.agent/llm.endpoint reached a repository worth reading, or
	// none of the configured docs exist - and is rendered as an explicit line
	// rather than an omitted section, for the same reason RelevantDecisions in
	// internal/app/prompt.go always renders: a silently missing section reads
	// as this feature never having run at all.
	RepositoryContext string
}

// RenderImpactOnUse returns a prompt asking the LLM to write field 4 of a
// decision record: what the decision changes for someone using the
// software, not what changed in the code. The other four fields are handed
// over as already-decided facts, not questions to re-answer - the composer's
// job is strictly the translation, never a second opinion on the choice
// itself. The repository context under its own heading is what makes that
// translation possible instead of a paraphrase of the four fields alone: use
// it to judge what changes for someone using THIS software, not as an
// invitation to re-open the decision it describes.
func RenderImpactOnUse(in ImpactOnUseInput) string {
	return fmt.Sprintf(`You are writing the fifth field of a decision record for a software repository: "impact on using the tool".

The first four fields were already written by the implementer, at the moment the decision was made, from inside one function. Your job is different: you are looking at the whole run, not one function, so say what this decision changes for someone USING the software - not what changed in the code, and not a restatement of the rationale. If the decision has no user-visible effect at all, say so plainly in one sentence rather than inventing significance it does not have.

## Repository context

%s

Use this only to judge what the decision below changes for someone using THIS software. It is not a second opinion to render: the decision is already made, and re-opening it or proposing an alternative is not your job.

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

Respond with 3-6 sentences of plain prose. No headings, no bullet points, no restating the question.`,
		repositoryContextOrExplicitAbsence(in.RepositoryContext),
		in.RepoPath, in.BeadTitle, in.DecisionTitle, in.DecisionContext, in.OptionsConsidered, in.TradeOffs, in.Outcome)
}

// repositoryContextOrExplicitAbsence renders an explicit line in place of an
// empty RepositoryContext, rather than leaving the heading above an empty
// body - see ImpactOnUseInput.RepositoryContext's own doc comment for why an
// omitted section is never the right default here.
func repositoryContextOrExplicitAbsence(context string) string {
	if strings.TrimSpace(context) == "" {
		return "No repository context is available for this run."
	}
	return context
}
