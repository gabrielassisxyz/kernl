package bookmarks

import (
	"fmt"
	"regexp"
	"strings"
)

// Defuddle extracts the readable content from raw HTML using the provided selectors.
// It tries selectors in order and strips unwanted tags like <script> or <style>.
func Defuddle(rawHTML string, selectors []string) (string, error) {
	for _, sel := range selectors {
		content, found := extractBySelector(rawHTML, sel)
		if found {
			return sanitizeHTML(content), nil
		}
	}
	return "", fmt.Errorf("no matching selector found")
}

func extractBySelector(html string, selector string) (string, bool) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", false
	}

	var tag string
	var id string
	var class string

	if strings.HasPrefix(selector, "#") {
		id = selector[1:]
	} else if strings.HasPrefix(selector, ".") {
		class = selector[1:]
	} else {
		if idx := strings.Index(selector, "#"); idx != -1 {
			tag = selector[:idx]
			id = selector[idx+1:]
		} else if idx := strings.Index(selector, "."); idx != -1 {
			tag = selector[:idx]
			class = selector[idx+1:]
		} else {
			tag = selector
		}
	}

	reStr := `<([a-zA-Z0-9\-]+)([^>]*)>`
	re := regexp.MustCompile(reStr)
	matches := re.FindAllStringSubmatchIndex(html, -1)

	for _, match := range matches {
		tagName := html[match[2]:match[3]]
		attrs := html[match[4]:match[5]]

		if tag != "" && !strings.EqualFold(tagName, tag) {
			continue
		}

		if id != "" {
			idRe := regexp.MustCompile(`(?i)\bid\s*=\s*(?:'([^']+)'|"([^"]+)"|([^\s>]+))`)
			idMatch := idRe.FindStringSubmatch(attrs)
			if len(idMatch) == 0 {
				continue
			}
			foundId := idMatch[1]
			if foundId == "" {
				foundId = idMatch[2]
			}
			if foundId == "" {
				foundId = idMatch[3]
			}
			if foundId != id {
				continue
			}
		}

		if class != "" {
			classRe := regexp.MustCompile(`(?i)\bclass\s*=\s*(?:'([^']+)'|"([^"]+)"|([^\s>]+))`)
			classMatch := classRe.FindStringSubmatch(attrs)
			if len(classMatch) == 0 {
				continue
			}
			foundClass := classMatch[1]
			if foundClass == "" {
				foundClass = classMatch[2]
			}
			if foundClass == "" {
				foundClass = classMatch[3]
			}
			classes := strings.Fields(foundClass)
			hasClass := false
			for _, c := range classes {
				if c == class {
					hasClass = true
					break
				}
			}
			if !hasClass {
				continue
			}
		}

		startIndex := match[0]
		endIndex := findClosingTag(html, tagName, match[1])
		if endIndex == -1 {
			endIndex = len(html)
		}
		return html[startIndex:endIndex], true
	}

	return "", false
}

func findClosingTag(html string, tag string, startFrom int) int {
	openRe := regexp.MustCompile(`(?i)<` + regexp.QuoteMeta(tag) + `(?:\s[^>]*)?>`)
	closeRe := regexp.MustCompile(`(?i)</` + regexp.QuoteMeta(tag) + `\s*>`)

	depth := 1
	pos := startFrom

	for pos < len(html) {
		nextOpen := openRe.FindStringIndex(html[pos:])
		nextClose := closeRe.FindStringIndex(html[pos:])

		if nextClose == nil {
			return -1
		}

		if nextOpen != nil && nextOpen[0] < nextClose[0] {
			depth++
			pos += nextOpen[1]
		} else {
			depth--
			if depth == 0 {
				return pos + nextClose[1]
			}
			pos += nextClose[1]
		}
	}
	return -1
}

func sanitizeHTML(html string) string {
	tagsToRemove := []string{"script", "style", "nav", "footer", "iframe", "noscript"}
	for _, tag := range tagsToRemove {
		openRe := regexp.MustCompile(`(?i)<` + tag + `(?:\s[^>]*)?>`)
		closeRe := regexp.MustCompile(`(?i)</` + tag + `\s*>`)

		for {
			openLoc := openRe.FindStringIndex(html)
			if openLoc == nil {
				break
			}

			closeLoc := closeRe.FindStringIndex(html[openLoc[1]:])
			if closeLoc == nil {
				html = html[:openLoc[0]] + html[openLoc[1]:]
				break
			}

			endIndex := openLoc[1] + closeLoc[1]
			html = html[:openLoc[0]] + html[endIndex:]
		}
	}
	return strings.TrimSpace(html)
}
