package notes

import (
	"strings"
	"testing"
)

// applyHunks applies replacement hunks right-to-left so earlier offsets stay
// valid, mirroring how the editor commits accepted hunks.
func applyHunks(current string, hunks []SuggestHunk) string {
	out := current
	for i := len(hunks) - 1; i >= 0; i-- {
		h := hunks[i]
		out = out[:h.From] + h.Content + out[h.To:]
	}
	return out
}

func TestDiffReconstructs(t *testing.T) {
	cases := []struct {
		name     string
		current  string
		proposed string
	}{
		{"identical", "a\nb\nc\n", "a\nb\nc\n"},
		{"replace middle line", "a\nb\nc\n", "a\nBETA\nc\n"},
		{"insert line", "a\nc\n", "a\nb\nc\n"},
		{"delete line", "a\nb\nc\n", "a\nc\n"},
		{"append at end (trailing nl)", "a\nb\n", "a\nb\nc\n"},
		{"append at end (no trailing nl)", "a\nb", "a\nb\nc"},
		{"prepend at start", "b\nc\n", "a\nb\nc\n"},
		{"full rewrite", "old content here", "totally different text"},
		{"empty to content", "", "hello world"},
		{"content to empty", "hello world", ""},
		{"frontmatter body edit", "---\nid: x\n---\n\nThe quick brown fox.\n", "---\nid: x\n---\n\nThe quick red fox jumped.\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hunks := Diff(tc.current, tc.proposed)
			got := applyHunks(tc.current, hunks)
			if got != tc.proposed {
				t.Errorf("reconstruction mismatch:\n current=%q\n hunks=%+v\n got=%q\n want=%q", tc.current, hunks, got, tc.proposed)
			}
			if tc.current == tc.proposed && hunks != nil {
				t.Errorf("expected nil hunks for identical input, got %+v", hunks)
			}
			if tc.current != tc.proposed && len(hunks) == 0 {
				t.Errorf("expected a hunk for differing input, got none")
			}
		})
	}
}

func TestDiffBodyProtectsFrontmatter(t *testing.T) {
	current := "---\nid: 019e-abc\ntitle: \"My Note\"\n---\n\nThe quick brown fox.\n"
	// LLM returns a rewritten body that omits the frontmatter entirely.
	proposedBody := "The quick RED fox jumped over the lazy dog.\n"

	hunks := DiffBody(current, proposedBody)
	if len(hunks) == 0 {
		t.Fatal("expected a hunk")
	}
	fm, _ := SplitFrontmatter(current)
	// Every hunk must start at or after the end of the frontmatter.
	for _, h := range hunks {
		if h.From < len(fm) {
			t.Errorf("hunk From=%d intrudes into frontmatter (len %d)", h.From, len(fm))
		}
	}
	// Applying the hunks must preserve the frontmatter verbatim.
	got := applyHunks(current, hunks)
	if !strings.HasPrefix(got, fm) {
		t.Errorf("frontmatter not preserved after applying hunks:\n%s", got)
	}
	if !strings.Contains(got, "id: 019e-abc") {
		t.Errorf("note id lost after suggestion:\n%s", got)
	}
}

func TestDiffHunkIsLineAligned(t *testing.T) {
	current := "alpha\nbravo\ncharlie\n"
	proposed := "alpha\nBRAVO\ncharlie\n"
	hunks := Diff(current, proposed)
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	// From should sit at the start of the "bravo" line (offset 6), To just past
	// its newline (offset 12) — a whole-line replacement.
	if hunks[0].From != 6 || hunks[0].To != 12 {
		t.Errorf("expected line-aligned [6,12), got [%d,%d) content=%q", hunks[0].From, hunks[0].To, hunks[0].Content)
	}
	if hunks[0].Content != "BRAVO\n" {
		t.Errorf("expected content %q, got %q", "BRAVO\n", hunks[0].Content)
	}
}

func TestApplySuggestHunks(t *testing.T) {
	// A DA-proposed whole-body rewrite must apply cleanly and leave frontmatter.
	current := "---\nid: n1\ntitle: T\n---\n\nold body line\n"
	proposedBody := "new body line\n"
	hunks := DiffBody(current, proposedBody)

	got := ApplySuggestHunks(current, hunks)
	fm, _ := SplitFrontmatter(current)
	if !strings.HasPrefix(got, fm) {
		t.Errorf("frontmatter not preserved:\n%q", got)
	}
	if !strings.Contains(got, "new body line") || strings.Contains(got, "old body line") {
		t.Errorf("body not replaced:\n%q", got)
	}

	// Empty hunk set is a no-op.
	if ApplySuggestHunks(current, nil) != current {
		t.Error("nil hunks should be a no-op")
	}

	// Out-of-range hunks are skipped, never panic or corrupt.
	bad := []SuggestHunk{{ID: "x", From: 1000, To: 2000, Content: "boom"}}
	if ApplySuggestHunks(current, bad) != current {
		t.Error("out-of-range hunk should be skipped")
	}
}
