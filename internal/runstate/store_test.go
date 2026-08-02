package runstate

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// Two `kernl epic run` processes share one runstate.db, and SQLite serves one
// writer at a time. Without a busy timeout the loser of a race does not wait,
// it fails immediately with SQLITE_BUSY - measured at 651 lost writes out of
// 1000 before this was set.
//
// Two Stores over one file is the same contention two processes produce: the
// timeout has to reach every connection the pool opens, which is why it is a
// DSN pragma and not a `PRAGMA busy_timeout` statement (that would apply to
// whichever single connection happened to serve it).
func TestConcurrentStoresDoNotLoseWritesToBusyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runstate.db")
	first, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer first.Close()
	second, err := Open(path)
	if err != nil {
		t.Fatalf("Open (second): %v", err)
	}
	defer second.Close()

	const writesPerStore = 200
	failures := make(chan error, 2*writesPerStore)
	var wg sync.WaitGroup
	for i, store := range []*Store{first, second} {
		wg.Add(1)
		go func(epic int, s *Store) {
			defer wg.Done()
			for n := 0; n < writesPerStore; n++ {
				if err := s.SetWorktree(fmt.Sprintf("epic%d", epic), fmt.Sprintf("bead%d", n), "/wt"); err != nil {
					failures <- err
				}
			}
		}(i, store)
	}
	wg.Wait()
	close(failures)

	if err, lost := <-failures, len(failures)+1; err != nil {
		t.Fatalf("%d of %d writes lost, first: %v", lost, 2*writesPerStore, err)
	}
}

func TestStoreRoundTripsWorktreeAndAgentRecords(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	if err := s.SetWorktree("e", "c1", "/tmp/wt/e/c1"); err != nil {
		t.Fatalf("SetWorktree: %v", err)
	}
	wt, ok := s.Worktree("e", "c1")
	if !ok || wt != "/tmp/wt/e/c1" {
		t.Errorf("Worktree = %q,%v", wt, ok)
	}
	s.RecordAgent("c1", "implementing", AgentRecord{AgentID: "opencode", SessionID: "term-1", Status: "running"})
	rec, ok := s.AgentRecord("c1", "implementing")
	if !ok || rec.SessionID != "term-1" {
		t.Errorf("AgentRecord = %+v,%v", rec, ok)
	}
}
