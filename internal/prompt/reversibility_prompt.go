package prompt

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// ReversibilityInput feeds the reversibility question the oracle answers after
// an integration review rejection.
//
// It carries text and only text. The actor answering this runs with no tools
// and no access to the repository (see app.CLIImpactComposer), so everything
// it is allowed to reason about has to arrive in these fields - the same
// arrangement RenderImpactOnUse already uses for the decision record it never
// goes and reads.
type ReversibilityInput struct {
	EpicID string
	// Objection is the reviewer's own "what is wrong" text.
	Objection string
	// ChangeSummary is what the branch changed, as text: a diffstat, not a
	// diff. The question is about cost of reversal, which the shape of a
	// change answers and its contents do not.
	ChangeSummary string
}

const reversibilityTemplate = `You are answering exactly one question about an in-progress software change. Nothing else is being asked of you.

An automated reviewer rejected the integration of epic {{.EpicID}}. The orchestrator can either send the work back for another automatic fix-up round, or stop and hand it to a human. It decides that on one criterion: how expensive this change would be to undo later, if continuing turns out to be the wrong call.

Two facts are already established and are not yours to re-check: nothing here has been published (no push, no pull request, no release), and the branch touched no path this repository declares irreversible.

The objection the reviewer raised:
{{.Objection}}

What the branch changed:
{{.ChangeSummary}}

CHEAP means undoing this later costs little: the work is a branch nobody depends on, and dropping or amending it leaves nothing behind that has to be migrated, reverted elsewhere, or announced.

EXPENSIVE means undoing it later is costly or impossible even from a branch: data or schemas have been migrated in place, state outside this repository was mutated, credentials or keys were rotated, or the change is one that other people or systems will have already consumed by the time anyone reconsiders.

Answer in exactly two parts and nothing else:

1. A first line that is exactly one of these, with no other text on it:
REVERSAL: CHEAP
REVERSAL: EXPENSIVE

2. Below it, one or two sentences saying why. This reason is recorded and read later by the person who was not asked, so it has to name what makes reversal cheap or expensive here, not restate the objection.

Do not review the code, do not propose a fix, and do not say whether the reviewer was right.`

var reversibilityTmpl = template.Must(template.New("reversibility").Parse(reversibilityTemplate))

// RenderReversibility renders the question. It refuses to render without an
// objection or a change summary: an answer produced from neither is a guess
// with a reason attached, which is worse than no gate at all, since the whole
// point of moving the human gate here is that this judgment is accountable.
func RenderReversibility(in ReversibilityInput) (string, error) {
	if strings.TrimSpace(in.Objection) == "" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: the reversibility question has no reviewer objection to weigh - Fix: pass the rejection's \"what is wrong\" text")
	}
	if strings.TrimSpace(in.ChangeSummary) == "" {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: the reversibility question has no summary of what the branch changed - Fix: pass a diffstat of the epic branch against its base")
	}
	var buf bytes.Buffer
	if err := reversibilityTmpl.Execute(&buf, in); err != nil {
		return "", err
	}
	return buf.String(), nil
}
