package bookmarks

import (
	"strings"
	"testing"
)

func TestDefuddle(t *testing.T) {
	htmlFixture := `
<html>
<head><title>Test</title></head>
<body>
  <nav><ul><li>Link</li></ul></nav>
  <div class="main-content">
     <article id="post">
        <h1>Hello World</h1>
        <p>This is the content.</p>
        <script>alert("bad")</script>
        <style>body { color: red; }</style>
        <footer><p>Copyright 2026</p></footer>
     </article>
  </div>
</body>
</html>
`
	// Test 1: extract by ID
	res, err := Defuddle(htmlFixture, []string{"#post"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(res, "Hello World") {
		t.Errorf("expected 'Hello World' in result")
	}
	if strings.Contains(res, "alert") {
		t.Errorf("expected script to be removed")
	}
	if strings.Contains(res, "Copyright") {
		t.Errorf("expected footer to be removed")
	}
	if strings.Contains(res, "body {") {
		t.Errorf("expected style to be removed")
	}
	if strings.Contains(res, "<nav>") {
		t.Errorf("expected nav to be removed")
	}

	// Test 2: extract by class
	res2, err := Defuddle(htmlFixture, []string{".main-content"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(res2, "Hello World") {
		t.Errorf("expected 'Hello World' in result")
	}
	if strings.Contains(res2, "alert") {
		t.Errorf("expected script to be removed")
	}

	// Test 3: fallback selectors
	res3, err := Defuddle(htmlFixture, []string{"#missing", "article#post"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(res3, "Hello World") {
		t.Errorf("expected 'Hello World' in result")
	}

	// Test 4: no matching selector
	_, err = Defuddle(htmlFixture, []string{".nonexistent"})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
