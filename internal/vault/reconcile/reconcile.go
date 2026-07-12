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
	"sync"
	"sync/atomic"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/tagname"
	"github.com/gabrielassisxyz/kernl/internal/graph/tags"
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

// defaultDeleteWindow is the default move/delete correlation window.
// A delete is held for this duration before being tombstoned. A same-UUID or
// same-hash create within the window cancels the tombstone (move semantics).
const defaultDeleteWindow = time.Second

// pendingDelete holds the metadata for a delete that has not yet been tombstoned.
type pendingDelete struct {
	deletedAt   time.Time
	contentHash string
	relPath     string // vault-relative path at time of delete
	nodeID      string
	stem        string // filename stem at time of delete
	title       string // node title at time of delete
}

// Stats captures in-memory counters exposed by the Reconciler for diagnostics.
type Stats struct {
	EventsProcessed  int64
	NotesCreated     int64
	DanglingPromoted int64
	NotesTombstoned  int64
	NotesRevived     int64
}

// Reconciler processes create/change/delete events from the watcher and
// mutates the graph. It owns the chokepoint for note creation, revision
// recording, wikilink resolution, path-cache maintenance, and
// move/delete-window correlation.
type Reconciler struct {
	g         *graph.Graph
	vaultRoot string // absolute path to the vault root for relative path resolution
	resolver  *wikilink.Resolver

	// move/delete window — injectable for deterministic testing
	window time.Duration
	now    func() time.Time

	pendingMu      sync.Mutex
	pendingDeletes map[string]*pendingDelete // keyed by nodeID

	// counters — updated atomically
	eventsProcessed  atomic.Int64
	notesCreated     atomic.Int64
	danglingPromoted atomic.Int64
	notesTombstoned  atomic.Int64
	notesRevived     atomic.Int64
}

// New creates a Reconciler for the given graph and vault root.
func New(g *graph.Graph, vaultRoot string) *Reconciler {
	abs, err := filepath.Abs(vaultRoot)
	if err != nil {
		abs = vaultRoot
	}
	return &Reconciler{
		g:              g,
		vaultRoot:      abs,
		resolver:       &wikilink.Resolver{},
		window:         defaultDeleteWindow,
		now:            time.Now,
		pendingDeletes: make(map[string]*pendingDelete),
	}
}

// SetDeleteWindow overrides the move/delete correlation window.
// Useful in tests to make the window expire immediately.
func (r *Reconciler) SetDeleteWindow(d time.Duration) {
	r.window = d
}

// SetClock overrides the clock used for pending-delete timestamps.
// Used in tests to advance time deterministically.
func (r *Reconciler) SetClock(fn func() time.Time) {
	r.now = fn
}

// Stats returns a snapshot of the reconciler's in-memory counters.
func (r *Reconciler) Stats() Stats {
	return Stats{
		EventsProcessed:  r.eventsProcessed.Load(),
		NotesCreated:     r.notesCreated.Load(),
		DanglingPromoted: r.danglingPromoted.Load(),
		NotesTombstoned:  r.notesTombstoned.Load(),
		NotesRevived:     r.notesRevived.Load(),
	}
}

// OnDelete handles a KindDelete event for absPath.
//
// It resolves the path to a UUID via the path cache, records a PENDING delete
// (using the injected clock), and forgets the path from the cache. It does NOT
// tombstone the node yet — that happens in FlushExpired after the window elapses.
// If the path is unknown (no cache entry), the call is a no-op.
func (r *Reconciler) OnDelete(ctx context.Context, absPath string) error {
	relPath := r.relPath(absPath)

	// Resolve path → UUID + nodeID inside a read tx
	var nodeUUID, nodeID, contentHash, nodeTitle string
	err := r.g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var found bool
		var err error
		nodeUUID, found, err = lookupInTx(tx, relPath)
		if err != nil {
			return fmt.Errorf("lookup path %q: %w", relPath, err)
		}
		if !found {
			return nil // unknown path — no-op
		}
		nodeID = nodeUUID // node id == frontmatter uuid

		// Read title for the pending-delete record
		err = tx.QueryRow(
			`SELECT title FROM nodes WHERE id = ? AND type = 'note' AND deleted_at IS NULL`,
			nodeID,
		).Scan(&nodeTitle)
		if err == sql.ErrNoRows {
			nodeID = "" // already tombstoned or not a note
			return nil
		}
		if err != nil {
			return fmt.Errorf("read note title %q: %w", nodeID, err)
		}

		// Read content hash from path cache
		err = tx.QueryRow(
			`SELECT content_hash FROM note_paths WHERE uuid = ?`,
			nodeUUID,
		).Scan(&contentHash)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("read content hash %q: %w", nodeUUID, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("reconcile OnDelete %q: %w", absPath, err)
	}

	if nodeID == "" {
		// No live note found for this path — nothing to do
		return nil
	}

	// Forget path from cache (synchronous — outside the window logic)
	if err := forgetPath(ctx, r.g, relPath); err != nil {
		return fmt.Errorf("reconcile OnDelete %q: forget: %w", absPath, err)
	}

	stem := stemFromPath(absPath)

	r.pendingMu.Lock()
	r.pendingDeletes[nodeID] = &pendingDelete{
		deletedAt:   r.now(),
		contentHash: contentHash,
		relPath:     relPath,
		nodeID:      nodeID,
		stem:        stem,
		title:       nodeTitle,
	}
	r.pendingMu.Unlock()

	r.eventsProcessed.Add(1)

	slog.Info("reconcile: delete pending",
		"event", "delete",
		"path", absPath,
		"node_id", nodeID,
		"decision", "pending",
	)
	return nil
}

// FlushExpired tombstones all pending deletes whose age exceeds the window.
// Tests call this directly after advancing the injected clock.
// Returns the count of notes tombstoned and any error.
func (r *Reconciler) FlushExpired(ctx context.Context) (tombstoned int, err error) {
	r.pendingMu.Lock()
	now := r.now()
	var expired []*pendingDelete
	for _, pd := range r.pendingDeletes {
		if now.Sub(pd.deletedAt) >= r.window {
			expired = append(expired, pd)
		}
	}
	// Remove from map now (before unlock) so concurrent callers don't double-tombstone
	for _, pd := range expired {
		delete(r.pendingDeletes, pd.nodeID)
	}
	r.pendingMu.Unlock()

	for _, pd := range expired {
		author := nodes.Author{Name: "human"}
		txErr := r.g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			return nodes.SoftDeleteNoteTx(ctx, tx, pd.nodeID, pd.stem, pd.title, author)
		})
		if txErr != nil {
			// If the node is already gone (e.g. concurrent revive), skip
			if txErr == graph.ErrNotFound {
				continue
			}
			err = fmt.Errorf("reconcile FlushExpired node %q: %w", pd.nodeID, txErr)
			return tombstoned, err
		}
		tombstoned++
		r.notesTombstoned.Add(1)

		slog.Info("reconcile: tombstone",
			"event", "delete",
			"node_id", pd.nodeID,
			"path", pd.relPath,
			"decision", "tombstone",
		)
	}
	return tombstoned, nil
}

// OnCreate handles a KindCreate event for absPath. It:
//  1. Reads the file bytes.
//  2. Parses frontmatter; injects a UUID if absent (writes back to disk).
//  3. Checks whether a pending delete matches by UUID or content hash
//     (move/revive detection).
//  4. In ONE DoWrite: either handles the create as a move/revive or as a
//     fresh create, resolves wikilinks, promotes dangling links, and upserts
//     the path-cache entry.
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

	// --- Move/revive correlation ---
	// Check whether this create matches a pending delete (move detection)
	// or a tombstoned node that is returning (revive-after-expiry).
	pending := r.consumePendingDelete(noteID, contentHash)

	if pending != nil {
		// Move: cancel the tombstone, update path cache, preserve identity.
		var promoted int
		err = r.g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			// Check if the node was tombstoned already (race: expiry ran before us)
			tombstoned, err := nodes.IsNoteTombstoned(ctx, tx, pending.nodeID)
			if err != nil {
				return fmt.Errorf("IsNoteTombstoned: %w", err)
			}

			if tombstoned {
				// Revive path: the expiry timer fired before we could cancel it
				if err := nodes.ReviveNoteTx(ctx, tx, pending.nodeID, pending.stem, title, author); err != nil {
					return fmt.Errorf("ReviveNoteTx: %w", err)
				}
				r.notesRevived.Add(1)
			}
			// If not tombstoned, just update the path cache — the node is live

			// Update note's title/body if the frontmatter changed
			if err := nodes.UpdateNote(ctx, tx, nodes.Note{
				ID:     pending.nodeID,
				Title:  title,
				Body:   body,
				Origin: fm.Origin,
				Author: fm.Author,
				Tags:   fm.Tags,
			}, author); err != nil {
				return fmt.Errorf("UpdateNote (move): %w", err)
			}

			// Re-resolve wikilinks
			if _, err := r.resolver.ResolveInTx(ctx, tx, pending.nodeID, body); err != nil {
				return fmt.Errorf("ResolveInTx (move): %w", err)
			}

			// Promote any dangling links that now point here
			keys := danglingKeysFor(title, absPath)
			p, err := wikilink.PromoteDanglingInTx(ctx, tx, pending.nodeID, keys...)
			if err != nil {
				return fmt.Errorf("PromoteDanglingInTx (move): %w", err)
			}
			promoted = p

			// Upsert path cache to new location
			if err := upsertInTx(tx, pending.nodeID, relPath, contentHash); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("reconcile OnCreate (move) %q: %w", absPath, err)
		}

		r.eventsProcessed.Add(1)
		r.danglingPromoted.Add(int64(promoted))

		slog.Info("reconcile: move",
			"event", "create",
			"path", absPath,
			"node_id", pending.nodeID,
			"decision", "move",
			"dangling_promoted", promoted,
		)
		return nil
	}

	// Check whether an already-tombstoned node with this UUID is coming back
	if noteID != "" {
		var wasTombstoned bool
		checkErr := r.g.DoRead(ctx, func(tx *graph.ReadTx) error {
			var err error
			wasTombstoned, err = nodes.IsNoteTombstoned(ctx, tx, noteID)
			return err
		})
		if checkErr != nil {
			return fmt.Errorf("reconcile OnCreate %q: tombstone check: %w", absPath, checkErr)
		}

		if wasTombstoned {
			// Revive after expiry — the pending window already fired
			var promoted int
			stem := stemFromPath(absPath)
			err = r.g.DoWrite(ctx, func(tx *graph.WriteTx) error {
				if err := nodes.ReviveNoteTx(ctx, tx, noteID, stem, title, author); err != nil {
					return fmt.Errorf("ReviveNoteTx: %w", err)
				}
				// Update title/body to the new file content
				if err := nodes.UpdateNote(ctx, tx, nodes.Note{
					ID:     noteID,
					Title:  title,
					Body:   body,
					Origin: fm.Origin,
					Author: fm.Author,
					Tags:   fm.Tags,
				}, author); err != nil {
					return fmt.Errorf("UpdateNote (revive): %w", err)
				}

				keys := danglingKeysFor(title, absPath)
				p, err := wikilink.PromoteDanglingInTx(ctx, tx, noteID, keys...)
				if err != nil {
					return fmt.Errorf("PromoteDanglingInTx (revive): %w", err)
				}
				promoted = p

				if _, err := r.resolver.ResolveInTx(ctx, tx, noteID, body); err != nil {
					return fmt.Errorf("ResolveInTx (revive): %w", err)
				}

				if err := upsertInTx(tx, noteID, relPath, contentHash); err != nil {
					return err
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("reconcile OnCreate (revive) %q: %w", absPath, err)
			}

			r.eventsProcessed.Add(1)
			r.notesRevived.Add(1)
			r.danglingPromoted.Add(int64(promoted))

			slog.Info("reconcile: revive",
				"event", "create",
				"path", absPath,
				"node_id", noteID,
				"decision", "revive",
				"dangling_promoted", promoted,
			)
			return nil
		}
	}

	// --- Fresh create ---
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
	fm.Tags = sanitizeTags(absPath, fm.Tags)

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

// sanitizeTags strips the tags a note's frontmatter must not put into the
// graph: reserved `sys/` tags, and names that break the nesting convention
// (`foo//bar`, `/foo`). The vault is the source of truth for a note's tags, so
// without this a user typing `tags: [sys/pending]` into a markdown file would
// mint a system tag in the graph and, for a capture, forge its way back into
// the inbox queue. Offenders are dropped rather than failing the reconcile: a
// malformed tag must not cost the user the rest of the file's content.
func sanitizeTags(absPath string, fmTags []string) []string {
	kept := make([]string, 0, len(fmTags))
	for _, t := range fmTags {
		if tags.IsSystem(t) {
			slog.Warn("reconcile: dropped reserved system tag from frontmatter",
				"path", absPath,
				"tag", t,
			)
			continue
		}
		if err := tagname.Validate(t); err != nil {
			slog.Warn("reconcile: dropped malformed tag from frontmatter",
				"path", absPath,
				"tag", t,
				"error", err,
			)
			continue
		}
		kept = append(kept, t)
	}
	return kept
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

// WriteNoteBody rewrites the body of a note's vault markdown file in place,
// preserving its frontmatter, so the file (the source of truth) reflects an
// in-graph body change. Callers that mutate a note's body directly via
// nodes.UpdateNote (e.g. ingest/inbox merges) MUST also call this — otherwise
// the file diverges and the merge is clobbered the next time the file is
// reconciled (OnChange re-derives the node body from the stale file).
//
// It returns written=false (no error) when the file cannot be located — no
// vault root, or no path cached and no frontmatter id match. The caller's
// node-level update still stands; only the file mirror is skipped.
func WriteNoteBody(ctx context.Context, g *graph.Graph, vaultRoot, noteID, newBody string) (written bool, err error) {
	if vaultRoot == "" {
		return false, nil
	}
	full, ok, err := locateNoteFile(ctx, g, vaultRoot, noteID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	raw, err := os.ReadFile(full)
	if err != nil {
		return false, fmt.Errorf("reconcile: read note %q: %w", full, err)
	}
	// extractBody returns the body region (a suffix of the file), so trimming it
	// off leaves the frontmatter block plus its separator untouched.
	body := extractBody(raw)
	prefix := strings.TrimSuffix(string(raw), body)
	if err := os.WriteFile(full, []byte(prefix+newBody), 0644); err != nil {
		return false, fmt.Errorf("reconcile: write note %q: %w", full, err)
	}
	return true, nil
}

// locateNoteFile resolves a note's absolute file path: first via the note_paths
// cache, then by scanning for a markdown whose frontmatter id matches (the cache
// may not be populated yet for a freshly created note).
func locateNoteFile(ctx context.Context, g *graph.Graph, vaultRoot, noteID string) (string, bool, error) {
	if rel, found, err := LookupByUUID(ctx, g, noteID); err != nil {
		return "", false, err
	} else if found && rel != "" {
		return filepath.Join(vaultRoot, rel), true, nil
	}

	var match string
	_ = filepath.WalkDir(vaultRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if fm, perr := frontmatter.Parse(raw); perr == nil && fm.ID == noteID {
			match = path
			return filepath.SkipAll
		}
		return nil
	})
	if match == "" {
		return "", false, nil
	}
	return match, true, nil
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

// consumePendingDelete checks whether a pending delete matches the given UUID or
// content hash. If a match is found, the entry is removed from pendingDeletes and
// returned so the caller can handle it as a move/revive.
func (r *Reconciler) consumePendingDelete(noteID, contentHash string) *pendingDelete {
	r.pendingMu.Lock()
	defer r.pendingMu.Unlock()

	// Primary match: same UUID
	if noteID != "" {
		if pd, ok := r.pendingDeletes[noteID]; ok {
			delete(r.pendingDeletes, noteID)
			return pd
		}
	}

	// Secondary match: content-hash tiebreak (UUID-less files)
	if contentHash != "" {
		for id, pd := range r.pendingDeletes {
			if pd.contentHash == contentHash {
				delete(r.pendingDeletes, id)
				return pd
			}
		}
	}

	return nil
}

// forgetPath removes a path from the note_paths cache.
func forgetPath(ctx context.Context, g *graph.Graph, relPath string) error {
	return g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		return forgetInTx(tx, relPath)
	})
}

// stemFromPath returns the filename stem (without extension) for an absolute path.
func stemFromPath(absPath string) string {
	base := filepath.Base(absPath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// cachedPathEntry is one row from note_paths used by ColdStart.
type cachedPathEntry struct {
	uuid        string
	path        string
	contentHash string
}

// listAllCachedPaths returns every row in note_paths as a slice.
// Used by ColdStart to build the disappeared-UUID set.
func listAllCachedPaths(ctx context.Context, g *graph.Graph) ([]cachedPathEntry, error) {
	var out []cachedPathEntry
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		rows, err := tx.Query(`SELECT uuid, path, content_hash FROM note_paths`)
		if err != nil {
			return fmt.Errorf("listAllCachedPaths: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var e cachedPathEntry
			var hash sql.NullString
			if err := rows.Scan(&e.uuid, &e.path, &hash); err != nil {
				return fmt.Errorf("listAllCachedPaths: scan: %w", err)
			}
			if hash.Valid {
				e.contentHash = hash.String
			}
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, err
}
