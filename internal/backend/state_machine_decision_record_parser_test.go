package backend

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// assertMissing compares missingDecisionRecordSections' output as a set,
// independent of order, since the function does not promise one.
func assertMissing(t *testing.T, content string, want []string) {
	t.Helper()
	got := missingDecisionRecordSections(content)
	gotSorted := append([]string(nil), got...)
	sort.Strings(gotSorted)
	wantSorted := append([]string(nil), want...)
	sort.Strings(wantSorted)
	if len(gotSorted) != len(wantSorted) {
		t.Fatalf("missing sections = %v, want %v", got, want)
	}
	for i := range gotSorted {
		if gotSorted[i] != wantSorted[i] {
			t.Fatalf("missing sections = %v, want %v", got, want)
		}
	}
}

var allFourKeys = []string{"decision", "options_considered", "trade_offs", "rationale"}

// --- Finding 1: fenced or commented headings must not satisfy the gate ---

// TestMissingDecisionRecordSections_CommentedHeadingsBypass uses the exact
// document from the review: every required heading exists only inside an
// HTML comment, so nothing is visibly written. All four sections must still
// be reported missing.
func TestMissingDecisionRecordSections_CommentedHeadingsBypass(t *testing.T) {
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
	assertMissing(t, doc, allFourKeys)
}

// TestMissingDecisionRecordSections_FencedHeadingsBypass uses the same
// all-four-headings content, this time inside a fenced code block instead of
// an HTML comment. A fence is literal content, not markdown structure, so
// none of the headings inside it are real.
func TestMissingDecisionRecordSections_FencedHeadingsBypass(t *testing.T) {
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
	assertMissing(t, doc, allFourKeys)
}

// TestMissingDecisionRecordSections_FencedHeadingBeforeRealSection proves a
// fence does not eat a *real* heading that follows it: fenced example text
// is literal content of whichever real section precedes it, and a real
// heading after the fence closes is still recognized.
func TestMissingDecisionRecordSections_FencedHeadingBeforeRealSection(t *testing.T) {
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
	assertMissing(t, doc, nil)
}

// TestMissingDecisionRecordSections_TildeFenceHeadingsBypass proves the
// tilde fence delimiter ("~~~") is recognized too, not only backticks.
func TestMissingDecisionRecordSections_TildeFenceHeadingsBypass(t *testing.T) {
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
	assertMissing(t, doc, allFourKeys)
}

// --- Finding 2: presentation-only bodies must not count as content ---

// TestMissingDecisionRecordSections_HorizontalRuleBodyBypass uses the
// review's exact case: every section body is nothing but "---".
func TestMissingDecisionRecordSections_HorizontalRuleBodyBypass(t *testing.T) {
	doc := "## Decision\n---\n" +
		"## Options Considered\n---\n" +
		"## Trade-offs\n---\n" +
		"## Rationale\n---\n"
	assertMissing(t, doc, allFourKeys)
}

// TestMissingDecisionRecordSections_CommentOnlyBodyBypass uses the review's
// other exact case: every section body is nothing but an HTML comment.
func TestMissingDecisionRecordSections_CommentOnlyBodyBypass(t *testing.T) {
	doc := "## Decision\n<!-- TODO -->\n" +
		"## Options Considered\n<!-- TODO -->\n" +
		"## Trade-offs\n<!-- TODO -->\n" +
		"## Rationale\n<!-- TODO -->\n"
	assertMissing(t, doc, allFourKeys)
}

// TestMissingDecisionRecordSections_ThematicBreakInsideRealBodyIsStripped
// proves a decorative "---" mixed into an otherwise real body is stripped,
// not counted, but does not wrongly blank the rest of the body.
func TestMissingDecisionRecordSections_ThematicBreakInsideRealBodyIsStripped(t *testing.T) {
	doc := "## Decision\nUse approach B.\n\n---\n\nSee below for why.\n" +
		"## Options Considered\nA and B.\n" +
		"## Trade-offs\nA is faster, B is simpler.\n" +
		"## Rationale\nB won.\n"
	assertMissing(t, doc, nil)
}

// TestMissingDecisionRecordSections_FencedHorizontalRuleStillCounts proves a
// "---" that is literal content inside a fenced code block (not a
// presentation token) is not stripped - only a real, unfenced thematic break
// is presentation-only.
func TestMissingDecisionRecordSections_FencedHorizontalRuleStillCounts(t *testing.T) {
	doc := "## Decision\n```text\n---\n```\n" +
		"## Options Considered\nA and B.\n" +
		"## Trade-offs\nA is faster, B is simpler.\n" +
		"## Rationale\nB won.\n"
	assertMissing(t, doc, nil)
}

// --- Finding 5: setext headings and a BOM must not be rejected ---

// TestMissingDecisionRecordSections_SetextHeadingsRecognized proves a record
// written entirely with setext-style (underlined) headings is accepted -
// well-formed markdown must not be rejected just because it is not ATX.
func TestMissingDecisionRecordSections_SetextHeadingsRecognized(t *testing.T) {
	doc := "Decision\n--------\nUse approach B.\n" +
		"Options Considered\n------------------\nA and B.\n" +
		"Trade-offs\n----------\nA is faster, B is simpler.\n" +
		"Rationale\n---------\nB won.\n"
	assertMissing(t, doc, nil)
}

// TestMissingDecisionRecordSections_SetextDoesNotConsumeATXHeadingLine
// proves a "---" immediately after an ATX heading line is not
// reinterpreted as turning that heading line into setext text (which would
// double-consume it) - it is correctly stripped as a thematic break instead.
func TestMissingDecisionRecordSections_SetextDoesNotConsumeATXHeadingLine(t *testing.T) {
	doc := "## Decision\n---\nUse approach B.\n" +
		"## Options Considered\nA and B.\n" +
		"## Trade-offs\nA is faster, B is simpler.\n" +
		"## Rationale\nB won.\n"
	assertMissing(t, doc, nil)
}

// TestMissingDecisionRecordSections_UTF8BOMStripped proves a UTF-8 BOM
// immediately before the first heading does not make that section report
// missing.
func TestMissingDecisionRecordSections_UTF8BOMStripped(t *testing.T) {
	doc := "\uFEFF## Decision\nUse approach B.\n" +
		"## Options Considered\nA and B.\n" +
		"## Trade-offs\nA is faster, B is simpler.\n" +
		"## Rationale\nB won.\n"
	assertMissing(t, doc, nil)
}

// TestMissingDecisionRecordSections_CRLFLineEndings proves a record using
// Windows-style CRLF line endings parses identically to one using LF.
func TestMissingDecisionRecordSections_CRLFLineEndings(t *testing.T) {
	doc := strings.ReplaceAll(
		"## Decision\nUse approach B.\n"+
			"## Options Considered\nA and B.\n"+
			"## Trade-offs\nA is faster, B is simpler.\n"+
			"## Rationale\nB won.\n",
		"\n", "\r\n",
	)
	assertMissing(t, doc, nil)
}

// --- Ordering and repetition ---

// TestMissingDecisionRecordSections_ReorderedSectionsStillRecognized proves
// the parser does not assume the four sections appear in the order they are
// asked for in the prompt.
func TestMissingDecisionRecordSections_ReorderedSectionsStillRecognized(t *testing.T) {
	doc := "## Rationale\nB won.\n" +
		"## Trade-offs\nA is faster, B is simpler.\n" +
		"## Decision\nUse approach B.\n" +
		"## Options Considered\nA and B.\n"
	assertMissing(t, doc, nil)
}

// TestMissingDecisionRecordSections_DuplicateHeadingAnyNonEmptyOccurrenceCounts
// documents the parser's behavior on a duplicated heading: since nothing
// requires the agent to write each section exactly once, a section is
// considered present if AT LEAST ONE occurrence of its heading has non-empty
// body content, regardless of position.
func TestMissingDecisionRecordSections_DuplicateHeadingAnyNonEmptyOccurrenceCounts(t *testing.T) {
	firstEmpty := "## Decision\n\n## Decision\nUse approach B.\n" +
		"## Options Considered\nA and B.\n" +
		"## Trade-offs\nA is faster, B is simpler.\n" +
		"## Rationale\nB won.\n"
	assertMissing(t, firstEmpty, nil)

	secondEmpty := "## Decision\nUse approach B.\n## Decision\n\n" +
		"## Options Considered\nA and B.\n" +
		"## Trade-offs\nA is faster, B is simpler.\n" +
		"## Rationale\nB won.\n"
	assertMissing(t, secondEmpty, nil)

	bothEmpty := "## Decision\n\n## Decision\n\n" +
		"## Options Considered\nA and B.\n" +
		"## Trade-offs\nA is faster, B is simpler.\n" +
		"## Rationale\nB won.\n"
	assertMissing(t, bothEmpty, []string{"decision"})
}

// --- End-to-end: the exact review bypass documents through EvaluateExitGate ---

// TestEvaluateExitGate_DecisionRecord_RejectsCommentedHeadingsBypass runs
// finding 1's exact HTML-comment document through the real gate entry point,
// not just the parser helper.
func TestEvaluateExitGate_DecisionRecord_RejectsCommentedHeadingsBypass(t *testing.T) {
	worktree := t.TempDir()
	artifactDir := t.TempDir()
	doc := "<!--\n## Decision\nplaceholder\n## Options Considered\nplaceholder\n" +
		"## Trade-offs\nplaceholder\n## Rationale\nplaceholder\n-->\n"
	if err := os.WriteFile(filepath.Join(artifactDir, "decision-record.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	wf := decisionRecordGateWorkflow()
	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: artifactDir, BeadID: "kb-1"})
	if ok {
		t.Fatal("a record whose headings exist only inside an HTML comment must not pass")
	}
	for _, key := range allFourKeys {
		if !strings.Contains(reason, key) {
			t.Errorf("reason must name %q as missing, got %q", key, reason)
		}
	}
}

// TestEvaluateExitGate_DecisionRecord_RejectsFencedHeadingsBypass runs
// finding 1's fenced-code-block variant through the real gate entry point.
func TestEvaluateExitGate_DecisionRecord_RejectsFencedHeadingsBypass(t *testing.T) {
	worktree := t.TempDir()
	artifactDir := t.TempDir()
	doc := "```\n## Decision\nplaceholder\n## Options Considered\nplaceholder\n" +
		"## Trade-offs\nplaceholder\n## Rationale\nplaceholder\n```\n"
	if err := os.WriteFile(filepath.Join(artifactDir, "decision-record.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	wf := decisionRecordGateWorkflow()
	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: artifactDir, BeadID: "kb-1"})
	if ok {
		t.Fatal("a record whose headings exist only inside a fenced code block must not pass")
	}
	for _, key := range allFourKeys {
		if !strings.Contains(reason, key) {
			t.Errorf("reason must name %q as missing, got %q", key, reason)
		}
	}
}

// TestEvaluateExitGate_DecisionRecord_RejectsHorizontalRuleBodies runs
// finding 2's "---" body variant through the real gate entry point.
func TestEvaluateExitGate_DecisionRecord_RejectsHorizontalRuleBodies(t *testing.T) {
	worktree := t.TempDir()
	artifactDir := t.TempDir()
	doc := "## Decision\n---\n## Options Considered\n---\n## Trade-offs\n---\n## Rationale\n---\n"
	if err := os.WriteFile(filepath.Join(artifactDir, "decision-record.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	wf := decisionRecordGateWorkflow()
	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: artifactDir, BeadID: "kb-1"})
	if ok {
		t.Fatal("a record whose bodies are only horizontal rules must not pass")
	}
	for _, key := range allFourKeys {
		if !strings.Contains(reason, key) {
			t.Errorf("reason must name %q as missing, got %q", key, reason)
		}
	}
}

// TestEvaluateExitGate_DecisionRecord_RejectsCommentOnlyBodies runs finding
// 2's HTML-comment body variant through the real gate entry point.
func TestEvaluateExitGate_DecisionRecord_RejectsCommentOnlyBodies(t *testing.T) {
	worktree := t.TempDir()
	artifactDir := t.TempDir()
	doc := "## Decision\n<!-- TODO -->\n## Options Considered\n<!-- TODO -->\n" +
		"## Trade-offs\n<!-- TODO -->\n## Rationale\n<!-- TODO -->\n"
	if err := os.WriteFile(filepath.Join(artifactDir, "decision-record.md"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	wf := decisionRecordGateWorkflow()
	ok, reason := EvaluateExitGate(wf, ExitGateContext{FromState: "implementation", WorktreePath: worktree, ArtifactDir: artifactDir, BeadID: "kb-1"})
	if ok {
		t.Fatal("a record whose bodies are only HTML comments must not pass")
	}
	for _, key := range allFourKeys {
		if !strings.Contains(reason, key) {
			t.Errorf("reason must name %q as missing, got %q", key, reason)
		}
	}
}
