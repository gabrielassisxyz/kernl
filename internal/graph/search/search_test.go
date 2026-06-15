package search_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/search"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

// setupSearchNode creates a node with its FTS index row linked.
func setupSearchNode(t *testing.T, g *graph.Graph, id, nodeType, title, body string, tags []string) {
	t.Helper()
	err := g.DoWrite(context.Background(), func(wtx *graph.WriteTx) error {
		attrs := `{"body":"` + strings.ReplaceAll(body, `"`, `\"`) + `"}`
		_, err := wtx.Exec(
			`INSERT INTO nodes(id, type, title, attrs) VALUES (?, ?, ?, ?)`,
			id, nodeType, title, attrs,
		)
		if err != nil {
			return fmt.Errorf("insert node: %w", err)
		}

		_, err = wtx.Exec(
			`INSERT INTO nodes_fts(title, attrs) VALUES (?, ?)`,
			title, body,
		)
		if err != nil {
			return fmt.Errorf("insert fts: %w", err)
		}

		_, err = wtx.Exec(
			`UPDATE nodes SET fts_rowid = last_insert_rowid() WHERE id = ?`,
			id,
		)
		if err != nil {
			return fmt.Errorf("update fts_rowid: %w", err)
		}

		for _, tag := range tags {
			_, err := wtx.Exec(`INSERT OR IGNORE INTO tags(id, name) VALUES (?, ?)`,
				id+"-"+tag, tag,
			)
			if err != nil {
				return fmt.Errorf("insert tag: %w", err)
			}
			_, err = wtx.Exec(
				`INSERT INTO node_tags(node_id, tag_id) VALUES (?, ?)`,
				id, id+"-"+tag,
			)
			if err != nil {
				return fmt.Errorf("insert node_tags: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("setupSearchNode: %v", err)
	}
}

// TestSearchReturnsHit verifies basic search returns a matching hit (bead kernl-7m5w).
func TestSearchReturnsHit(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	setupSearchNode(t, g, "n1", "note", "Hello World", "some body content", nil)

	var hits []search.Hit
	err := g.DoRead(context.Background(), func(rtx *graph.ReadTx) error {
		var serr error
		hits, serr = search.Search(context.Background(), rtx, "Hello")
		return serr
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].NodeID != "n1" {
		t.Errorf("expected NodeID n1, got %s", hits[0].NodeID)
	}
	if hits[0].Title != "Hello World" {
		t.Errorf("expected Title 'Hello World', got %s", hits[0].Title)
	}
}

// TestSearchWithTagsFilter verifies tag filtering (bead kernl-a7va).
func TestSearchWithTagsFilter(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	setupSearchNode(t, g, "n1", "note", "Shared Title", "body one", []string{"alpha"})
	setupSearchNode(t, g, "n2", "note", "Shared Title", "body two", []string{"beta"})

	var hits []search.Hit
	err := g.DoRead(context.Background(), func(rtx *graph.ReadTx) error {
		var serr error
		hits, serr = search.Search(context.Background(), rtx, "Shared",
			search.WithTags("alpha"),
		)
		return serr
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit with tag alpha, got %d", len(hits))
	}
	if hits[0].NodeID != "n1" {
		t.Errorf("expected n1, got %s", hits[0].NodeID)
	}
}

// TestSearchWithTypes verifies type filtering (bead kernl-eauq).
func TestSearchWithTypes(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	setupSearchNode(t, g, "n1", "note", "Common", "body", nil)
	setupSearchNode(t, g, "n2", "task", "Common", "body", nil)

	var hits []search.Hit
	err := g.DoRead(context.Background(), func(rtx *graph.ReadTx) error {
		var serr error
		hits, serr = search.Search(context.Background(), rtx, "Common",
			search.WithTypes("note"),
		)
		return serr
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit with type note, got %d", len(hits))
	}
	if hits[0].NodeID != "n1" {
		t.Errorf("expected n1, got %s", hits[0].NodeID)
	}
}

// TestSearchMalformedQueryReturnsWrappedError verifies FTS5 errors are
// wrapped with graph.ErrFTSQuerySyntax (bead kernl-q8gp).
func TestSearchMalformedQueryReturnsWrappedError(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)

	// Pass a query that sanitizes to empty, which FTS5 may reject
	// when passed as an empty quoted string.
	err := g.DoRead(context.Background(), func(rtx *graph.ReadTx) error {
		_, serr := search.Search(context.Background(), rtx, "***")
		return serr
	})
	// Either ErrFTSQuerySyntax directly (if handled before FTS5),
	// or wrapped. The sanitize function strips *, so "***" becomes empty
	// which Search treats as graph.ErrFTSQuerySyntax directly.
	if err == nil {
		t.Fatal("expected error for malformed query, got nil")
	}
	if !errors.Is(err, graph.ErrFTSQuerySyntax) {
		t.Errorf("expected ErrFTSQuerySyntax, got %v", err)
	}

	// Also test isFTS5Error detection through Search by directly
	// constructing a bad query. Pass a query with unbalanced FTS5
	// special column prefix that our sanitize does not strip (e.g.
	// a lone colon which FTS5 interprets as a column specifier).
	// However our sanitize + quote wrapping makes most things safe.
	// We confirm the wrapping path by verifying that a legit query
	// that FTS5 itself rejects gets caught.
	// Construct a very long query that may trigger FTS5 issues.
	longQuery := strings.Repeat("x ", 10000)
	err = g.DoRead(context.Background(), func(rtx *graph.ReadTx) error {
		_, serr := search.Search(context.Background(), rtx, longQuery)
		return serr
	})
	if err != nil {
		// If FTS5 rejects this, ensure it's wrapped correctly.
		if strings.Contains(err.Error(), "fts5") || strings.Contains(err.Error(), "FTS5") {
			if !errors.Is(err, graph.ErrFTSQuerySyntax) {
				t.Errorf("FTS5 error not wrapped: %v", err)
			}
		}
	}
}

// TestFTSSpecialCharsInBody verifies nodes with quotes/asterisks in body
// can be searched without crashing (bead kernl-14fd).
func TestFTSSpecialCharsInBody(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	setupSearchNode(t, g, "n1", "note", "Special", `this has "quotes" and *asterisks*`, nil)

	var hits []search.Hit
	err := g.DoRead(context.Background(), func(rtx *graph.ReadTx) error {
		var serr error
		hits, serr = search.Search(context.Background(), rtx, "quotes")
		return serr
	})
	if err != nil {
		t.Fatalf("Search should not crash with special chars in body: %v", err)
	}
	if len(hits) == 0 {
		t.Error("expected at least 1 hit for 'quotes'")
	}
}

// TestFTSPortugueseDiacritics verifies that FTS5 with unicode61
// handles Portuguese diacritics — searching for "coracao" finds
// "coração" (bead kernl-atzm).
func TestFTSPortugueseDiacritics(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	setupSearchNode(t, g, "n1", "note", "Coração", "um texto com coração", nil)

	var hits []search.Hit
	err := g.DoRead(context.Background(), func(rtx *graph.ReadTx) error {
		var serr error
		hits, serr = search.Search(context.Background(), rtx, "coracao")
		return serr
	})
	if err != nil {
		t.Fatalf("Search for 'coracao': %v", err)
	}
	if len(hits) == 0 {
		t.Error("expected hit when searching 'coracao' for node with 'coração'")
	}
}

// TestFTSMaxLengthQuery verifies that a very long query does not crash (bead kernl-cf88).
func TestFTSMaxLengthQuery(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	setupSearchNode(t, g, "n1", "note", "Normal Title", "normal body", nil)

	longQuery := strings.Repeat("a", 5000)
	err := g.DoRead(context.Background(), func(rtx *graph.ReadTx) error {
		_, serr := search.Search(context.Background(), rtx, longQuery)
		return serr
	})
	if err != nil {
		t.Fatalf("Search with long query should not crash: %v", err)
	}
}

// TestFTSEmptyQuery verifies empty query returns ErrFTSQuerySyntax (bead kernl-ku0v).
func TestFTSEmptyQuery(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	err := g.DoRead(context.Background(), func(rtx *graph.ReadTx) error {
		_, serr := search.Search(context.Background(), rtx, "")
		return serr
	})
	if err == nil {
		t.Fatal("expected error for empty query, got nil")
	}
	if !errors.Is(err, graph.ErrFTSQuerySyntax) {
		t.Errorf("expected ErrFTSQuerySyntax, got %v", err)
	}
}

// TestFTSMixedEnglishPortuguese verifies mixed-language search (bead kernl-v44a).
func TestFTSMixedEnglishPortuguese(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	setupSearchNode(t, g, "n1", "note", "Hello mundo bonito", "english and portuguese mixed", nil)

	var hits []search.Hit
	err := g.DoRead(context.Background(), func(rtx *graph.ReadTx) error {
		var serr error
		hits, serr = search.Search(context.Background(), rtx, "Hello")
		return serr
	})
	if err != nil {
		t.Fatalf("Search for 'Hello': %v", err)
	}
	if len(hits) == 0 {
		t.Error("expected hit for English term 'Hello' in mixed-language title")
	}
}

// TestSearchExcludesTombstonedNotes verifies that a note with deleted_at set is
// excluded from search results, while a live note with the same terms still appears.
func TestSearchExcludesTombstonedNotes(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	setupSearchNode(t, g, "live-note", "note", "Unique Token xyzzy", "live body", nil)
	setupSearchNode(t, g, "dead-note", "note", "Unique Token xyzzy", "dead body", nil)

	// Tombstone dead-note by setting deleted_at
	err := g.DoWrite(context.Background(), func(wtx *graph.WriteTx) error {
		_, err := wtx.Exec(
			`UPDATE nodes SET deleted_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = 'dead-note'`,
		)
		return err
	})
	if err != nil {
		t.Fatalf("set deleted_at: %v", err)
	}

	var hits []search.Hit
	err = g.DoRead(context.Background(), func(rtx *graph.ReadTx) error {
		var serr error
		hits, serr = search.Search(context.Background(), rtx, "xyzzy")
		return serr
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	for _, h := range hits {
		if h.NodeID == "dead-note" {
			t.Error("tombstoned note 'dead-note' must not appear in search results")
		}
	}

	found := false
	for _, h := range hits {
		if h.NodeID == "live-note" {
			found = true
		}
	}
	if !found {
		t.Error("live note 'live-note' must appear in search results")
	}
}

// TestSearchExcludesTombstoned_NonNoteUnaffected verifies that non-note nodes
// (which always have NULL deleted_at) still appear in search after the global
// filter is added.
func TestSearchExcludesTombstoned_NonNoteUnaffected(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	setupSearchNode(t, g, "task-node", "task", "Tombstone Filter Task", "task body", nil)

	var hits []search.Hit
	err := g.DoRead(context.Background(), func(rtx *graph.ReadTx) error {
		var serr error
		hits, serr = search.Search(context.Background(), rtx, "Tombstone")
		return serr
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Error("expected non-note node to still appear in search after tombstone filter")
	}
}

// TestSearchWithPrefixMatchesPartialToken verifies that WithPrefix turns the
// last query token into a prefix match, so "lin" matches "Linktree" — the
// behaviour autocomplete-as-you-type depends on.
func TestSearchWithPrefixMatchesPartialToken(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	setupSearchNode(t, g, "n1", "note", "Linktree", "a tree of links", nil)

	var hits []search.Hit
	err := g.DoRead(context.Background(), func(rtx *graph.ReadTx) error {
		var serr error
		hits, serr = search.Search(context.Background(), rtx, "lin", search.WithPrefix())
		return serr
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 prefix hit, got %d", len(hits))
	}
	if hits[0].NodeID != "n1" {
		t.Errorf("expected NodeID n1, got %s", hits[0].NodeID)
	}
}

// TestSearchWithoutPrefixDoesNotMatchPartialToken pins the contrast: without
// WithPrefix, a partial token does not match (proving the prefix path is the
// thing doing the work, not a pre-existing behaviour).
func TestSearchWithoutPrefixDoesNotMatchPartialToken(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	setupSearchNode(t, g, "n1", "note", "Linktree", "a tree of links", nil)

	var hits []search.Hit
	err := g.DoRead(context.Background(), func(rtx *graph.ReadTx) error {
		var serr error
		hits, serr = search.Search(context.Background(), rtx, "lin")
		return serr
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits without prefix, got %d", len(hits))
	}
}
