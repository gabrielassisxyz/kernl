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

// TestInjectTagsSpecialCharactersRoundTrip is the regression for hand-rolled
// YAML: a tag written bare that happens to be a YAML metacharacter either
// breaks the whole frontmatter's parse (a colon-space turns the tag into a
// nested map) or silently vanishes (a leading '#' reads as a comment, a
// leading '-' as another list item). Each case here must both re-parse and
// come back byte-identical to what was passed in.
func TestInjectTagsSpecialCharactersRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		tag  string
	}{
		{"colon-space", "foo: bar"},
		{"hash", "#hash"},
		{"leading-dash-space", "- dash"},
		{"leading-bracket", "[weird"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orig := []byte("---\nid: abc-123\ntitle: Foo\n---\n# Body\n")
			got, err := InjectTags(orig, []string{tc.tag, "plain"})
			if err != nil {
				t.Fatalf("InjectTags returned an error for a legitimate tag %q: %v", tc.tag, err)
			}
			fm, parseErr := Parse(got)
			if parseErr != nil {
				t.Fatalf("result does not parse for tag %q:\n%s\nerror: %v", tc.tag, got, parseErr)
			}
			if len(fm.Tags) != 2 || fm.Tags[0] != tc.tag || fm.Tags[1] != "plain" {
				t.Fatalf("tags did not round-trip for %q: got %v", tc.tag, fm.Tags)
			}
			if fm.ID != "abc-123" {
				t.Fatalf("id must survive untouched, got %q", fm.ID)
			}
		})
	}
}

// TestInjectTagsValidatesItsOwnOutput exercises the safety net directly: even
// if a future change to the rendering path produced YAML that does not carry
// the tags it was asked to write, InjectTags's own re-parse must catch it
// rather than let a 200-and-corrupted (or 200-and-silently-wrong) file reach
// disk.
func TestInjectTagsValidatesItsOwnOutput(t *testing.T) {
	// A result whose tags: block, read back, does not match what was asked
	// for - as if the renderer had silently dropped or mangled a value.
	bad := []byte("---\nid: abc-123\ntags:\n  - only-one\n---\n# Body\n")
	if _, err := validateInjectedTags(bad, []string{"only-one", "two"}); err == nil {
		t.Fatal("expected an error when the parsed-back tags do not match what was written")
	}

	// A result that does not parse at all.
	unparseable := []byte("---\n\t\tnot: valid: yaml\n---\n# Body\n")
	if _, err := validateInjectedTags(unparseable, []string{"a"}); err == nil {
		t.Fatal("expected an error when the result does not parse")
	}

	// The honest case still passes through.
	good := []byte("---\nid: abc-123\ntags:\n  - a\n  - b\n---\n# Body\n")
	out, err := validateInjectedTags(good, []string{"a", "b"})
	if err != nil {
		t.Fatalf("unexpected error on a genuinely matching result: %v", err)
	}
	if string(out) != string(good) {
		t.Fatalf("validateInjectedTags must return the bytes unchanged, got:\n%s", out)
	}
}
