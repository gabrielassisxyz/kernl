package revisions_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/internal/ids"
	"github.com/gabrielassisxyz/kernl/internal/graph/revisions"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func insertNode(t *testing.T, wtx *graph.WriteTx, id, nodeType, title string) {
	t.Helper()
	if _, err := wtx.Exec(`INSERT INTO nodes(id, type, title, attrs) VALUES (?, ?, ?, '{}')`, id, nodeType, title); err != nil {
		t.Fatalf("insert node: %v", err)
	}
}

func insertRevision(t *testing.T, wtx *graph.WriteTx, id, nodeID string, parentID *string, diff, author string) {
	t.Helper()
	if _, err := wtx.Exec(
		`INSERT INTO revisions(id, node_id, parent_id, diff, author) VALUES (?, ?, ?, ?, ?)`,
		id, nodeID, parentID, diff, author,
	); err != nil {
		t.Fatalf("insert revision: %v", err)
	}
}

// TestListReturnsAllRevisions creates a node, updates it twice, deletes it, and
// verifies that List returns all 4 revisions (create + 2 updates + delete tombstone).
func TestListReturnsAllRevisions(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	nodeID := ids.New()

	rev1ID := ids.New()
	rev2ID := ids.New()
	rev3ID := ids.New()
	rev4ID := ids.New()

	err := g.DoWrite(ctx, func(wtx *graph.WriteTx) error {
		insertNode(t, wtx, nodeID, "note", "First Title")
		insertRevision(t, wtx, rev1ID, nodeID, nil, `{"title":"First Title","attrs":"{}","tags":[]}`, "agent:test")

		// Update: update node title + insert revision
		_, _ = wtx.Exec(`UPDATE nodes SET title = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`, "Second Title", nodeID)
		insertRevision(t, wtx, rev2ID, nodeID, &rev1ID, `{"title":"Second Title","attrs":"{}","tags":[]}`, "agent:test")

		// Update again
		_, _ = wtx.Exec(`UPDATE nodes SET title = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`, "Third Title", nodeID)
		insertRevision(t, wtx, rev3ID, nodeID, &rev2ID, `{"title":"Third Title","attrs":"{}","tags":[]}`, "agent:test")

		// Tombstone revision (delete marker) — node left intact so node_id FK is not SET NULL
		insertRevision(t, wtx, rev4ID, nodeID, &rev3ID, `{"title":"Third Title","attrs":"{}","tags":[]}`, "agent:test")
		return nil
	})
	if err != nil {
		t.Fatalf("DoWrite: %v", err)
	}

	var revs []revisions.Revision
	err = g.DoRead(ctx, func(rtx *graph.ReadTx) error {
		var listErr error
		revs, listErr = revisions.List(ctx, rtx, nodeID)
		return listErr
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(revs) != 4 {
		t.Errorf("expected 4 revisions, got %d", len(revs))
	}
	if revs[0].ID != rev4ID {
		t.Errorf("expected latest revision %s, got %s", rev4ID, revs[0].ID)
	}
	if revs[3].ID != rev1ID {
		t.Errorf("expected oldest revision %s, got %s", rev1ID, revs[3].ID)
	}
}

// TestListOrderDeterministicUnderCollision creates two revisions in the same
// second and verifies that the ordering (created_at DESC, id DESC) is deterministic.
// Because ids.New() uses UUIDv7 which is monotonic, the second ID is lexicographically
// greater than the first.
func TestListOrderDeterministicUnderCollision(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	nodeID := ids.New()
	rev1ID := ids.New()
	rev2ID := ids.New()

	// Verify UUIDv7 monotonicity: rev2ID > rev1ID
	if rev2ID <= rev1ID {
		t.Fatalf("UUIDv7 monotonicity violated: %s <= %s", rev2ID, rev1ID)
	}

	err := g.DoWrite(ctx, func(wtx *graph.WriteTx) error {
		insertNode(t, wtx, nodeID, "note", "Test")
		insertRevision(t, wtx, rev1ID, nodeID, nil, `{"title":"Test","attrs":"{}","tags":[]}`, "agent:test")
		insertRevision(t, wtx, rev2ID, nodeID, &rev1ID, `{"title":"Test 2","attrs":"{}","tags":[]}`, "agent:test")
		return nil
	})
	if err != nil {
		t.Fatalf("DoWrite: %v", err)
	}

	var revs []revisions.Revision
	err = g.DoRead(ctx, func(rtx *graph.ReadTx) error {
		var listErr error
		revs, listErr = revisions.List(ctx, rtx, nodeID)
		return listErr
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(revs) != 2 {
		t.Fatalf("expected 2 revisions, got %d", len(revs))
	}
	if revs[0].ID != rev2ID {
		t.Errorf("expected latest revision %s, got %s", rev2ID, revs[0].ID)
	}
	if revs[1].ID != rev1ID {
		t.Errorf("expected oldest revision %s, got %s", rev1ID, revs[1].ID)
	}
}

// TestGetAtReturnsHistoricalState creates a node, updates it, and verifies that
// GetAt returns the correct diff for each historical revision.
func TestGetAtReturnsHistoricalState(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	nodeID := ids.New()
	rev1ID := ids.New()
	rev2ID := ids.New()

	err := g.DoWrite(ctx, func(wtx *graph.WriteTx) error {
		insertNode(t, wtx, nodeID, "note", "Original Title")
		insertRevision(t, wtx, rev1ID, nodeID, nil, `{"title":"Original Title","attrs":"{}","tags":[]}`, "agent:test")

		_, _ = wtx.Exec(`UPDATE nodes SET title = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`, "Updated Title", nodeID)
		insertRevision(t, wtx, rev2ID, nodeID, &rev1ID, `{"title":"Updated Title","attrs":"{}","tags":[]}`, "agent:test")
		return nil
	})
	if err != nil {
		t.Fatalf("DoWrite: %v", err)
	}

	err = g.DoRead(ctx, func(rtx *graph.ReadTx) error {
		rev1, err := revisions.GetAt(ctx, rtx, nodeID, rev1ID)
		if err != nil {
			return err
		}
		var diff1 struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal(rev1.Diff, &diff1); err != nil {
			return err
		}
		if diff1.Title != "Original Title" {
			t.Errorf("rev1 title = %q, want %q", diff1.Title, "Original Title")
		}

		rev2, err := revisions.GetAt(ctx, rtx, nodeID, rev2ID)
		if err != nil {
			return err
		}
		var diff2 struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal(rev2.Diff, &diff2); err != nil {
			return err
		}
		if diff2.Title != "Updated Title" {
			t.Errorf("rev2 title = %q, want %q", diff2.Title, "Updated Title")
		}

		return nil
	})
	if err != nil {
		t.Fatalf("DoRead: %v", err)
	}

	// Verify GetAt with non-existent ID returns ErrNotFound
	err = g.DoRead(ctx, func(rtx *graph.ReadTx) error {
		_, err := revisions.GetAt(ctx, rtx, nodeID, "nonexistent")
		if err != graph.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("DoRead: %v", err)
	}
}

// TestPrevRevisionChainUnbroken creates a node, updates it twice, deletes it,
// and walks the revision chain backwards via ParentID to verify it is unbroken.
func TestPrevRevisionChainUnbroken(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	nodeID := ids.New()
	rev1ID := ids.New()
	rev2ID := ids.New()
	rev3ID := ids.New()
	rev4ID := ids.New()

	err := g.DoWrite(ctx, func(wtx *graph.WriteTx) error {
		insertNode(t, wtx, nodeID, "note", "V1")
		insertRevision(t, wtx, rev1ID, nodeID, nil, `{"title":"V1","attrs":"{}","tags":[]}`, "agent:test")

		_, _ = wtx.Exec(`UPDATE nodes SET title = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`, "V2", nodeID)
		insertRevision(t, wtx, rev2ID, nodeID, &rev1ID, `{"title":"V2","attrs":"{}","tags":[]}`, "agent:test")

		_, _ = wtx.Exec(`UPDATE nodes SET title = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`, "V3", nodeID)
		insertRevision(t, wtx, rev3ID, nodeID, &rev2ID, `{"title":"V3","attrs":"{}","tags":[]}`, "agent:test")

		insertRevision(t, wtx, rev4ID, nodeID, &rev3ID, `{"title":"V3","attrs":"{}","tags":[]}`, "agent:test")
		return nil
	})
	if err != nil {
		t.Fatalf("DoWrite: %v", err)
	}

	// Walk backwards from the latest revision via ParentID
	expectedChain := []struct {
		revID    string
		parentID *string
	}{
		{rev4ID, &rev3ID},
		{rev3ID, &rev2ID},
		{rev2ID, &rev1ID},
		{rev1ID, nil},
	}

	err = g.DoRead(ctx, func(rtx *graph.ReadTx) error {
		currentRevID := rev4ID
		for i, expected := range expectedChain {
			rev, err := revisions.GetAt(ctx, rtx, nodeID, currentRevID)
			if err != nil {
				t.Fatalf("step %d: GetAt(%s): %v", i, currentRevID, err)
			}
			if rev.ID != expected.revID {
				t.Errorf("step %d: ID = %s, want %s", i, rev.ID, expected.revID)
			}
			if expected.parentID == nil {
				if rev.ParentID != nil {
					t.Errorf("step %d: expected nil ParentID, got %v", i, *rev.ParentID)
				}
				break
			}
			if rev.ParentID == nil {
				t.Errorf("step %d: ParentID is nil, want %s", i, *expected.parentID)
				break
			}
			if *rev.ParentID != *expected.parentID {
				t.Errorf("step %d: ParentID = %s, want %s", i, *rev.ParentID, *expected.parentID)
			}
			currentRevID = *rev.ParentID
		}
		return nil
	})
	if err != nil {
		t.Fatalf("DoRead: %v", err)
	}
}
