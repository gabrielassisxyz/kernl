package prompt_test

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/prompt"
)

func sampleReversibilityInput() prompt.ReversibilityInput {
	return prompt.ReversibilityInput{
		EpicID:        "ep-1",
		Objection:     "percent-encoded paths are not normalized before matching",
		ChangeSummary: " 3 files changed, 41 insertions(+), 2 deletions(-)",
	}
}

func TestRenderReversibility_AsksOneQuestionAndFixesTheAnswerShape(t *testing.T) {
	out, err := prompt.RenderReversibility(sampleReversibilityInput())
	if err != nil {
		t.Fatal(err)
	}
	mustContain := []string{
		"ep-1",
		"percent-encoded paths are not normalized before matching",
		"3 files changed, 41 insertions(+), 2 deletions(-)",
		// The two-part shape ParseReversibilityAnswer reads back. The prompt
		// and the parser have to keep naming the same tokens, or an answer
		// that followed this text to the letter is still undecidable.
		"REVERSAL: CHEAP",
		"REVERSAL: EXPENSIVE",
		"one or two sentences saying why",
		// The oracle is tool-less and outside the repository: everything it may
		// reason about arrives in the text, and it must not try to review.
		"Do not review the code",
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("reversibility prompt missing %q\n---\n%s\n---", want, out)
		}
	}
}

// An answer produced with nothing to weigh is a guess with a reason attached,
// which is worse than no gate at all: the whole point of moving the human gate
// here is that this judgment can be audited afterwards.
func TestRenderReversibility_RefusesWithNothingToWeigh(t *testing.T) {
	cases := map[string]func(*prompt.ReversibilityInput){
		"no objection":      func(in *prompt.ReversibilityInput) { in.Objection = "  " },
		"no change summary": func(in *prompt.ReversibilityInput) { in.ChangeSummary = "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := sampleReversibilityInput()
			mutate(&in)
			if _, err := prompt.RenderReversibility(in); err == nil {
				t.Fatal("expected an error")
			} else if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
				t.Errorf("expected KERNL DISPATCH FAILURE, got %v", err)
			}
		})
	}
}
