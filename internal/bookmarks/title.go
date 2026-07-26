package bookmarks

import (
	"html"
	"regexp"
	"strings"
)

var (
	titleTagRe    = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	h1TagRe       = regexp.MustCompile(`(?is)<h1[^>]*>(.*?)</h1>`)
	metaTagRe     = regexp.MustCompile(`(?is)<meta\s[^>]*>`)
	ogTitlePropRe = regexp.MustCompile(`(?i)\b(?:property|name)\s*=\s*["']?og:title["']?[\s">']`)
	contentAttrRe = regexp.MustCompile(`(?i)\bcontent\s*=\s*(?:'([^']*)'|"([^"]*)"|([^\s>]+))`)
)

// maxTitleRunes caps an extracted title. A page is free to put a paragraph in
// its <title>, and this value ends up as a node title and, for a companion
// note, in a file name.
const maxTitleRunes = 200

// ExtractTitle derives a page title from raw HTML. Deterministic - no network,
// no LLM - the same contract as ExtractExcerpt.
//
// og:title is preferred over <title> because it is what a page declares for
// consumers other than the browser tab: <title> is routinely padded with the
// site name ("The Post - Substack", "The Video - YouTube") while og:title
// carries the bare thing. <h1> is the last resort, for pages that declare
// neither. Returns "" when the HTML states no title at all; the caller decides
// what to fall back to.
func ExtractTitle(rawHTML string) string {
	if t := cleanTitle(ogTitle(rawHTML)); t != "" {
		return t
	}
	if m := titleTagRe.FindStringSubmatch(rawHTML); m != nil {
		if t := cleanTitle(m[1]); t != "" {
			return t
		}
	}
	// The <h1> search runs on sanitized HTML so a heading buried in a <nav> or
	// <footer> - site furniture, not the page's subject - cannot win.
	if m := h1TagRe.FindStringSubmatch(sanitizeHTML(rawHTML)); m != nil {
		if t := cleanTitle(m[1]); t != "" {
			return t
		}
	}
	return ""
}

func ogTitle(rawHTML string) string {
	for _, tag := range metaTagRe.FindAllString(rawHTML, -1) {
		if !ogTitlePropRe.MatchString(tag) {
			continue
		}
		m := contentAttrRe.FindStringSubmatch(tag)
		if m == nil {
			continue
		}
		for _, quoted := range m[1:] {
			if quoted != "" {
				return quoted
			}
		}
	}
	return ""
}

// cleanTitle turns a raw title fragment into a single line of plain text:
// nested markup out (an <h1> may wrap a <span>), entities decoded, whitespace
// collapsed - a <title> split across source lines is still one title.
func cleanTitle(raw string) string {
	text := tagRe.ReplaceAllString(raw, " ")
	text = html.UnescapeString(text)
	text = strings.TrimSpace(wsRe.ReplaceAllString(text, " "))

	if r := []rune(text); len(r) > maxTitleRunes {
		return strings.TrimSpace(string(r[:maxTitleRunes])) + "…"
	}
	return text
}

// placeholderTitles are the strings the creation paths stamped on a bookmark
// before its page had been fetched. They are not titles, they are the absence
// of one - and nothing ever replaced them: there was no title field on any
// create path and no update route, so "Pending" was permanent.
var placeholderTitles = []string{"Pending", "Imported via CLI"}

// IsPlaceholderTitle reports whether a bookmark's title carries no information
// about the page: empty, one of the legacy placeholders, or the URL echoed
// back. Only such a title may be overwritten by extraction - a title that came
// from outside (a Pocket export, an inbox capture, an explicit --title) is the
// user's and stands, however much a scraped <title> might differ.
func IsPlaceholderTitle(title, url string) bool {
	t := strings.TrimSpace(title)
	if t == "" || t == strings.TrimSpace(url) {
		return true
	}
	for _, p := range placeholderTitles {
		if strings.EqualFold(t, p) {
			return true
		}
	}
	return false
}
