package testutil_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

func openTestGraph(t *testing.T) *graph.Graph {
	t.Helper()
	g, err := graph.Open(context.Background(), graph.Config{InMemory: true, Path: "mem-test-" + t.Name()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return g
}

func TestCloseWithInFlightTxsDoesNotPanic(t *testing.T) {
	g := openTestGraph(t)

	// Seed a row so reads have something to hit.
	if err := g.DoWrite(context.Background(), func(wtx *graph.WriteTx) error {
		_, err := wtx.Exec(`INSERT INTO nodes(id, type, title) VALUES ('seed', 's', 'seed')`)
		return err
	}); err != nil {
		t.Fatalf("seed write: %v", err)
	}

	stop := make(chan struct{})
	done := make(chan struct{}, 4)

	for i := 0; i < 4; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			for {
				select {
				case <-stop:
					return
				default:
				}
				// best-effort: ignore errors from closed graph
				_ = g.DoRead(context.Background(), func(rtx *graph.ReadTx) error {
					var v int
					return rtx.QueryRow(`PRAGMA user_version`).Scan(&v)
				})
			}
		}(i)
	}

	time.Sleep(20 * time.Millisecond)
	close(stop)

	if err := g.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	// Wait for all goroutines to exit.
	for i := 0; i < 4; i++ {
		<-done
	}
}

func TestIsolationAcrossParallelTests(t *testing.T) {
	type row struct {
		id    string
		typ   string
		title string
	}

	tests := []struct {
		name     string
		sentinel row
	}{
		{name: "A", sentinel: row{id: "iso-a", typ: "t", title: "isolated A"}},
		{name: "B", sentinel: row{id: "iso-b", typ: "t", title: "isolated B"}},
		{name: "C", sentinel: row{id: "iso-c", typ: "t", title: "isolated C"}},
		{name: "D", sentinel: row{id: "iso-d", typ: "t", title: "isolated D"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := openTestGraph(t)
			defer g.Close()

			// Insert this test's sentinel row.
			if err := g.DoWrite(context.Background(), func(wtx *graph.WriteTx) error {
				_, err := wtx.Exec(`INSERT INTO nodes(id, type, title) VALUES (?, ?, ?)`,
					tt.sentinel.id, tt.sentinel.typ, tt.sentinel.title)
				return err
			}); err != nil {
				t.Fatalf("insert sentinel: %v", err)
			}

			// Assert our sentinel exists exactly once.
			var count int
			if err := g.DoRead(context.Background(), func(rtx *graph.ReadTx) error {
				return rtx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id = ?`, tt.sentinel.id).Scan(&count)
			}); err != nil {
				t.Fatalf("count own sentinel: %v", err)
			}
			if count != 1 {
				t.Errorf("expected 1 row for sentinel %s, got %d", tt.sentinel.id, count)
			}

			// Assert no OTHER sentinel row leaked in.
			for _, other := range tests {
				if other.name == tt.name {
					continue
				}
				var otherCount int
				if err := g.DoRead(context.Background(), func(rtx *graph.ReadTx) error {
					return rtx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id = ?`, other.sentinel.id).Scan(&otherCount)
				}); err != nil {
					t.Fatalf("count other sentinel %s: %v", other.sentinel.id, err)
				}
				if otherCount != 0 {
					t.Errorf("leaked %d row(s) for sentinel %s into test %s (expected 0, separate in-memory databases)", otherCount, other.sentinel.id, tt.name)
				}
			}
		})
	}
}

// Ensure the testutil package compiles standalone.
func TestCompiles(t *testing.T) {
	_ = fmt.Sprintf("%v", graph.ErrSchemaLocked)
}
