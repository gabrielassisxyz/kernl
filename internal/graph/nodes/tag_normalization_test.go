package nodes_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/tagname"
	"github.com/gabrielassisxyz/kernl/internal/graph/tags"
)

// Note, task and project tags are written by the chokepoint, not by tags.Add —
// normalisation that only lived in tags.Add would never fire for them, and
// `Homelab` and `homelab` would become two subjects that never match.
func TestChokepointNormalizesTagsOnCreate(t *testing.T) {
	ctx := context.Background()
	g := openTestGraph(t)
	author := nodes.Author{Name: "test"}

	var taskID, noteID string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		taskID, err = nodes.CreateTask(ctx, tx, nodes.Task{
			Title: "Swap the NAS disks",
			Tags:  []string{"Homelab", " homelab ", "Homelab/NAS"},
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
		if got := sorted(task.Tags); !slices.Equal(got, []string{"homelab", "homelab/nas"}) {
			t.Errorf("task tags = %v, want [homelab homelab/nas]", got)
		}

		// The convergence that matters: the task and the note land on the same
		// tag row, so one query finds both.
		tagged, err := tags.Nodes(ctx, tx, "homelab")
		if err != nil {
			return err
		}
		if !slices.Contains(tagged, taskID) || !slices.Contains(tagged, noteID) {
			t.Errorf("tags.Nodes(homelab) = %v, want both the task and the note", tagged)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
}

// The update path diffs the incoming tags against the stored ones. Without
// normalising first, re-saving "Homelab" over a stored "homelab" would read as
// a removal plus an addition of the same tag.
func TestChokepointNormalizesTagsOnUpdate(t *testing.T) {
	ctx := context.Background()
	g := openTestGraph(t)
	author := nodes.Author{Name: "test"}

	var taskID string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		taskID, err = nodes.CreateTask(ctx, tx, nodes.Task{
			Title: "Swap the NAS disks",
			Tags:  []string{"homelab"},
		}, author)
		return err
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.SetTaskTags(ctx, tx, taskID, []string{"HOMELAB", "Hardware"}, author)
	})
	if err != nil {
		t.Fatalf("SetTaskTags: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		task, err := nodes.GetTask(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if got := sorted(task.Tags); !slices.Equal(got, []string{"hardware", "homelab"}) {
			t.Errorf("task tags = %v, want [hardware homelab]", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
}

func TestChokepointRejectsMalformedTag(t *testing.T) {
	ctx := context.Background()
	g := openTestGraph(t)
	author := nodes.Author{Name: "test"}

	for _, tag := range []string{"/foo", "foo/", "foo//bar"} {
		err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			_, err := nodes.CreateTask(ctx, tx, nodes.Task{Title: "t", Tags: []string{tag}}, author)
			return err
		})
		if !errors.Is(err, tagname.ErrInvalid) {
			t.Errorf("CreateTask with tag %q: error = %v, want ErrInvalid", tag, err)
		}
	}
}
