package schema_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph/internal/migrate"
	"github.com/gabrielassisxyz/kernl/internal/graph/internal/sqlite"
	"github.com/gabrielassisxyz/kernl/internal/graph/schema"
)

func schemaOpenTemp(t *testing.T) *sql.DB {
	t.Helper()
	f, err := os.CreateTemp("", "kernl-schema-test-*.db")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()
	t.Cleanup(func() { _ = os.Remove(f.Name()) })

	pool, err := sqlite.Open(sqlite.Config{Path: f.Name()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	return pool.Write
}

func TestMigration0001UpDown(t *testing.T) {
	db := schemaOpenTemp(t)
	r, err := migrate.New(db, schema.FS)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	ctx := context.Background()
	if err := r.UpTo(ctx, 1); err != nil {
		t.Fatalf("UpTo 1: %v", err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='nodes'").Scan(&count); err != nil {
		t.Fatalf("check nodes table: %v", err)
	}
	if count != 1 {
		t.Errorf("nodes table should exist after Up")
	}
	if err := r.Down(ctx); err != nil {
		t.Fatalf("Down: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='nodes'").Scan(&count); err != nil {
		t.Fatalf("check nodes table gone: %v", err)
	}
	if count != 0 {
		t.Errorf("nodes table should be gone after Down")
	}
}

func TestMigration0002IndexesRoundTrip(t *testing.T) {
	db := schemaOpenTemp(t)
	r, err := migrate.New(db, schema.FS)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	ctx := context.Background()
	if err := r.UpTo(ctx, 2); err != nil {
		t.Fatalf("UpTo 2: %v", err)
	}
	ver, dirty, err := r.Current(ctx)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if dirty {
		t.Fatal("schema dirty after Up")
	}
	if ver != 2 {
		t.Errorf("expected version 2, got %d", ver)
	}
	if err := r.Down(ctx); err != nil {
		t.Fatalf("Down: %v", err)
	}
	ver, _, err = r.Current(ctx)
	if err != nil {
		t.Fatalf("Current after Down: %v", err)
	}
	if ver != 1 {
		t.Errorf("expected version 1 after Down, got %d", ver)
	}
}

func TestMigration003NotesRoundTrip(t *testing.T) {
	db := schemaOpenTemp(t)

	r, err := migrate.New(db, schema.FS)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}

	ctx := context.Background()

	// Up to v3
	if err := r.UpTo(ctx, 3); err != nil {
		t.Fatalf("UpTo 3: %v", err)
	}
	ver, dirty, err := r.Current(ctx)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if dirty {
		t.Fatal("schema_migrations is dirty after Up")
	}
	if ver != 3 {
		t.Errorf("expected version 3, got %d", ver)
	}

	// Verify new tables exist
	for _, table := range []string{"note_paths", "dangling_links"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count); err != nil {
			t.Fatalf("checking %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("table %s should exist after 0003", table)
		}
	}

	// Verify deleted_at column exists on nodes
	var colCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('nodes') WHERE name='deleted_at'").Scan(&colCount); err != nil {
		t.Fatalf("checking deleted_at column: %v", err)
	}
	if colCount != 1 {
		t.Errorf("deleted_at column should exist on nodes")
	}

	// Verify indexes exist
	for _, idx := range []string{"idx_note_paths_path", "idx_dangling_links_target_key"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?", idx).Scan(&count); err != nil {
			t.Fatalf("checking index %s: %v", idx, err)
		}
		if count != 1 {
			t.Errorf("index %s should exist after 0003", idx)
		}
	}

	// Down to v2
	if err := r.Down(ctx); err != nil {
		t.Fatalf("Down from v3: %v", err)
	}
	ver, _, err = r.Current(ctx)
	if err != nil {
		t.Fatalf("Current after Down: %v", err)
	}
	if ver != 2 {
		t.Fatalf("expected version 2 after Down, got %d", ver)
	}

	// Verify tables are gone
	for _, table := range []string{"note_paths", "dangling_links"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count); err != nil {
			t.Fatalf("checking %s: %v", table, err)
		}
		if count != 0 {
			t.Errorf("table %s should be gone after down", table)
		}
	}

	// Verify deleted_at is gone
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('nodes') WHERE name='deleted_at'").Scan(&colCount); err != nil {
		t.Fatalf("checking deleted_at gone: %v", err)
	}
	if colCount != 0 {
		t.Errorf("deleted_at should be gone")
	}

	// Re-Up to v3
	if err := r.UpTo(ctx, 3); err != nil {
		t.Fatalf("Re-Up to 3: %v", err)
	}
	ver, _, err = r.Current(ctx)
	if err != nil {
		t.Fatalf("Current after Re-Up: %v", err)
	}
	if ver != 3 {
		t.Errorf("expected version 3 after Re-Up, got %d", ver)
	}
}

func TestMigration003NotePathsConstraints(t *testing.T) {
	db := schemaOpenTemp(t)

	r, err := migrate.New(db, schema.FS)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}

	ctx := context.Background()
	if err := r.UpTo(ctx, 3); err != nil {
		t.Fatalf("UpTo 3: %v", err)
	}

	// Insert a note_path
	if _, err := db.ExecContext(ctx, `INSERT INTO note_paths(uuid, path) VALUES (?, ?)`, "u1", "/notes/foo.md"); err != nil {
		t.Fatalf("insert note_path: %v", err)
	}

	// Duplicate uuid should fail
	if _, err := db.ExecContext(ctx, `INSERT INTO note_paths(uuid, path) VALUES (?, ?)`, "u1", "/notes/bar.md"); err == nil {
		t.Error("expected duplicate uuid to fail")
	}

	// Duplicate path should fail
	if _, err := db.ExecContext(ctx, `INSERT INTO note_paths(uuid, path) VALUES (?, ?)`, "u2", "/notes/foo.md"); err == nil {
		t.Error("expected duplicate path to fail")
	}
}

func TestMigration003DanglingLinksConstraints(t *testing.T) {
	db := schemaOpenTemp(t)

	r, err := migrate.New(db, schema.FS)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}

	ctx := context.Background()
	if err := r.UpTo(ctx, 3); err != nil {
		t.Fatalf("UpTo 3: %v", err)
	}

	// src_node_id references a non-existent node
	if _, err := db.ExecContext(ctx, `INSERT INTO dangling_links(id, src_node_id, target_key, target_kind) VALUES (?, ?, ?, ?)`, "d1", "nope", "foo", "stem"); err == nil {
		t.Error("expected FK violation for missing src_node_id")
	}

	// target_kind outside enum should fail
	if _, err := db.ExecContext(ctx, `INSERT INTO dangling_links(id, src_node_id, target_key, target_kind) VALUES (?, ?, ?, ?)`, "d2", "n1", "foo", "invalid"); err == nil {
		t.Error("expected CHECK violation for target_kind")
	}
}

func TestMigration004BatchLogsRoundTrip(t *testing.T) {
	db := schemaOpenTemp(t)

	r, err := migrate.New(db, schema.FS)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}

	ctx := context.Background()
	if err := r.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}
	ver, dirty, err := r.Current(ctx)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if dirty {
		t.Fatal("schema_migrations is dirty after Up")
	}
	if ver != 4 {
		t.Fatalf("expected version 4, got %d", ver)
	}

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='batch_logs'").Scan(&count); err != nil {
		t.Fatalf("checking batch_logs: %v", err)
	}
	if count != 1 {
		t.Errorf("batch_logs table should exist after 0004")
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO batch_logs(id, raw_text) VALUES (?, ?)`, "b1", "paste"); err != nil {
		t.Fatalf("insert batch_log: %v", err)
	}

	if err := r.Down(ctx); err != nil {
		t.Fatalf("Down from v4: %v", err)
	}
	ver, _, err = r.Current(ctx)
	if err != nil {
		t.Fatalf("Current after Down: %v", err)
	}
	if ver != 3 {
		t.Fatalf("expected version 3 after Down, got %d", ver)
	}

	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='batch_logs'").Scan(&count); err != nil {
		t.Fatalf("checking batch_logs gone: %v", err)
	}
	if count != 0 {
		t.Errorf("batch_logs table should be gone after down")
	}
}

func TestSchemaFilesExist(t *testing.T) {
	entries, err := schema.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("schema.FS is empty")
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if _, err := schema.FS.ReadFile(e.Name()); err != nil {
			t.Errorf("cannot read %s: %v", e.Name(), err)
		}
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}
