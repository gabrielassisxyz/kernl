// Package reconcile provides the path↔uuid cache layer over the note_paths
// table. Identity is the UUID; path is a disposable cache entry.
//
// Later beads (U7 create/change handlers, U11 delete handlers) will add
// functions to this file. Keep cache operations grouped here, and put
// handler-specific logic in separate files within this package.
package reconcile

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/gabrielassisxyz/kernl/internal/graph"
)

// --- Path cache operations ---

// Lookup returns the UUID for the given vault-relative path.
// Returns found=false (no error) when no row exists for the path.
func Lookup(ctx context.Context, g *graph.Graph, path string) (uuid string, found bool, err error) {
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		uuid, found, err = lookupInTx(tx, path)
		return err
	})
	return
}

// lookupInTx is the inner path→uuid query, usable inside any read or write tx.
func lookupInTx(tx interface {
	QueryRow(string, ...any) *sql.Row
}, path string) (string, bool, error) {
	var uuid string
	err := tx.QueryRow(`SELECT uuid FROM note_paths WHERE path = ?`, path).Scan(&uuid)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("reconcile: lookup path %q: %w", path, err)
	}
	return uuid, true, nil
}

// LookupByUUID returns the current path for a given UUID.
// Returns found=false (no error) when no row exists for the UUID.
func LookupByUUID(ctx context.Context, g *graph.Graph, uuid string) (path string, found bool, err error) {
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		path, found, err = lookupByUUIDInTx(tx, uuid)
		return err
	})
	return
}

// lookupByUUIDInTx is the inner uuid→path query.
func lookupByUUIDInTx(tx interface {
	QueryRow(string, ...any) *sql.Row
}, uuid string) (string, bool, error) {
	var path string
	err := tx.QueryRow(`SELECT path FROM note_paths WHERE uuid = ?`, uuid).Scan(&path)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("reconcile: lookup uuid %q: %w", uuid, err)
	}
	return path, true, nil
}

// Upsert inserts or updates the note_paths row keyed by UUID.
//
// A move is expressed as Upsert with the same UUID and a new path: the row's
// path column is updated and the old path is no longer reachable. A re-upsert
// of the identical (uuid, path, hash) triple is a no-op (idempotent).
//
// This operation touches ONLY note_paths. It never writes nodes, edges, or
// revisions — those are the responsibility of higher-level reconciler logic.
func Upsert(ctx context.Context, g *graph.Graph, uuid, path, contentHash string) error {
	return g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return upsertInTx(tx, uuid, path, contentHash)
	})
}

// upsertInTx performs the INSERT OR REPLACE inside a caller-supplied write tx.
// Exposed so U7/U11 can batch multiple cache ops inside a single transaction.
func upsertInTx(tx *graph.WriteTx, uuid, path, contentHash string) error {
	_, err := tx.Exec(
		`INSERT INTO note_paths (uuid, path, content_hash, updated_at)
		 VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		 ON CONFLICT(uuid) DO UPDATE SET
		     path         = excluded.path,
		     content_hash = excluded.content_hash,
		     updated_at   = excluded.updated_at
		 WHERE path != excluded.path OR content_hash IS NOT excluded.content_hash`,
		uuid, path, contentHash,
	)
	if err != nil {
		return fmt.Errorf("reconcile: upsert uuid=%q path=%q: %w", uuid, path, err)
	}
	return nil
}

// Forget removes the note_paths row whose path equals the given value.
// It is a no-op (no error) when no such row exists.
// Only the row for that exact path is removed; other rows are untouched.
func Forget(ctx context.Context, g *graph.Graph, path string) error {
	return g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return forgetInTx(tx, path)
	})
}

// forgetInTx deletes by path inside a caller-supplied write tx.
func forgetInTx(tx *graph.WriteTx, path string) error {
	_, err := tx.Exec(`DELETE FROM note_paths WHERE path = ?`, path)
	if err != nil {
		return fmt.Errorf("reconcile: forget path %q: %w", path, err)
	}
	return nil
}

// FindByContentHash returns the UUID of the note_paths row whose content_hash
// matches hash. Returns found=false (no error) when no row matches.
//
// This is the content-hash tiebreak primitive consumed by U11's move/revive
// window: a UUID-less file whose hash matches a known/tombstoned node is a
// move candidate rather than a fresh node.
func FindByContentHash(ctx context.Context, g *graph.Graph, hash string) (uuid string, found bool, err error) {
	err = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		uuid, found, err = findByContentHashInTx(tx, hash)
		return err
	})
	return
}

// findByContentHashInTx is the inner hash→uuid query.
func findByContentHashInTx(tx interface {
	QueryRow(string, ...any) *sql.Row
}, hash string) (string, bool, error) {
	var uuid string
	err := tx.QueryRow(`SELECT uuid FROM note_paths WHERE content_hash = ? LIMIT 1`, hash).Scan(&uuid)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("reconcile: find by hash: %w", err)
	}
	return uuid, true, nil
}

// --- Content-hash helpers ---

// HashFile computes the SHA-256 hex digest of the file at the given path.
// Used by U7/U11 before calling Upsert so the hash is always consistent.
func HashFile(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("reconcile: hash file %q: %w", filePath, err)
	}
	defer f.Close()
	return hashReader(f)
}

// HashBytes computes the SHA-256 hex digest of a byte slice.
// Convenience for callers that already have the content in memory.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hashReader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", fmt.Errorf("reconcile: hash: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
