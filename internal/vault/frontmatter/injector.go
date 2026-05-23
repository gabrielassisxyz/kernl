package frontmatter

import (
	"bytes"
	"fmt"
	"strings"
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
		// Already has an id — idempotent no-op
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
		// No frontmatter block — prepend minimal block
		return prependBlock(bom, content, uuid), nil

	}

	// After "---" we expect newline
	lineEnd := 3
	if lineEnd < len(content) && content[lineEnd] == '\r' {
		lineEnd++
	}
	if lineEnd >= len(content) || content[lineEnd] != '\n' {
		// "---" not followed by newline — treat as no block
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

// determineNewline probes the first 1024 bytes for CRLF anywhere, defaulting to LF.
func determineNewline(b []byte) string {
	for i := 0; i < len(b)-1 && i < 1024; i++ {
		if b[i] == '\r' && b[i+1] == '\n' {
			return "\r\n"
		}
	}
	return "\n"
}
