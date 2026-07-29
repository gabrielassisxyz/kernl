package inbox_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/inbox"
	"github.com/gabrielassisxyz/kernl/internal/vault/companion"
	"github.com/gabrielassisxyz/kernl/internal/vault/layout"
)

// companionPathOf returns the vault-relative path note_paths records for the
// companion of an entity, or "" when the entity has none.
func companionPathOf(t *testing.T, g *graph.Graph, entityID string) string {
	t.Helper()
	var path string
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		return tx.QueryRow(
			`SELECT p.path FROM edges e
			 JOIN nodes n ON n.id = e.src AND n.type = 'note' AND n.deleted_at IS NULL
			 JOIN note_paths p ON p.uuid = n.id
			 WHERE e.dst = ? AND e.label = ?`,
			entityID, companion.EdgeLabel,
		).Scan(&path)
	}); err != nil {
		return ""
	}
	return path
}

// A task born from a capture must reach the vault, not only the graph.
//
// The regression this guards is a whole class of loss rather than one bug: the
// graph database is git-ignored and only the markdown is backed up, so an entity
// with no companion file exists nowhere a restore can find it. Creating a task
// through the API wrote one; processing a capture into a task did not, because
// the two paths were separate implementations and only one knew about companions.
func TestCaptureProcessedIntoTaskGetsACompanionFile(t *testing.T) {
	ctx := context.Background()
	g := openInboxGraph(t)
	vault := t.TempDir()

	captureID := seedCapture(t, g, "varredura de notes que extrai os acionáveis perdidos")
	if err := inbox.ProcessCapture(ctx, g, vault, nil, captureID, inbox.ProcessRequest{
		Actions: []inbox.Action{{Target: "task", Title: "Varredura de notes", Body: "extrair acionáveis"}},
	}); err != nil {
		t.Fatalf("ProcessCapture: %v", err)
	}

	var taskID string
	for id, typ := range derivedOf(t, g, captureID) {
		if typ == "task" {
			taskID = id
		}
	}
	if taskID == "" {
		t.Fatal("no task derived from the capture")
	}

	relPath := companionPathOf(t, g, taskID)
	if relPath == "" {
		t.Fatal("task has no companion note")
	}
	if !strings.HasPrefix(relPath, layout.TasksFolder+"/") {
		t.Errorf("companion path = %q, want it under %s", relPath, layout.TasksFolder)
	}

	// The file, its id and the description all have to be there: a note_paths row
	// pointing at nothing is the harder version of the same failure.
	data, err := os.ReadFile(filepath.Join(vault, filepath.FromSlash(relPath)))
	if err != nil {
		t.Fatalf("companion file: %v", err)
	}
	if !strings.Contains(string(data), "extrair acionáveis") {
		t.Errorf("companion frontmatter lost the description:\n%s", data)
	}
	if !strings.Contains(string(data), taskID) {
		t.Errorf("companion body does not link the task it describes:\n%s", data)
	}
}

// A project fans out into itself plus its initial tasks, and each of those is a
// task in its own right, so each needs its own companion. Without this the
// project's own note existed and its tasks were the only nodes in the vault with
// nothing to annotate.
func TestCaptureProcessedIntoProjectGivesEveryNodeACompanion(t *testing.T) {
	ctx := context.Background()
	g := openInboxGraph(t)
	vault := t.TempDir()

	captureID := seedCapture(t, g, "biblioteca de músicas selfhosted e com qualidade")
	if err := inbox.ProcessCapture(ctx, g, vault, nil, captureID, inbox.ProcessRequest{
		Actions: []inbox.Action{{
			Target:       "project",
			ProjectTitle: "music-library",
			Body:         "biblioteca selfhosted",
			InitialTasks: []string{"Escolher o servidor", "Importar a coleção"},
		}},
	}); err != nil {
		t.Fatalf("ProcessCapture: %v", err)
	}

	derived := derivedOf(t, g, captureID)
	if len(derived) != 3 {
		t.Fatalf("expected the project and its 2 tasks, got %v", derived)
	}
	for id, typ := range derived {
		relPath := companionPathOf(t, g, id)
		if relPath == "" {
			t.Errorf("%s %s has no companion note", typ, id)
			continue
		}
		if _, err := os.Stat(filepath.Join(vault, filepath.FromSlash(relPath))); err != nil {
			t.Errorf("%s %s: companion file missing: %v", typ, id, err)
		}
	}
}

// Undo has to take the companion with the node. The companion is reached by the
// describes edge, not by derived_from, so the walk that powers undo does not see
// it: without the extra step a capture could be reopened, processed again, and
// leave a note in the vault describing an entity that no longer exists.
func TestReopenRemovesTheCompanionToo(t *testing.T) {
	ctx := context.Background()
	g := openInboxGraph(t)
	vault := t.TempDir()

	captureID := seedCapture(t, g, "dashboard de consumo de tokens")
	if err := inbox.ProcessCapture(ctx, g, vault, nil, captureID, inbox.ProcessRequest{
		Actions: []inbox.Action{{Target: "task", Title: "Dashboard de tokens"}},
	}); err != nil {
		t.Fatalf("ProcessCapture: %v", err)
	}
	var taskID string
	for id := range derivedOf(t, g, captureID) {
		taskID = id
	}
	relPath := companionPathOf(t, g, taskID)
	if relPath == "" {
		t.Fatal("task has no companion to begin with")
	}

	if err := inbox.Reopen(ctx, g, vault, captureID); err != nil {
		t.Fatalf("Reopen: %v", err)
	}

	if _, err := os.Stat(filepath.Join(vault, filepath.FromSlash(relPath))); !os.IsNotExist(err) {
		t.Errorf("companion file survived the undo: %v", err)
	}
	if got := companionPathOf(t, g, taskID); got != "" {
		t.Errorf("companion note still in the graph at %q", got)
	}
	if left := markdownUnder(t, vault); len(left) != 0 {
		t.Errorf("undo left markdown behind: %v", left)
	}
}
