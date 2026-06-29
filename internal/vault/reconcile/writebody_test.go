package reconcile_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

// TestWriteNoteBody_PreservesFrontmatter rewrites only the body, leaving the
// frontmatter (and the note id) untouched.
func TestWriteNoteBody_PreservesFrontmatter(t *testing.T) {
	ctx := context.Background()
	g, vault, rec := newColdStartHarness(t)

	path := filepath.Join(vault, "topic.md")
	writeFile(t, path, "---\nid: note-1\ntitle: Topic\n---\n\nOriginal body.\n")
	if err := rec.OnCreate(ctx, path); err != nil {
		t.Fatalf("OnCreate: %v", err)
	}

	written, err := reconcile.WriteNoteBody(ctx, g, vault, "note-1", "Merged body.\n")
	if err != nil {
		t.Fatalf("WriteNoteBody: %v", err)
	}
	if !written {
		t.Fatalf("expected the file to be located and written")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, "id: note-1") || !strings.Contains(got, "title: Topic") {
		t.Errorf("frontmatter not preserved: %q", got)
	}
	if !strings.Contains(got, "Merged body.") || strings.Contains(got, "Original body.") {
		t.Errorf("body not replaced: %q", got)
	}
}

// TestWriteNoteBody_LocatesByFrontmatterWhenUncached finds the file by its
// frontmatter id even when the path cache has no row (e.g. a freshly written
// note the watcher has not seen yet).
func TestWriteNoteBody_LocatesByFrontmatterWhenUncached(t *testing.T) {
	ctx := context.Background()
	g, vault, _ := newColdStartHarness(t)

	// Write a file directly; never reconcile it, so note_paths has no entry.
	path := filepath.Join(vault, "uncached.md")
	writeFile(t, path, "---\nid: note-x\n---\n\nBefore.\n")

	written, err := reconcile.WriteNoteBody(ctx, g, vault, "note-x", "After.\n")
	if err != nil {
		t.Fatalf("WriteNoteBody: %v", err)
	}
	if !written {
		t.Fatalf("expected fallback frontmatter scan to locate the file")
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "After.") {
		t.Errorf("body not written via fallback: %q", string(raw))
	}
}

// TestWriteNoteBody_NoMatchIsNoOp returns written=false (no error) when the
// note cannot be located.
func TestWriteNoteBody_NoMatchIsNoOp(t *testing.T) {
	ctx := context.Background()
	g, vault, _ := newColdStartHarness(t)

	written, err := reconcile.WriteNoteBody(ctx, g, vault, "ghost", "x")
	if err != nil {
		t.Fatalf("WriteNoteBody: %v", err)
	}
	if written {
		t.Errorf("expected written=false for an unlocatable note")
	}
}
