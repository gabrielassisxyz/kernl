package bookmarks

import (
	"strings"
	"testing"
)

func TestExtractExcerpt(t *testing.T) {
	html := `<html><head><style>.x{color:red}</style></head><body>
		<nav>Home About</nav>
		<article><h1>Title</h1><p>The quick brown fox &amp; the lazy dog.</p></article>
		<script>tracking()</script>
		<footer>copyright</footer>
	</body></html>`

	got := ExtractExcerpt(html, 500)

	if !strings.Contains(got, "The quick brown fox & the lazy dog.") {
		t.Errorf("expected article text with unescaped entity, got %q", got)
	}
	for _, junk := range []string{"<", ">", "tracking()", "color:red"} {
		if strings.Contains(got, junk) {
			t.Errorf("excerpt should not contain %q, got %q", junk, got)
		}
	}
}

func TestExtractExcerptTruncates(t *testing.T) {
	long := "<p>" + strings.Repeat("word ", 300) + "</p>"
	got := ExtractExcerpt(long, 50)
	if len([]rune(got)) > 51 { // 50 runes + ellipsis
		t.Errorf("expected truncation to ~50 runes, got %d", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis suffix, got %q", got)
	}
}

func TestExtractExcerptFallbackNoArticle(t *testing.T) {
	// No article/main/etc — should still extract body text via sanitize+strip.
	html := `<div><p>Plain content here.</p></div>`
	got := ExtractExcerpt(html, 500)
	if !strings.Contains(got, "Plain content here.") {
		t.Errorf("expected fallback extraction, got %q", got)
	}
}
