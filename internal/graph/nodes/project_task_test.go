package nodes_test

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/tags"
)

func openTestGraph(t *testing.T) *graph.Graph {
	t.Helper()
	f, err := os.CreateTemp("", "kernl-nodes-test-*.db")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()
	t.Cleanup(func() { _ = os.Remove(f.Name()) })

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

// Tasks and projects carry tags on the same axis as every other node type: a
// task tagged "homelab" must come back from a tag query next to the notes that
// share it, which is the whole point of a universal tag.
func TestProjectAndTaskCarryTags(t *testing.T) {
	ctx := context.Background()
	g := openTestGraph(t)
	author := nodes.Author{Name: "test"}

	var projectID, taskID, noteID string

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		projectID, err = nodes.CreateProject(ctx, tx, nodes.Project{
			Title: "Homelab rebuild",
			Tags:  []string{"homelab"},
		}, author)
		if err != nil {
			return err
		}
		// Duplicates on create collapse to a single tag.
		taskID, err = nodes.CreateTask(ctx, tx, nodes.Task{
			Title:     "Swap the NAS disks",
			ProjectID: projectID,
			Tags:      []string{"homelab", "hardware", "homelab"},
		}, author)
		if err != nil {
			return err
		}
		noteID, err = nodes.CreateNote(ctx, tx, nodes.Note{
			Title: "NAS notes",
			Tags:  []string{"homelab"},
		}, author)
		return err
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		task, err := nodes.GetTask(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if got := sorted(task.Tags); !slices.Equal(got, []string{"hardware", "homelab"}) {
			t.Errorf("GetTask tags = %v, want [hardware homelab] (deduplicated)", got)
		}

		project, err := nodes.GetProject(ctx, tx, projectID)
		if err != nil {
			return err
		}
		if !slices.Equal(project.Tags, []string{"homelab"}) {
			t.Errorf("GetProject tags = %v, want [homelab]", project.Tags)
		}

		// ListTasks / ListProjects hydrate tags too, not just the single-node reads.
		list, err := nodes.ListTasks(ctx, tx, "")
		if err != nil {
			return err
		}
		if len(list) != 1 || len(list[0].Tags) != 2 {
			t.Errorf("ListTasks tags = %v, want 2 tags hydrated", list)
		}
		projects, err := nodes.ListProjects(ctx, tx)
		if err != nil {
			return err
		}
		if len(projects) != 1 || !slices.Equal(projects[0].Tags, []string{"homelab"}) {
			t.Errorf("ListProjects tags = %v, want [homelab]", projects)
		}

		// The payoff: one tag query, three node types.
		tagged, err := tags.Nodes(ctx, tx, "homelab")
		if err != nil {
			return err
		}
		for _, want := range []string{noteID, taskID, projectID} {
			if !slices.Contains(tagged, want) {
				t.Errorf("tags.Nodes(homelab) = %v, missing %s", tagged, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// A status update must not disturb the tags — it writes attrs, not node_tags.
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.SetTaskStatus(ctx, tx, taskID, nodes.TaskStatusDone, author)
	}); err != nil {
		t.Fatalf("SetTaskStatus: %v", err)
	}

	// Setting tags replaces the whole set; an empty slice clears it.
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if err := nodes.SetTaskTags(ctx, tx, taskID, []string{"storage"}, author); err != nil {
			return err
		}
		return nodes.SetProjectTags(ctx, tx, projectID, []string{}, author)
	}); err != nil {
		t.Fatalf("set tags: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		task, err := nodes.GetTask(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if !slices.Equal(task.Tags, []string{"storage"}) {
			t.Errorf("after SetTaskTags, tags = %v, want [storage]", task.Tags)
		}
		if task.Status != nodes.TaskStatusDone || task.Title != "Swap the NAS disks" || task.ProjectID != projectID {
			t.Errorf("SetTaskTags clobbered a non-tag field: %+v", task)
		}
		project, err := nodes.GetProject(ctx, tx, projectID)
		if err != nil {
			return err
		}
		if len(project.Tags) != 0 {
			t.Errorf("after SetProjectTags([]), tags = %v, want none", project.Tags)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read after set tags: %v", err)
	}

	// Tagging a missing node is a not-found, not a silent no-op.
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.SetTaskTags(ctx, tx, "nope", []string{"x"}, author)
	}); err != graph.ErrNotFound {
		t.Errorf("SetTaskTags(missing) = %v, want ErrNotFound", err)
	}
}

func sorted(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return out
}
