package nodes_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

func day(t *testing.T, s string) *time.Time {
	t.Helper()
	d, err := nodes.ParseDueDate(s)
	if err != nil {
		t.Fatalf("ParseDueDate(%q): %v", s, err)
	}
	return d
}

func TestParseDueDate(t *testing.T) {
	if got, err := nodes.ParseDueDate(""); err != nil || got != nil {
		t.Fatalf("empty string: want (nil, nil), got (%v, %v)", got, err)
	}
	got := day(t, "2026-04-02")
	if got.Year() != 2026 || got.Month() != time.April || got.Day() != 2 {
		t.Fatalf("2026-04-02 parsed as %v", got)
	}
	if nodes.FormatDueDate(got) != "2026-04-02" {
		t.Fatalf("round trip: got %q", nodes.FormatDueDate(got))
	}
	if nodes.FormatDueDate(nil) != "" {
		t.Fatalf("no due date should format to the empty string")
	}
	// A timestamp is not a due date: a due date is a calendar day.
	if _, err := nodes.ParseDueDate("2026-04-02T00:00:00Z"); err == nil {
		t.Fatal("expected an RFC3339 timestamp to be rejected")
	}
	if _, err := nodes.ParseDueDate("tomorrow"); err == nil {
		t.Fatal("expected unparseable text to be rejected")
	}
}

// A due date lands in the generic attrs blob — no migration — and must survive
// the round trip through both readers.
func TestTaskDueDateRoundTrip(t *testing.T) {
	ctx := context.Background()
	g := openTestGraph(t)
	author := nodes.Author{Name: "test"}

	var withDue, without string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		withDue, err = nodes.CreateTask(ctx, tx, nodes.Task{
			Title:   "Ship the substack resources",
			DueDate: day(t, "2026-04-02"),
		}, author)
		if err != nil {
			return err
		}
		without, err = nodes.CreateTask(ctx, tx, nodes.Task{Title: "Someday"}, author)
		return err
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		got, err := nodes.GetTask(ctx, tx, withDue)
		if err != nil {
			return err
		}
		if nodes.FormatDueDate(got.DueDate) != "2026-04-02" {
			t.Errorf("GetTask due date: got %q, want 2026-04-02", nodes.FormatDueDate(got.DueDate))
		}
		none, err := nodes.GetTask(ctx, tx, without)
		if err != nil {
			return err
		}
		if none.DueDate != nil {
			t.Errorf("a task created without a due date has one: %v", none.DueDate)
		}

		tasks, err := nodes.ListTasks(ctx, tx, "")
		if err != nil {
			return err
		}
		for _, task := range tasks {
			if task.ID != withDue {
				continue
			}
			if nodes.FormatDueDate(task.DueDate) != "2026-04-02" {
				t.Errorf("ListTasks due date: got %q, want 2026-04-02", nodes.FormatDueDate(task.DueDate))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
}

func TestSetTaskDueDate(t *testing.T) {
	ctx := context.Background()
	g := openTestGraph(t)
	author := nodes.Author{Name: "test"}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateTask(ctx, tx, nodes.Task{Title: "Read the PDFs"}, author)
		return err
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.SetTaskDueDate(ctx, tx, id, day(t, "2026-04-03"), author)
	}); err != nil {
		t.Fatalf("SetTaskDueDate: %v", err)
	}
	if got := readTaskDue(t, g, id); got != "2026-04-03" {
		t.Fatalf("after set: got %q, want 2026-04-03", got)
	}

	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.SetTaskDueDate(ctx, tx, id, nil, author)
	}); err != nil {
		t.Fatalf("SetTaskDueDate(nil): %v", err)
	}
	if got := readTaskDue(t, g, id); got != "" {
		t.Fatalf("after clear: got %q, want no due date", got)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.SetTaskDueDate(ctx, tx, "no-such-task", day(t, "2026-04-03"), author)
	})
	if err != graph.ErrNotFound {
		t.Fatalf("missing task: got %v, want ErrNotFound", err)
	}
}

// The partial updates read-modify-write the whole node, so each one has to carry
// the fields it does not mean to touch. Editing one must never erase the other.
func TestTaskPartialUpdatesPreserveEachOther(t *testing.T) {
	ctx := context.Background()
	g := openTestGraph(t)
	author := nodes.Author{Name: "test"}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateTask(ctx, tx, nodes.Task{
			Title:   "Set up the git backup for the dotfiles",
			Tags:    []string{"homelab"},
			DueDate: day(t, "2026-04-02"),
		}, author)
		return err
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Retagging must not drop the due date...
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.SetTaskTags(ctx, tx, id, []string{"homelab", "backup"}, author)
	}); err != nil {
		t.Fatalf("SetTaskTags: %v", err)
	}
	if got := readTaskDue(t, g, id); got != "2026-04-02" {
		t.Fatalf("retagging erased the due date: got %q", got)
	}

	// ...and moving the due date must not drop the tags.
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.SetTaskDueDate(ctx, tx, id, day(t, "2026-04-06"), author)
	}); err != nil {
		t.Fatalf("SetTaskDueDate: %v", err)
	}
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		task, err := nodes.GetTask(ctx, tx, id)
		if err != nil {
			return err
		}
		slices.Sort(task.Tags)
		if !slices.Equal(task.Tags, []string{"backup", "homelab"}) {
			t.Errorf("moving the due date erased the tags: %v", task.Tags)
		}
		if task.Status != nodes.TaskStatusTodo {
			t.Errorf("status changed: %q", task.Status)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	// A status change goes through json_set, so it must leave the date alone too.
	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.SetTaskStatus(ctx, tx, id, nodes.TaskStatusDone, author)
	}); err != nil {
		t.Fatalf("SetTaskStatus: %v", err)
	}
	if got := readTaskDue(t, g, id); got != "2026-04-06" {
		t.Fatalf("completing the task erased the due date: got %q", got)
	}
}

func readTaskDue(t *testing.T, g *graph.Graph, id string) string {
	t.Helper()
	var due string
	err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		task, err := nodes.GetTask(context.Background(), tx, id)
		if err != nil {
			return err
		}
		due = nodes.FormatDueDate(task.DueDate)
		return nil
	})
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	return due
}
