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
	"github.com/gabrielassisxyz/kernl/internal/graph/tags"
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
			Tags:  []string{tags.Pending},
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
			if tag == tags.Triaged {
				hasTriaged = true
			}
			if tag == tags.Pending {
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

// TestProcessTaskUnderProject verifies a capture can be filed as a task linked
// to a project (part_of edge + mirrored ProjectID), with a derived_from edge
// back to the capture and a title override applied.
func TestProcessTaskUnderProject(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	var projectID, captureID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		pid, err := nodes.CreateProject(ctx, tx, nodes.Project{Title: "Web UI"}, nodes.Author{Name: "tester"})
		if err != nil {
			return err
		}
		projectID = pid
		cid, err := nodes.CreateCapture(ctx, tx, nodes.Capture{Body: "fix the inbox", Tags: []string{tags.Pending}}, nodes.Author{Name: "tester"})
		captureID = cid
		return err
	}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := inbox.ProcessCapture(ctx, g, t.TempDir(), nil, captureID, inbox.ProcessRequest{
		Target:    "task",
		ProjectID: projectID,
		Title:     "Fix inbox triage",
	}); err != nil {
		t.Fatalf("ProcessCapture: %v", err)
	}

	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		// Task exists under the project with the overridden title.
		tasks, err := nodes.ListTasks(ctx, tx, projectID)
		if err != nil {
			return err
		}
		if len(tasks) != 1 {
			t.Fatalf("expected 1 task under project, got %d", len(tasks))
		}
		if tasks[0].Title != "Fix inbox triage" {
			t.Errorf("title override not applied: got %q", tasks[0].Title)
		}
		if tasks[0].ProjectID != projectID {
			t.Errorf("task ProjectID = %q, want %q", tasks[0].ProjectID, projectID)
		}

		// part_of edge task -> project.
		out, err := edges.Outgoing(ctx, tx, tasks[0].ID)
		if err != nil {
			return err
		}
		var hasPartOf, hasDerivedFrom bool
		for _, e := range out {
			if e.Label == "part_of" && e.Dst == projectID {
				hasPartOf = true
			}
			if e.Label == "derived_from" && e.Dst == captureID {
				hasDerivedFrom = true
			}
		}
		if !hasPartOf {
			t.Errorf("expected part_of edge task->project")
		}
		if !hasDerivedFrom {
			t.Errorf("expected derived_from edge task->capture")
		}
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
	}
}

// TestProcessTaskUnfiled verifies a task with no project lands unfiled (no
// part_of edge, empty ProjectID) — the "unprocessed tasks" bucket.
func TestProcessTaskUnfiled(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	var captureID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		id, err := nodes.CreateCapture(ctx, tx, nodes.Capture{Body: "some loose idea", Tags: []string{tags.Pending}}, nodes.Author{Name: "tester"})
		captureID = id
		return err
	}); err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}

	if err := inbox.ProcessCapture(ctx, g, t.TempDir(), nil, captureID, inbox.ProcessRequest{Target: "task"}); err != nil {
		t.Fatalf("ProcessCapture: %v", err)
	}

	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		all, err := nodes.ListTasks(ctx, tx, "")
		if err != nil {
			return err
		}
		if len(all) != 1 {
			t.Fatalf("expected 1 task, got %d", len(all))
		}
		if all[0].ProjectID != "" {
			t.Errorf("expected unfiled task (empty ProjectID), got %q", all[0].ProjectID)
		}
		out, err := edges.Outgoing(ctx, tx, all[0].ID, edges.WithType(edges.EdgeTypePartOf))
		if err != nil {
			return err
		}
		if len(out) != 0 {
			t.Errorf("expected no part_of edge for unfiled task, got %d", len(out))
		}
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
	}
}

// TestListProcessed verifies processed captures report what they became: a
// triaged task names its target, a discarded capture reports "discard".
func TestListProcessed(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	var projectID, taskCap, junkCap string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		pid, err := nodes.CreateProject(ctx, tx, nodes.Project{Title: "Web UI"}, nodes.Author{Name: "t"})
		if err != nil {
			return err
		}
		projectID = pid
		a, err := nodes.CreateCapture(ctx, tx, nodes.Capture{Body: "do a thing", Tags: []string{tags.Pending}}, nodes.Author{Name: "t"})
		if err != nil {
			return err
		}
		taskCap = a
		b, err := nodes.CreateCapture(ctx, tx, nodes.Capture{Body: "junk", Tags: []string{tags.Pending}}, nodes.Author{Name: "t"})
		junkCap = b
		return err
	}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := inbox.ProcessCapture(ctx, g, t.TempDir(), nil, taskCap, inbox.ProcessRequest{Target: "task", ProjectID: projectID, Title: "Do a thing"}); err != nil {
		t.Fatalf("process task: %v", err)
	}
	if err := inbox.Process(ctx, g, t.TempDir(), nil, junkCap, "discard"); err != nil {
		t.Fatalf("discard: %v", err)
	}

	items, err := inbox.ListProcessed(ctx, g)
	if err != nil {
		t.Fatalf("ListProcessed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 processed items, got %d", len(items))
	}
	byCapture := map[string]inbox.ProcessedItem{}
	for _, it := range items {
		byCapture[it.CaptureID] = it
	}
	if got := byCapture[taskCap]; got.Became != "task" || got.ProjectID != projectID || got.TargetTitle != "Do a thing" {
		t.Errorf("task item = %+v, want became=task project=%s title=Do a thing", got, projectID)
	}
	if got := byCapture[junkCap]; got.Became != "discard" || got.TargetID != "" {
		t.Errorf("discard item = %+v, want became=discard no target", got)
	}
}

// TestReopenNote verifies undo of a note: the derived note is gone, its vault
// markdown is removed, and the capture is pending again.
func TestReopenNote(t *testing.T) {
	ctx := context.Background()
	vaultRoot := t.TempDir()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	var captureID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		id, err := nodes.CreateCapture(ctx, tx, nodes.Capture{Body: "a thought", Tags: []string{tags.Pending}}, nodes.Author{Name: "t"})
		captureID = id
		return err
	}); err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}

	if err := inbox.Process(ctx, g, vaultRoot, nil, captureID, "note"); err != nil {
		t.Fatalf("Process: %v", err)
	}

	// A markdown file exists and a note was derived.
	var noteID string
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		in, err := edges.Incoming(ctx, tx, captureID)
		if err != nil {
			return err
		}
		if len(in) != 1 {
			t.Fatalf("expected 1 derived_from edge, got %d", len(in))
		}
		noteID = in[0].Src
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
	}
	files, _ := os.ReadDir(vaultRoot)
	if len(files) != 1 {
		t.Fatalf("expected 1 md file before reopen, got %d", len(files))
	}

	if err := inbox.Reopen(ctx, g, vaultRoot, captureID); err != nil {
		t.Fatalf("Reopen: %v", err)
	}

	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		// Note is gone.
		if _, err := nodes.GetNote(ctx, tx, noteID); err == nil {
			t.Errorf("expected note to be deleted")
		}
		// Capture is pending again.
		cap, err := nodes.GetCapture(ctx, tx, captureID)
		if err != nil {
			return err
		}
		pending, triaged := false, false
		for _, tg := range cap.Tags {
			if tg == tags.Pending {
				pending = true
			}
			if tg == tags.Triaged {
				triaged = true
			}
		}
		if !pending || triaged {
			t.Errorf("capture tags = %v, want pending and not triaged", cap.Tags)
		}
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
	}

	// Markdown removed (scan fallback, since note_paths is unpopulated in tests).
	files, _ = os.ReadDir(vaultRoot)
	if len(files) != 0 {
		t.Errorf("expected md file removed after reopen, got %d files", len(files))
	}
}

// TestReopenTask verifies undo of a task removes the task and re-pends the capture.
func TestReopenTask(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	var captureID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		id, err := nodes.CreateCapture(ctx, tx, nodes.Capture{Body: "do it", Tags: []string{tags.Pending}}, nodes.Author{Name: "t"})
		captureID = id
		return err
	}); err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}
	if err := inbox.ProcessCapture(ctx, g, t.TempDir(), nil, captureID, inbox.ProcessRequest{Target: "task"}); err != nil {
		t.Fatalf("ProcessCapture: %v", err)
	}
	if err := inbox.Reopen(ctx, g, t.TempDir(), captureID); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		tasks, err := nodes.ListTasks(ctx, tx, "")
		if err != nil {
			return err
		}
		if len(tasks) != 0 {
			t.Errorf("expected task removed, got %d", len(tasks))
		}
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
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
				id, err := nodes.CreateCapture(ctx, tx, nodes.Capture{Body: tc.body, Tags: []string{tags.Pending}}, nodes.Author{Name: "tester"})
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

func TestProcessCaptureProjectCreatesInitialTasks(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	var captureID string
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		id, err := nodes.CreateCapture(ctx, tx, nodes.Capture{
			Body: "Build an ai-memory explainer with task ideas.",
			Tags: []string{tags.Pending},
		}, nodes.Author{Name: "tester"})
		captureID = id
		return err
	}); err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}

	if err := inbox.ProcessCapture(ctx, g, t.TempDir(), nil, captureID, inbox.ProcessRequest{
		Target:             "project",
		ProjectTitle:       "ai-memory explainer",
		ProjectDescription: "Explain ai-memory from repository material.",
		InitialTasks:       []string{"Map architecture", "Write usage examples"},
	}); err != nil {
		t.Fatalf("ProcessCapture: %v", err)
	}

	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		projects, err := nodes.ListProjects(ctx, tx)
		if err != nil {
			return err
		}
		if len(projects) != 1 {
			t.Fatalf("expected 1 project, got %d", len(projects))
		}
		if projects[0].Title != "ai-memory explainer" {
			t.Fatalf("project title = %q", projects[0].Title)
		}
		tasks, err := nodes.ListTasks(ctx, tx, projects[0].ID)
		if err != nil {
			return err
		}
		if len(tasks) != 2 {
			t.Fatalf("expected 2 initial tasks, got %d", len(tasks))
		}
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
	}
}
