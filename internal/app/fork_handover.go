package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/prompt"
)

// ForkHandover is what an implementer writes at <artifact_dir>/fork.md and
// then STOPS, instead of choosing alone and silently, the moment it meets a
// fork the bead, this repository's own docs, and existing precedent in the
// code do not already determine (decision model §1, plan §3.1). Its
// sections deliberately reuse the decision record's old "Decision / Options
// Considered" markdown vocabulary rather than inventing a third one, parsed
// through backend.MarkdownSectionsByHeading below - the same fence- and
// HTML-comment-aware engine the decision_record gate's own markdown parser
// used before that artifact moved to agent-authored JSON (see
// backend.ParseDecisionRecordDocument). Unlike the decision record, this
// artifact stayed markdown: it is a one-shot escalation, not a growing list,
// so there is no "more than one" to model.
type ForkHandover struct {
	// Fork is the choice the implementer cannot make ("## Fork").
	Fork string
	// OptionsConsidered are the options it weighed ("## Options
	// Considered") - at least the real alternatives, the same requirement a
	// decision record's own Options Considered section carries.
	OptionsConsidered string
	// WhatWouldHaveToAgree is the implementer's own answer to what outside
	// this bead would have to agree with the choice, or an explicit
	// statement that nothing does ("## What Would Have To Agree").
	WhatWouldHaveToAgree string
}

// forkHandoverHeadingKeys are the only headings this parser recognizes -
// anything else still closes the previous section (see
// backend.MarkdownSectionsByHeading's own doc comment) but contributes
// nothing.
var forkHandoverHeadingKeys = map[string]string{
	"fork":                     "fork",
	"options considered":       "options_considered",
	"what would have to agree": "what_would_have_to_agree",
}

// forkHandoverSections splits content on its ATX/setext headings via
// backend.MarkdownSectionsByHeading - the same fence- and HTML-comment-aware
// pass integrationRejectionSections already reuses for a different heading
// vocabulary - rather than a second, naively regexp-only parser a hidden
// heading could defeat. ok=false means either two real headings claimed the
// same recognized key, or the split is otherwise not decidable content.
func forkHandoverSections(content string) (sections map[string]string, ok bool) {
	sections, dupKey := backend.MarkdownSectionsByHeading(content, func(headingText string) (string, bool) {
		key, recognized := forkHandoverHeadingKeys[strings.ToLower(strings.TrimSpace(headingText))]
		return key, recognized
	})
	return sections, dupKey == ""
}

// ParseForkHandover extracts the implementer's declaration from a fork.md
// artifact. It returns nil - not an error - when the declaration is
// unusable: missing a required section, or ambiguous because two real
// headings claimed the same key. Per the plan (§3), an unusable declaration
// is treated exactly like an explicit "decision" in the fix-up gate's own
// vocabulary - ambiguous is never guessed into a decision, it escalates. See
// DecideForkAction, the caller that acts on this.
func ParseForkHandover(content string) *ForkHandover {
	sections, ok := forkHandoverSections(content)
	if !ok {
		return nil
	}

	fork := strings.TrimSpace(sections["fork"])
	options := strings.TrimSpace(sections["options_considered"])
	agree := strings.TrimSpace(sections["what_would_have_to_agree"])
	if fork == "" || options == "" || agree == "" {
		return nil
	}
	return &ForkHandover{Fork: fork, OptionsConsidered: options, WhatWouldHaveToAgree: agree}
}

// ForkAction is what DecideForkAction resolves a fork handover to.
type ForkAction int

const (
	// ForkActionDecided: the DA chose one of the options and recorded why.
	ForkActionDecided ForkAction = iota
	// ForkActionEscalate: this fork goes to the operator instead.
	ForkActionEscalate
)

// ForkGateCause names which layer of DecideForkAction produced one decision,
// printed verbatim so a run's own output can be grepped for the cause that
// fired rather than for the prose that happened to describe it - the same
// role GateCause plays for the fix-up gate.
type ForkGateCause string

const (
	// ForkCauseUnreadableHandover: fork.md could not be read as a fork at
	// all - missing a required section, or two real headings claiming the
	// same one. An ambiguous handover escalates rather than being guessed
	// into a decision.
	ForkCauseUnreadableHandover ForkGateCause = "unreadable_handover"
	// ForkCauseOpenDependents: a sibling bead in this epic depends on the
	// one handing the fork over and has not yet closed - something outside
	// this bead has to agree with the choice.
	ForkCauseOpenDependents ForkGateCause = "open_dependents"
	// ForkCauseNoDAConfigured: nothing is configured to decide this on the
	// operator's behalf.
	ForkCauseNoDAConfigured ForkGateCause = "no_da_configured"
	// ForkCauseDAUnavailable: a DA is configured but could not be reached or
	// failed to answer.
	ForkCauseDAUnavailable ForkGateCause = "da_unavailable"
	// ForkCauseDAAnswerUnparseable: the DA answered, but not in a shape this
	// package can read. A malformed or empty reply means the DA did not
	// answer at all, never a reason to fabricate a choice out of silence.
	ForkCauseDAAnswerUnparseable ForkGateCause = "da_answer_unparseable"
	// ForkCauseDADecided: the DA chose one option and recorded why - the
	// only cause that continues.
	ForkCauseDADecided ForkGateCause = "da_decided"
	// ForkCauseDAEscalated: the DA's own judgment was to hand this to the
	// operator.
	ForkCauseDAEscalated ForkGateCause = "da_escalated"
)

// ForkDecision is the outcome of the policy, with the reason that produced
// it. Every path sets Reason, including the one that continues - a decision
// this consequential has to leave a record even when nobody had to be woken
// up for it.
type ForkDecision struct {
	Action ForkAction
	Cause  ForkGateCause
	// ChosenOption is set only when Action is ForkActionDecided - copied
	// verbatim from the DA's own CHOSEN line, which the prompt requires to
	// name one of the handover's own Options Considered.
	ChosenOption string
	Reason       string
}

// DecideForkAction is the policy for one fork handover.
//
// Three layers, in this order, and the order is the point - exactly the same
// shape DecideFixupAction already established: the measured, certain answers
// come first, and the DA is asked only the residue nothing else can settle.
//
//  1. An unreadable handover escalates without asking anything - an
//     ambiguous declaration is never guessed into a decision.
//  2. An open sibling dependent escalates without asking the DA - this is
//     the one layer here that rests on a MEASURED PROXY rather than a
//     certainty (see ForkScopeFacts.OpenDependents' own doc comment), which
//     is why its own reason names exactly what was found rather than just
//     asserting the cause.
//  3. No DA configured escalates - there is nothing to ask.
//
// Only past all three does the DA get asked anything at all - and per §3.3
// of the plan, the DA is told to lean toward deciding when it is genuinely
// unsure which option is right (see prompt.RenderForkHandover), because
// being interrupted is the failure that costs the operator the most.
//
// Two different failures, and they must not be conflated:
//   - The DA being unsure WHICH OPTION WINS is the DA's own judgment call,
//     made in the prompt text, and never reaches this function at all - it
//     is resolved before Consult returns, one way or the other.
//   - A reply this package cannot parse is not the DA being unsure - it is
//     the DA not answering in the required shape at all, for any reason
//     (a bad model, output truncated, the scope-lock ambient-content trap
//     the prompt guards against). ParseForkAnswer escalates on that,
//     regardless of the lean-toward-deciding instruction the DA itself was
//     given: a malformed or empty reply must never be read as silence
//     meaning "decide", or the failure of this gate would land on the side
//     that proceeds instead of the side that stops.
func DecideForkAction(ctx context.Context, handover *ForkHandover, facts ForkScopeFacts, da DA) ForkDecision {
	if handover == nil {
		return ForkDecision{
			Action: ForkActionEscalate,
			Cause:  ForkCauseUnreadableHandover,
			Reason: "the fork handover artifact could not be read as a fork, and an ambiguous handover escalates rather than being guessed into a decision",
		}
	}
	if len(facts.OpenDependents) > 0 {
		slog.Info("fork gate escalating: sibling beads depend on this one and have not yet closed",
			"epic", facts.EpicID, "bead", facts.BeadID, "openDependents", facts.OpenDependents)
		return ForkDecision{
			Action: ForkActionEscalate,
			Cause:  ForkCauseOpenDependents,
			Reason: fmt.Sprintf("%s in epic %s depend on this bead and have not yet closed, so something outside this bead has to agree with the choice", strings.Join(facts.OpenDependents, ", "), facts.EpicID),
		}
	}
	if da == nil {
		return ForkDecision{
			Action: ForkActionEscalate,
			Cause:  ForkCauseNoDAConfigured,
			Reason: "nothing is configured to decide this on the operator's behalf - Fix: set da.agent and da.workDir in kernl.yaml",
		}
	}

	question, err := prompt.RenderForkHandover(prompt.ForkHandoverInput{
		EpicID:               facts.EpicID,
		BeadID:               facts.BeadID,
		Fork:                 handover.Fork,
		OptionsConsidered:    handover.OptionsConsidered,
		WhatWouldHaveToAgree: handover.WhatWouldHaveToAgree,
		RelatedDecisions:     facts.RelatedDecisions,
		RepositoryContext:    facts.RepositoryContext,
	})
	if err != nil {
		return ForkDecision{Action: ForkActionEscalate, Cause: ForkCauseUnreadableHandover, Reason: err.Error()}
	}

	answer, err := da.Consult(ctx, question)
	if err != nil {
		return ForkDecision{
			Action: ForkActionEscalate,
			Cause:  ForkCauseDAUnavailable,
			Reason: fmt.Sprintf("the DA could not be asked to decide this fork: %v", err),
		}
	}
	decision, err := ParseForkAnswer(answer, handover)
	if err != nil {
		return ForkDecision{
			Action: ForkActionEscalate,
			Cause:  ForkCauseDAAnswerUnparseable,
			Reason: fmt.Sprintf("the DA's answer could not be read: %v", err),
		}
	}
	return decision
}

// ParseForkAnswer reads the DA's reply. handover is the same fork the answer
// is FOR - passed through so a DECIDE can be checked against the options it
// actually offered (see chosenOptionWasOffered) rather than trusted as a
// verbatim label with nowhere to have come from.
//
// Strict in one direction, exactly like ParseReversibilityAnswer, and for the
// same stated reason: the failure of a gate must land on the side that
// stops, never the side that proceeds. An answer whose verdict line is
// missing or unrecognized, a DECIDE with no CHOSEN line, a CHOSEN line naming
// an option nobody weighed, or any verdict with no reason below it, is an
// error - never a default guessed in its place.
func ParseForkAnswer(answer string, handover *ForkHandover) (ForkDecision, error) {
	trimmed := strings.TrimSpace(answer)
	if trimmed == "" {
		return ForkDecision{}, fmt.Errorf("the DA answered the fork question with nothing at all")
	}
	firstLine, rest, _ := strings.Cut(trimmed, "\n")
	verdict := strings.ToUpper(strings.Trim(strings.TrimSpace(firstLine), "*`_ "))

	switch verdict {
	case prompt.ForkDecideLine:
		return parseForkDecideAnswer(rest, handover)
	case prompt.ForkEscalateLine:
		reason := strings.TrimSpace(rest)
		if reason == "" {
			return ForkDecision{}, fmt.Errorf("the DA answered %q but gave no reason", prompt.ForkEscalateLine)
		}
		return ForkDecision{Action: ForkActionEscalate, Cause: ForkCauseDAEscalated, Reason: reason}, nil
	default:
		return ForkDecision{}, fmt.Errorf("the DA's answer does not begin with a %q or %q line, so its verdict is not decidable: %q", prompt.ForkDecideLine, prompt.ForkEscalateLine, firstLine)
	}
}

// parseForkDecideAnswer reads everything after a recognized "FORK: DECIDE"
// line: the CHOSEN line immediately below it, and the reason below that.
func parseForkDecideAnswer(afterVerdict string, handover *ForkHandover) (ForkDecision, error) {
	chosenLine, reasonRest, _ := strings.Cut(strings.TrimLeft(afterVerdict, "\n"), "\n")
	chosenLine = strings.TrimSpace(chosenLine)

	upper := strings.ToUpper(chosenLine)
	if !strings.HasPrefix(upper, prompt.ForkChosenPrefix) {
		return ForkDecision{}, fmt.Errorf("the DA answered %q but its next line is not a %q line: %q", prompt.ForkDecideLine, prompt.ForkChosenPrefix, chosenLine)
	}
	chosen := strings.TrimSpace(chosenLine[len(prompt.ForkChosenPrefix):])
	if chosen == "" {
		return ForkDecision{}, fmt.Errorf("the DA answered %q with an empty %q line", prompt.ForkDecideLine, prompt.ForkChosenPrefix)
	}
	if !chosenOptionWasOffered(chosen, handover) {
		return ForkDecision{}, fmt.Errorf("the DA answered %q with %s%q, but that option does not appear anywhere in the fork's own Options Considered - an answer naming an option nobody weighed is unusable and must escalate rather than proceed with a silent decision", prompt.ForkDecideLine, prompt.ForkChosenPrefix, chosen)
	}

	reason := strings.TrimSpace(reasonRest)
	if reason == "" {
		return ForkDecision{}, fmt.Errorf("the DA answered %q but gave no reason below its %q line", prompt.ForkDecideLine, prompt.ForkChosenPrefix)
	}
	return ForkDecision{Action: ForkActionDecided, Cause: ForkCauseDADecided, ChosenOption: chosen, Reason: reason}, nil
}

// chosenOptionWasOffered reports whether chosen appears, case- and
// whitespace-insensitively (including the line wrapping inside either side),
// somewhere within handover's own OptionsConsidered prose.
//
// Deliberately CONTAINMENT, not equality. OptionsConsidered is free prose
// under one heading ("bm25 or embeddings, whichever ranks better"), not an
// enumerated list a parser could split and compare exactly - exact-string
// equality against a parsed option list would reject correct answers
// constantly (a real "bm25" chosen against prose that spells it
// "BM25-based ranking" would fail an equality check for no good reason).
// Containment only refuses the specific failure this exists for: an option
// that appears NOWHERE in the prose the implementer actually offered, e.g.
// "hybrid" chosen against a fork that only ever weighed "bm25" and
// "embeddings" - a silent decision nobody weighed.
func chosenOptionWasOffered(chosen string, handover *ForkHandover) bool {
	if handover == nil {
		return false
	}
	needle := collapseForComparison(chosen)
	haystack := collapseForComparison(handover.OptionsConsidered)
	return needle != "" && strings.Contains(haystack, needle)
}

// collapseForComparison lowercases and reduces every run of whitespace to a
// single space. Collapsing the INTERIOR is what makes the containment above
// work at all: the handover is wrapped prose, so an option's own sentence
// carries line breaks, while the CHOSEN line quoting it back is one line. A
// comparison of the raw strings refuses every option longer than one line -
// which is nearly all of them - and reports it as an option nobody weighed.
func collapseForComparison(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}
