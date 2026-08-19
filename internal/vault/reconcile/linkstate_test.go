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

// TestLinkStateSurvivesAWriteThatOffersNothing guards the receipt against the
// write that carries no offer of its own. A save from the web UI never offers
// links, so it arrives with an empty state; wholesale replacement made that
// save erase the offer the assistant had just recorded, with no error and no
// log line. The derived accepted/rejected of every later write went with it,
// and a retroactive pass would have re-offered the same links forever.
func TestLinkStateSurvivesAWriteThatOffersNothing(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()
	vault := newVaultDir(t)

	path := filepath.Join(vault, "kept.md")
	writeFile(t, path, "---\nid: kept-note\ntitle: Kept\n---\n\nBody.\n")

	rec := reconcile.New(g, vault)
	if err := rec.OnCreate(ctx, path); err != nil {
		t.Fatalf("OnCreate: %v", err)
	}

	offered := reconcile.LinkState{
		Channel:       "cli",
		Suggestions:   []nodes.LinkCandidate{{ID: "a", Title: "A"}, {ID: "b", Title: "B"}},
		NoLinksReason: "nothing fit",
	}
	if err := reconcile.SetLinkState(ctx, g, vault, path, offered); err != nil {
		t.Fatalf("SetLinkState cli: %v", err)
	}

	// The same note saved from the web UI: a real write, with nothing to offer.
	if err := reconcile.SetLinkState(ctx, g, vault, path, reconcile.LinkState{Channel: "ui"}); err != nil {
		t.Fatalf("SetLinkState ui: %v", err)
	}

	got, err := reconcile.LinkStateFor(ctx, g, "kept-note")
	if err != nil {
		t.Fatalf("LinkStateFor: %v", err)
	}
	if got.Channel != "ui" {
		t.Errorf("channel = %q, want %q: the channel is a fact about the last write", got.Channel, "ui")
	}
	if len(got.Suggestions) != 2 {
		t.Errorf("the offer was erased by a write that offered nothing: %d, want 2", len(got.Suggestions))
	}
	if got.NoLinksReason != "nothing fit" {
		t.Errorf("the reason was erased by a write that declared none: %q", got.NoLinksReason)
	}
}
