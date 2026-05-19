package schema_test

import (
	"database/sql"
	"os"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/graph/schema"
	_ "modernc.org/sqlite"
)

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

	_, err = db.Exec(`DELETE FROM nodes WHERE id = 'n1'`)
	if err == nil {
		t.Error("expected DELETE to fail because revisions.node_id prevents deletion (no cascade), but it succeeded")
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM revisions WHERE id = 'r1'`).Scan(&count); err != nil {
		t.Fatalf("count revisions: %v", err)
	}
	if count != 1 {
		t.Error("revision should survive the failed DELETE attempt")
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
