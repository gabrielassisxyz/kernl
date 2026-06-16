package sqlite_test

import (
	"os"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph/internal/sqlite"
)

func TestOpenAppliesPragmas(t *testing.T) {
	pool := sqliteOpenTemp(t)
	defer pool.Close()

	var journalMode string
	if err := pool.Read.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("expected journal_mode=wal, got %s", journalMode)
	}

	var foreignKeys int
	if err := pool.Read.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("expected foreign_keys=1, got %d", foreignKeys)
	}
}

func TestWritePoolSerializesWrites(t *testing.T) {
	pool := sqliteOpenTemp(t)
	defer pool.Close()

	if _, err := pool.Write.Exec("CREATE TABLE test (id INTEGER PRIMARY KEY, val TEXT)"); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}

	const goroutines = 50
	errs := make(chan error, goroutines)
	for i := range goroutines {
		go func(n int) {
			_, err := pool.Write.Exec("INSERT INTO test (id, val) VALUES (?, ?)", n, "x")
			errs <- err
		}(i)
	}

	var failures int
	for range goroutines {
		if err := <-errs; err != nil {
			failures++
		}
	}

	if failures > 0 {
		t.Errorf("expected 0 write failures with MaxOpenConns(1), got %d", failures)
	}
}

func sqliteOpenTemp(t *testing.T) *sqlite.Pool {
	t.Helper()
	f, err := os.CreateTemp("", "kernl-test-*.db")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()
	t.Cleanup(func() { _ = os.Remove(f.Name()) })

	pool, err := sqlite.Open(sqlite.Config{Path: f.Name()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return pool
}
