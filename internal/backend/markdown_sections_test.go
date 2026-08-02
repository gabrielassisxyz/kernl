package backend

import (
	"sort"
	"strings"
	"testing"
)

// markdownSectionsTestKeyFn recognizes the same four heading names the old
// decision_record markdown gate used, mapped to the same section keys - not
// because this package still parses decision records this way (it does
// not: ParseDecisionRecordDocument reads agent-authored JSON, and
// SplitDecisionBody recovers Decision.Body's own two parts by an
// exact-length preamble, not by re-parsing headings - see
// internal/app/decision_record.go), but because these are the exact
// fixtures the retired decision-specific parser was proven correct against,
// and MarkdownSectionsByHeading is the generic engine those fixtures
// actually exercised underneath. fork_handover.go, ParseIntegrationRejection
// and ParseImplementationRejection each supply their OWN keyFn for their own
// heading vocabularies; this one exists only so this file can drive the
// shared engine directly, the way those three still do.
func markdownSectionsTestKeyFn(headingText string) (string, bool) {
	switch strings.TrimSpace(headingText) {
	case "Decision":
		return "decision", true
	case "Options Considered":
		return "options_considered", true
	case "Trade-offs":
		return "trade_offs", true
	case "Rationale":
		return "rationale", true
	default:
		return "", false
	}
}

var markdownSectionsAllFourKeys = []string{"decision", "options_considered", "trade_offs", "rationale"}

// assertPresentKeys compares MarkdownSectionsByHeading's returned section
// keys against want, independent of order and independent of dupKey (tests
// that care about dupKey check it themselves).
func assertPresentKeys(t *testing.T, content string, want []string) {
	t.Helper()
	sections, _ := MarkdownSectionsByHeading(content, markdownSectionsTestKeyFn)
	var got []string
	for key, body := range sections {
		if body != "" {
			got = append(got, key)
		}
	}
	sort.Strings(got)
	wantSorted := append([]string(nil), want...)
	sort.Strings(wantSorted)
	if len(got) != len(wantSorted) {
		t.Fatalf("present keys = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != wantSorted[i] {
			t.Fatalf("present keys = %v, want %v", got, want)
		}
	}
}

// --- Fenced or commented headings must not read as real headings. ---

func TestMarkdownSectionsByHeading_CommentedHeadingsBypass(t *testing.T) {
	doc := "<!--\n" +
		"## Decision\n" +
		"placeholder\n" +
		"## Options Considered\n" +
		"placeholder\n" +
		"## Trade-offs\n" +
		"placeholder\n" +
		"## Rationale\n" +
		"placeholder\n" +
		"-->\n"
	assertPresentKeys(t, doc, nil)
}

func TestMarkdownSectionsByHeading_FencedHeadingsBypass(t *testing.T) {
	doc := "```\n" +
		"## Decision\n" +
		"placeholder\n" +
		"## Options Considered\n" +
		"placeholder\n" +
		"## Trade-offs\n" +
		"placeholder\n" +
		"## Rationale\n" +
		"placeholder\n" +
		"```\n"
	assertPresentKeys(t, doc, nil)
}

// TestMarkdownSectionsByHeading_FencedHeadingBeforeRealSection proves a
// fence does not eat a REAL heading that follows it: fenced example text is
// literal content of whichever real section precedes it, and a real heading
// after the fence closes is still recognized.
func TestMarkdownSectionsByHeading_FencedHeadingBeforeRealSection(t *testing.T) {
	doc := "## Decision\n" +
		"Here is an example of what NOT to write:\n" +
		"```\n" +
		"## Decision\n" +
		"placeholder\n" +
		"```\n" +
		"We chose approach B because it is simpler.\n" +
		"## Options Considered\n" +
		"A and B.\n" +
		"## Trade-offs\n" +
		"A is faster, B is simpler.\n" +
		"## Rationale\n" +
		"B won.\n"
	assertPresentKeys(t, doc, markdownSectionsAllFourKeys)
}

func TestMarkdownSectionsByHeading_TildeFenceHeadingsBypass(t *testing.T) {
	doc := "~~~\n" +
		"## Decision\n" +
		"placeholder\n" +
		"## Options Considered\n" +
		"placeholder\n" +
		"## Trade-offs\n" +
		"placeholder\n" +
		"## Rationale\n" +
		"placeholder\n" +
		"~~~\n"
	assertPresentKeys(t, doc, nil)
}

// --- Presentation-only bodies must not count as content. ---

func TestMarkdownSectionsByHeading_HorizontalRuleBodyBypass(t *testing.T) {
	doc := "## Decision\n---\n" +
		"## Options Considered\n---\n" +
		"## Trade-offs\n---\n" +
		"## Rationale\n---\n"
	assertPresentKeys(t, doc, nil)
}

func TestMarkdownSectionsByHeading_CommentOnlyBodyBypass(t *testing.T) {
	doc := "## Decision\n<!-- TODO -->\n" +
		"## Options Considered\n<!-- TODO -->\n" +
		"## Trade-offs\n<!-- TODO -->\n" +
		"## Rationale\n<!-- TODO -->\n"
	assertPresentKeys(t, doc, nil)
}

// TestMarkdownSectionsByHeading_ThematicBreakInsideRealBodyIsStripped proves
// a decorative "---" mixed into an otherwise real body is stripped, not
// counted, but does not wrongly blank the rest of the body.
func TestMarkdownSectionsByHeading_ThematicBreakInsideRealBodyIsStripped(t *testing.T) {
	doc := "## Decision\nUse approach B.\n\n---\n\nSee below for why.\n" +
		"## Options Considered\nA and B.\n" +
		"## Trade-offs\nA is faster, B is simpler.\n" +
		"## Rationale\nB won.\n"
	sections, dupKey := MarkdownSectionsByHeading(doc, markdownSectionsTestKeyFn)
	if dupKey != "" {
		t.Fatalf("unexpected dupKey %q", dupKey)
	}
	if strings.Contains(sections["decision"], "---") {
		t.Errorf("decision body still contains the thematic break: %q", sections["decision"])
	}
	if !strings.Contains(sections["decision"], "See below for why.") {
		t.Errorf("decision body lost real content around the thematic break: %q", sections["decision"])
	}
}

// TestMarkdownSectionsByHeading_FencedHorizontalRuleStillCounts proves a
// "---" that is literal content inside a fenced code block (not a
// presentation token) is not stripped - only a real, unfenced thematic
// break is presentation-only.
func TestMarkdownSectionsByHeading_FencedHorizontalRuleStillCounts(t *testing.T) {
	doc := "## Decision\n```text\n---\n```\n" +
		"## Options Considered\nA and B.\n" +
		"## Trade-offs\nA is faster, B is simpler.\n" +
		"## Rationale\nB won.\n"
	sections, _ := MarkdownSectionsByHeading(doc, markdownSectionsTestKeyFn)
	if !strings.Contains(sections["decision"], "---") {
		t.Errorf("decision body lost a fenced (literal) horizontal rule: %q", sections["decision"])
	}
}

// --- Setext headings, a BOM, and CRLF must be recognized, not rejected. ---

func TestMarkdownSectionsByHeading_SetextHeadingsRecognized(t *testing.T) {
	doc := "Decision\n--------\nUse approach B.\n" +
		"Options Considered\n------------------\nA and B.\n" +
		"Trade-offs\n----------\nA is faster, B is simpler.\n" +
		"Rationale\n---------\nB won.\n"
	assertPresentKeys(t, doc, markdownSectionsAllFourKeys)
}

// TestMarkdownSectionsByHeading_SetextDoesNotConsumeATXHeadingLine proves a
// "---" immediately after an ATX heading line is not reinterpreted as
// turning that heading line into setext text (which would double-consume
// it) - it is correctly stripped as a thematic break instead.
func TestMarkdownSectionsByHeading_SetextDoesNotConsumeATXHeadingLine(t *testing.T) {
	doc := "## Decision\n---\nUse approach B.\n" +
		"## Options Considered\nA and B.\n" +
		"## Trade-offs\nA is faster, B is simpler.\n" +
		"## Rationale\nB won.\n"
	assertPresentKeys(t, doc, markdownSectionsAllFourKeys)
}

func TestMarkdownSectionsByHeading_UTF8BOMStripped(t *testing.T) {
	doc := "\uFEFF## Decision\nUse approach B.\n" +
		"## Options Considered\nA and B.\n" +
		"## Trade-offs\nA is faster, B is simpler.\n" +
		"## Rationale\nB won.\n"
	assertPresentKeys(t, doc, markdownSectionsAllFourKeys)
}

func TestMarkdownSectionsByHeading_CRLFLineEndings(t *testing.T) {
	doc := strings.ReplaceAll(
		"## Decision\nUse approach B.\n"+
			"## Options Considered\nA and B.\n"+
			"## Trade-offs\nA is faster, B is simpler.\n"+
			"## Rationale\nB won.\n",
		"\n", "\r\n",
	)
	assertPresentKeys(t, doc, markdownSectionsAllFourKeys)
}

// --- Ordering, nesting, and an unrecognized same-level heading. ---

func TestMarkdownSectionsByHeading_ReorderedSectionsStillRecognized(t *testing.T) {
	doc := "## Rationale\nB won.\n" +
		"## Trade-offs\nA is faster, B is simpler.\n" +
		"## Decision\nUse approach B.\n" +
		"## Options Considered\nA and B.\n"
	assertPresentKeys(t, doc, markdownSectionsAllFourKeys)
}

// TestMarkdownSectionsByHeading_SubheadingsAreBodyNotTerminator is the
// archeion arch-tws case, reduced. A real decision record organised its
// options into numbered subsections, which is ordinary writing; a parser
// that treats ANY heading as a section terminator (regardless of depth)
// reads the required section as empty the moment its content is organised
// this way.
func TestMarkdownSectionsByHeading_SubheadingsAreBodyNotTerminator(t *testing.T) {
	record := `# Decision record: honouring a Disallow with an interior wildcard

## Decision

Four things were open once the bug was confirmed.

## Options Considered

### 1. The regex feature as the fix

- **1a. Add ` + "`regex`" + ` to the feature list**, as the bead proposed.
- **1b. Own the path matching here**, leaving the feature list alone.

### 2. Where the rules come from

- **2a. Fetch and parse it in this project.**
- **2b. Reuse the engine's parse and replace only the matching.**

## Trade-offs

Owning the matching costs a parser this project now maintains.

## Rationale

The vendored implementation is wrong on the file that motivated the bug.
`
	sections, dupKey := MarkdownSectionsByHeading(record, markdownSectionsTestKeyFn)
	if dupKey != "" {
		t.Fatalf("unexpected dupKey %q", dupKey)
	}

	options, ok := sections["options_considered"]
	if !ok {
		t.Fatal("options_considered is absent - a section whose content is organised into subheadings read as empty")
	}
	for _, want := range []string{"1. The regex feature as the fix", "2b. Reuse the engine's parse"} {
		if !strings.Contains(options, want) {
			t.Errorf("options_considered body does not contain %q", want)
		}
	}

	// The sibling sections must not have been swallowed along with the
	// subheadings: a fix that made every heading body-content would pass
	// the assertion above and destroy the document's structure.
	if !strings.Contains(sections["trade_offs"], "costs a parser") {
		t.Errorf("trade_offs = %q, want its own body", sections["trade_offs"])
	}
	if strings.Contains(options, "costs a parser") {
		t.Error("options_considered swallowed the following ## section - a same-level heading must still close it")
	}
	if strings.Contains(sections["decision"], "1. The regex feature") {
		t.Error("decision swallowed the next ## section")
	}
}

// TestMarkdownSectionsByHeading_UnrecognisedSameLevelHeadingStillCloses is
// the fence the level rule has to keep: an unrecognised heading at the same
// depth still terminates the section before it, so a "## Context" preamble
// is never counted as another section's content.
func TestMarkdownSectionsByHeading_UnrecognisedSameLevelHeadingStillCloses(t *testing.T) {
	record := `## Decision

The decided thing.

## Context

Background nobody asked for.

## Options Considered

a and b

## Trade-offs

t

## Rationale

r
`
	sections, _ := MarkdownSectionsByHeading(record, markdownSectionsTestKeyFn)
	if strings.Contains(sections["decision"], "Background nobody asked for") {
		t.Error("an unrecognised ## heading was folded into the preceding section")
	}
	if sections["decision"] != "The decided thing." {
		t.Errorf("decision = %q, want only its own body", sections["decision"])
	}
}

// TestMarkdownSectionsByHeading_SubheadingAloneIsEnoughBody covers a deeper
// heading directly under a required section, with no prose of its own
// between them: the section's entire body is its one subsection.
func TestMarkdownSectionsByHeading_SubheadingAloneIsEnoughBody(t *testing.T) {
	record := `## Decision

d

## Options Considered

### only option

it was this one

## Trade-offs

t

## Rationale

r
`
	sections, _ := MarkdownSectionsByHeading(record, markdownSectionsTestKeyFn)
	if _, ok := sections["options_considered"]; !ok {
		t.Fatal("options_considered absent when its whole body is one subsection")
	}
}

// --- dupKey: FIX 2's structural fix no longer needs this signal for
// Decision.Body (see SplitDecisionBody's own doc comment), but
// fork_handover.go, ParseIntegrationRejection and ParseImplementationRejection
// still read genuinely agent-authored markdown through this same engine and
// still depend on it - see forkHandoverSections's own "ok = dupKey == """
// pattern. ---

// TestMarkdownSectionsByHeading_DuplicateRecognizedHeadingSetsDupKey proves
// two real (non-fenced, non-commented) headings claiming the same
// recognized key are reported via dupKey, not silently resolved to
// whichever one the scan happened to see last.
func TestMarkdownSectionsByHeading_DuplicateRecognizedHeadingSetsDupKey(t *testing.T) {
	doc := "## Decision\nFirst.\n" +
		"## Decision\nSecond.\n" +
		"## Options Considered\na and b\n" +
		"## Trade-offs\nt\n" +
		"## Rationale\nr\n"
	_, dupKey := MarkdownSectionsByHeading(doc, markdownSectionsTestKeyFn)
	if dupKey != "decision" {
		t.Errorf("dupKey = %q, want %q", dupKey, "decision")
	}
}

// TestMarkdownSectionsByHeading_NoDuplicateLeavesDupKeyEmpty is
// DuplicateRecognizedHeadingSetsDupKey's negative: a well-formed document
// with each recognized heading exactly once must report dupKey == "", the
// signal every dupKey-honoring caller treats as "unambiguous."
func TestMarkdownSectionsByHeading_NoDuplicateLeavesDupKeyEmpty(t *testing.T) {
	doc := "## Decision\nUse approach B.\n" +
		"## Options Considered\nA and B.\n" +
		"## Trade-offs\nA is faster, B is simpler.\n" +
		"## Rationale\nB won.\n"
	_, dupKey := MarkdownSectionsByHeading(doc, markdownSectionsTestKeyFn)
	if dupKey != "" {
		t.Errorf("dupKey = %q, want empty for a document with no repeated recognized heading", dupKey)
	}
}

// TestMarkdownSectionsByHeading_DuplicateUnrecognizedHeadingLeavesDupKeyEmpty
// proves dupKey tracks only RECOGNIZED keys: two unrelated, unrecognized
// headings sharing text (e.g. two "## Notes" asides) is not this engine's
// concern - keyFn already said "" for them, so there is no key to collide
// on.
func TestMarkdownSectionsByHeading_DuplicateUnrecognizedHeadingLeavesDupKeyEmpty(t *testing.T) {
	doc := "## Notes\nfirst aside\n" +
		"## Decision\nUse approach B.\n" +
		"## Notes\nsecond aside\n" +
		"## Options Considered\nA and B.\n" +
		"## Trade-offs\nA is faster, B is simpler.\n" +
		"## Rationale\nB won.\n"
	_, dupKey := MarkdownSectionsByHeading(doc, markdownSectionsTestKeyFn)
	if dupKey != "" {
		t.Errorf("dupKey = %q, want empty - the repeated heading (\"Notes\") is not a recognized key", dupKey)
	}
}

// TestMarkdownSectionsByHeading_FencedDuplicateDoesNotSetDupKey proves a
// heading-looking line inside a fence is not real, so it cannot trigger
// dupKey even when it repeats a key a real heading elsewhere also uses.
func TestMarkdownSectionsByHeading_FencedDuplicateDoesNotSetDupKey(t *testing.T) {
	doc := "## Decision\nUse approach B.\n" +
		"```\n## Decision\nfenced, not real\n```\n" +
		"## Options Considered\nA and B.\n" +
		"## Trade-offs\nA is faster, B is simpler.\n" +
		"## Rationale\nB won.\n"
	_, dupKey := MarkdownSectionsByHeading(doc, markdownSectionsTestKeyFn)
	if dupKey != "" {
		t.Errorf("dupKey = %q, want empty - the duplicate is fenced, not a real heading", dupKey)
	}
}
