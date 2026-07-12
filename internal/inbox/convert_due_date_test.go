package inbox_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/inbox"
)

// The end of the chain: the due date the human confirmed in the modal has to
// reach the task node — and only the task node.
func TestProcessCaptureCarriesDueDateOntoTheTask(t *testing.T) {
	ctx := context.Background()
	vaultRoot := t.TempDir()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("graph.Open: %v", err)
	}
	defer g.Close()

	var captureID string
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		captureID, err = nodes.CreateCapture(ctx, tx, nodes.Capture{
			Body:           "amanhã: juntar os recursos do substack",
			Tags:           []string{"pending"},
			BatchTimestamp: "4/1/26 21:08",
		}, nodes.Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}

	due, err := nodes.ParseDueDate("2026-04-02")
	if err != nil {
		t.Fatal(err)
	}
	// A note alongside it, carrying a due date it has no business keeping.
	err = inbox.ProcessCapture(ctx, g, vaultRoot, nil, captureID, inbox.ProcessRequest{
		Actions: []inbox.Action{
			{Target: "task", Title: "Gather the substack resources", DueDate: due},
			{Target: "task", Title: "No deadline was stated for this one"},
			{Target: "note", Title: "A stray date on a note", DueDate: due},
		},
	})
	if err != nil {
		t.Fatalf("ProcessCapture: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		tasks, err := nodes.ListTasks(ctx, tx, "")
		if err != nil {
			return err
		}
		if len(tasks) != 2 {
			t.Fatalf("expected 2 tasks, got %d", len(tasks))
		}
		byTitle := map[string]*nodes.Task{}
		for _, task := range tasks {
			byTitle[task.Title] = task
		}
		withDue, ok := byTitle["Gather the substack resources"]
		if !ok {
			t.Fatalf("task not created: %v", byTitle)
		}
		if got := nodes.FormatDueDate(withDue.DueDate); got != "2026-04-02" {
			t.Errorf("task due date = %q, want 2026-04-02", got)
		}
		if withDue.DueDate.Year() != 2026 || withDue.DueDate.Month() != time.April {
			t.Errorf("due date is not in April 2026: %v", withDue.DueDate)
		}
		if none := byTitle["No deadline was stated for this one"]; none == nil || none.DueDate != nil {
			t.Errorf("a task with no proposed deadline must have none: %v", none)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
}
