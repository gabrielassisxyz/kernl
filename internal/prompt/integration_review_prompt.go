package prompt

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/gabrielassisxyz/kernl/internal/review"
)

// IntegrationReviewInput feeds the integration_review prompt template.
type IntegrationReviewInput struct {
	EpicID    string
	EpicTitle string
	// ArtifactPath is the already-resolved absolute path to
	// integration-review.md - the exit gate this stage must satisfy reads
	// exactly this file (backend's artifact_verdict gate for state
	// integration_review), so the agent is told the real path rather than
	// the <artifact_dir> placeholder only Go code resolves.
	ArtifactPath string
	// VerifyCommand is how the target repository says "this works".
	VerifyCommand string
	// TrackerCommand is how this repository's tracker is typed from inside a
	// worktree - unused by this template directly today (the reviewer
	// declares its verdict in the artifact file, not the tracker), but kept
	// alongside EpicID/EpicTitle for the same reason RenderIntegration and
	// RenderShipment both require it: a future revision of this template
	// that needs it should not have to thread a new parameter through every
	// call site first.
	TrackerCommand string
}

const integrationReviewTemplate = `You are the kernl integration reviewer for epic {{.EpicID}}: "{{.EpicTitle}}".

Your job: review the just-merged integration branch. Verify every child's merge conflicts were resolved correctly, no regressions were introduced, and the combined codebase passes this repository's own check:
{{.VerifyCommand}}

You may NOT modify any source file (` + "`.go`, `.ts`, `.py`, `.rs`" + `, or any other code) to fix something you find wrong. If the merge and the code it produced are correct, PASS it. If something is genuinely wrong, REJECT it and describe the defect - do not fix it yourself, and do not merge-conflict-resolve your way around it either; that is the integration stage's job, already finished by the time you run.

Write your review to exactly this file: {{.ArtifactPath}}

If everything is correct, end the file with the literal line:
VERDICT: PASS

If something is wrong, the file must ALSO contain these markdown sections before that trailing line:

## Classification

Exactly one word: ` + "`fixup`" + ` or ` + "`decision`" + `. Ask yourself: "can the acceptance criteria for correcting this be written without choosing anything?" If yes - the defect and the correct behavior are both already knowable, e.g. "the manifest is written before the last file is flushed" - answer ` + "`fixup`" + `. If no - answering it means picking between real alternatives, e.g. two children established incompatible rules and choosing one invalidates what the other already produced - answer ` + "`decision`" + `. This choice is yours to make, not the orchestrator's: an unclear or missing answer here is always treated as needing a human, never guessed into a fix-up.

## What is wrong

The defect you found, and why it is wrong - specific enough that someone who did not do this review can act on it.

## Fix-up acceptance criteria

Required ONLY when Classification is ` + "`fixup`" + `. State the criteria a correction must satisfy - written so a different implementer, with no more context than this file, can tell when it is done. Omit this section entirely when Classification is ` + "`decision`" + `.

## Question for the operator

Required ONLY when Classification is ` + "`decision`" + `. State exactly what needs to be decided and why the answer is not already determined by this repository's docs, tests, or precedent. Omit this section entirely when Classification is ` + "`fixup`" + `.

Then end the file with the literal line:
VERDICT: REJECT

The full classification enum, for reference:{{range .Kinds}}
  - {{.}}
{{end}}`

type integrationReviewView struct {
	IntegrationReviewInput
	Kinds []review.Kind
}

var integrationReviewTmpl = template.Must(template.New("integration_review").Parse(integrationReviewTemplate))

// RenderIntegrationReview renders the integration_review-stage prompt for a
// given epic.
func RenderIntegrationReview(in IntegrationReviewInput) (string, error) {
	if in.ArtifactPath == "" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: integration_review prompt has no artifact path - Fix: resolve it with backend.ResolveArtifactPath before dispatch, the same way the exit gate itself resolves the file it reads")
	}
	if in.VerifyCommand == "" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: integration_review prompt has no verify command, so the reviewer would be told to check nothing - Fix: resolve it with epic.ResolveVerifyCommand before dispatch")
	}
	var buf bytes.Buffer
	if err := integrationReviewTmpl.Execute(&buf, integrationReviewView{IntegrationReviewInput: in, Kinds: review.All()}); err != nil {
		return "", err
	}
	return buf.String(), nil
}
