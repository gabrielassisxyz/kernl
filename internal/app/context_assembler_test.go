package app

import (
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

	if len(got) > contextBudgetBytes+512 {
		// Some slack for the heading and the cut marker themselves, but the
		// file content must not have sailed through uncut.
		t.Errorf("output is %d bytes, want it capped near the %d byte budget", len(got), contextBudgetBytes)
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
