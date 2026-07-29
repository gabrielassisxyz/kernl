package notes

import "strings"

// AppendBlock returns note with block added at the bottom, separated from the
// existing content by exactly one blank line.
//
// Trailing blank lines in the note are collapsed rather than preserved: an
// append-only log grows one block at a time, and a caller that hands over a
// block ending in "\n\n" would otherwise widen the gap on every single write
// until the file is mostly whitespace.
func AppendBlock(note, block string) string {
	b := normalizeBlock(block)
	head := strings.TrimRight(note, "\n")
	if head == "" {
		return b
	}
	return head + "\n\n" + b
}

// PrependBlock returns note with block added at the top of the BODY - after
// the YAML frontmatter, never before it.
//
// Inserting at byte 0 would push the opening `---` off the first line, and a
// note whose frontmatter no longer parses has lost its id: the reconciler stops
// recognising the file as the node it already indexed and adopts it as a brand
// new note, orphaning every edge pointing at the old one. The frontmatter is
// kernl's identity record, so "the start of the note" can only mean the start
// of what the user wrote.
func PrependBlock(note, block string) string {
	fm, body := SplitFrontmatter(note)
	b := normalizeBlock(block)
	rest := strings.TrimLeft(body, "\n")

	var out strings.Builder
	out.WriteString(fm)
	if fm != "" {
		// Editors and the vault's own notes keep a blank line under the closing
		// fence; without it the first heading renders glued to the frontmatter.
		out.WriteString("\n")
	}
	out.WriteString(b)
	if rest != "" {
		out.WriteString("\n")
		out.WriteString(rest)
	}
	return out.String()
}

// InsertAfterThematicBreak returns note with block placed directly below its
// first thematic break, and reports whether such a break was found.
//
// This is the shape a newest-first log actually has: frontmatter, then a
// preamble the reader must see (do not read this file whole, here are the
// searches that answer a question without loading 160 KB), then a `---`, then
// the newest entry. PrependBlock puts the block above that preamble, which is
// correct for "the top of the note" and wrong for this file; the entry belongs
// under the rule.
//
// The anchor is structural rather than numeric on purpose: there is no offset
// to go stale between the caller reading the note and writing to it. Either the
// break is there or the caller is told it is not - ok is false, and NOTHING is
// guessed. A log whose separator someone removed must fail rather than quietly
// start collecting entries above the preamble, because every entry would still
// look right on its own and only the file's shape would rot.
func InsertAfterThematicBreak(note, block string) (string, bool) {
	fm, body := SplitFrontmatter(note)
	cut := afterFirstThematicBreak(body)
	if cut < 0 {
		return note, false
	}
	b := normalizeBlock(block)
	rest := strings.TrimLeft(body[cut:], "\n")

	var out strings.Builder
	out.WriteString(fm)
	out.WriteString(body[:cut]) // through the break line and its newline
	out.WriteString("\n")
	out.WriteString(b)
	if rest != "" {
		out.WriteString("\n")
		out.WriteString(rest)
	}
	return out.String(), true
}

// afterFirstThematicBreak returns the offset just past the newline of the first
// thematic break in body, or -1 when there is none.
//
// body is the note MINUS its frontmatter, which is what keeps the frontmatter's
// closing fence from being mistaken for a rule. That confusion is not
// hypothetical: a caller that searched the whole file for the first "\n---\n"
// found the fence on every note that has frontmatter, and inserted above the
// preamble while believing it was inserting below the rule.
func afterFirstThematicBreak(body string) int {
	offset := 0
	prevBlank := true // the first line of the body begins a block
	var fence string  // the ``` or ~~~ run that opened the code block we are inside
	for _, line := range strings.SplitAfter(body, "\n") {
		if line == "" {
			break // SplitAfter's trailing empty element
		}
		text := strings.TrimRight(line, "\n")
		switch {
		case fence != "":
			if closesCodeFence(text, fence) {
				fence = ""
			}
		case openCodeFence(text) != "":
			fence = openCodeFence(text)
		case isThematicBreak(text) && !isSetextUnderline(text, prevBlank):
			return offset + len(line)
		}
		prevBlank = strings.TrimSpace(text) == ""
		offset += len(line)
	}
	return -1
}

// isSetextUnderline reports whether a line that looks like a thematic break is
// really the underline of a setext heading. Only the `-` form is ambiguous:
// `Title` followed by `---` is an h2, and inserting between the two would strip
// a heading off its own text. `***` and `___` interrupt a paragraph as rules
// and carry no such second meaning, so they are not held to the blank line.
func isSetextUnderline(line string, prevBlank bool) bool {
	return !prevBlank && strings.Contains(line, "-")
}

// isThematicBreak reports whether a line is a CommonMark thematic break: up to
// three leading spaces, then three or more of `-`, `_` or `*`, with any amount
// of internal whitespace and nothing else.
//
// The `-` form is the one these notes use, but a rule written `***` is the same
// construct and a verb named after thematic breaks that only saw one third of
// them would be lying about what it looks for.
//
// Whether a matching line is really a rule also depends on what precedes it;
// that second half is isSetextUnderline's job.
func isThematicBreak(line string) bool {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return false // four spaces makes it an indented code block
	}
	stripped := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, trimmed)
	if len(stripped) < 3 {
		return false
	}
	marker := stripped[0]
	if marker != '-' && marker != '_' && marker != '*' {
		return false
	}
	return strings.Count(stripped, string(marker)) == len(stripped)
}

// openCodeFence returns the fence that a line opens (the run of backticks or
// tildes), or "" when the line opens nothing. Fenced blocks have to be tracked
// because these logs are full of shell recipes, and a `---` inside one is
// sample text, not a rule the note is divided by.
func openCodeFence(line string) string {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return ""
	}
	for _, marker := range []byte{'`', '~'} {
		run := leadingRun(trimmed, marker)
		if run < 3 {
			continue
		}
		// An info string may not contain a backtick, which is what keeps
		// `code` spans in a paragraph from reading as a fence opener.
		if marker == '`' && strings.ContainsRune(trimmed[run:], '`') {
			continue
		}
		return trimmed[:run]
	}
	return ""
}

// closesCodeFence reports whether a line closes the given open fence: the same
// marker, at least as long, and nothing else on the line.
func closesCodeFence(line, fence string) bool {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return false
	}
	run := leadingRun(trimmed, fence[0])
	return run >= len(fence) && strings.TrimSpace(trimmed[run:]) == ""
}

func leadingRun(s string, c byte) int {
	n := 0
	for n < len(s) && s[n] == c {
		n++
	}
	return n
}

// normalizeBlock strips the blank lines around a block and gives it exactly one
// trailing newline, so the separator between block and note is decided in one
// place instead of depending on how the caller's heredoc happened to end.
func normalizeBlock(block string) string {
	trimmed := strings.Trim(block, "\n")
	if trimmed == "" {
		return ""
	}
	return trimmed + "\n"
}
