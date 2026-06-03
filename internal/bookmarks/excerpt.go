package bookmarks

import (
	"html"
	"regexp"
	"strings"
)

var (
	tagRe            = regexp.MustCompile(`(?s)<[^>]*>`)
	wsRe             = regexp.MustCompile(`\s+`)
	excerptSelectors = []string{"article", "main", ".post-content", ".entry-content", "#content"}
)

// ExtractExcerpt derives a plain-text excerpt from raw HTML. It isolates the
// likely article region (falling back to the whole document with scripts and
// styles removed), strips remaining tags, unescapes entities, collapses
// whitespace, and truncates to max runes. Deterministic — no network, no LLM.
func ExtractExcerpt(rawHTML string, max int) string {
	region := sanitizeHTML(rawHTML)
	if content, err := Defuddle(rawHTML, excerptSelectors); err == nil && strings.TrimSpace(content) != "" {
		region = content
	}

	text := tagRe.ReplaceAllString(region, " ")
	text = html.UnescapeString(text)
	text = strings.TrimSpace(wsRe.ReplaceAllString(text, " "))

	if max > 0 {
		r := []rune(text)
		if len(r) > max {
			return strings.TrimSpace(string(r[:max])) + "…"
		}
	}
	return text
}
