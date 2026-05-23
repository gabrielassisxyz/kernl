package schema_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph/internal/migrate"
	"github.com/gabrielassisxyz/kernl/internal/graph/schema"
	_ "modernc.org/sqlite"
)

func TestMigration002IndexesUp(t *testing.T) {
	db := schemaOpenTemp(t)
	defer db.Close()

	r, err := migrate.New(db, schema.FS)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}

	ctx := context.Background()
	if err := r.UpTo(ctx, 2); err != nil {
		t.Fatalf("Up to v2: %v", err)
	}

	ver, dirty, err := r.Current(ctx)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if dirty {
		t.Fatal("schema_migrations is dirty after Up")
	}
	if ver != 2 {
		t.Errorf("expected version 2, got %d", ver)
	}

	indexes := map[string]bool{
		"idx_edges_src_label":  false,
		"idx_edges_dst_label":  false,
		"idx_nodes_type":       false,
		"idx_node_tags_tag_id": false,
	}

	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='index'`)
	if err != nil {
		t.Fatalf("query indexes: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan index name: %v", err)
		}
		if _, ok := indexes[name]; ok {
			indexes[name] = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}

	for name, found := range indexes {
		if !found {
			t.Errorf("index %s not found after Up to v2", name)
		}
	}
}

func TestMigration002IndexesRoundTrip(t *testing.T) {
	db := schemaOpenTemp(t)
	defer db.Close()

	r, err := migrate.New(db, schema.FS)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}

	ctx := context.Background()

	// Up to v2
	if err := r.UpTo(ctx, 2); err != nil {
		t.Fatalf("Up to v2: %v", err)
	}
	ver, _, err := r.Current(ctx)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if ver != 2 {
		t.Fatalf("expected version 2 after Up, got %d", ver)
	}

	// Down from v2 back to v1
	if err := r.Down(ctx); err != nil {
		t.Fatalf("Down from v2: %v", err)
	}
	ver, _, err = r.Current(ctx)
	if err != nil {
		t.Fatalf("Current after Down: %v", err)
	}
	if ver != 1 {
		t.Fatalf("expected version 1 after Down, got %d", ver)
	}

	// Verify the four indexes are gone
	expectedIndexes := []string{
		"idx_edges_src_label",
		"idx_edges_dst_label",
		"idx_nodes_type",
		"idx_node_tags_tag_id",
	}
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='index'`)
	if err != nil {
		t.Fatalf("query indexes: %v", err)
	}
	defer rows.Close()
	present := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan index name: %v", err)
		}
		present[name] = true
	}
	for _, name := range expectedIndexes {
		if present[name] {
			t.Errorf("index %s should be gone after Down from v2", name)
		}
	}

	// Re-Up to v2 (idempotent round-trip)
	if err := r.UpTo(ctx, 2); err != nil {
		t.Fatalf("Re-Up to v2: %v", err)
	}
	ver, _, err = r.Current(ctx)
	if err != nil {
		t.Fatalf("Current after Re-Up: %v", err)
	}
	if ver != 2 {
		t.Errorf("expected version 2 after Re-Up, got %d", ver)
	}
}

func TestMigration003NotesRoundTrip(t *testing.T) {
	db := schemaOpenTemp(t)
	defer db.Close()

	r, err := migrate.New(db, schema.FS)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}

	ctx := context.Background()

	// Up all the way to v3
	if err := r.Up(ctx); err != nil {
		t.Fatalf("Up to latest: %v", err)
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
		t.Errorf("deleted_at should be gone after down")
	}

	// Re-Up to v3
	if err := r.Up(ctx); err != nil {
		t.Fatalf("Re-Up to v3: %v", err)
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
	defer db.Close()

	r, err := migrate.New(db, schema.FS)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}

	ctx := context.Background()
	if err := r.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
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
	defer db.Close()

	r, err := migrate.New(db, schema.FS)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}

	ctx := context.Background()
	if err := r.Up(ctx); err != nil {
		t.Fatalf("Up: %v", err)
	}

	// Insert a node first (FK target)
	if _, err := db.ExecContext(ctx, `INSERT INTO nodes(id, type, title) VALUES (?, ?, ?)`, "node-1", "note", "Test"); err != nil {
		t.Fatalf("insert node: %v", err)
	}

	// Insert a dangling_link
	if _, err := db.ExecContext(ctx, `INSERT INTO dangling_links(id, src_node_id, target_key, target_kind) VALUES (?, ?, ?, 'stem')`, "dl1", "node-1", "Roadmap"); err != nil {
		t.Fatalf("insert dangling_link: %v", err)
	}

	// Invalid target_kind should fail
	if _, err := db.ExecContext(ctx, `INSERT INTO dangling_links(id, src_node_id, target_key, target_kind) VALUES (?, ?, ?, 'bogus')`, "dl2", "node-1", "X"); err == nil {
		t.Error("expected bogus target_kind to fail CHECK constraint")
	}
}

func TestMigration002NoOp(t *testing.T) {
	db := schemaOpenTemp(t)
	defer db.Close()

	r, err := migrate.New(db, schema.FS)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}

	ctx := context.Background()

	// Up to v2
	if err := r.UpTo(ctx, 2); err != nil {
		t.Fatalf("Up to v2: %v", err)
	}

	// Up again — must be a no-op (no duplicate-index error)
	if err := r.UpTo(ctx, 2); err != nil {
		t.Fatalf("no-op Up: %v", err)
	}

	ver, dirty, err := r.Current(ctx)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if dirty {
		t.Fatal("schema_migrations is dirty after no-op Up")
	}
	if ver != 2 {
		t.Errorf("expected version 2, got %d", ver)
	}
}

func TestInitialSchemaApplies(t *testing.T) {
	db := schemaOpenTemp(t)
	defer db.Close()

	schemaSQL, err := schema.FS.ReadFile("0001_init.up.sql")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if _, err := db.Exec(string(schemaSQL)); err != nil {
		t.Fatalf("Exec schema: %v", err)
	}

	tables := []string{"nodes", "edges", "revisions", "tags", "node_tags", "nodes_fts"}
	for _, table := range tables {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name=? COLLATE NOCASE", table).Scan(&count)
		if err != nil {
			t.Errorf("checking %s: %v", table, err)
			continue
		}
		if count == 0 {
			t.Errorf("table %s not found in sqlite_master", table)
		}
	}
}

func TestAttrsRejectInvalidJSON(t *testing.T) {
	db := schemaOpenTemp(t)
	defer db.Close()

	schemaSQL, err := schema.FS.ReadFile("0001_init.up.sql")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if _, err := db.Exec(string(schemaSQL)); err != nil {
		t.Fatalf("Exec schema: %v", err)
	}

	_, err = db.Exec(`INSERT INTO nodes(id, type, title, attrs) VALUES (?, ?, ?, ?)`, "test-1", "test", "Test Node", "not json")
	if err == nil {
		t.Error("expected CHECK constraint to reject invalid JSON, but INSERT succeeded")
	}
}

func TestSchemaCorrections(t *testing.T) {
	db := schemaOpenTemp(t)
	defer db.Close()

	schemaSQL, err := schema.FS.ReadFile("0001_init.up.sql")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if _, err := db.Exec(string(schemaSQL)); err != nil {
		t.Fatalf("Exec schema: %v", err)
	}

	_, err = db.Exec(`INSERT INTO nodes(id, type, title, attrs, bogus) VALUES ('x', 't', 'Test', '{}', 1)`)
	if err == nil {
		t.Error("expected STRICT table to reject unknown column 'bogus', but INSERT succeeded")
	}

	if _, err := db.Exec(`INSERT INTO nodes(id, type, title) VALUES ('n1', 't', 'Test')`); err != nil {
		t.Fatalf("insert node: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO revisions(id, node_id, diff, author) VALUES ('r1', 'n1', '{}', 'tester')`); err != nil {
		t.Fatalf("insert revision: %v", err)
	}

	// Revisions survive node deletion to preserve audit history.
	_, err = db.Exec(`DELETE FROM nodes WHERE id = 'n1'`)
	if err != nil {
		t.Fatalf("DELETE returned error: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM revisions WHERE id = 'r1'`).Scan(&count); err != nil {
		t.Fatalf("count revisions: %v", err)
	}
	if count != 1 {
		t.Errorf("revision should survive the node deletion (node_id becomes NULL)")
	}

	var nodeID sql.NullString
	if err := db.QueryRow(`SELECT node_id FROM revisions WHERE id = 'r1'`).Scan(&nodeID); err != nil {
		t.Fatalf("select node_id: %v", err)
	}
	if nodeID.Valid {
		t.Errorf("expected node_id to be NULL after DELETE (ON DELETE SET NULL), got %q", nodeID.String)
	}
}

func TestInitialSchemaRoundTrip(t *testing.T) {
	db := schemaOpenTemp(t)
	defer db.Close()

	upSQL, err := schema.FS.ReadFile("0001_init.up.sql")
	if err != nil {
		t.Fatalf("ReadFile up: %v", err)
	}
	downSQL, err := schema.FS.ReadFile("0001_init.down.sql")
	if err != nil {
		t.Fatalf("ReadFile down: %v", err)
	}

	if _, err := db.Exec(string(upSQL)); err != nil {
		t.Fatalf("up: %v", err)
	}
	if _, err := db.Exec(string(downSQL)); err != nil {
		t.Fatalf("down: %v", err)
	}
	if _, err := db.Exec(string(upSQL)); err != nil {
		t.Fatalf("up again: %v", err)
	}

	tables := []string{"nodes", "edges", "revisions", "tags", "node_tags", "nodes_fts"}
	for _, table := range tables {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name=? COLLATE NOCASE", table).Scan(&count); err != nil {
			t.Fatalf("checking %s: %v", table, err)
		}
		if count == 0 {
			t.Errorf("table %s not found after round-trip", table)
		}
	}
}

func schemaOpenTemp(t *testing.T) *sql.DB {
	t.Helper()
	f, err := os.CreateTemp("", "kernl-schema-test-*.db")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	dsn := "file:" + f.Name() + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return db
}
