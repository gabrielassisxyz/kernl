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
