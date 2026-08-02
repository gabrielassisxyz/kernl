package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeContextFile(t *testing.T, repoPath, rel, content string) {
	t.Helper()
	full := filepath.Join(repoPath, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAssembleContext_PresentFileIsRenderedUnderItsOwnHeading(t *testing.T) {
	repo := t.TempDir()
	writeContextFile(t, repo, "README.md", "kernl is a knowledge-graph substrate.")

	got := AssembleContext(repo, []string{"README.md"})

	if !strings.Contains(got, "README.md") {
		t.Errorf("output %q does not name the file", got)
	}
	if !strings.Contains(got, "kernl is a knowledge-graph substrate.") {
		t.Errorf("output %q does not contain the file's content", got)
	}
}

func TestAssembleContext_AbsentFileRendersAnExplicitNotFoundLine(t *testing.T) {
	repo := t.TempDir()

	got := AssembleContext(repo, []string{"ROADMAP.md"})

	if !strings.Contains(got, "ROADMAP.md") {
		t.Errorf("output %q does not name the missing file", got)
	}
	if !strings.Contains(got, "not found") {
		t.Errorf("output %q does not say the file was not found - an omitted section reads as this feature never having run", got)
	}
}

func TestAssembleContext_OrderIsRespected(t *testing.T) {
	repo := t.TempDir()
	writeContextFile(t, repo, "first.md", "first content")
	writeContextFile(t, repo, "second.md", "second content")

	got := AssembleContext(repo, []string{"first.md", "second.md"})

	firstIdx := strings.Index(got, "first content")
	secondIdx := strings.Index(got, "second content")
	if firstIdx == -1 || secondIdx == -1 || firstIdx > secondIdx {
		t.Errorf("output %q did not preserve doc order", got)
	}

	reversed := AssembleContext(repo, []string{"second.md", "first.md"})
	firstIdx = strings.Index(reversed, "first content")
	secondIdx = strings.Index(reversed, "second content")
	if firstIdx == -1 || secondIdx == -1 || secondIdx > firstIdx {
		t.Errorf("output %q did not follow the reversed doc order", reversed)
	}
}

// The budget is a loud backstop against a misconfigured contextDocs list,
// not a design budget, so a file that exceeds it must be cut at a line
// boundary - never mid-line - and the cut must say so explicitly rather
// than silently handing the Oracle a shorter file.
func TestAssembleContext_BudgetIsEnforcedOnALineBoundary(t *testing.T) {
	repo := t.TempDir()
	// One line repeated well past the 32KB budget, so the cut must land
	// between two lines rather than in the middle of one.
	line := strings.Repeat("x", 100) + "\n"
	var big strings.Builder
	for i := 0; i < 400; i++ { // ~40KB, comfortably over the 32KB budget
		big.WriteString(line)
	}
	writeContextFile(t, repo, "BIG.md", big.String())

	got := AssembleContext(repo, []string{"BIG.md"})

	// The marker itself is written AFTER the cut, so it lands past the 32KB
	// backstop on purpose - the budget bounds the CONTENT, and the marker
	// has to be visible right at the boundary it names. The only thing this
	// bound must catch is content sailing through uncut, so it is exact:
	// the budget, plus the exact marker this file's own content produces,
	// with no arbitrary slack on top.
	marker := fmt.Sprintf("\n[... %s truncated: the %d KB context budget was exceeded ...]\n", "BIG.md", contextBudgetBytes/1024)
	wantMax := contextBudgetBytes + len(marker)
	if len(got) > wantMax {
		t.Errorf("output is %d bytes, want at most %d (the %d byte budget plus the %d byte marker)", len(got), wantMax, contextBudgetBytes, len(marker))
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("output %q has no cut marker", got)
	}
	// Everything before the cut marker must end in a full line: no line
	// fragment is left dangling in front of the marker.
	before, _, found := strings.Cut(got, "truncated")
	if !found {
		t.Fatal("cut marker not found")
	}
	beforeMarker := before[:strings.LastIndexByte(before, '[')]
	if beforeMarker != "" && !strings.HasSuffix(beforeMarker, "\n") {
		t.Errorf("content before the cut marker does not end on a line boundary: %q", beforeMarker[max(0, len(beforeMarker)-20):])
	}
	for _, fragment := range strings.Split(beforeMarker, "\n") {
		if fragment != "" && fragment != strings.Repeat("x", 100) && !strings.HasPrefix(fragment, "###") {
			t.Errorf("found a partial line before the cut marker: %q", fragment)
		}
	}
}

// If the first entry consumes the budget EXACTLY, remaining hits 0 without
// ever going negative - the case a plain "remaining <= 0: break" silently
// drops every later entry for. This must not fall silently off the end.
func TestAssembleContext_ExactBudgetExhaustionStillNamesLaterEntries(t *testing.T) {
	repo := t.TempDir()
	// A single file whose own rendered section is exactly contextBudgetBytes:
	// not one byte less, not one byte more. renderContextDoc's format is
	// "### %s\n\n%s\n\n" and a no-trailing-newline body is never trimmed, so
	// the fixed overhead is computable in closed form and the content fills
	// the rest exactly.
	const rel = "EXACT.md"
	overhead := len(fmt.Sprintf("### %s\n\n%s\n\n", rel, ""))
	writeContextFile(t, repo, rel, strings.Repeat("x", contextBudgetBytes-overhead))
	if got := len(renderContextDoc(repo, rel)); got != contextBudgetBytes {
		t.Fatalf("fixture's own section is %d bytes, want exactly %d", got, contextBudgetBytes)
	}

	got := AssembleContext(repo, []string{rel, "SECOND.md"})

	if !strings.Contains(got, "SECOND.md") {
		t.Errorf("output %q silently dropped SECOND.md after the budget was exhausted exactly, instead of naming it", got)
	}
}

// A read error that is not "the file does not exist" (a directory in this
// entry's place, standing in for any I/O failure) must not be folded into
// the same sentence as a normal missing file - that would tell the Oracle
// this repository publishes nothing when in fact something is broken.
func TestAssembleContext_UnreadableEntryIsReportedAsAFailureNotAMissingFile(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "NOTES.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := AssembleContext(repo, []string{"NOTES.md"})

	if strings.Contains(got, "was not found") || strings.Contains(got, "publishes none") {
		t.Errorf("output %q reported a read failure as a missing file", got)
	}
	if !strings.Contains(got, "could not be read") {
		t.Errorf("output %q does not say the entry could not be read", got)
	}
}

// A contextDocs entry that walks outside the repository (a misconfigured
// "../") must be rejected rather than silently forwarding a sibling
// directory's content to whatever llm.agent/llm.endpoint points at.
func TestAssembleContext_EntryEscapingTheRepoIsRejected(t *testing.T) {
	parent := t.TempDir()
	repo := filepath.Join(parent, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "secret.md"), []byte("outside-repo-secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := AssembleContext(repo, []string{"../secret.md"})

	if strings.Contains(got, "outside-repo-secret") {
		t.Errorf("output %q leaked a file outside the repository root", got)
	}
	if !strings.Contains(got, "../secret.md") {
		t.Errorf("output %q does not name the rejected entry", got)
	}
	if !strings.Contains(got, "outside this repository") {
		t.Errorf("output %q does not say why the entry was rejected", got)
	}
}

// An absolute rel is already contained by filepath.Join (Join("/repo",
// "/etc/passwd") == "/repo/etc/passwd"), so it must read as a normal missing
// file under the repo, not be flagged as an escape.
func TestAssembleContext_AbsoluteEntryIsContainedNotFlaggedAsAnEscape(t *testing.T) {
	repo := t.TempDir()

	got := AssembleContext(repo, []string{"/etc/passwd"})

	if strings.Contains(got, "outside this repository") {
		t.Errorf("output %q flagged an absolute entry as escaping the repository, but Join already contains it", got)
	}
	if !strings.Contains(got, "was not found") {
		t.Errorf("output %q should read as a normal missing file under the repo", got)
	}
}

// A present-but-empty file must say so plainly rather than leaving a heading
// with a blank body for the reader to guess at.
func TestAssembleContext_EmptyFileSaysSoPlainly(t *testing.T) {
	repo := t.TempDir()
	writeContextFile(t, repo, "EMPTY.md", "")

	got := AssembleContext(repo, []string{"EMPTY.md"})

	if !strings.Contains(got, "EMPTY.md") {
		t.Errorf("output %q does not name the file", got)
	}
	if !strings.Contains(got, "present but empty") {
		t.Errorf("output %q does not say the file is present but empty", got)
	}
}

func TestCutOnLineBoundary(t *testing.T) {
	cases := []struct {
		name  string
		s     string
		limit int
		want  string
	}{
		{"limit covers the whole string", "abc\ndef\n", 100, "abc\ndef\n"},
		{"cuts at the last newline within the limit", "abc\ndef\nghi\n", 9, "abc\ndef\n"},
		{"limit smaller than the first line drops it entirely", "abcdefgh\n", 4, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cutOnLineBoundary(tc.s, tc.limit); got != tc.want {
				t.Errorf("cutOnLineBoundary(%q, %d) = %q, want %q", tc.s, tc.limit, got, tc.want)
			}
		})
	}
}
