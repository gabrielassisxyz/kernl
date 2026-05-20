package nodes

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// TestDecisionRoundtrip verifies CreateDecision → GetDecision returns identical fields.
func TestDecisionRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	decidedAt := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	d := Decision{
		Title:     "Use Rust",
		Body:      "We decided to use Rust for the core engine.",
		Context:   "Need a fast, safe language for graph processing.",
		Outcome:   "Adopt Rust with the substrate pattern.",
		DecidedAt: decidedAt,
		Tags:      []string{"tech", "architecture"},
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateDecision(ctx, tx, d, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	var got *Decision
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		got, err = GetDecision(ctx, tx, id)
		return err
	})
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}

	if got.ID != id {
		t.Errorf("id = %q, want %q", got.ID, id)
	}
	if got.Title != d.Title {
		t.Errorf("title = %q, want %q", got.Title, d.Title)
	}
	if got.Body != d.Body {
		t.Errorf("body = %q, want %q", got.Body, d.Body)
	}
	if got.Context != d.Context {
		t.Errorf("context = %q, want %q", got.Context, d.Context)
	}
	if got.Outcome != d.Outcome {
		t.Errorf("outcome = %q, want %q", got.Outcome, d.Outcome)
	}
	if len(got.Tags) != len(d.Tags) {
		t.Errorf("tags len = %d, want %d", len(got.Tags), len(d.Tags))
	}
}

// TestDecisionUpdateProducesOneRevision verifies updating writes a second revision.
func TestDecisionUpdateProducesOneRevision(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	d := Decision{
		Title:     "Original",
		Body:      "before",
		Context:   "ctx",
		Outcome:   "out",
		DecidedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Tags:      []string{"alpha"},
	}

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateDecision(ctx, tx, d, Author{Name: "creator"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}

	updated := Decision{
		ID:        id,
		Title:     "Updated",
		Body:      "after",
		Context:   "ctx",
		Outcome:   "out",
		DecidedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Tags:      []string{"beta"},
	}
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateDecision(ctx, tx, updated, Author{Name: "updater"})
	})
	if err != nil {
		t.Fatalf("UpdateDecision: %v", err)
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

// TestDecisionDeletePreservesRevisions verifies 3 revision rows survive after C+U+D.
func TestDecisionDeletePreservesRevisions(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = CreateDecision(ctx, tx, Decision{
			Title: "Del", Body: "d", Context: "c", Outcome: "o",
			DecidedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		}, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return UpdateDecision(ctx, tx, Decision{ID: id, Title: "Del2"}, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("UpdateDecision: %v", err)
	}

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return DeleteDecision(ctx, tx, id, Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("DeleteDecision: %v", err)
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

// TestDecisionFTSRoundtrip verifies Body, Context, and Outcome are all indexed by FTS.
func TestDecisionFTSRoundtrip(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	d := Decision{
		Title:     "FTS Decision",
		Body:      "tokendecbody123 contains body token.",
		Context:   "tokendecctx456 in context.",
		Outcome:   "tokendecout789 in outcome.",
		DecidedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		_, err := CreateDecision(ctx, tx, d, Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		for _, token := range []string{"tokendecbody123", "tokendecctx456", "tokendecout789"} {
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

// TestDecisionListFilter verifies filtering by Since returns only recent decisions.
func TestDecisionListFilter(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	early := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mid := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if _, err := CreateDecision(ctx, tx, Decision{Title: "Early", Body: "e", Context: "e", Outcome: "e", DecidedAt: early}, Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := CreateDecision(ctx, tx, Decision{Title: "Mid", Body: "m", Context: "m", Outcome: "m", DecidedAt: mid}, Author{Name: "test"}); err != nil {
			return err
		}
		if _, err := CreateDecision(ctx, tx, Decision{Title: "Late", Body: "l", Context: "l", Outcome: "l", DecidedAt: late}, Author{Name: "test"}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("CreateDecision: %v", err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		since := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		items, err := ListDecisions(ctx, tx, DecisionFilter{Since: &since})
		if err != nil {
			return err
		}
		if len(items) != 2 {
			t.Errorf("Since mid-2024: expected 2, got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		since := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		items, err := ListDecisions(ctx, tx, DecisionFilter{Since: &since})
		if err != nil {
			return err
		}
		if len(items) != 1 {
			t.Errorf("Since 2025: expected 1, got %d", len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
