package companion

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/gabrielassisxyz/kernl/internal/vault/frontmatter"
)

// A companion file is written by kernl and read back by kernl's own reconciler,
// so the two have to agree. They did not: the frontmatter was concatenated by
// hand, and a title carrying a colon ("AI-SEO: llms.txt + JSON-LD") produced
// `title: AI-SEO: llms.txt + JSON-LD` - which YAML reads as a nested mapping.
// Those files failed to parse on every cold start, forever.
func TestCompanionFrontmatterSurvivesAwkwardTitles(t *testing.T) {
	awkward := map[string]string{
		"a colon":         "AI-SEO: llms.txt + JSON-LD",
		"a hash":          "Fix #42 in the parser",
		"a leading dash":  "- not a list item",
		"quotes":          `The "hard" part`,
		"a leading brace": "{templated} title",
		"a colon at end":  "TODO:",
	}

	for name, title := range awkward {
		t.Run(name, func(t *testing.T) {
			raw := renderMarkdown("019f-abc", title, "", "Body text.\n", []string{"task"})

			fm, err := frontmatter.Parse(raw)
			if err != nil {
				t.Fatalf("kernl wrote a file kernl cannot read: %v\n%s", err, raw)
			}
			if fm.Title != title {
				t.Errorf("title did not survive the round trip:\n  wrote %q\n  read  %q", title, fm.Title)
			}
			if fm.ID != "019f-abc" {
				t.Errorf("id did not survive: %q", fm.ID)
			}
			if len(fm.Tags) != 1 || fm.Tags[0] != "task" {
				t.Errorf("tags did not survive: %v", fm.Tags)
			}
			if !strings.Contains(string(raw), "Body text.") {
				t.Error("the body is missing")
			}
		})
	}
}

// A note with no tags must not write an empty "tags:" key - that reads back as a
// nil list and is noise in a file the user opens in Obsidian. Same for a task
// with no description, which is the common case.
func TestCompanionFrontmatterOmitsEmptyTags(t *testing.T) {
	raw := string(renderMarkdown("019f-abc", "Plain", "", "Body.\n", nil))
	if strings.Contains(raw, "tags:") {
		t.Errorf("expected no tags key when there are none:\n%s", raw)
	}
	if strings.Contains(raw, "description:") {
		t.Errorf("expected no description key when there is none:\n%s", raw)
	}
	if strings.Contains(raw, "author:") || strings.Contains(raw, "origin:") {
		t.Errorf("empty read-contract fields leaked into the file:\n%s", raw)
	}
}

// The description is written by kernl and read back by the editor's YAML parser,
// so it has to survive the same punctuation the title does - it is prose typed by
// a human, so a colon or a quote in it is ordinary, not exotic.
func TestCompanionFrontmatterCarriesDescription(t *testing.T) {
	descriptions := map[string]string{
		"plain":       "For simple tasks that need no human input.",
		"a colon":     "Rule: the file loses, the node wins.",
		"quotes":      `The "hard" part is the sync`,
		"a newline":   "First line.\nSecond line.",
		"a leading -": "- looks like a list item",
		"a wikilink":  "Blocked by [[019f-other|the other task]]",
	}

	for name, description := range descriptions {
		t.Run(name, func(t *testing.T) {
			raw := renderMarkdown("019f-abc", "A task", description, "Body text.\n", []string{"task"})

			fm, err := frontmatter.Parse(raw)
			if err != nil {
				t.Fatalf("kernl wrote a file kernl cannot read: %v\n%s", err, raw)
			}
			var parsed struct {
				Description string `yaml:"description"`
			}
			if err := yaml.Unmarshal(frontmatterBlock(t, raw), &parsed); err != nil {
				t.Fatalf("frontmatter does not parse: %v\n%s", err, raw)
			}
			if parsed.Description != description {
				t.Errorf("description did not survive the round trip:\n  wrote %q\n  read  %q", description, parsed.Description)
			}
			if fm.Title != "A task" {
				t.Errorf("the description displaced the title: %q", fm.Title)
			}
			if !strings.Contains(string(raw), "Body text.") {
				t.Error("the body is missing")
			}
		})
	}
}

// frontmatterBlock returns the YAML between the fences. The frontmatter package's
// own read contract has no description field, so the assertion above parses the
// block directly rather than through it.
func frontmatterBlock(t *testing.T, raw []byte) []byte {
	t.Helper()
	s := string(raw)
	if !strings.HasPrefix(s, "---\n") {
		t.Fatalf("no frontmatter block:\n%s", raw)
	}
	end := strings.Index(s[4:], "\n---\n")
	if end < 0 {
		t.Fatalf("unterminated frontmatter block:\n%s", raw)
	}
	return []byte(s[4 : 4+end+1])
}
