package notes_test

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/notes"
)

const noteWithFrontmatter = "---\nid: 019f14ca-58ad-7203-8cf8-487f765f0001\ntitle: Log\n---\n\n# Log\n\n## older entry\n\ntext\n"

func TestAppendBlockPutsTheBlockLastWithOneBlankLine(t *testing.T) {
	got := notes.AppendBlock(noteWithFrontmatter, "## newer entry\n\nmore text")
	want := noteWithFrontmatter + "\n## newer entry\n\nmore text\n"
	if got != want {
		t.Fatalf("append produced:\n%q\nwant:\n%q", got, want)
	}
}

// Repeated appends must not widen the gap between blocks: the log is written one
// entry at a time and the spacing has to be a fixed point, not a function of how
// many times the verb has run.
func TestAppendBlockSpacingIsStableAcrossRepeatedAppends(t *testing.T) {
	doc := "---\nid: x\n---\n\n# Log\n"
	for i := 0; i < 3; i++ {
		doc = notes.AppendBlock(doc, "\n## entry\n\n\n")
	}
	if strings.Contains(doc, "\n\n\n") {
		t.Fatalf("repeated appends must not accumulate blank lines, got:\n%q", doc)
	}
	if n := strings.Count(doc, "## entry"); n != 3 {
		t.Fatalf("expected 3 entries, got %d in:\n%q", n, doc)
	}
	if !strings.HasSuffix(doc, "## entry\n") {
		t.Fatalf("the note must end with exactly one newline, got:\n%q", doc)
	}
}

func TestAppendBlockOnAnEmptyNoteIsJustTheBlock(t *testing.T) {
	if got := notes.AppendBlock("", "## only\n"); got != "## only\n" {
		t.Fatalf("got %q", got)
	}
	if got := notes.AppendBlock("\n\n", "## only"); got != "## only\n" {
		t.Fatalf("a whitespace-only note must not leave leading blank lines, got %q", got)
	}
}

// The load-bearing one: a prepend that lands before the closing `---` destroys
// the note's id and the reconciler adopts the file as a new node.
func TestPrependBlockKeepsFrontmatterFirst(t *testing.T) {
	got := notes.PrependBlock(noteWithFrontmatter, "## newest entry\n\nfresh")
	want := "---\nid: 019f14ca-58ad-7203-8cf8-487f765f0001\ntitle: Log\n---\n\n## newest entry\n\nfresh\n\n# Log\n\n## older entry\n\ntext\n"
	if got != want {
		t.Fatalf("prepend produced:\n%q\nwant:\n%q", got, want)
	}
	if !strings.HasPrefix(got, "---\nid: ") {
		t.Fatal("frontmatter must still open the file")
	}
}

func TestPrependBlockWithoutFrontmatterStartsAtByteZero(t *testing.T) {
	got := notes.PrependBlock("# Plain\n\nbody\n", "## newest")
	want := "## newest\n\n# Plain\n\nbody\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPrependBlockOnAFrontmatterOnlyNoteAddsNoTrailingBlankLine(t *testing.T) {
	got := notes.PrependBlock("---\nid: x\n---\n", "## first")
	want := "---\nid: x\n---\n\n## first\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// logShapedNote is what a newest-first machine log looks like: frontmatter, a
// preamble the reader must see, a fenced recipe inside it, the rule, then
// entries newest first.
const logShapedNote = "---\nid: 019f14ca-58ad-7203-8cf8-487f765f0003\ntitle: Log\n---\n\n" +
	"# Log\n\nDo not read this file whole.\n\n" +
	"```sh\nrg '^## ' \"$LOG\"\n\n---\ncat -- - <<'X'\n```\n\n" +
	"Closing word of the preamble.\n\n" +
	"---\n\n## 2026-07-29 - newest\n\nbody\n"

func TestInsertAfterThematicBreakLandsUnderTheRule(t *testing.T) {
	got, ok := notes.InsertAfterThematicBreak(logShapedNote, "## 2026-07-30 - newer\n\nfresh")
	if !ok {
		t.Fatal("the note has a rule; the insert must find it")
	}
	want := "---\nid: 019f14ca-58ad-7203-8cf8-487f765f0003\ntitle: Log\n---\n\n" +
		"# Log\n\nDo not read this file whole.\n\n" +
		"```sh\nrg '^## ' \"$LOG\"\n\n---\ncat -- - <<'X'\n```\n\n" +
		"Closing word of the preamble.\n\n" +
		"---\n\n## 2026-07-30 - newer\n\nfresh\n\n## 2026-07-29 - newest\n\nbody\n"
	if got != want {
		t.Fatalf("insert produced:\n%q\nwant:\n%q", got, want)
	}
}

// The `---` inside the fenced recipe is sample text. Landing there would bury
// the entry in the middle of a shell block where nothing renders it as an entry.
func TestInsertAfterThematicBreakIgnoresRulesInsideCodeFences(t *testing.T) {
	got, _ := notes.InsertAfterThematicBreak(logShapedNote, "## newer")
	fenced := strings.Index(got, "```sh")
	inserted := strings.Index(got, "## newer")
	if inserted < fenced {
		t.Fatalf("the block landed inside or above the fenced recipe:\n%s", got)
	}
	if !strings.Contains(got, "```sh\nrg '^## ' \"$LOG\"\n\n---\ncat -- - <<'X'\n```") {
		t.Fatal("the fenced recipe must come through untouched")
	}
}

func TestInsertAfterThematicBreakIgnoresTildeFencesToo(t *testing.T) {
	note := "~~~\n\n---\n~~~\n\n---\n\nafter\n"
	got, ok := notes.InsertAfterThematicBreak(note, "X")
	if !ok {
		t.Fatal("the rule outside the fence must be found")
	}
	if got != "~~~\n\n---\n~~~\n\n---\n\nX\n\nafter\n" {
		t.Fatalf("got %q", got)
	}
}

// The frontmatter's closing fence is three dashes on their own line and is not
// a rule. Treating it as one is the bug this placement exists to avoid.
func TestInsertAfterThematicBreakNeverTreatsTheFrontmatterFenceAsARule(t *testing.T) {
	note := "---\nid: x\ntitle: No rule here\n---\n\n# Only a heading\n\nbody\n"
	if _, ok := notes.InsertAfterThematicBreak(note, "X"); ok {
		t.Fatal("a note whose only `---` lines are the frontmatter has no thematic break")
	}
}

// Failing loudly is the requirement: a log whose separator someone deleted must
// not quietly start collecting entries somewhere else.
func TestInsertAfterThematicBreakReportsAMissingRuleAndChangesNothing(t *testing.T) {
	note := "---\nid: x\n---\n\n# Log\n\n## entry\n"
	got, ok := notes.InsertAfterThematicBreak(note, "X")
	if ok {
		t.Fatal("there is no rule to insert after")
	}
	if got != note {
		t.Fatalf("the note must come back untouched, got %q", got)
	}
}

// `Title` over `---` is a setext h2, not a rule; inserting between the two would
// strip the heading off its own text.
func TestInsertAfterThematicBreakSkipsASetextHeadingUnderline(t *testing.T) {
	note := "---\nid: x\n---\n\nA setext heading\n---\n\nprose\n\n---\n\nnewest\n"
	got, ok := notes.InsertAfterThematicBreak(note, "X")
	if !ok {
		t.Fatal("the real rule further down must still be found")
	}
	if !strings.Contains(got, "A setext heading\n---\n\nprose") {
		t.Fatalf("the heading and its underline must stay together, got:\n%s", got)
	}
	if !strings.Contains(got, "prose\n\n---\n\nX\n\nnewest\n") {
		t.Fatalf("the block must land under the real rule, got:\n%s", got)
	}
}

func TestInsertAfterThematicBreakAcceptsTheOtherRuleMarkers(t *testing.T) {
	for _, rule := range []string{"***", "___", "* * *", "- - -", "-----"} {
		note := "---\nid: x\n---\n\npreamble\n\n" + rule + "\n\nnewest\n"
		got, ok := notes.InsertAfterThematicBreak(note, "X")
		if !ok {
			t.Fatalf("%q is a thematic break and must be found", rule)
		}
		if !strings.Contains(got, rule+"\n\nX\n\nnewest\n") {
			t.Fatalf("%q: block landed wrong:\n%s", rule, got)
		}
	}
}

// Spacing has to be a fixed point here too, or a log drifts a blank line wider
// on every entry.
func TestInsertAfterThematicBreakSpacingIsStableAcrossRepeatedInserts(t *testing.T) {
	doc := "---\nid: x\n---\n\npreamble\n\n---\n\n## first\n"
	for i := 0; i < 3; i++ {
		var ok bool
		doc, ok = notes.InsertAfterThematicBreak(doc, "\n## entry\n\n\n")
		if !ok {
			t.Fatal("the rule must survive its own inserts")
		}
	}
	if strings.Contains(doc, "\n\n\n") {
		t.Fatalf("repeated inserts must not accumulate blank lines, got:\n%q", doc)
	}
	if !strings.HasPrefix(doc, "---\nid: x\n---\n\npreamble\n\n---\n\n## entry\n\n## entry\n\n## entry\n\n## first\n") {
		t.Fatalf("unexpected shape:\n%q", doc)
	}
}

func TestInsertAfterThematicBreakWhenTheRuleIsTheLastLine(t *testing.T) {
	got, ok := notes.InsertAfterThematicBreak("---\nid: x\n---\n\npreamble\n\n---\n", "X")
	if !ok {
		t.Fatal("a trailing rule is still a rule")
	}
	if got != "---\nid: x\n---\n\npreamble\n\n---\n\nX\n" {
		t.Fatalf("got %q", got)
	}
}

func TestInsertAfterThematicBreakWorksWithoutFrontmatter(t *testing.T) {
	got, ok := notes.InsertAfterThematicBreak("preamble\n\n---\n\nnewest\n", "X")
	if !ok {
		t.Fatal("no frontmatter is not the same as no rule")
	}
	if got != "preamble\n\n---\n\nX\n\nnewest\n" {
		t.Fatalf("got %q", got)
	}
}
