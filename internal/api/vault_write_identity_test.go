package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/api"
	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

// TestVaultWriteKeepsThePathsIdentity is the regression for an identity churn that
// destroyed a note's history in the live vault on 2026-08-01: writing a path that
// already had a node, with frontmatter that omitted id, minted a FRESH uuid, and the
// next cold start tombstoned the original node and adopted the new one as a stranger.
// The body survived; the revisions and every edge pointing at the old id did not.
//
// The write handler injects an id on purpose - without one the reconciler injects it
// out of band right after the editor loads the file, bumping the mtime and turning the
// editor's next autosave into a false 409. The defect was never the injection, it was
// minting a new id for a path whose id is already known.
func TestVaultWriteKeepsThePathsIdentity(t *testing.T) {
	root := t.TempDir()
	g := testutil.NewInMemoryTestGraph(t)
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}, Graph: g}

	const path = "kernl-redesign-brief.md"
	ctx := context.Background()

	// The note as the graph already holds it: a live node AND the note_paths row
	// pointing at this file. Both halves matter - the lookup deliberately ignores a
	// path row whose node is tombstoned.
	var existingID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		existingID, err = nodes.CreateNote(ctx, tx, nodes.Note{Title: "Brief", Body: "the first draft"},
			nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, path),
		[]byte("---\nid: "+existingID+"\ntitle: Brief\n---\n\nthe first draft\n"), 0644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := reconcile.Upsert(ctx, g, existingID, path, "seed"); err != nil {
		t.Fatalf("seed note_paths: %v", err)
	}

	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)

	// The editor's own save shape: the body it round-trips has no id, because the
	// user never typed one and the file it loaded may predate injection.
	body := "---\ntitle: Brief\n---\n\nthe second draft\n"
	req := httptest.NewRequest("POST", "/api/vault/file?path="+path, bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("write returned %d: %s", w.Code, w.Body.String())
	}

	written, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(string(written), existingID) {
		t.Fatalf("the write minted a new identity for a path that already had one.\n"+
			"want id %s to survive, got:\n%s", existingID, written)
	}
}

// TestVaultWriteStillMintsAnIDForANewPath is the other half, and it is the reason the
// fix is a lookup rather than dropping the injection: a genuinely new path has no id to
// keep, and leaving it without one is what the injection exists to prevent.
func TestVaultWriteStillMintsAnIDForANewPath(t *testing.T) {
	root := t.TempDir()
	g := testutil.NewInMemoryTestGraph(t)
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}, Graph: g}

	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)

	req := httptest.NewRequest("POST", "/api/vault/file?path=brand-new.md",
		bytes.NewBufferString("---\ntitle: New\n---\n\nbody\n"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("write returned %d: %s", w.Code, w.Body.String())
	}
	written, err := os.ReadFile(filepath.Join(root, "brand-new.md"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(string(written), "id: ") {
		t.Fatalf("a new path must still get an id injected, got:\n%s", written)
	}
}

// TestVaultWriteDoesNotResurrectATombstonedID guards the half of the lookup that is easy
// to drop as redundant: it asks for a LIVE node, not merely a note_paths row.
//
// A path row outlives the node it pointed at. Handing a new file the id of a deleted one
// reattaches it to a history that was deliberately ended - the opposite mistake to the one
// this change fixes, and the same class: a file wearing an identity that is not its own.
func TestVaultWriteDoesNotResurrectATombstonedID(t *testing.T) {
	root := t.TempDir()
	g := testutil.NewInMemoryTestGraph(t)
	a := &app.App{Config: &config.Config{Vault: config.VaultConfig{Root: root}}, Graph: g}
	ctx := context.Background()

	const path = "deleted-then-rewritten.md"
	var deadID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		deadID, err = nodes.CreateNote(ctx, tx, nodes.Note{Title: "Gone", Body: "old body"},
			nodes.Author{Name: "test"})
		if err != nil {
			return err
		}
		return nodes.SoftDeleteNoteTx(ctx, tx, deadID, "deleted-then-rewritten", "Gone",
			nodes.Author{Name: "test"})
	}); err != nil {
		t.Fatalf("seed tombstone: %v", err)
	}
	if err := reconcile.Upsert(ctx, g, deadID, path, "seed"); err != nil {
		t.Fatalf("seed note_paths: %v", err)
	}

	mux := http.NewServeMux()
	api.RegisterVaultRoutes(mux, a)
	req := httptest.NewRequest("POST", "/api/vault/file?path="+path,
		bytes.NewBufferString("---\ntitle: A new note at the same path\n---\n\nfresh body\n"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("write returned %d: %s", w.Code, w.Body.String())
	}

	written, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if strings.Contains(string(written), deadID) {
		t.Fatalf("the write took the identity of a tombstoned node (%s):\n%s", deadID, written)
	}
	if !strings.Contains(string(written), "id: ") {
		t.Fatalf("the write must still mint an id of its own, got:\n%s", written)
	}
}
