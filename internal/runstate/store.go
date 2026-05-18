package runstate

import (
	"database/sql"
	"fmt"
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

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: open sqlite: %w — cause: %v — Fix: verify path is writable", err, err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: enable WAL mode: %w — cause: %v — Fix: check SQLite library compatibility", err, err)
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
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: create schema: %w — cause: %v — Fix: verify SQLite DDL compatibility", err, err)
	}

	return &Store{db: db}, nil
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
		return fmt.Errorf("KERNL DISPATCH FAILURE: SetWorktree(%s, %s): %w — cause: write to SQLite failed — Fix: check disk space and permissions", epicID, beadID, err)
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
	s.db.Exec(
		"INSERT OR REPLACE INTO agent_records (bead_id, state, agent_id, session_id, status, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		beadID, state, rec.AgentID, rec.SessionID, rec.Status, time.Now().UTC().Format(time.RFC3339),
	)
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
