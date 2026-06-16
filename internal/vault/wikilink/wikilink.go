// Package wikilink parses [[...]] wikilinks from vault note bodies,
// resolves them against the graph, and manages unresolved (dangling) links
// that are promoted to real edges when the target note appears.
package wikilink

import (
	"regexp"
	"strings"
)

// linkRe matches [[target]] or [[target|alias]].
// Target groups: [[, optional uuid-, the target, optional |alias, ]].
var linkRe = regexp.MustCompile(`\[\[([^\]|]+?)(?:\|([^\]]+?))?\]\]`)

// Link is a parsed wikilink with position info.
type Link struct {
	Target  string // raw target text (e.g., "Roadmap", "uuid-here")
	Alias   string // display alias, empty if none
	Line    int    // 0-indexed line number
	Col     int    // 0-indexed column of the opening '['
	RawText string // full [[...]] text including brackets
}

// TargetKind classifies how a target should be resolved.
type TargetKind string

const (
	KindUUID  TargetKind = "uuid"
	KindStem  TargetKind = "stem"
	KindTitle TargetKind = "title"
)

// ClassifyTarget determines the TargetKind based on the raw target string.
// UUIDs are strings that parse as a valid UUID; everything else is a stem
// (the resolver will fall back to title if stem lookup fails).
func ClassifyTarget(target string) TargetKind {
	if isUUID(target) {
		return KindUUID
	}
	return KindStem
}

// uuidRe matches UUID format (8-4-4-4-12 hex)
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isUUID(s string) bool {
	return uuidRe.MatchString(s)
}

// Parse extracts all wikilinks from markdown content, skipping those inside
// code fences (```...```) and inline code (`...`). Results are deduplicated
// by Target+Alias.
func Parse(content string) []Link {
	lines := strings.Split(content, "\n")
	var links []Link
	inFence := false

	for i, line := range lines {
		line, inFence = stripFences(line, inFence)
		if inFence {
			continue
		}

		// Strip inline code spans so [[inside `code`]] is not matched.
		cleanLine := stripInlineCode(line)

		matches := linkRe.FindAllStringSubmatchIndex(cleanLine, -1)
		for _, m := range matches {
			if len(m) < 4 {
				continue
			}
			// m[0], m[1] = full match; m[2], m[3] = group 1 (target); m[4], m[5] = group 2 (alias)
			targetStart, targetEnd := m[2], m[3]
			target := cleanLine[targetStart:targetEnd]
			alias := ""
			if m[4] >= 0 && m[5] >= 0 {
				alias = cleanLine[m[4]:m[5]]
			}

			links = append(links, Link{
				Target:  target,
				Alias:   alias,
				Line:    i,
				Col:     m[0],
				RawText: cleanLine[m[0]:m[1]],
			})
		}
	}

	return dedupLinks(links)
}

// stripFences tracks fenced code blocks (```).
// Returns the line with fence removed (or empty if fence line itself) and the new inFence state.
func stripFences(line string, inFence bool) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "```") {
		return "", !inFence
	}
	if inFence {
		return "", true
	}
	return line, false
}

// stripInlineCode removes backtick-delimited inline code spans from a line.
func stripInlineCode(line string) string {
	var buf strings.Builder
	i := 0
	inCode := false
	codeStart := 0

	for i < len(line) {
		if line[i] == '`' {
			tickLen := countConsecutive(line, i, '`')
			if !inCode {
				codeStart = i
				inCode = true
			} else {
				// Only close if matching backtick count
				inCode = false
				// Write the part before the code span
				buf.WriteString(line[:codeStart])
				line = line[i+tickLen:]
				i = 0
				continue
			}
			i += tickLen
		} else {
			i++
		}
	}

	if inCode {
		// Unclosed backtick — treat rest as code
		return buf.String()
	}
	buf.WriteString(line)
	return buf.String()
}

func countConsecutive(s string, start int, ch byte) int {
	n := 0
	for start+n < len(s) && s[start+n] == ch {
		n++
	}
	return n
}

// dedupLinks removes duplicate links (same Target+Alias), keeping first occurrence.
func dedupLinks(links []Link) []Link {
	seen := make(map[string]struct{}, len(links))
	var out []Link
	for _, l := range links {
		key := l.Target + "|" + l.Alias
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, l)
	}
	return out
}
