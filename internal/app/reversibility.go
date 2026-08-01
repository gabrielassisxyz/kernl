package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/prompt"
)

// ReversibilityQuestion is what the mayor is asked about one rejection: the
// objection that was raised, and the shape of the change it was raised
// against. Nothing else, because the actor answering it has nothing else -
// see prompt.ReversibilityInput.
type ReversibilityQuestion struct {
	EpicID        string
	Objection     string
	ChangeSummary string
}

// ReversibilityVerdict is the answer, and its reason. The reason is not
// decoration: moving the human gate from "the budget ran out" to "this would
// be expensive to undo" only pays for itself if the new gate can be audited
// afterwards, and an unexplained verdict cannot be.
type ReversibilityVerdict struct {
	Expensive bool
	Reason    string
}

// ReversibilityJudge answers whether undoing a change later would be
// expensive. An interface so DecideFixupAction can be exercised against a
// named fake, and so the tests that matter most - the ones asserting the judge
// is NEVER consulted - can prove it by counting calls.
type ReversibilityJudge interface {
	JudgeReversibility(ctx context.Context, q ReversibilityQuestion) (ReversibilityVerdict, error)
}

// reversibilityJudgeTimeout bounds one question, for the same reason
// impactComposeTimeout bounds the impact one: neither the provider client nor
// a spawned CLI carries a deadline of its own, and a mayor that accepts the
// question and never answers would hang the run at the exact moment it is
// deciding whether to involve a human.
const reversibilityJudgeTimeout = 90 * time.Second

// MayorReversibilityJudge asks the configured mayor, tool-less and outside the
// repository. Deliberately so: the caller computes every fact from the
// repository and hands them over as text. A mayor that could go and inspect
// the repository to judge it is a much larger change with a much larger blast
// radius, and this gate does not need one.
type MayorReversibilityJudge struct {
	Mayor Mayor
}

// JudgeReversibility implements ReversibilityJudge.
func (j MayorReversibilityJudge) JudgeReversibility(ctx context.Context, q ReversibilityQuestion) (ReversibilityVerdict, error) {
	if j.Mayor == nil {
		return ReversibilityVerdict{}, fmt.Errorf("no mayor is configured to answer it - Fix: set llm.provider and llm.endpoint, or llm.agent, in kernl.yaml")
	}
	question, err := prompt.RenderReversibility(prompt.ReversibilityInput{
		EpicID:        q.EpicID,
		Objection:     q.Objection,
		ChangeSummary: q.ChangeSummary,
	})
	if err != nil {
		return ReversibilityVerdict{}, err
	}
	askCtx, cancel := context.WithTimeout(ctx, reversibilityJudgeTimeout)
	defer cancel()
	answer, err := j.Mayor.Ask(askCtx, question)
	if err != nil {
		return ReversibilityVerdict{}, err
	}
	return ParseReversibilityAnswer(answer)
}

// ParseReversibilityAnswer reads the mayor's reply.
//
// It is strict in one direction on purpose: an answer whose verdict line is
// missing or unrecognized, or that carries no reason, is an error rather than
// a default. "Cheap" is the answer that lets a run continue without a human,
// so a malformed reply must never be able to produce it - the failure of a
// gate has to land on the side that stops, not the side that proceeds.
func ParseReversibilityAnswer(answer string) (ReversibilityVerdict, error) {
	trimmed := strings.TrimSpace(answer)
	if trimmed == "" {
		return ReversibilityVerdict{}, fmt.Errorf("the mayor answered the reversibility question with nothing at all")
	}
	verdictLine, rest, _ := strings.Cut(trimmed, "\n")
	// A model that wraps the line in bold or backticks answered correctly and
	// styled it; a model that answered something else entirely still fails
	// below, because the enum itself is matched exactly.
	normalized := strings.ToUpper(strings.Trim(strings.TrimSpace(verdictLine), "*`_ "))
	value, ok := strings.CutPrefix(normalized, "REVERSAL:")
	if !ok {
		return ReversibilityVerdict{}, fmt.Errorf("the mayor's answer does not begin with a REVERSAL: line, so its verdict is not decidable: %q", firstLineOf(trimmed))
	}
	reason := strings.TrimSpace(rest)
	if reason == "" {
		return ReversibilityVerdict{}, fmt.Errorf("the mayor answered %q but gave no reason, and an unaccountable verdict is exactly what this gate must not act on", strings.TrimSpace(verdictLine))
	}
	switch strings.TrimSpace(value) {
	case "CHEAP":
		return ReversibilityVerdict{Expensive: false, Reason: reason}, nil
	case "EXPENSIVE":
		return ReversibilityVerdict{Expensive: true, Reason: reason}, nil
	default:
		return ReversibilityVerdict{}, fmt.Errorf("the mayor answered %q, which is neither CHEAP nor EXPENSIVE", strings.TrimSpace(verdictLine))
	}
}

func firstLineOf(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
