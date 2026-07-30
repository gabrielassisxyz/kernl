package api

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/ingest"
	"github.com/gabrielassisxyz/kernl/internal/vault/layout"
)

// An ingest run records its source as a bookmark node, and that node owes a
// companion note like any other bookmark.
//
// Not for symmetry's sake: the doctor check and the backfill sweep both look for
// bookmarks with no companion, so a source node without one would be reported as
// drift forever and then repaired by the sweep anyway. Whichever way it goes, it
// has to go the same way in both places.
func TestIngestSourceGetsACompanion(t *testing.T) {
	a, _ := newIngestApp(t)

	// The unit under test rather than the HTTP route: createIngestSourceNode is
	// reached only by the URL-fetch handler, whose fetcher wants the network, and
	// what changed here is the node the function writes.
	id, err := createIngestSourceNode(context.Background(), a, ingest.SourceDocument{
		Kind:    "article",
		URL:     "https://example.com/um-artigo",
		Title:   "Um artigo",
		Content: "o corpo",
	})
	if err != nil {
		t.Fatalf("createIngestSourceNode: %v", err)
	}

	relPath := ingestCompanionPath(t, a, id)
	if !strings.HasPrefix(relPath, layout.BookmarksFolder+"/") {
		t.Errorf("companion path = %q, want it under %s", relPath, layout.BookmarksFolder)
	}
	data, err := os.ReadFile(filepath.Join(a.Config.Vault.Root, filepath.FromSlash(relPath)))
	if err != nil {
		t.Fatalf("companion file: %v", err)
	}
	// The tag rides along, which is what keeps a machine-made source node
	// distinguishable from a link the user saved on purpose.
	if !bytes.Contains(data, []byte("ingest-source")) {
		t.Errorf("companion lost the ingest-source tag:\n%s", data)
	}
}

// ingestCompanionPath returns the note_paths path of an entity's companion, and
// fails the test when there is none.
func ingestCompanionPath(t *testing.T, a *app.App, entityID string) string {
	t.Helper()
	var path string
	if err := a.Graph.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		return tx.QueryRow(
			`SELECT p.path FROM edges e
			 JOIN nodes n ON n.id = e.src AND n.type = 'note' AND n.deleted_at IS NULL
			 JOIN note_paths p ON p.uuid = n.id
			 WHERE e.dst = ? AND e.label = 'describes'`,
			entityID,
		).Scan(&path)
	}); err != nil {
		t.Fatalf("no companion for ingest source %s: %v", entityID, err)
	}
	return path
}
