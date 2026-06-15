package nodes_test

import (
	"context"
	"os"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

func openTestGraph(t *testing.T) *graph.Graph {
	t.Helper()
	f, err := os.CreateTemp("", "kernl-nodes-test-*.db")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	g, err := graph.Open(context.Background(), graph.Config{Path: f.Name()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { g.Close() })
	return g
}

func TestProjectAndTaskAreGraphNodes(t *testing.T) {
	ctx := context.Background()
	g := openTestGraph(t)
	author := nodes.Author{Name: "test"}

	var projectID, taskID, looseID string

	// Create a project, a task in that project, and a project-less task.
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		projectID, err = nodes.CreateProject(ctx, tx, nodes.Project{Title: "Home reno"}, author)
		if err != nil {
			return err
		}
		taskID, err = nodes.CreateTask(ctx, tx, nodes.Task{Title: "Paint walls", ProjectID: projectID, Status: nodes.TaskStatusInProgress}, author)
		if err != nil {
			return err
		}
		looseID, err = nodes.CreateTask(ctx, tx, nodes.Task{Title: "Buy milk"}, author)
		return err
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Both must land in the generic nodes table with the right types — i.e. they
	// are real graph nodes, not beads.
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var typ string
		if err := tx.QueryRow(`SELECT type FROM nodes WHERE id = ?`, projectID).Scan(&typ); err != nil {
			return err
		}
		if typ != "project" {
			t.Errorf("project node type = %q, want project", typ)
		}
		if err := tx.QueryRow(`SELECT type FROM nodes WHERE id = ?`, taskID).Scan(&typ); err != nil {
			return err
		}
		if typ != "task" {
			t.Errorf("task node type = %q, want task", typ)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read types: %v", err)
	}

	// Defaults applied.
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		p, err := nodes.GetProject(ctx, tx, projectID)
		if err != nil {
			return err
		}
		if p.Status != nodes.DefaultProjectStatus {
			t.Errorf("project status = %q, want %q", p.Status, nodes.DefaultProjectStatus)
		}
		loose, err := nodes.GetTask(ctx, tx, looseID)
		if err != nil {
			return err
		}
		if loose.Status != nodes.DefaultTaskStatus {
			t.Errorf("loose task status = %q, want %q", loose.Status, nodes.DefaultTaskStatus)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read defaults: %v", err)
	}

	// Filtering by project returns only the in-project task.
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		scoped, err := nodes.ListTasks(ctx, tx, projectID)
		if err != nil {
			return err
		}
		if len(scoped) != 1 || scoped[0].ID != taskID {
			t.Errorf("ListTasks(project) = %d tasks, want 1 (%s)", len(scoped), taskID)
		}
		all, err := nodes.ListTasks(ctx, tx, "")
		if err != nil {
			return err
		}
		if len(all) != 2 {
			t.Errorf("ListTasks(all) = %d tasks, want 2", len(all))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	// Status update persists.
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.SetTaskStatus(ctx, tx, taskID, nodes.TaskStatusDone, author)
	}); err != nil {
		t.Fatalf("SetTaskStatus: %v", err)
	}
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		got, err := nodes.GetTask(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if got.Status != nodes.TaskStatusDone {
			t.Errorf("after SetTaskStatus, status = %q, want done", got.Status)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read after status: %v", err)
	}

	// Updating a non-existent task reports ErrNotFound.
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.SetProjectStatus(ctx, tx, "nope", "done", author)
	}); err != graph.ErrNotFound {
		t.Errorf("SetProjectStatus(missing) = %v, want ErrNotFound", err)
	}
}
