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
