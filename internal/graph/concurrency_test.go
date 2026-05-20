package graph_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/testutil"
)

func TestConcurrentWritesSerialize(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	const n = 100
	var wg sync.WaitGroup
	errCh := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
				_, err := nodes.CreateCapture(ctx, tx, nodes.Capture{
					Title: fmt.Sprintf("concurrent-%d", idx),
					Body:  fmt.Sprintf("body-%d", idx),
				}, nodes.Author{Name: "test"})
				return err
			})
			if err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	sqliteBusyCount := 0
	for err := range errCh {
		if strings.Contains(err.Error(), "BUSY") {
			sqliteBusyCount++
		} else {
			t.Errorf("unexpected error: %v", err)
		}
	}
	if sqliteBusyCount != 0 {
		t.Fatalf("expected 0 SQLITE_BUSY, got %d", sqliteBusyCount)
	}
}

type readResult struct {
	seen bool
	err  error
}

func TestReaderSeesPreDeleteSnapshot(t *testing.T) {
	g := testutil.NewInMemoryTestGraph(t)
	ctx := context.Background()

	// Setup: create a node
	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var err error
		id, err = nodes.CreateCapture(ctx, tx, nodes.Capture{Title: "survivor", Body: "b"}, nodes.Author{Name: "test"})
		return err
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	readStarted := make(chan struct{})
	resultCh := make(chan readResult, 1)

	go func() {
		var seen bool
		var readErr error
		readErr = g.DoRead(ctx, func(tx *graph.ReadTx) error {
			close(readStarted) // signal AFTER BeginTx
			var dummy string
			err := tx.QueryRow("SELECT id FROM nodes WHERE id = ?", id).Scan(&dummy)
			if err == nil {
				seen = true
			} else if err == sql.ErrNoRows {
				seen = false
			} else {
				readErr = err
			}
			return nil
		})
		resultCh <- readResult{seen: seen, err: readErr}
	}()

	<-readStarted // wait for read tx to begin

	// Now delete the node
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return nodes.DeleteCapture(ctx, tx, id, nodes.Author{Name: "test"})
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	res := <-resultCh
	if res.err != nil {
		t.Fatalf("read goroutine error: %v", res.err)
	}
	if !res.seen {
		t.Fatal("expected reader to still see the node in pre-delete snapshot, but it was gone")
	}
}
