package frontmatter

import "testing"

func TestInjectTagsAbsentBlock(t *testing.T) {
	orig := []byte("# Just body\n")
	got, err := InjectTags(orig, []string{"handoff", "checkpoint"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "---\ntags:\n  - handoff\n  - checkpoint\n---\n# Just body\n"
	if string(got) != want {
		t.Errorf("unexpected output:\ngot:\n%s\nwant:\n%s", string(got), want)
	}
}

func TestInjectTagsAbsentBlockEmptyTagsIsNoOp(t *testing.T) {
	orig := []byte("# Just body\n")
	got, err := InjectTags(orig, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(orig) {
		t.Errorf("expected no-op, got:\n%s", string(got))
	}
}

func TestInjectTagsNoExistingTagsKeyAppendsOne(t *testing.T) {
	orig := []byte("---\nid: abc-123\ntitle: Foo\n---\n# Body\n")
	got, err := InjectTags(orig, []string{"a", "b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "---\nid: abc-123\ntitle: Foo\ntags:\n  - a\n  - b\n---\n# Body\n"
	if string(got) != want {
		t.Errorf("unexpected output:\ngot:\n%s\nwant:\n%s", string(got), want)
	}
}

func TestInjectTagsReplacesExistingBlockList(t *testing.T) {
	orig := []byte("---\nid: abc-123\ntags:\n  - old\n  - stale\ntitle: Foo\n---\n# Body\n")
	got, err := InjectTags(orig, []string{"new"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "---\nid: abc-123\ntags:\n  - new\ntitle: Foo\n---\n# Body\n"
	if string(got) != want {
		t.Errorf("unexpected output:\ngot:\n%s\nwant:\n%s", string(got), want)
	}
}

func TestInjectTagsReplacesExistingFlowStyle(t *testing.T) {
	orig := []byte("---\ntags: [old, stale]\ntitle: Foo\n---\n# Body\n")
	got, err := InjectTags(orig, []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "---\ntags:\n  - a\n  - b\n  - c\ntitle: Foo\n---\n# Body\n"
	if string(got) != want {
		t.Errorf("unexpected output:\ngot:\n%s\nwant:\n%s", string(got), want)
	}
}

func TestInjectTagsEmptyListRemovesExistingKey(t *testing.T) {
	orig := []byte("---\nid: abc-123\ntags:\n  - old\n  - stale\ntitle: Foo\n---\n# Body\n")
	got, err := InjectTags(orig, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "---\nid: abc-123\ntitle: Foo\n---\n# Body\n"
	if string(got) != want {
		t.Errorf("unexpected output:\ngot:\n%s\nwant:\n%s", string(got), want)
	}
}

// TestInjectTagsPreservesIDWhenReplacingTags is the acceptance criterion that
// InjectTags never disturbs the id: line, whatever order the two keys are in.
func TestInjectTagsPreservesIDWhenReplacingTags(t *testing.T) {
	orig := []byte("---\ntags:\n  - old\nid: keep-me\n---\n# Body\n")
	got, err := InjectTags(orig, []string{"new"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "---\ntags:\n  - new\nid: keep-me\n---\n# Body\n"
	if string(got) != want {
		t.Errorf("unexpected output:\ngot:\n%s\nwant:\n%s", string(got), want)
	}
}

func TestInjectTagsPreservesCRLF(t *testing.T) {
	orig := []byte("---\r\ntags:\r\n  - old\r\ntitle: Foo\r\n---\r\n# Body\r\n")
	got, err := InjectTags(orig, []string{"new"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "---\r\ntags:\r\n  - new\r\ntitle: Foo\r\n---\r\n# Body\r\n"
	if string(got) != want {
		t.Errorf("unexpected output:\ngot:\n%s\nwant:\n%s", string(got), want)
	}
}

func TestInjectTagsMalformedYAMLIsRefused(t *testing.T) {
	orig := []byte("---\n\t\tt\n---\n")
	if _, err := InjectTags(orig, []string{"a"}); err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
}
