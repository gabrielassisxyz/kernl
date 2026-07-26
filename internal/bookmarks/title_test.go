package bookmarks

import (
	"strings"
	"testing"
)

func TestExtractTitlePrefersOpenGraph(t *testing.T) {
	// The case that motivated the ordering: <title> padded with the site name,
	// og:title carrying the bare thing.
	html := `<html><head>
		<meta property="og:title" content="12 Books That Will Make You Dangerous">
		<title>12 Books That Will Make You Dangerous - The Book Club</title>
	</head><body><h1>Ignored</h1></body></html>`

	if got := ExtractTitle(html); got != "12 Books That Will Make You Dangerous" {
		t.Errorf("og:title should win, got %q", got)
	}
}

func TestExtractTitleFallsBackThroughSources(t *testing.T) {
	cases := []struct {
		name string
		html string
		want string
	}{
		{"title tag", `<html><head><title>Just The Title</title></head></html>`, "Just The Title"},
		{"h1 when nothing else", `<html><body><h1>Only A Heading</h1></body></html>`, "Only A Heading"},
		{"h1 unwraps nested markup", `<body><h1><span>Wrapped</span> Heading</h1></body>`, "Wrapped Heading"},
		{"single-quoted og:title", `<meta content='Quoted Loosely' property='og:title'>`, "Quoted Loosely"},
		{"entities and newlines", "<title>Rust &amp;\n  Go</title>", "Rust & Go"},
		{"empty title tag falls through", `<html><head><title>  </title></head><body><h1>Real</h1></body></html>`, "Real"},
		{"no title at all", `<html><body><p>nothing here</p></body></html>`, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExtractTitle(tc.html); got != tc.want {
				t.Errorf("ExtractTitle() = %q, want %q", got, tc.want)
			}
		})
	}
}

// A heading inside site furniture is not the page's subject. Without the
// sanitize pass an <h1> in the masthead would title every page on the site
// identically.
func TestExtractTitleIgnoresHeadingInNav(t *testing.T) {
	html := `<body><nav><h1>Site Name</h1></nav><h1>The Actual Article</h1></body>`
	if got := ExtractTitle(html); got != "The Actual Article" {
		t.Errorf("nav heading should not win, got %q", got)
	}
}

// A title is a node title and can reach a companion note's file name, so a
// page that puts a paragraph in its <title> must not set the length.
func TestExtractTitleTruncates(t *testing.T) {
	html := "<title>" + strings.Repeat("word ", 200) + "</title>"
	got := ExtractTitle(html)
	if len([]rune(got)) > maxTitleRunes+1 {
		t.Errorf("expected truncation to ~%d runes, got %d", maxTitleRunes, len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis suffix, got %q", got)
	}
}

func TestIsPlaceholderTitle(t *testing.T) {
	const url = "https://example.com/post"

	cases := []struct {
		title string
		want  bool
	}{
		{"", true},
		{"   ", true},
		{"Pending", true},
		{"pending", true},          // the guard is about the value, not its casing
		{"Imported via CLI", true}, // the CLI's own former placeholder
		{url, true},                // the URL standing in for a title it does not have
		{"A Real Title", false},
		{"Pending Approval", false}, // a legitimate title that merely starts the same way
	}

	for _, tc := range cases {
		if got := IsPlaceholderTitle(tc.title, url); got != tc.want {
			t.Errorf("IsPlaceholderTitle(%q) = %v, want %v", tc.title, got, tc.want)
		}
	}
}
