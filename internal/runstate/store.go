package runstate

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "modernc.org/sqlite"
)

type AgentRecord struct {
	AgentID   string
	SessionID string
	Status    string
}

type Store struct {
	db *sql.DB
}

// busyTimeoutMs is how long a writer waits for the database instead of failing
// on the spot. One runstate.db is shared by every `kernl epic run` on the
// machine, and SQLite serves one writer at a time, so without this the loser of
// a race gets SQLITE_BUSY immediately. The writes here are single-row upserts
// that finish in microseconds; five seconds is far past any legitimate wait and
// is the same value the graph store uses.
const busyTimeoutMs = 5000

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: open sqlite: %w - cause: %v - Fix: verify path is writable", err, err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: enable WAL mode: %w - cause: %v - Fix: check SQLite library compatibility", err, err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS worktrees (
		epic_id TEXT NOT NULL,
		bead_id TEXT NOT NULL,
		path    TEXT NOT NULL,
		PRIMARY KEY (epic_id, bead_id)
	);
	CREATE TABLE IF NOT EXISTS agent_records (
		bead_id    TEXT NOT NULL,
		state      TEXT NOT NULL,
		agent_id   TEXT NOT NULL,
		session_id TEXT NOT NULL,
		status     TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		PRIMARY KEY (bead_id, state)
	);
	`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: create schema: %w - cause: %v - Fix: verify SQLite DDL compatibility", err, err)
	}

	return &Store{db: db}, nil
}

// dsn carries the busy timeout as a connection string pragma, which is the only
// placement that reaches every connection database/sql opens. A
// `PRAGMA busy_timeout` statement would set it on whichever pooled connection
// happened to serve that one Exec, leaving the rest of the pool without it -
// the failure would come back intermittently and look like disk trouble.
//
// journal_mode stays an Exec because WAL is a property of the file, set once
// and read by every connection afterwards.
func dsn(path string) string {
	if path == ":memory:" {
		// Every connection in the pool must reach the same in-memory database,
		// or the schema created on one is missing from the next.
		return fmt.Sprintf("file::memory:?cache=shared&_pragma=busy_timeout(%d)", busyTimeoutMs)
	}
	return fmt.Sprintf("file:%s?_pragma=busy_timeout(%d)", path, busyTimeoutMs)
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SetWorktree(epicID, beadID, path string) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO worktrees (epic_id, bead_id, path) VALUES (?, ?, ?)",
		epicID, beadID, path,
	)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: SetWorktree(%s, %s): %w - cause: write to SQLite failed - Fix: check disk space and permissions", epicID, beadID, err)
	}
	return nil
}

func (s *Store) Worktree(epicID, beadID string) (string, bool) {
	var path string
	err := s.db.QueryRow(
		"SELECT path FROM worktrees WHERE epic_id = ? AND bead_id = ?",
		epicID, beadID,
	).Scan(&path)
	if err != nil {
		return "", false
	}
	return path, true
}

func (s *Store) RecordAgent(beadID, state string, rec AgentRecord) {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO agent_records (bead_id, state, agent_id, session_id, status, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		beadID, state, rec.AgentID, rec.SessionID, rec.Status, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		slog.Warn("runstate: recording agent state failed", "beadId", beadID, "state", state, "error", err)
	}
}

func (s *Store) AgentRecord(beadID, state string) (AgentRecord, bool) {
	var rec AgentRecord
	err := s.db.QueryRow(
		"SELECT agent_id, session_id, status FROM agent_records WHERE bead_id = ? AND state = ?",
		beadID, state,
	).Scan(&rec.AgentID, &rec.SessionID, &rec.Status)
	if err != nil {
		return AgentRecord{}, false
	}
	return rec, true
}
