package inbox_test

import (
	"context"
	"os"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/inbox"
)

// seedCapture creates one pending capture and returns its id.
func seedCapture(t *testing.T, g *graph.Graph, body string) string {
	t.Helper()
	var id string
	if err := g.DoWrite(context.Background(), func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateCapture(context.Background(), tx, nodes.Capture{
			Body: body,
			Tags: []string{"pending"},
		}, nodes.Author{Name: "test"})
		return err
	}); err != nil {
		t.Fatalf("CreateCapture: %v", err)
	}
	return id
}

// derivedOf returns the id→type map of the nodes derived from a capture.
func derivedOf(t *testing.T, g *graph.Graph, captureID string) map[string]string {
	t.Helper()
	out := map[string]string{}
	if err := g.DoRead(context.Background(), func(tx *graph.ReadTx) error {
		in, err := edges.Incoming(context.Background(), tx, captureID)
		if err != nil {
			return err
		}
		for _, e := range in {
			if e.Label != "derived_from" {
				continue
			}
			var typ string
			if err := tx.QueryRow(`SELECT type FROM nodes WHERE id = ? AND deleted_at IS NULL`, e.Src).Scan(&typ); err != nil {
				continue // node gone
			}
			out[e.Src] = typ
		}
		return nil
	}); err != nil {
		t.Fatalf("derivedOf: %v", err)
	}
	return out
}

// The structural change: one capture becomes several nodes. A reflection that
// also implies a next step is a note AND a task — both derived from the capture,
// and related to each other.
func TestProcessCaptureFansOutIntoSeveralNodes(t *testing.T) {
	ctx := context.Background()
	g := openInboxGraph(t)
	vault := t.TempDir()

	captureID := seedCapture(t, g, "1h por dia. Preciso de uma forma concreta de visualizar o progresso.")

	if err := inbox.ProcessCapture(ctx, g, vault, nil, captureID, inbox.ProcessRequest{
		Actions: []inbox.Action{
			{Target: "note", Title: "One hour a day compounds", Body: "An hour a day is the whole game."},
			{Target: "task", Title: "Find a concrete way to visualize progress"},
		},
	}); err != nil {
		t.Fatalf("ProcessCapture: %v", err)
	}

	derived := derivedOf(t, g, captureID)
	if len(derived) != 2 {
		t.Fatalf("expected 2 derived nodes, got %d: %v", len(derived), derived)
	}
	types := map[string]bool{}
	for _, typ := range derived {
		types[typ] = true
	}
	if !types["note"] || !types["task"] {
		t.Errorf("expected a note and a task, got %v", derived)
	}

	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		// The fanned-out nodes are related to each other: they are one thought.
		var related bool
		for id := range derived {
			out, err := edges.Outgoing(ctx, tx, id, edges.WithType(edges.EdgeTypeRelated))
			if err != nil {
				return err
			}
			for _, e := range out {
				if _, ok := derived[e.Dst]; ok {
					related = true
				}
			}
		}
		if !related {
			t.Errorf("expected a related edge between the nodes fanned out of one capture")
		}

		// Each action kept its own title and body — not the capture's.
		tasks, err := nodes.ListTasks(ctx, tx, "")
		if err != nil {
			return err
		}
		if len(tasks) != 1 || tasks[0].Title != "Find a concrete way to visualize progress" {
			t.Errorf("task title = %#v, want the action's own title", tasks)
		}

		c, err := nodes.GetCapture(ctx, tx, captureID)
		if err != nil {
			return err
		}
		if !hasTagT(c.Tags, "triaged") || hasTagT(c.Tags, "pending") {
			t.Errorf("capture tags = %v, want triaged", c.Tags)
		}
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
	}
}

// A "tomorrow:" message is four tasks, not one task titled "Tomorrow:".
func TestProcessCaptureFansOutIntoFourTasks(t *testing.T) {
	ctx := context.Background()
	g := openInboxGraph(t)

	captureID := seedCapture(t, g, "amanhã: substack resources, plainenglish PDFs, to-read backlog, substack next steps")

	if err := inbox.ProcessCapture(ctx, g, t.TempDir(), nil, captureID, inbox.ProcessRequest{
		Actions: []inbox.Action{
			{Target: "task", Title: "Collect Substack resources"},
			{Target: "task", Title: "Download the plainenglish PDFs"},
			{Target: "task", Title: "Triage the to-read backlog"},
			{Target: "task", Title: "Decide the Substack next steps"},
		},
	}); err != nil {
		t.Fatalf("ProcessCapture: %v", err)
	}

	if got := len(derivedOf(t, g, captureID)); got != 4 {
		t.Fatalf("expected 4 derived tasks, got %d", got)
	}
}

// The fan-out is one write transaction: a bad action anywhere leaves the graph
// untouched and the capture pending, rather than half-processing it.
func TestProcessCaptureFanOutIsAllOrNothing(t *testing.T) {
	ctx := context.Background()
	g := openInboxGraph(t)
	vault := t.TempDir()

	captureID := seedCapture(t, g, "a thought that implies a task")

	err := inbox.ProcessCapture(ctx, g, vault, nil, captureID, inbox.ProcessRequest{
		Actions: []inbox.Action{
			{Target: "note", Title: "The thought"},
			{Target: "sandwich", Title: "Not a node kind"},
		},
	})
	if err == nil {
		t.Fatal("expected an unknown target to fail the whole request")
	}

	if got := len(derivedOf(t, g, captureID)); got != 0 {
		t.Errorf("expected no nodes created on a failed fan-out, got %d", got)
	}
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		c, err := nodes.GetCapture(ctx, tx, captureID)
		if err != nil {
			return err
		}
		if !hasTagT(c.Tags, "pending") {
			t.Errorf("capture tags = %v, want still pending", c.Tags)
		}
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
	}
	// Nothing is written to the vault either: the action list is validated
	// before the transaction opens, so a bad leg costs no side effects.
	if files, _ := os.ReadDir(vault); len(files) != 0 {
		t.Errorf("expected no vault file on a failed fan-out, got %d", len(files))
	}
}

// Undo takes back everything the capture became — all four nodes, not the first.
func TestReopenRemovesEveryDerivedNode(t *testing.T) {
	ctx := context.Background()
	g := openInboxGraph(t)
	vault := t.TempDir()

	captureID := seedCapture(t, g, "a project, a note, a bookmark and a task walk into a capture")

	if err := inbox.ProcessCapture(ctx, g, vault, nil, captureID, inbox.ProcessRequest{
		Actions: []inbox.Action{
			{Target: "note", Title: "The reflection"},
			{Target: "task", Title: "The next step"},
			{Target: "bookmark", Title: "The link", Body: "https://example.com/read-me"},
			{Target: "project", ProjectTitle: "The effort", InitialTasks: []string{"First slice"}},
		},
	}); err != nil {
		t.Fatalf("ProcessCapture: %v", err)
	}

	// note + task + bookmark + project + the project's initial task.
	if got := len(derivedOf(t, g, captureID)); got != 5 {
		t.Fatalf("expected 5 derived nodes, got %d", got)
	}

	if err := inbox.Reopen(ctx, g, vault, captureID); err != nil {
		t.Fatalf("Reopen: %v", err)
	}

	if got := len(derivedOf(t, g, captureID)); got != 0 {
		t.Errorf("undo left %d derived nodes behind, want 0", got)
	}
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		tasks, err := nodes.ListTasks(ctx, tx, "")
		if err != nil {
			return err
		}
		if len(tasks) != 0 {
			t.Errorf("undo left %d tasks behind (the project's initial tasks are derived too)", len(tasks))
		}
		projects, err := nodes.ListProjects(ctx, tx)
		if err != nil {
			return err
		}
		if len(projects) != 0 {
			t.Errorf("undo left %d projects behind", len(projects))
		}
		c, err := nodes.GetCapture(ctx, tx, captureID)
		if err != nil {
			return err
		}
		if !hasTagT(c.Tags, "pending") {
			t.Errorf("capture tags = %v, want pending after undo", c.Tags)
		}
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
	}
	if files, _ := os.ReadDir(vault); len(files) != 0 {
		t.Errorf("undo left %d vault files behind", len(files))
	}
}

// A discard among several actions means "this fragment is noise" — the capture
// is still triaged, because it did produce a node.
func TestProcessCaptureDiscardsOneFragmentNotTheCapture(t *testing.T) {
	ctx := context.Background()
	g := openInboxGraph(t)

	captureID := seedCapture(t, g, "one good idea and one line of filler")

	if err := inbox.ProcessCapture(ctx, g, t.TempDir(), nil, captureID, inbox.ProcessRequest{
		Actions: []inbox.Action{
			{Target: "task", Title: "The good idea"},
			{Target: "discard", Title: "The filler"},
		},
	}); err != nil {
		t.Fatalf("ProcessCapture: %v", err)
	}

	if got := len(derivedOf(t, g, captureID)); got != 1 {
		t.Errorf("expected 1 derived node, got %d", got)
	}
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		c, err := nodes.GetCapture(ctx, tx, captureID)
		if err != nil {
			return err
		}
		if !hasTagT(c.Tags, "triaged") || hasTagT(c.Tags, "discarded") {
			t.Errorf("capture tags = %v, want triaged (a discarded fragment does not discard the capture)", c.Tags)
		}
		return nil
	}); err != nil {
		t.Fatalf("DoRead: %v", err)
	}
}

// An update is reviewed hunk by hunk against one note, so it cannot be one leg
// of a fan-out — the request is rejected rather than silently dropping content.
func TestProcessCaptureRejectsUpdateAlongsideOtherActions(t *testing.T) {
	ctx := context.Background()
	g := openInboxGraph(t)

	captureID := seedCapture(t, g, "extends a note and also implies a task")

	err := inbox.ProcessCapture(ctx, g, t.TempDir(), nil, captureID, inbox.ProcessRequest{
		Actions: []inbox.Action{
			{Target: "update"},
			{Target: "task", Title: "The next step"},
		},
	})
	if err == nil {
		t.Fatal("expected an update combined with another action to be rejected")
	}
	if got := len(derivedOf(t, g, captureID)); got != 0 {
		t.Errorf("expected nothing created, got %d nodes", got)
	}
}
