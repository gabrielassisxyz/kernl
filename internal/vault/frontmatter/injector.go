package frontmatter

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// InjectID surgically inserts an `id: <uuid>` line into a file's frontmatter
// without disturbing any other byte. It returns the modified content with the
// injected UUID, or the original content unchanged if an id is already present.
//
// Behavior:
//   - If frontmatter exists and has no `id` line: inserts `id: <uuid>` immediately
//     after the opening "---\n" fence, preserving all other bytes.
//   - If frontmatter exists and already has an `id` line: returns original bytes unchanged.
//   - If no frontmatter block exists: prepends a minimal `---\nid: <uuid>\n---\n` block.
//   - Preserves BOM, line endings (LF/CRLF), comments, blank lines, and key order.
func InjectID(raw []byte, uuid string) ([]byte, error) {
	if uuid == "" {
		return nil, fmt.Errorf("frontmatter: uuid must not be empty")
	}

	// Check if an id already exists
	fm, err := Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("frontmatter: cannot inject into malformed YAML: %w", err)
	}
	if fm.ID != "" {
		// Already has an id - idempotent no-op
		return bytes.Clone(raw), nil
	}

	// Determine BOM prefix
	var bom []byte
	content := raw
	if len(content) >= 3 && content[0] == 0xEF && content[1] == 0xBB && content[2] == 0xBF {
		bom = content[:3]
		content = content[3:]
	}

	// Check if frontmatter block exists
	if len(content) < 4 || content[0] != '-' || content[1] != '-' || content[2] != '-' {
		// No frontmatter block - prepend minimal block
		return prependBlock(bom, content, uuid), nil

	}

	// After "---" we expect newline
	lineEnd := 3
	if lineEnd < len(content) && content[lineEnd] == '\r' {
		lineEnd++
	}
	if lineEnd >= len(content) || content[lineEnd] != '\n' {
		// "---" not followed by newline - treat as no block
		return prependBlock(bom, content, uuid), nil
	}

	// Determine the newline sequence used in the file.
	nl := determineNewline(content)

	// Insert `id: <uuid>` right after the opening "---\n" or "---\r\n"
	insertPos := len(bom) + lineEnd + 1 // after "---" + newline

	// Check if id already exists in the frontmatter block to avoid double-insertion
	blockEnd := -1
	for i := insertPos; i < len(raw); i++ {
		if raw[i] == '-' && i+2 < len(raw) && raw[i+1] == '-' && raw[i+2] == '-' {
			if i > 0 && (raw[i-1] == '\n' || (raw[i-1] == '\r' && i > 1 && raw[i-2] == '\n')) {
				blockEnd = i
				break
			}
		}
	}

	if blockEnd > insertPos {
		blockContent := string(raw[insertPos:blockEnd])
		if strings.Contains(blockContent, "\nid:") || strings.Contains(blockContent, "\r\nid:") || strings.HasPrefix(blockContent, "id:") {
			return bytes.Clone(raw), nil
		}
	}

	var buf bytes.Buffer
	buf.Write(raw[:insertPos])
	buf.WriteString("id: " + uuid)
	buf.WriteString(nl)
	buf.Write(raw[insertPos:])

	return buf.Bytes(), nil
}

// prependBlock creates a minimal frontmatter block with just the id.
func prependBlock(bom, content []byte, uuid string) []byte {
	nl := determineNewline(content)
	var buf bytes.Buffer
	buf.Write(bom)
	buf.WriteString("---" + nl)
	buf.WriteString("id: " + uuid + nl)
	buf.WriteString("---" + nl)
	buf.Write(content)
	return buf.Bytes()
}

// InjectTags surgically replaces the `tags:` key in a file's frontmatter with
// the given list, or removes the key entirely when tags is empty. Unlike
// InjectID this is never a no-op by presence: it always REPLACES whatever
// tags: block is already there, because a merge that does not show up in the
// file is exactly the bug this exists to prevent - see the note-write CLI's
// --tags flag.
//
// Behavior:
//   - Frontmatter exists and has a `tags:` key (block-list or flow style):
//     that key and everything indented under it is replaced in place, all
//     other bytes preserved untouched (an `id:` line included).
//   - Frontmatter exists, no `tags:` key, tags is non-empty: a `tags:` block
//     is appended at the end of the frontmatter, right before the closing
//     fence.
//   - Frontmatter exists, no `tags:` key, tags is empty: no-op, nothing to
//     remove.
//   - No frontmatter block at all, tags is non-empty: a minimal block is
//     created holding just tags, in the same shape InjectID produces for id.
//   - No frontmatter block at all, tags is empty: no-op, nothing to inject.
func InjectTags(raw []byte, tags []string) ([]byte, error) {
	if _, err := Parse(raw); err != nil {
		return nil, fmt.Errorf("frontmatter: cannot inject into malformed YAML: %w", err)
	}

	var bom []byte
	content := raw
	if len(content) >= 3 && content[0] == 0xEF && content[1] == 0xBB && content[2] == 0xBF {
		bom = content[:3]
		content = content[3:]
	}

	if len(content) < 4 || content[0] != '-' || content[1] != '-' || content[2] != '-' {
		if len(tags) == 0 {
			return bytes.Clone(raw), nil
		}
		result, err := prependTagsBlock(bom, content, tags)
		if err != nil {
			return nil, err
		}
		return validateInjectedTags(result, tags)
	}

	lineEnd := 3
	if lineEnd < len(content) && content[lineEnd] == '\r' {
		lineEnd++
	}
	if lineEnd >= len(content) || content[lineEnd] != '\n' {
		if len(tags) == 0 {
			return bytes.Clone(raw), nil
		}
		result, err := prependTagsBlock(bom, content, tags)
		if err != nil {
			return nil, err
		}
		return validateInjectedTags(result, tags)
	}

	nl := determineNewline(content)
	insertPos := len(bom) + lineEnd + 1

	blockEnd := -1
	for i := insertPos; i < len(raw); i++ {
		if raw[i] == '-' && i+2 < len(raw) && raw[i+1] == '-' && raw[i+2] == '-' {
			if i > 0 && (raw[i-1] == '\n' || (raw[i-1] == '\r' && i > 1 && raw[i-2] == '\n')) {
				blockEnd = i
				break
			}
		}
	}
	if blockEnd < insertPos {
		// Unterminated block - leave the bytes alone rather than guess.
		return bytes.Clone(raw), nil
	}

	start, end, found := findTagsSpan(raw, insertPos, blockEnd)

	if found {
		var buf bytes.Buffer
		buf.Write(raw[:start])
		if len(tags) > 0 {
			rendered, err := renderTagsBlock(tags, nl)
			if err != nil {
				return nil, err
			}
			buf.WriteString(rendered)
		}
		buf.Write(raw[end:])
		return validateInjectedTags(buf.Bytes(), tags)
	}

	if len(tags) == 0 {
		return bytes.Clone(raw), nil
	}
	rendered, err := renderTagsBlock(tags, nl)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.Write(raw[:blockEnd])
	buf.WriteString(rendered)
	buf.Write(raw[blockEnd:])
	return validateInjectedTags(buf.Bytes(), tags)
}

// validateInjectedTags is the assertion that closes the hole a hand-rolled
// renderer left open: a rendering bug that produces a bare, YAML-special
// value (say a tag containing "# " or starting with "- ") would otherwise
// reach disk as HTTP 200 with corrupted or vanished frontmatter, discovered
// only later - the same id-loss shape 2026-08-01 already cost a history for.
// Rather than trust the byte surgery, InjectTags re-parses its own output
// and refuses to return it if the result does not parse, or if the tags it
// parses back are not exactly the tags it was asked to write.
func validateInjectedTags(result []byte, tags []string) ([]byte, error) {
	fm, err := Parse(result)
	if err != nil {
		return nil, fmt.Errorf("frontmatter: InjectTags produced unparseable YAML: %w", err)
	}
	if !tagsEqual(fm.Tags, tags) {
		return nil, fmt.Errorf("frontmatter: InjectTags produced a mismatch - wrote %v, read back %v", tags, fm.Tags)
	}
	return result, nil
}

// tagsEqual compares two tag lists positionally, treating nil and an empty
// slice as equal - Parse returns nil for an absent tags: key and this
// package's callers pass an empty (non-nil) slice to mean "no tags", and
// those are the same intent.
func tagsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// findTagsSpan locates a top-level `tags:` line within raw[from:to] and
// everything indented beneath it (its block-list items, or nothing at all
// for a flow-style `tags: [a, b]`). Returns the byte range to replace.
func findTagsSpan(raw []byte, from, to int) (start, end int, found bool) {
	pos := from
	for pos < to {
		lineEnd := lineEndAt(raw, pos, to)
		line := bytes.TrimRight(raw[pos:lineEnd], "\r")
		if bytes.HasPrefix(line, []byte("tags:")) {
			next := lineEnd
			if next < to {
				next++ // past the '\n'
			}
			return pos, tagsSpanEnd(raw, next, to), true
		}
		pos = lineEnd
		if pos < to {
			pos++
		}
	}
	return 0, 0, false
}

// tagsSpanEnd walks forward from the line after `tags:` while each line is
// indented (a block-list continuation), stopping at the first blank line or
// unindented line - the next top-level key, or the block's end.
func tagsSpanEnd(raw []byte, from, to int) int {
	pos := from
	for pos < to {
		lineEnd := lineEndAt(raw, pos, to)
		line := bytes.TrimRight(raw[pos:lineEnd], "\r")
		if len(line) == 0 || (line[0] != ' ' && line[0] != '\t') {
			return pos
		}
		pos = lineEnd
		if pos < to {
			pos++
		}
	}
	return to
}

// lineEndAt returns the offset of the next '\n' at or after pos, capped at to.
func lineEndAt(raw []byte, pos, to int) int {
	if i := bytes.IndexByte(raw[pos:to], '\n'); i >= 0 {
		return pos + i
	}
	return to
}

// renderTagsBlock renders a `tags:` key as a block list, one tag per line -
// the same style existing frontmatter in this vault already uses. It goes
// through the real YAML encoder rather than deciding by hand which
// characters need escaping: a tag like "foo: bar" or "#hash" written bare
// either breaks the parse or silently disappears as a comment, and the
// encoder quotes exactly the values that need it while leaving an ordinary
// tag exactly as bare as InjectID's own id: line.
func renderTagsBlock(tags []string, nl string) (string, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(map[string]any{"tags": tags}); err != nil {
		_ = enc.Close()
		return "", fmt.Errorf("frontmatter: rendering tags block: %w", err)
	}
	if err := enc.Close(); err != nil {
		return "", fmt.Errorf("frontmatter: rendering tags block: %w", err)
	}
	rendered := buf.String()
	if nl != "\n" {
		rendered = strings.ReplaceAll(rendered, "\n", nl)
	}
	return rendered, nil
}

// prependTagsBlock creates a minimal frontmatter block holding just the tags.
func prependTagsBlock(bom, content []byte, tags []string) ([]byte, error) {
	nl := determineNewline(content)
	rendered, err := renderTagsBlock(tags, nl)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.Write(bom)
	buf.WriteString("---" + nl)
	buf.WriteString(rendered)
	buf.WriteString("---" + nl)
	buf.Write(content)
	return buf.Bytes(), nil
}

// determineNewline probes the first 1024 bytes for CRLF anywhere, defaulting to LF.
func determineNewline(b []byte) string {
	for i := 0; i < len(b)-1 && i < 1024; i++ {
		if b[i] == '\r' && b[i+1] == '\n' {
			return "\r\n"
		}
	}
	return "\n"
}
