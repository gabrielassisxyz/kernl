package ingest_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/gabrielassisxyz/kernl/internal/ingest"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

// TestMergeSurvivesReconciliation proves the U6 fix: an Update merge is mirrored
// into the note's vault file, so a subsequent reconciliation of that file (the
// source of truth) does not clobber the merged body back out of the node.
func TestMergeSurvivesReconciliation(t *testing.T) {
	ctx := context.Background()
	g := testutil.NewInMemoryTestGraph(t)
	vault := t.TempDir()
	rec := reconcile.New(g, vault)

	// A real vault note reconciled into the graph (node + note_paths + FTS).
	notePath := filepath.Join(vault, "topic.md")
	if err := os.WriteFile(notePath, []byte("---\nid: note-1\ntitle: Topic\n---\n\nOriginal line.\n"), 0644); err != nil {
		t.Fatalf("write note: %v", err)
	}
	if err := rec.OnCreate(ctx, notePath); err != nil {
		t.Fatalf("OnCreate: %v", err)
	}

	// An ingest review resolved as an Update merge into that note.
	var reviewID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var e error
		reviewID, e = nodes.CreateIngestReview(ctx, tx, nodes.IngestReview{
			Title:   "Add detail",
			Payload: "Topic also has a second aspect.",
		}, nodes.Author{Name: "test"})
		return e
	}); err != nil {
		t.Fatalf("seed review: %v", err)
	}

	update := &ingest.UpdateInput{
		TargetNoteID:  "note-1",
		AcceptedHunks: []ingest.MergeHunk{{ID: "0", Content: "Second aspect added."}},
	}
	if err := ingest.ResolveReview(ctx, g, vault, reviewID, "Update", update); err != nil {
		t.Fatalf("ResolveReview: %v", err)
	}

	// The merge landed in BOTH the node and the vault file.
	if got := nodeBody(t, g, "note-1"); !strings.Contains(got, "Second aspect added.") {
		t.Fatalf("merge missing from node body: %q", got)
	}
	raw, _ := os.ReadFile(notePath)
	if !strings.Contains(string(raw), "Second aspect added.") {
		t.Fatalf("merge missing from vault file: %q", string(raw))
	}
	if !strings.Contains(string(raw), "id: note-1") {
		t.Fatalf("frontmatter id lost: %q", string(raw))
	}

	// The clobber trigger: the file is reconciled again (what the watcher does
	// on any touch). Before the fix this re-derived the node from a stale file
	// and dropped the merge; with the file mirrored, the merge survives.
	if err := rec.OnChange(ctx, notePath); err != nil {
		t.Fatalf("OnChange: %v", err)
	}
	if got := nodeBody(t, g, "note-1"); !strings.Contains(got, "Second aspect added.") {
		t.Errorf("merge clobbered by reconciliation: %q", got)
	}
}

func nodeBody(t *testing.T, g *graph.Graph, id string) string {
	t.Helper()
	var n *nodes.Note
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		var e error
		n, e = nodes.GetNote(context.Background(), tx, id)
		return e
	}); err != nil {
		t.Fatalf("GetNote: %v", err)
	}
	return n.Body
}
