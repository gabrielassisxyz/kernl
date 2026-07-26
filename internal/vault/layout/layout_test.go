package layout

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

func newTestGraph(t *testing.T) *graph.Graph {
	t.Helper()
	g, err := graph.Open(context.Background(), graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	t.Cleanup(func() { g.Close() })
	return g
}

// writeNote drops a markdown file in the vault and returns its relative path.
func writeNote(t *testing.T, vault, relPath, body string) string {
	t.Helper()
	full := filepath.Join(vault, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", relPath, err)
	}
	return relPath
}

// anchorNote registers a note in the graph, points it at an entity with the
// given label, and maps it to relPath - the shape a companion note has after
// creation.
func anchorNote(t *testing.T, g *graph.Graph, relPath, label string) {
	t.Helper()
	ctx := context.Background()
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		author := nodes.Author{Name: "test"}
		entityID, err := nodes.CreateTask(ctx, tx, nodes.Task{Title: "Entity"}, author)
		if err != nil {
			return err
		}
		noteID, err := nodes.CreateNote(ctx, tx, nodes.Note{Title: "Companion"}, author)
		if err != nil {
			return err
		}
		if label != "" {
			if _, err := edges.Create(ctx, tx, edges.Edge{Src: noteID, Dst: entityID, Label: label}, author); err != nil {
				return err
			}
		}
		_, err = tx.Exec(
			`INSERT INTO note_paths (uuid, path, content_hash, updated_at)
			 VALUES (?, ?, '', strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))`,
			noteID, relPath,
		)
		return err
	})
	if err != nil {
		t.Fatalf("anchor %s: %v", relPath, err)
	}
}

func TestScanOrphansIgnoresAnchoredNotes(t *testing.T) {
	g := newTestGraph(t)
	vault := t.TempDir()

	anchorNote(t, g, writeNote(t, vault, TasksFolder+"/ship-it.md", "companion\n"), DescribesLabel)
	anchorNote(t, g, writeNote(t, vault, ProjectsFolder+"/launch.md", "companion\n"), DescribesLabel)
	anchorNote(t, g, writeNote(t, vault, DefaultDAFolder+"/prep-1.md", "primer\n"), PreparedForLabel)
	// A note at the vault root is the user's own territory - never reported.
	writeNote(t, vault, "loose-thought.md", "mine\n")

	orphans, err := ScanOrphans(context.Background(), g, vault, "")
	if err != nil {
		t.Fatalf("ScanOrphans: %v", err)
	}
	if len(orphans) != 0 {
		t.Errorf("expected no orphans, got %v", orphans)
	}
}

func TestScanOrphansReportsHandWrittenNote(t *testing.T) {
	g := newTestGraph(t)
	vault := t.TempDir()

	anchorNote(t, g, writeNote(t, vault, TasksFolder+"/ship-it.md", "companion\n"), DescribesLabel)
	writeNote(t, vault, TasksFolder+"/my-own-idea.md", "typed by hand, never indexed\n")
	// Indexed as a note, but tied to nothing: the entity it described is gone.
	anchorNote(t, g, writeNote(t, vault, ProjectsFolder+"/stale.md", "companion\n"), "")

	orphans, err := ScanOrphans(context.Background(), g, vault, "")
	if err != nil {
		t.Fatalf("ScanOrphans: %v", err)
	}
	if len(orphans) != 2 {
		t.Fatalf("expected 2 orphans, got %v", orphans)
	}
	if orphans[0].Path != ProjectsFolder+"/stale.md" || orphans[1].Path != TasksFolder+"/my-own-idea.md" {
		t.Errorf("unexpected orphan set (want sorted by path): %v", orphans)
	}
	if orphans[0].Reason == "" || orphans[1].Reason == "" {
		t.Errorf("every orphan needs a reason: %v", orphans)
	}
}

func TestScanOrphansHonoursConfiguredDAFolder(t *testing.T) {
	g := newTestGraph(t)
	vault := t.TempDir()

	writeNote(t, vault, "briefings/prep-1.md", "primer\n")

	orphans, err := ScanOrphans(context.Background(), g, vault, "briefings")
	if err != nil {
		t.Fatalf("ScanOrphans: %v", err)
	}
	if len(orphans) != 1 || orphans[0].Path != "briefings/prep-1.md" {
		t.Fatalf("configured DA folder not scanned: %v", orphans)
	}
}

func TestScanOrphansSkipsMissingVault(t *testing.T) {
	g := newTestGraph(t)
	orphans, err := ScanOrphans(context.Background(), g, "", "")
	if err != nil {
		t.Fatalf("ScanOrphans with no vault: %v", err)
	}
	if orphans != nil {
		t.Errorf("expected no orphans without a vault, got %v", orphans)
	}
}

// TestScanOrphansAcceptsABriefedPrimer covers the DA folder's second anchor: a
// primer's prepared_for edge does not outlive the capture being processed, and
// what stays is the briefing edge the resulting node points back with. Judging
// it by prepared_for alone reports every primer the user actually acted on.
func TestScanOrphansAcceptsABriefedPrimer(t *testing.T) {
	g := newTestGraph(t)
	vault := t.TempDir()
	relPath := writeNote(t, vault, DefaultDAFolder+"/prep-1.md", "primer\n")

	ctx := context.Background()
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		author := nodes.Author{Name: "test"}
		noteID, err := nodes.CreateNote(ctx, tx, nodes.Note{Title: "Primer"}, author)
		if err != nil {
			return err
		}
		taskID, err := nodes.CreateTask(ctx, tx, nodes.Task{Title: "Acted on it"}, author)
		if err != nil {
			return err
		}
		if _, err := edges.Create(ctx, tx, edges.Edge{Src: taskID, Dst: noteID, Label: BriefingLabel}, author); err != nil {
			return err
		}
		_, err = tx.Exec(
			`INSERT INTO note_paths (uuid, path, content_hash, updated_at)
			 VALUES (?, ?, '', strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))`,
			noteID, relPath,
		)
		return err
	})
	if err != nil {
		t.Fatalf("seed briefed primer: %v", err)
	}

	orphans, err := ScanOrphans(ctx, g, vault, "")
	if err != nil {
		t.Fatalf("ScanOrphans: %v", err)
	}
	if len(orphans) != 0 {
		t.Errorf("a briefed primer is anchored, not an orphan: %v", orphans)
	}
}
