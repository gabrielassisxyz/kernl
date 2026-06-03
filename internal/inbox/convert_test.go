package inbox_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/inbox"
)

func TestProcess(t *testing.T) {
	ctx := context.Background()
	vaultRoot := t.TempDir()

	dbPath := filepath.Join(t.TempDir(), "graph.db")
	g, err := graph.Open(ctx, graph.Config{Path: dbPath})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	var captureID string
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		id, err := nodes.CreateCapture(ctx, tx, nodes.Capture{
			Title: "Test Capture",
			Body:  "Test Body",
			Tags:  []string{"pending"},
		}, nodes.Author{Name: "tester"})
		captureID = id
		return err
	})
	if err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}

	err = inbox.Process(ctx, g, vaultRoot, nil, captureID, "note")
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	// Verify Capture is triaged and edge is created
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		cap, err := nodes.GetCapture(ctx, tx, captureID)
		if err != nil {
			return err
		}

		hasTriaged := false
		for _, tag := range cap.Tags {
			if tag == "triaged" {
				hasTriaged = true
			}
			if tag == "pending" {
				t.Errorf("expected 'pending' tag to be removed")
			}
		}
		if !hasTriaged {
			t.Errorf("expected 'triaged' tag")
		}

		inEdges, err := edges.Incoming(ctx, tx, captureID)
		if err != nil {
			return err
		}
		if len(inEdges) != 1 {
			t.Errorf("expected 1 incoming edge (derived_from), got %d", len(inEdges))
		} else {
			if inEdges[0].Label != "derived_from" {
				t.Errorf("expected edge label 'derived_from', got %q", inEdges[0].Label)
			}

			// Verify the source node is a Note
			note, err := nodes.GetNote(ctx, tx, inEdges[0].Src)
			if err != nil {
				t.Errorf("GetNote for src node failed: %v", err)
			} else {
				if note.Title != "Test Capture" {
					t.Errorf("expected Note Title 'Test Capture', got %q", note.Title)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("DoRead: %v", err)
	}

	// Verify markdown file is written
	files, err := os.ReadDir(vaultRoot)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 file written to vault, got %d", len(files))
	} else {
		content, _ := os.ReadFile(filepath.Join(vaultRoot, files[0].Name()))
		if !strings.Contains(string(content), "id:") {
			t.Errorf("expected id in markdown file, got:\n%s", string(content))
		}
	}
}

// TestProcessConvertInfersTarget verifies the single "convert" action infers a
// bookmark for URL bodies and a note for everything else (UI has one button).
func TestProcessConvertInfersTarget(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantType string
	}{
		{"url becomes bookmark", "https://example.com/article", "bookmark"},
		{"text becomes note", "remember to water the plants", "note"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			vaultRoot := t.TempDir()
			g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
			if err != nil {
				t.Fatalf("graph.Open: %v", err)
			}
			defer g.Close()

			var captureID string
			if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
				id, err := nodes.CreateCapture(ctx, tx, nodes.Capture{Body: tc.body, Tags: []string{"pending"}}, nodes.Author{Name: "tester"})
				captureID = id
				return err
			}); err != nil {
				t.Fatalf("CreateCapture: %v", err)
			}

			if err := inbox.Process(ctx, g, vaultRoot, nil, captureID, "convert"); err != nil {
				t.Fatalf("Process: %v", err)
			}

			if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
				inEdges, err := edges.Incoming(ctx, tx, captureID)
				if err != nil {
					return err
				}
				if len(inEdges) != 1 {
					t.Fatalf("expected 1 derived_from edge, got %d", len(inEdges))
				}
				var gotType string
				if err := tx.QueryRow(`SELECT type FROM nodes WHERE id = ?`, inEdges[0].Src).Scan(&gotType); err != nil {
					return err
				}
				if gotType != tc.wantType {
					t.Errorf("convert(%q): derived node type = %q, want %q", tc.body, gotType, tc.wantType)
				}
				return nil
			}); err != nil {
				t.Fatalf("DoRead: %v", err)
			}
		})
	}
}
