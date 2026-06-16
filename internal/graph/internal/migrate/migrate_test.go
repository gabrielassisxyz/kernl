package migrate_test

import (
	"context"
	"embed"
	"os"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph/internal/migrate"
	"github.com/gabrielassisxyz/kernl/internal/graph/internal/sqlite"
)

//go:embed testdata/schema/*.sql
var testSchema embed.FS

//go:embed testdata/schema_broken/*.sql
var testSchemaBroken embed.FS

func TestUpAppliesPendingMigrations(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()

	r, err := migrate.New(pool.Write, testSchema)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := r.Up(context.Background()); err != nil {
		t.Fatalf("Up: %v", err)
	}

	v, dirty, err := r.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if v != 1 {
		t.Errorf("expected version=1, got %d", v)
	}
	if dirty {
		t.Error("expected dirty=false after successful migration")
	}
}

func TestDownRollsBack(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()

	r, err := migrate.New(pool.Write, testSchema)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := r.Up(context.Background()); err != nil {
		t.Fatalf("Up: %v", err)
	}

	if err := r.Down(context.Background()); err != nil {
		t.Fatalf("Down: %v", err)
	}

	v, dirty, err := r.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if v != 0 {
		t.Errorf("expected version=0 after down, got %d", v)
	}
	if dirty {
		t.Error("expected dirty=false after successful down")
	}

	_, err = pool.Read.Query("SELECT * FROM test_migrate")
	if err == nil {
		t.Error("expected table to not exist after down")
	}
}

func TestUpDownUpRoundTrip(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()

	r, err := migrate.New(pool.Write, testSchema)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := r.Up(context.Background()); err != nil {
			t.Fatalf("Up iteration %d: %v", i, err)
		}
		if err := r.Down(context.Background()); err != nil {
			t.Fatalf("Down iteration %d: %v", i, err)
		}
	}

	v, _, err := r.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if v != 0 {
		t.Errorf("expected version=0 after round-trips, got %d", v)
	}
}

func TestDirtyStickyAfterFailedMigration(t *testing.T) {
	pool := openTestDB(t)
	defer pool.Close()

	r, err := migrate.New(pool.Write, testSchemaBroken)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = r.Up(context.Background())
	if err == nil {
		t.Fatal("expected Up to fail on broken SQL")
	}

	_, dirty, err := r.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if !dirty {
		t.Error("expected dirty=true after failed migration")
	}

	err = r.Up(context.Background())
	if err == nil {
		t.Fatal("expected second Up to return ErrDirty")
	}
	if err != migrate.ErrDirty {
		t.Errorf("expected ErrDirty, got %v", err)
	}
}

func openTestDB(t *testing.T) *sqlite.Pool {
	t.Helper()
	f, err := os.CreateTemp("", "kernl-migrate-test-*.db")
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
