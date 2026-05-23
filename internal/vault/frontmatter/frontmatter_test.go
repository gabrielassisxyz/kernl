package frontmatter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAbsentBlockReturnsEmpty(t *testing.T) {
	raw := []byte("plain content\nno frontmatter\n")
	fm, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.ID != "" || fm.Title != "" || len(fm.Tags) != 0 {
		t.Fatalf("expected empty frontmatter, got %+v", fm)
	}
}

func TestParseWithId(t *testing.T) {
	raw := []byte("---\nid: abc-123\ntitle: Hello\ntags:\n  - one\n  - two\n---\n# Body\n")
	fm, err := Parse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.ID != "abc-123" {
		t.Errorf("id = %q, want abc-123", fm.ID)
	}
	if fm.Title != "Hello" {
		t.Errorf("title = %q, want Hello", fm.Title)
	}
	if len(fm.Tags) != 2 || fm.Tags[0] != "one" {
		t.Errorf("tags = %v, want [one two]", fm.Tags)
	}
}

func TestParseMalformedYAML(t *testing.T) {
	raw := []byte("---\n\t\tt\n---\n")
	_, err := Parse(raw)
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
}

func TestInjectIDAbsentBlock(t *testing.T) {
	orig := []byte("# Just body\n")
	got, err := InjectID(orig, "u-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "---\nid: u-1\n---\n# Just body\n" {
		t.Errorf("unexpected output: %q", string(got))
	}
}

func TestInjectIDExistingIdIsNoOp(t *testing.T) {
	orig := []byte("---\nid: existing\ntitle: Foo\n---\n# Body\n")
	got, err := InjectID(orig, "u-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != string(orig) {
		t.Errorf("expected no-op, got:\n%s", string(got))
	}
}

func TestInjectIDPreservesCommentsAndOrder(t *testing.T) {
	orig := []byte("---\n# comment\ntitle:  Foo\n---\n# Body\n")
	got, err := InjectID(orig, "u-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Golden fixtures place id at the top of the block, after the opening fence.
	want := "---\nid: u-1\n# comment\ntitle:  Foo\n---\n# Body\n"
	if string(got) != want {
		t.Errorf("unexpected output:\ngot:\n%s\nwant:\n%s", string(got), want)
	}
}

func TestInjectIDPreservesCRLF(t *testing.T) {
	orig := []byte("---\r\ntitle: Foo\r\n---\r\n# Body\r\n")
	got, err := InjectID(orig, "u-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "---\r\nid: u-1\r\ntitle: Foo\r\n---\r\n# Body\r\n"
	if string(got) != want {
		t.Errorf("unexpected output:\ngot:\n%s\nwant:\n%s", string(got), want)
	}
}

func TestRoundTripGoldenFiles(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("testdata"))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) != ".md" {
			continue
		}
		// Skip read-only testing files
		if name == "no_frontmatter_read.md" || name == "malformed_unterminated.md" {
			continue
		}
		// Skip "expected" files — they pair with the base name
		if len(name) > 8 && name[len(name)-8:len(name)] == "expected"+".md" {
			continue
		}

		base := filepath.Join("testdata", name)
		expect := filepath.Join("testdata", name[:len(name)-3]+"_expected.md")
		wantRaw, err := os.ReadFile(expect)
		if err != nil {
			// No expected file; skip
			continue
		}
		inRaw, err := os.ReadFile(base)
		if err != nil {
			t.Fatalf("read %s: %v", base, err)
		}

		// Use whatever UUID appears in the expected file's first line after the opening fence.
		var testUUID string
		for i := 0; i < len(wantRaw)-1; i++ {
			if wantRaw[i] == 'i' && wantRaw[i+1] == 'd' {
				// find end of line
				for j := i + 3; j < len(wantRaw); j++ {
					if wantRaw[j] == '\n' {
						testUUID = string(wantRaw[i+3 : j])
						testUUID = strings.TrimSpace(testUUID)
						break
					}
				}
				break
			}
		}
		if testUUID == "" {
			t.Fatalf("could not extract UUID from expected file %s", expect)
		}

		gotRaw, err := InjectID(inRaw, testUUID)
		if err != nil {
			t.Fatalf("InjectID %s: %v", name, err)
		}
		if string(gotRaw) != string(wantRaw) {
			t.Errorf("golden mismatch for %s:\ngot:\n%s\nwant:\n%s", name, string(gotRaw), string(wantRaw))
		}
	}
}
