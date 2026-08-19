package reconcile_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

// TestSetLinkStateWritesLinkState verifies the endpoint's write path: link
// state lands on the note's attrs and reads back through LinkStateFor.
func TestSetLinkStateWritesLinkState(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	vault := newVaultDir(t)

	path := filepath.Join(vault, "note.md")
	writeFile(t, path, "---\nid: link-state-note\ntitle: Link State\n---\n\nBody.\n")

	rec := reconcile.New(g, vault)
	if err := rec.OnCreate(ctx, path); err != nil {
		t.Fatalf("OnCreate: %v", err)
	}

	state := reconcile.LinkState{
		Channel:       "cli",
		Suggestions:   []nodes.LinkCandidate{{ID: "id-1", Title: "Kernl", Snippet: "s"}},
		NoLinksReason: "scratch",
	}
	if err := reconcile.SetLinkState(ctx, g, vault, path, state); err != nil {
		t.Fatalf("SetLinkState: %v", err)
	}

	got, err := reconcile.LinkStateFor(ctx, g, "link-state-note")
	if err != nil {
		t.Fatalf("LinkStateFor: %v", err)
	}
	if got.Channel != "cli" {
		t.Errorf("channel = %q, want %q", got.Channel, "cli")
	}
	if len(got.Suggestions) != 1 || got.Suggestions[0].ID != "id-1" {
		t.Errorf("suggestions = %+v, want [id-1]", got.Suggestions)
	}
	if got.NoLinksReason != "scratch" {
		t.Errorf("noLinksReason = %q, want %q", got.NoLinksReason, "scratch")
	}
}

// TestLinkStateSurvivesReconcile is the guard for option B's one real risk: a
// reconcile pass rebuilds the node from frontmatter, and the link state is not
// in frontmatter, so it must be carried over explicitly. A sixth construction
// site added later that forgets this drops the state silently - this test is
// what turns that silence into a red build.
func TestLinkStateSurvivesReconcile(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	vault := newVaultDir(t)

	path := filepath.Join(vault, "note.md")
	writeFile(t, path, "---\nid: survive-note\ntitle: Survive\n---\n\nBody.\n")

	rec := reconcile.New(g, vault)
	if err := rec.OnCreate(ctx, path); err != nil {
		t.Fatalf("OnCreate: %v", err)
	}

	state := reconcile.LinkState{
		Channel:       "cli",
		Suggestions:   []nodes.LinkCandidate{{ID: "id-1", Title: "Kernl", Snippet: "s"}},
		NoLinksReason: "scratch",
	}
	if err := reconcile.SetLinkState(ctx, g, vault, path, state); err != nil {
		t.Fatalf("SetLinkState: %v", err)
	}

	// A reconcile pass over the file rebuilds the node from frontmatter.
	writeFile(t, path, "---\nid: survive-note\ntitle: Survive\n---\n\nBody changed.\n")
	if err := rec.OnChange(ctx, path); err != nil {
		t.Fatalf("OnChange: %v", err)
	}

	got, err := reconcile.LinkStateFor(ctx, g, "survive-note")
	if err != nil {
		t.Fatalf("LinkStateFor: %v", err)
	}
	if got.Channel != "cli" {
		t.Errorf("channel = %q, want %q after reconcile", got.Channel, "cli")
	}
	if len(got.Suggestions) != 1 || got.Suggestions[0].ID != "id-1" {
		t.Errorf("suggestions = %+v, want [id-1] after reconcile", got.Suggestions)
	}
	if got.NoLinksReason != "scratch" {
		t.Errorf("noLinksReason = %q, want %q after reconcile", got.NoLinksReason, "scratch")
	}
}
