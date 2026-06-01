package notes

import (
	"testing"
)

func TestParseDiff(t *testing.T) {
	payload := []byte(`[{"id":"h1", "line":1, "action":"add", "content":"hello"}]`)
	hunks, err := ParseDiff(payload)
	if err != nil {
		t.Fatalf("failed to parse diff: %v", err)
	}
	if len(hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(hunks))
	}
	if hunks[0].ID != "h1" || hunks[0].Content != "hello" {
		t.Fatalf("hunk parsed incorrectly: %+v", hunks[0])
	}
}
