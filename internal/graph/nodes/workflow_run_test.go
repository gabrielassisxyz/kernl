package nodes

import (
	"context"
	"fmt"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// TestWorkflowRunRoundtrip verifies CreateWorkflowRun → GetWorkflowRun returns identical fields.
func TestWorkflowRunRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	wr := WorkflowRun{
		Title:        "Run 1",
		WorkflowName: "data-pipeline",
		Status:       "running",
		RunData:      `{"step": "ingest", "rows": 1000}`,
		Tags:         []string{"pipeline", "data"},
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateWorkflowRun(ctx, tx, wr, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	var got *WorkflowRun
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = GetWorkflowRun(ctx, tx, id)
		return err
	})
	if err != nil {
		t.Fatalf("GetWorkflowRun: %v", err)
	}

	if got.ID != id {
		t.Errorf("id = %q, want %q", got.ID, id)
	}
	if got.Title != wr.Title {
		t.Errorf("title = %q, want %q", got.Title, wr.Title)
	}
	if got.WorkflowName != wr.WorkflowName {
		t.Errorf("workflow_name = %q, want %q", got.WorkflowName, wr.WorkflowName)
	}
	if got.Status != wr.Status {
		t.Errorf("status = %q, want %q", got.Status, wr.Status)
	}
	if got.RunData != wr.RunData {
		t.Errorf("run_data = %q, want %q", got.RunData, wr.RunData)
	}
	if len(got.Tags) != len(wr.Tags) {
		t.Errorf("tags len = %d, want %d", len(got.Tags), len(wr.Tags))
	}
}

// TestWorkflowRunUpdateProducesOneRevision verifies updating writes a second revision.
func TestWorkflowRunUpdateProducesOneRevision(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	wr := WorkflowRun{
		Title:        "Original",
		WorkflowName: "wf-1",
		Status:       "running",
		RunData:      `{}`,
		Tags:         []string{"alpha"},
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateWorkflowRun(ctx, tx, wr, Author{Name: "creator"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}

	updated := WorkflowRun{
		ID:           id,
		Title:        "Updated",
		WorkflowName: "wf-1",
		Status:       "done",
		RunData:      `{}`,
		Tags:         []string{"beta"},
	}
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateWorkflowRun(ctx, tx, updated, Author{Name: "updater"})
	})
	if err != nil {
		t.Fatalf("UpdateWorkflowRun: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		if err := tx.QueryRow("SELECT COUNT(*) FROM revisions", id).Scan(&count); err != nil {
			return err
		}
		if count != 2 {
			t.Errorf("expected 2 revisions after update, got %d", count)
		}

		var author string
		if err := tx.QueryRow(
			"SELECT author FROM revisions WHERE node_id = ? ORDER BY created_at DESC, id DESC LIMIT 1",
			id,
		).Scan(&author); err != nil {
			return err
		}
		if author != "updater" {
			t.Errorf("latest author = %q, want %q", author, "updater")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestWorkflowRunDeletePreservesRevisions verifies 3 revision rows survive after C+U+D.
func TestWorkflowRunDeletePreservesRevisions(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateWorkflowRun(ctx, tx, WorkflowRun{
			Title: "Del", WorkflowName: "wf-d", Status: "running", RunData: `{}`,
		}, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateWorkflowRun(ctx, tx, WorkflowRun{ID: id, Title: "Del2"}, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("UpdateWorkflowRun: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return DeleteWorkflowRun(ctx, tx, id, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("DeleteWorkflowRun: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var revCount int
		if err := tx.QueryRow("SELECT COUNT(*) FROM revisions", id).Scan(&revCount); err != nil {
			return err
		}
		if revCount != 3 {
			return fmt.Errorf("expected 3 revisions, got %d", revCount)
		}
		var nodeCount int
		if err := tx.QueryRow("SELECT COUNT(*) FROM nodes WHERE id = ?", id).Scan(&nodeCount); err != nil {
			return err
		}
		if nodeCount != 0 {
			return fmt.Errorf("expected node deleted, got %d", nodeCount)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestWorkflowRunFTSRoundtrip verifies WorkflowName and Status are indexed by FTS.
func TestWorkflowRunFTSRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	wr := WorkflowRun{
		Title:        "FTS Searchable",
		WorkflowName: "wrftsuniquewfname",
		Status:       "running",
		RunData:      `{}`,
	}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := CreateWorkflowRun(ctx, tx, wr, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var count int
		if err := tx.QueryRow(
			"SELECT COUNT(*) FROM nodes_fts WHERE nodes_fts MATCH 'wrftsuniquewfname'",
		).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("expected FTS to find 'wrftsuniquewfname' once, got %d", count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestWorkflowRunListFilter verifies filtering by WorkflowName and Status.
func TestWorkflowRunListFilter(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if _, err := CreateWorkflowRun(ctx, tx, WorkflowRun{
			Title: "WR1", WorkflowName: "wf-1", Status: "running", RunData: `{}`,
		}, Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := CreateWorkflowRun(ctx, tx, WorkflowRun{
			Title: "WR2", WorkflowName: "wf-1", Status: "running", RunData: `{}`,
		}, Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := CreateWorkflowRun(ctx, tx, WorkflowRun{
			Title: "WR3", WorkflowName: "wf-1", Status: "done", RunData: `{}`,
		}, Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := CreateWorkflowRun(ctx, tx, WorkflowRun{
			Title: "WR4", WorkflowName: "wf-2", Status: "running", RunData: `{}`,
		}, Author{Name: "test"}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		items, err := ListWorkflowRuns(ctx, tx, WorkflowRunFilter{WorkflowName: "wf-1"})
		if err != nil {
			return err
		}
		if len(items) != 3 {
			t.Errorf("WorkflowName='wf-1': expected 3, got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		items, err := ListWorkflowRuns(ctx, tx, WorkflowRunFilter{Status: "running"})
		if err != nil {
			return err
		}
		if len(items) != 3 {
			t.Errorf("Status='running': expected 3, got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
