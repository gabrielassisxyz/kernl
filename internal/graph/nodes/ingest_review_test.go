package nodes

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// TestIngestReviewRoundtrip verifies CreateIngestReview → GetIngestReview returns identical fields.
func TestIngestReviewRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	ir := IngestReview{
		Title:        "Review Action",
		SourceNodeID: "node_123",
		Action:       "Create Page",
		Payload:      "payload_data",
		ContentHash:  "hash123",
		Tags:         []string{"ingest", "review"},
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateIngestReview(ctx, tx, ir, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateIngestReview: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	var got *IngestReview
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = GetIngestReview(ctx, tx, id)
		return err
	})
	if err != nil {
		t.Fatalf("GetIngestReview: %v", err)
	}

	if got.ID != id {
		t.Errorf("id = %q, want %q", got.ID, id)
	}
	if got.Title != ir.Title {
		t.Errorf("title = %q, want %q", got.Title, ir.Title)
	}
	if got.SourceNodeID != ir.SourceNodeID {
		t.Errorf("source_node_id = %q, want %q", got.SourceNodeID, ir.SourceNodeID)
	}
	if got.Action != ir.Action {
		t.Errorf("action = %q, want %q", got.Action, ir.Action)
	}
	if got.Payload != ir.Payload {
		t.Errorf("payload = %q, want %q", got.Payload, ir.Payload)
	}
	if got.ContentHash != ir.ContentHash {
		t.Errorf("content_hash = %q, want %q", got.ContentHash, ir.ContentHash)
	}
	if len(got.Tags) != len(ir.Tags) {
		t.Errorf("tags len = %d, want %d", len(got.Tags), len(ir.Tags))
	}
}

// TestIngestReviewUpdateProducesOneRevision verifies updating writes a second revision.
func TestIngestReviewUpdateProducesOneRevision(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	ir := IngestReview{
		Title:        "Original",
		SourceNodeID: "src_before",
		Action:       "Skip",
		Payload:      "pay_before",
		ContentHash:  "hash_before",
		Tags:         []string{"alpha"},
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateIngestReview(ctx, tx, ir, Author{Name: "creator"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateIngestReview: %v", err)
	}

	updated := IngestReview{
		ID:           id,
		Title:        "Updated",
		SourceNodeID: "src_after",
		Action:       "Create Page",
		Payload:      "pay_after",
		ContentHash:  "hash_after",
		Tags:         []string{"beta"},
	}
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateIngestReview(ctx, tx, updated, Author{Name: "updater"})
	})
	if err != nil {
		t.Fatalf("UpdateIngestReview: %v", err)
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

// TestIngestReviewDeletePreservesRevisions verifies 3 revision rows survive after C+U+D.
func TestIngestReviewDeletePreservesRevisions(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateIngestReview(ctx, tx, IngestReview{
			Title: "Del", SourceNodeID: "s", Action: "a", Payload: "p", ContentHash: "c",
		}, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateIngestReview: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateIngestReview(ctx, tx, IngestReview{ID: id, Title: "Del2"}, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("UpdateIngestReview: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return DeleteIngestReview(ctx, tx, id, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("DeleteIngestReview: %v", err)
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

// TestIngestReviewFTSRoundtrip verifies Action, Payload, and ContentHash are all indexed by FTS.
func TestIngestReviewFTSRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	ir := IngestReview{
		Title:        "FTS Review",
		SourceNodeID: "src123",
		Action:       "tokenaction123",
		Payload:      "tokenpayload456",
		ContentHash:  "tokenhash789",
	}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := CreateIngestReview(ctx, tx, ir, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateIngestReview: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		for _, token := range []string{"tokenaction123", "tokenpayload456", "tokenhash789"} {
			var count int
			if err := tx.QueryRow(
				"SELECT COUNT(*) FROM nodes_fts WHERE nodes_fts MATCH ?",
				token,
			).Scan(&count); err != nil {
				return err
			}
			if count != 1 {
				return fmt.Errorf("expected FTS to find %q once, got %d", token, count)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestIngestReviewListFilter verifies filtering by Tags returns only matching reviews.
func TestIngestReviewListFilter(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if _, err := CreateIngestReview(ctx, tx, IngestReview{Title: "One", Tags: []string{"a"}}, Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := CreateIngestReview(ctx, tx, IngestReview{Title: "Two", Tags: []string{"a", "b"}}, Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := CreateIngestReview(ctx, tx, IngestReview{Title: "Three", Tags: []string{"c"}}, Author{Name: "test"}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("CreateIngestReview: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		items, err := ListIngestReviews(ctx, tx, IngestReviewFilter{Tags: []string{"a"}})
		if err != nil {
			return err
		}
		if len(items) != 2 {
			t.Errorf("Tag a: expected 2, got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestIngestReviewMissingPayload verifies missing payload is handled gracefully.
func TestIngestReviewMissingPayload(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	// Direct insert to simulate missing payload in attrs
	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		stmt := `INSERT INTO nodes (id, type, title, attrs, created_at, updated_at)
			VALUES (?, 'ingest_review', 'No Payload', '{"source_node_id":"abc", "action":"Skip"}', ?, ?)`
		now := time.Now().UTC().Format(time.RFC3339)
		id = "ir_test_" + fmt.Sprint(time.Now().UnixNano())
		_, err := tx.Exec(stmt, id, now, now)
		return err
	})
	if err != nil {
		t.Fatalf("Raw insert: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		got, err := GetIngestReview(ctx, tx, id)
		if err != nil {
			return err
		}
		if got.Payload != "" {
			t.Errorf("expected empty payload, got %q", got.Payload)
		}
		if got.SourceNodeID != "abc" {
			t.Errorf("expected source_node_id 'abc', got %q", got.SourceNodeID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
