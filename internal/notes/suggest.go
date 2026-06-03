package notes

import "strings"

// SuggestHunk is a suggested edit: replace the [From,To) byte range of the
// current document with Content. This matches the editor's acceptHunk shape
// (CodeMirror changes{from,to,insert}), so accepting a hunk is a single,
// reversible range replacement — the user always commits, never the LLM.
type SuggestHunk struct {
	ID      string `json:"id"`
	From    int    `json:"from"`
	To      int    `json:"to"`
	Content string `json:"content"`
}

// SplitFrontmatter separates a leading YAML frontmatter block (including its
// closing `---` line and trailing newline) from the body. When there is no
// frontmatter, fm is "" and body is the whole input.
func SplitFrontmatter(s string) (fm, body string) {
	if !strings.HasPrefix(s, "---\n") {
		return "", s
	}
	idx := strings.Index(s[4:], "\n---")
	if idx < 0 {
		return "", s
	}
	closeStart := 4 + idx + 1 // start of the closing `---`
	rest := s[closeStart:]
	nl := strings.IndexByte(rest, '\n')
	if nl < 0 {
		return s, "" // entire input is frontmatter
	}
	split := closeStart + nl + 1
	return s[:split], s[split:]
}

// DiffBody computes suggestion hunks for a note while protecting its
// frontmatter: it diffs only the body against proposedBody and offsets the
// hunks so they apply to the full document. The frontmatter (and the note's
// id) is never altered — accepting a suggestion can never break graph identity.
func DiffBody(current, proposedBody string) []SuggestHunk {
	fm, body := SplitFrontmatter(current)
	hunks := Diff(body, proposedBody)
	if len(fm) > 0 {
		for i := range hunks {
			hunks[i].From += len(fm)
			hunks[i].To += len(fm)
		}
	}
	return hunks
}

// Diff computes the minimal line-aligned replacement that transforms current
// into proposed, returned as a single hunk. The hunk is the tightest byte
// range around the change, snapped outward to line boundaries for display.
//
// Invariant (verified by tests): for the returned hunk h,
//
//	current[:h.From] + h.Content + current[h.To:] == proposed
//
// so applying it can never corrupt the document. Returns nil when identical.
// (Single-hunk by design for v1; multi-region granularity is a future step.)
func Diff(current, proposed string) []SuggestHunk {
	if current == proposed {
		return nil
	}

	// Longest common prefix (byte-wise).
	p := 0
	for p < len(current) && p < len(proposed) && current[p] == proposed[p] {
		p++
	}
	// Longest common suffix that does not overlap the prefix.
	s := 0
	for s < len(current)-p && s < len(proposed)-p &&
		current[len(current)-1-s] == proposed[len(proposed)-1-s] {
		s++
	}

	from := p
	to := len(current) - s

	// Snap the range outward to whole lines so the diff panel shows line-aligned
	// edits. Snapping outward preserves the common-prefix/suffix invariant.
	from = lineStart(current, from)
	to = lineEnd(current, to)

	content := proposed[from : len(proposed)-(len(current)-to)]

	return []SuggestHunk{{ID: "h0", From: from, To: to, Content: content}}
}

// lineStart returns the offset of the start of the line containing pos.
func lineStart(s string, pos int) int {
	if i := strings.LastIndexByte(s[:pos], '\n'); i >= 0 {
		return i + 1
	}
	return 0
}

// lineEnd returns the offset just past the newline ending the line containing
// pos (or len(s) if pos is on the last, unterminated line).
func lineEnd(s string, pos int) int {
	if pos >= len(s) {
		return len(s)
	}
	if i := strings.IndexByte(s[pos:], '\n'); i >= 0 {
		return pos + i + 1
	}
	return len(s)
}
