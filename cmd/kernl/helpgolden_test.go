package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureRootHelp runs printHelp with stdout redirected. The root page is the
// one help surface that does not go through renderCommandHelp.
func captureRootHelp(t *testing.T) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	printErr := printHelp()
	os.Stdout = saved
	w.Close()
	if printErr != nil {
		t.Fatalf("printHelp: %v", printErr)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimRight(string(out), "\n")
}

// TestEveryCommandWithFlagsRendersThem ties the two halves of the flag contract
// together. Structured Flags with no {{flags}} token would be a command whose
// help silently omits its own flags; a token with no Flags would render an
// empty heading. Both are invisible until someone reads that one page.
func TestEveryCommandWithFlagsRendersThem(t *testing.T) {
	var walk func(table []commandMeta, prefix string)
	walk = func(table []commandMeta, prefix string) {
		for _, c := range table {
			name := strings.TrimSpace(prefix + " " + c.Name)
			hasToken := strings.Contains(c.Details, flagsPlaceholder)
			switch {
			case len(c.Flags) > 0 && !hasToken:
				t.Errorf("%q declares %d flags but its Details never renders them (missing %s)", name, len(c.Flags), flagsPlaceholder)
			case hasToken && len(c.Flags) == 0:
				t.Errorf("%q has a %s token but declares no flags", name, flagsPlaceholder)
			}
			if c.FlagsHeading != "" && len(c.Flags) == 0 {
				t.Errorf("%q overrides the flags heading but has no flags", name)
			}
			walk(c.Subs, name)
		}
	}
	walk(commandTable, "")
}

// TestNoHandWrittenFlagBlocksRemain guards the migration itself: a new command
// added by copying an old one would bring back a prose block, which renders
// fine and is invisible to `capabilities --json` — the exact defect this
// change removed.
func TestNoHandWrittenFlagBlocksRemain(t *testing.T) {
	var walk func(table []commandMeta, prefix string)
	walk = func(table []commandMeta, prefix string) {
		for _, c := range table {
			name := strings.TrimSpace(prefix + " " + c.Name)
			for _, line := range strings.Split(c.Details, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "Flags:") {
					t.Errorf("%q still documents flags as prose (%q) — move them to the Flags field", name, line)
				}
			}
			walk(c.Subs, name)
		}
	}
	walk(commandTable, "")
}

// helpPaths walks commandTable and returns every help topic, deepest included.
func helpPaths(table []commandMeta, prefix []string) [][]string {
	var out [][]string
	for _, c := range table {
		path := append(append([]string{}, prefix...), c.Name)
		out = append(out, path)
		out = append(out, helpPaths(c.Subs, path)...)
	}
	return out
}

// TestHelpTextIsUnchanged pins the rendered help of every command in the table.
//
// WHY a golden file. The "Flags:" block in each page is generated from
// commandMeta.Flags rather than hand-written into Details, so that
// `capabilities --json` and `--help` cannot disagree about what a flag is. That
// migration moved 52 blocks of prose into structured data, and the one thing it
// must not do is change what a reader sees. This file is the evidence: it was
// captured from the pre-migration binary, and it has to keep matching.
//
// Regenerate deliberately, never reflexively — a diff here means a help page
// changed, which is either the point of your commit or a bug in it:
//
//	go test ./cmd/kernl/ -run TestHelpTextIsUnchanged -update-help-golden
func TestHelpTextIsUnchanged(t *testing.T) {
	var b strings.Builder
	// The root page first. It is rendered by printHelp rather than
	// renderCommandHelp, and leaving it out is how the flag-block migration
	// nearly changed it unnoticed — its global-flags block is generated too.
	b.WriteString("### kernl\n" + captureRootHelp(t) + "\n\n")
	for _, path := range helpPaths(commandTable, nil) {
		cmd := findCommand(commandTable, path[0])
		for _, name := range path[1:] {
			cmd = findCommand(cmd.Subs, name)
		}
		b.WriteString("### kernl " + strings.Join(path, " ") + "\n")
		b.WriteString(renderCommandHelp("kernl "+strings.Join(path, " "), cmd))
		b.WriteString("\n\n")
	}
	got := b.String()

	golden := filepath.Join("testdata", "help_golden.txt")
	if os.Getenv("UPDATE_HELP_GOLDEN") != "" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("help golden rewritten")
		return
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("missing help golden (%v) — capture it with UPDATE_HELP_GOLDEN=1", err)
	}
	if got == string(want) {
		return
	}
	// Report the first differing line rather than dumping two 1000-line pages.
	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(string(want), "\n")
	for i := 0; i < len(gotLines) || i < len(wantLines); i++ {
		g, w := "", ""
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if g != w {
			t.Fatalf("help text changed at line %d:\n  want: %q\n   got: %q\n\nIf this change is intended, re-record with UPDATE_HELP_GOLDEN=1", i+1, w, g)
		}
	}
}
