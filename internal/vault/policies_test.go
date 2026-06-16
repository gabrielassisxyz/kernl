package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanReadGlobal_NoFile_AllowsAll(t *testing.T) {
	dir := t.TempDir()
	// no .kernl-policies file → allow all
	pp := NewPolicyParser(dir)

	if !pp.CanReadGlobal("anything.md") {
		t.Error("expected allow when file is missing")
	}
}

func TestCanReadGlobal_EmptyPath_Allows(t *testing.T) {
	dir := t.TempDir()
	pp := NewPolicyParser(dir)

	// empty path → allow
	if !pp.CanReadGlobal("") {
		t.Error("expected empty path to be allowed")
	}
	// slash-only → also empty after trim → allow
	if !pp.CanReadGlobal("/") {
		t.Error("expected '/' path to be allowed")
	}
}

func TestCanReadGlobal_DenyPattern(t *testing.T) {
	dir := t.TempDir()
	writePolicies(t, dir, "*.secret.md")

	pp := NewPolicyParser(dir)

	// matches pattern → deny
	if pp.CanReadGlobal("notes.secret.md") {
		t.Error("expected notes.secret.md to be denied by *.secret.md")
	}
	// does not match → allow (default permissive)
	if !pp.CanReadGlobal("notes.md") {
		t.Error("expected notes.md to be allowed (no matching rule)")
	}
}

func TestCanReadGlobal_LastRuleWins(t *testing.T) {
	dir := t.TempDir()
	writePolicies(t, dir,
		"*.secret.md",
		"!important.secret.md",
	)

	pp := NewPolicyParser(dir)

	// important.secret.md matches both, but last (!allow) wins
	if !pp.CanReadGlobal("important.secret.md") {
		t.Error("expected important.secret.md to be allowed (last rule wins)")
	}
	// notes.secret.md only matches the deny pattern
	if pp.CanReadGlobal("notes.secret.md") {
		t.Error("expected notes.secret.md to be denied")
	}
}

func TestCanReadGlobal_AsteriskDoesNotCrossSlash(t *testing.T) {
	dir := t.TempDir()
	writePolicies(t, dir, "*.secret.md")

	pp := NewPolicyParser(dir)

	// path.Match's * does NOT cross /, so sub/notes.secret.md won't match *.secret.md
	if !pp.CanReadGlobal("sub/notes.secret.md") {
		t.Error("expected sub/notes.secret.md to be allowed (glob * does not cross /)")
	}
}

func TestCanReadGlobal_LeadingSlashStripped(t *testing.T) {
	dir := t.TempDir()
	writePolicies(t, dir, "*.secret.md")

	pp := NewPolicyParser(dir)

	// leading slash is stripped before matching
	if pp.CanReadGlobal("/notes.secret.md") {
		t.Error("expected /notes.secret.md to be denied (leading slash stripped)")
	}
}

func TestCanReadGlobal_SubDirectoryPatterns(t *testing.T) {
	dir := t.TempDir()
	writePolicies(t, dir,
		"sub/*.secret.md",
		"!sub/public.secret.md",
	)

	pp := NewPolicyParser(dir)

	// matches sub/*.secret.md → deny
	if pp.CanReadGlobal("sub/private.secret.md") {
		t.Error("expected sub/private.secret.md to be denied by sub/*.secret.md")
	}
	// allowed by exception
	if !pp.CanReadGlobal("sub/public.secret.md") {
		t.Error("expected sub/public.secret.md to be allowed by !sub/public.secret.md")
	}
}

func TestCanReadGlobal_CacheReloadsOnMtimeChange(t *testing.T) {
	dir := t.TempDir()

	// start with deny rule
	writePolicies(t, dir, "*.secret.md")
	pp := NewPolicyParser(dir)

	if pp.CanReadGlobal("notes.secret.md") {
		t.Error("expected deny before file change")
	}

	// replace policies file with empty rule → default allow
	writePolicies(t, dir, "# no rules")

	if !pp.CanReadGlobal("notes.secret.md") {
		t.Error("expected allow after policies file changed (no matching rules)")
	}
}

func TestCanReadGlobal_CommentAndBlankLines(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, ".kernl-policies"), []byte("# comment\n\n*.secret.md\n\n"), 0644)

	pp := NewPolicyParser(dir)

	if pp.CanReadGlobal("notes.secret.md") {
		t.Error("expected deny despite comments and blank lines")
	}
}

// helpers

func writePolicies(t *testing.T, dir string, lines ...string) {
	t.Helper()
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	path := filepath.Join(dir, ".kernl-policies")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writePolicies: %v", err)
	}
}
