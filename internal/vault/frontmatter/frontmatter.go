// Package frontmatter reads YAML frontmatter from Markdown-style files and
// provides safe, byte-level UUID injection without disturbing any other content.
//
// Design principles:
//   - Read: extract typed values via gopkg.in/yaml.v3 (id, title, author, origin, tags).
//   - Write: NEVER marshal. Inject surgically at the byte level.
//   - Preserve BOM, line endings (LF/CRLF), comments, blank lines, and key order.
//   - Malformed YAML: detect, do NOT inject, return recoverable error; leave bytes intact.
package frontmatter

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Frontmatter holds parsed YAML frontmatter values.
type Frontmatter struct {
	ID     string   `yaml:"id"`
	Title  string   `yaml:"title"`
	Author string   `yaml:"author"`
	Origin string   `yaml:"origin"`
	Tags   []string `yaml:"tags"`
}

// Parse extracts frontmatter values from raw bytes.
// It looks for a YAML block delimited by leading "---\n" and closing "\n---\n" or "\n---"
// at end of file. If no frontmatter block is found, an empty Frontmatter is returned
// with no error — callers should treat absent frontmatter as a valid state.
func Parse(raw []byte) (*Frontmatter, error) {
	block, err := extractBlock(raw)
	if err != nil {
		return nil, err
	}
	if block == nil {
		return &Frontmatter{}, nil
	}

	var fm Frontmatter
	if err := yaml.Unmarshal(block, &fm); err != nil {
		return nil, fmt.Errorf("frontmatter: yaml parse error: %w", err)
	}
	return &fm, nil
}

// extractBlock finds the YAML frontmatter content between "---" fences.
// Returns nil, nil if no block is found.
func extractBlock(raw []byte) ([]byte, error) {
	content := raw

	// Strip UTF-8 BOM if present
	if len(content) >= 3 && content[0] == 0xEF && content[1] == 0xBB && content[2] == 0xBF {
		content = content[3:]
	}

	if len(content) < 4 {
		// Too short to have a valid block
		return nil, nil
	}

	// Must start with "---\n" or "---\r\n"
	if content[0] != '-' || content[1] != '-' || content[2] != '-' {
		return nil, nil
	}

	// After "---" we expect newline
	lineEnd := 3
	if lineEnd < len(content) && content[lineEnd] == '\r' {
		lineEnd++ // skip \r
	}
	if lineEnd >= len(content) || content[lineEnd] != '\n' {
		// "---" not followed by newline → malformed
		return nil, nil
	}

	start := lineEnd + 1 // skip past "---" + newline

	// Find closing "---"
	// The closing fence must be on its own line: "\n---\n" or "\n---" at EOF
	end, ok := findClosingFence(content, start)
	if !ok {
		return nil, fmt.Errorf("frontmatter: unterminated frontmatter block")
	}

	return content[start:end], nil
}

// findClosingFence locates the closing "---" fence starting from position `from`.
// Returns the end position (exclusive) and true if found.
func findClosingFence(content []byte, from int) (int, bool) {
	remaining := content[from:]
	search := content[from:]

	for len(search) >= 3 {
		if search[0] == '-' && search[1] == '-' && search[2] == '-' {
			// Check it's on its own line: preceded by \n
			absIdx := from + len(remaining) - len(search)
			if absIdx > 0 && content[absIdx-1] == '\n' {
				// Check what follows: \n or EOF
				nextIdx := absIdx + 3
				if nextIdx >= len(content) {
					return absIdx, true
				}
				if content[nextIdx] == '\n' {
					return absIdx, true
				}
				if content[nextIdx] == '\r' && nextIdx+1 < len(content) && content[nextIdx+1] == '\n' {
					return absIdx, true
				}
			}
		}
		search = search[1:]
	}
	return 0, false
}
