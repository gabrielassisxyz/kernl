package api

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/vault/frontmatter"
)

// A companion file is written by kernl and read back by kernl's own reconciler,
// so the two have to agree. They did not: the frontmatter was concatenated by
// hand, and a title carrying a colon ("AI-SEO: llms.txt + JSON-LD") produced
// `title: AI-SEO: llms.txt + JSON-LD` — which YAML reads as a nested mapping.
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
			raw := renderCompanionMarkdown("019f-abc", title, "Body text.\n", []string{"task"})

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

// A note with no tags must not write an empty "tags:" key — that reads back as a
// nil list and is noise in a file the user opens in Obsidian.
func TestCompanionFrontmatterOmitsEmptyTags(t *testing.T) {
	raw := string(renderCompanionMarkdown("019f-abc", "Plain", "Body.\n", nil))
	if strings.Contains(raw, "tags:") {
		t.Errorf("expected no tags key when there are none:\n%s", raw)
	}
	if strings.Contains(raw, "author:") || strings.Contains(raw, "origin:") {
		t.Errorf("empty read-contract fields leaked into the file:\n%s", raw)
	}
}
