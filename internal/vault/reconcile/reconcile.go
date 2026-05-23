// Package reconcile provides the path↔uuid cache layer over the note_paths
// table, and the create/change event handlers that turn filesystem events into
// graph mutations (U7). Identity is the UUID; path is a disposable cache entry.
package reconcile

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/vault/frontmatter"
	"github.com/gabrielassisxyz/kernl/internal/vault/wikilink"
	"github.com/google/uuid"
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

// --- Reconciler ---

// Stats captures in-memory counters exposed by the Reconciler for diagnostics.
type Stats struct {
	EventsProcessed int64
	NotesCreated    int64
	DanglingPromoted int64
}

// Reconciler processes create/change events from the watcher and mutates the graph.
// It owns the P0.1 chokepoint for note creation, revision recording, wikilink
// resolution, and path-cache maintenance.
type Reconciler struct {
	g         *graph.Graph
	vaultRoot string // absolute path to the vault root for relative path resolution
	resolver  *wikilink.Resolver

	// counters — updated atomically
	eventsProcessed  atomic.Int64
	notesCreated     atomic.Int64
	danglingPromoted atomic.Int64
}

// New creates a Reconciler for the given graph and vault root.
func New(g *graph.Graph, vaultRoot string) *Reconciler {
	abs, err := filepath.Abs(vaultRoot)
	if err != nil {
		abs = vaultRoot
	}
	return &Reconciler{
		g:         g,
		vaultRoot: abs,
		resolver:  &wikilink.Resolver{},
	}
}

// Stats returns a snapshot of the reconciler's in-memory counters.
func (r *Reconciler) Stats() Stats {
	return Stats{
		EventsProcessed: r.eventsProcessed.Load(),
		NotesCreated:    r.notesCreated.Load(),
		DanglingPromoted: r.danglingPromoted.Load(),
	}
}

// OnCreate handles a KindCreate event for absPath. It:
//  1. Reads the file bytes.
//  2. Parses frontmatter; injects a UUID if absent (writes back to disk).
//  3. In ONE DoWrite: CreateNote, resolves wikilinks, promotes dangling links
//     that now resolve to this note, and upserts the path-cache entry.
func (r *Reconciler) OnCreate(ctx context.Context, absPath string) error {
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("reconcile OnCreate %q: read file: %w", absPath, err)
	}

	fm, raw, err := parseAndInject(absPath, raw)
	if err != nil {
		return fmt.Errorf("reconcile OnCreate %q: frontmatter: %w", absPath, err)
	}

	author := resolveAuthor(fm.Author)
	title := resolveTitle(fm, absPath)
	body := extractBody(raw)
	relPath := r.relPath(absPath)
	contentHash := HashBytes(raw)
	noteID := fm.ID

	var promoted int
	err = r.g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		n := nodes.Note{
			ID:     noteID,
			Title:  title,
			Body:   body,
			Origin: fm.Origin,
			Author: fm.Author,
			Tags:   fm.Tags,
		}

		id, err := nodes.CreateNote(ctx, tx, n, author)
		if err != nil {
			return fmt.Errorf("CreateNote: %w", err)
		}
		noteID = id

		// Resolve wikilinks in body
		if _, err := r.resolver.ResolveInTx(ctx, tx, noteID, body); err != nil {
			return fmt.Errorf("ResolveInTx: %w", err)
		}

		// Promote any previously-dangling links pointing at this note
		keys := danglingKeysFor(title, absPath)
		p, err := wikilink.PromoteDanglingInTx(ctx, tx, noteID, keys...)
		if err != nil {
			return fmt.Errorf("PromoteDanglingInTx: %w", err)
		}
		promoted = p

		// Upsert path cache
		if err := upsertInTx(tx, noteID, relPath, contentHash); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("reconcile OnCreate %q: %w", absPath, err)
	}

	r.eventsProcessed.Add(1)
	r.notesCreated.Add(1)
	r.danglingPromoted.Add(int64(promoted))

	slog.Info("reconcile: create",
		"event", "create",
		"path", absPath,
		"node_id", noteID,
		"decision", "create",
		"dangling_promoted", promoted,
	)
	return nil
}

// OnChange handles a KindChange event for absPath. It:
//  1. Reads the current file bytes.
//  2. Parses frontmatter (UUID must already exist; if missing, treats as create).
//  3. In ONE DoWrite: UpdateNote (diff revision), re-resolves wikilinks,
//     and refreshes the path-cache content hash.
func (r *Reconciler) OnChange(ctx context.Context, absPath string) error {
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("reconcile OnChange %q: read file: %w", absPath, err)
	}

	fm, raw, err := parseAndInject(absPath, raw)
	if err != nil {
		return fmt.Errorf("reconcile OnChange %q: frontmatter: %w", absPath, err)
	}

	// If there's still no UUID after inject (very unusual), fall back to create.
	if fm.ID == "" {
		return r.OnCreate(ctx, absPath)
	}

	author := resolveAuthor(fm.Author)
	title := resolveTitle(fm, absPath)
	body := extractBody(raw)
	relPath := r.relPath(absPath)
	contentHash := HashBytes(raw)

	err = r.g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		n := nodes.Note{
			ID:     fm.ID,
			Title:  title,
			Body:   body,
			Origin: fm.Origin,
			Author: fm.Author,
			Tags:   fm.Tags,
		}
		if err := nodes.UpdateNote(ctx, tx, n, author); err != nil {
			return fmt.Errorf("UpdateNote: %w", err)
		}

		// Re-resolve wikilinks
		if _, err := r.resolver.ResolveInTx(ctx, tx, fm.ID, body); err != nil {
			return fmt.Errorf("ResolveInTx: %w", err)
		}

		// Refresh path cache
		if err := upsertInTx(tx, fm.ID, relPath, contentHash); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("reconcile OnChange %q: %w", absPath, err)
	}

	r.eventsProcessed.Add(1)

	slog.Info("reconcile: change",
		"event", "change",
		"path", absPath,
		"node_id", fm.ID,
		"decision", "change",
	)
	return nil
}

// --- helpers ---

// parseAndInject reads frontmatter and injects a UUID if absent.
// It writes the updated bytes back to disk only when injection was needed.
// Returns the updated Frontmatter and (possibly updated) raw bytes.
func parseAndInject(absPath string, raw []byte) (*frontmatter.Frontmatter, []byte, error) {
	fm, err := frontmatter.Parse(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("parse: %w", err)
	}

	if fm.ID == "" {
		newUUID := uuid.Must(uuid.NewV7()).String()
		updated, err := frontmatter.InjectID(raw, newUUID)
		if err != nil {
			return nil, nil, fmt.Errorf("inject id: %w", err)
		}
		if err := os.WriteFile(absPath, updated, 0o644); err != nil {
			return nil, nil, fmt.Errorf("write injected id: %w", err)
		}
		raw = updated
		fm.ID = newUUID
	}
	return fm, raw, nil
}

// resolveAuthor maps frontmatter author to a nodes.Author following R15/AE6:
//   - absent / empty → Author{Name: "human"}
//   - already prefixed "agent:*" → preserved
//   - "da" → agent:da
//   - any other value → Author{Name: value} (treated as human identifier)
func resolveAuthor(fmAuthor string) nodes.Author {
	if fmAuthor == "" {
		return nodes.Author{Name: "human"}
	}
	if strings.HasPrefix(fmAuthor, "agent:") {
		return nodes.Author{Name: fmAuthor}
	}
	if fmAuthor == "da" {
		return nodes.AuthorAgent("da")
	}
	return nodes.Author{Name: fmAuthor}
}

// resolveTitle returns the title from frontmatter or falls back to the filename stem.
func resolveTitle(fm *frontmatter.Frontmatter, absPath string) string {
	if fm.Title != "" {
		return fm.Title
	}
	base := filepath.Base(absPath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// extractBody returns the content after the frontmatter block.
// If no frontmatter is present, returns the full content.
func extractBody(raw []byte) string {
	// Strip BOM
	content := raw
	if len(content) >= 3 && content[0] == 0xEF && content[1] == 0xBB && content[2] == 0xBF {
		content = content[3:]
	}

	// Must start with "---\n" or "---\r\n"
	if len(content) < 4 || content[0] != '-' || content[1] != '-' || content[2] != '-' {
		return string(raw)
	}
	lineEnd := 3
	if lineEnd < len(content) && content[lineEnd] == '\r' {
		lineEnd++
	}
	if lineEnd >= len(content) || content[lineEnd] != '\n' {
		return string(raw)
	}
	// Find the closing "---" fence
	start := lineEnd + 1
	for i := start; i < len(content)-2; i++ {
		if content[i] == '-' && content[i+1] == '-' && content[i+2] == '-' {
			// Must be preceded by newline
			if i > 0 && content[i-1] == '\n' {
				// Find end of closing fence line
				j := i + 3
				if j < len(content) && content[j] == '\r' {
					j++
				}
				if j < len(content) && content[j] == '\n' {
					j++
				}
				return string(bytes.TrimLeft(content[j:], "\n\r"))
			}
		}
	}
	return string(raw)
}

// danglingKeysFor returns the PromoteKeys for a note: stem and title.
// Stem is derived from the filename; title is the resolved display title.
func danglingKeysFor(title, absPath string) []wikilink.PromoteKey {
	base := filepath.Base(absPath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))

	var keys []wikilink.PromoteKey
	keys = append(keys, wikilink.PromoteKey{Key: stem, Kind: "stem"})
	if title != "" && title != stem {
		keys = append(keys, wikilink.PromoteKey{Key: title, Kind: "title"})
	}
	return keys
}

// relPath returns the path relative to the vault root.
// If absPath does not start with vaultRoot, returns absPath unchanged.
func (r *Reconciler) relPath(absPath string) string {
	rel, err := filepath.Rel(r.vaultRoot, absPath)
	if err != nil {
		return absPath
	}
	return rel
}
