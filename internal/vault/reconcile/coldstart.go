package reconcile

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/vault/frontmatter"
)

// ColdStart reconciles the vault on startup against the path cache and graph.
//
// It walks the vault root, classifies each .md file against the stored state,
// and applies the minimum mutations to make the graph match disk:
//
//   - UUID unknown to the graph → OnCreate (injects UUID if absent, creates node).
//   - Known UUID, same path, hash unchanged → no-op.
//   - Known UUID, same path, hash changed → OnChange (records one diff revision).
//   - Known UUID at a new path → move: path cache updated; OnChange if hash also changed.
//
// After the walk, disappeared entries (UUIDs in note_paths with no matching
// on-disk file) are soft-deleted immediately (no move window — the watcher is
// not running yet).
//
// Idempotency: calling ColdStart twice with no intervening disk changes is a
// strict no-op: no revisions are added, no cache rows are mutated, no UUID is
// re-injected.
func (r *Reconciler) ColdStart(ctx context.Context) error {
	// --- Phase 1: snapshot the stored cache ---
	// Build uuid → cachedPathEntry map from note_paths.
	cached, err := listAllCachedPaths(ctx, r.g)
	if err != nil {
		return fmt.Errorf("coldstart: list cached paths: %w", err)
	}
	// byUUID: uuid → entry (for move/same-file detection)
	byUUID := make(map[string]cachedPathEntry, len(cached))
	// byHash: content_hash → uuid (for hash-tiebreak move detection)
	byHash := make(map[string]string, len(cached))
	for _, e := range cached {
		byUUID[e.uuid] = e
		if e.contentHash != "" {
			byHash[e.contentHash] = e.uuid
		}
	}

	// --- Phase 2: walk vault, classify, act ---
	// seenUUIDs collects every UUID that is live on disk after the walk.
	seenUUIDs := make(map[string]struct{})
	// hashTakenBy maps content_hash → uuid for files that were already processed as creates.
	// Used so a "new" file with the same hash as a disappeared UUID is treated as a move.
	hashTakenBy := make(map[string]string)

	walkErr := filepath.WalkDir(r.vaultRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Permission errors etc. — skip, do not abort.
			return nil
		}
		// Skip dotfiles and dotdirs (mirrors watcher U6 filter).
		if strings.HasPrefix(d.Name(), ".") && path != r.vaultRoot {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}

		return r.reconcileFile(ctx, path, byUUID, byHash, seenUUIDs, hashTakenBy)
	})
	if walkErr != nil {
		return fmt.Errorf("coldstart: walk: %w", walkErr)
	}

	// --- Phase 3: tombstone disappeared entries ---
	for _, e := range cached {
		if _, seen := seenUUIDs[e.uuid]; seen {
			continue
		}
		// UUID was in cache but never seen on disk.
		// Check if it was already handled as a move (its hash appeared in a "new" file).
		if e.contentHash != "" {
			if claimant, ok := hashTakenBy[e.contentHash]; ok && claimant == e.uuid {
				// The file was reconciled as a move in reconcileFile; skip tombstone.
				continue
			}
		}
		// Soft-delete immediately: no live watcher stream, no window needed.
		if err := r.softDeleteCached(ctx, e); err != nil {
			return fmt.Errorf("coldstart: tombstone uuid=%q: %w", e.uuid, err)
		}
	}

	return nil
}

// reconcileFile classifies and acts on one on-disk .md file.
// It updates seenUUIDs and hashTakenBy for phase-3 use.
func (r *Reconciler) reconcileFile(
	ctx context.Context,
	absPath string,
	byUUID map[string]cachedPathEntry,
	byHash map[string]string,
	seenUUIDs map[string]struct{},
	hashTakenBy map[string]string,
) error {
	raw, err := os.ReadFile(absPath)
	if err != nil {
		slog.Warn("coldstart: read file failed", "path", absPath, "error", err)
		return nil
	}

	fm, parseErr := frontmatter.Parse(raw)
	if parseErr != nil {
		slog.Warn("coldstart: frontmatter parse failed", "path", absPath, "error", parseErr)
		return nil
	}

	diskHash := HashBytes(raw)
	relPath := r.relPath(absPath)

	// --- Classify ---

	// Case A: UUID present and known to the cache.
	if fm.ID != "" {
		if entry, known := byUUID[fm.ID]; known {
			seenUUIDs[fm.ID] = struct{}{}

			sameHash := entry.contentHash == diskHash
			samePath := entry.path == relPath

			switch {
			case samePath && sameHash:
				// Exact no-op: nothing changed.
				slog.Debug("coldstart: no-op", "path", absPath, "uuid", fm.ID)

			case samePath && !sameHash:
				// Content changed, same location.
				slog.Debug("coldstart: change", "path", absPath, "uuid", fm.ID)
				if err := r.OnChange(ctx, absPath); err != nil {
					return fmt.Errorf("OnChange %q: %w", absPath, err)
				}

			case !samePath:
				// Path moved while off.
				if err := r.applyMove(ctx, entry, absPath, relPath, diskHash, raw, sameHash); err != nil {
					return fmt.Errorf("move uuid=%q: %w", fm.ID, err)
				}
			}
			return nil
		}

		// UUID present but not in cache — node may be tombstoned or truly new.
		// Check graph for a tombstoned node with this UUID.
		var tombstoned bool
		checkErr := r.g.DoRead(ctx, func(tx *graph.ReadTx) error {
			var e error
			tombstoned, e = nodes.IsNoteTombstoned(ctx, tx, fm.ID)
			return e
		})
		if checkErr != nil {
			return fmt.Errorf("tombstone check %q: %w", fm.ID, checkErr)
		}
		if !tombstoned {
			// Genuinely new — fall through to create below.
		} else {
			// Tombstoned node returning — let OnCreate handle revival.
			seenUUIDs[fm.ID] = struct{}{}
			slog.Debug("coldstart: revive (tombstoned)", "path", absPath, "uuid", fm.ID)
			if err := r.OnCreate(ctx, absPath); err != nil {
				return fmt.Errorf("OnCreate (revive) %q: %w", absPath, err)
			}
			return nil
		}
	}

	// Case B: UUID absent OR UUID not in cache and not tombstoned.
	// Before treating as a fresh create, check if the content hash matches a
	// cached entry whose UUID has not been seen yet — that is a move.
	if movedFromUUID, ok := byHash[diskHash]; ok {
		if _, alreadySeen := seenUUIDs[movedFromUUID]; !alreadySeen {
			oldEntry := byUUID[movedFromUUID]
			seenUUIDs[movedFromUUID] = struct{}{}
			// Mark hash as taken by this UUID so phase-3 skips tombstone.
			hashTakenBy[diskHash] = movedFromUUID
			slog.Debug("coldstart: move (hash match)", "path", absPath, "uuid", movedFromUUID)
			sameHash := true // by definition — hash matched
			if err := r.applyMove(ctx, oldEntry, absPath, relPath, diskHash, raw, sameHash); err != nil {
				return fmt.Errorf("move (hash) uuid=%q: %w", movedFromUUID, err)
			}
			return nil
		}
	}

	// Fresh create (also handles UUID injection if missing).
	if fm.ID != "" {
		seenUUIDs[fm.ID] = struct{}{}
	}
	slog.Debug("coldstart: create", "path", absPath)
	if err := r.OnCreate(ctx, absPath); err != nil {
		return fmt.Errorf("OnCreate %q: %w", absPath, err)
	}
	// After OnCreate the UUID may have been injected; re-read and mark seen.
	if fm.ID == "" {
		raw2, readErr := os.ReadFile(absPath)
		if readErr == nil {
			fm2, parseErr2 := frontmatter.Parse(raw2)
			if parseErr2 == nil && fm2.ID != "" {
				seenUUIDs[fm2.ID] = struct{}{}
			}
		}
	}
	return nil
}

// applyMove updates the path cache for a known UUID that moved to a new path.
// If the hash is also different (sameHash==false), it additionally calls OnChange
// to record the content revision.
func (r *Reconciler) applyMove(
	ctx context.Context,
	old cachedPathEntry,
	absPath, newRelPath, diskHash string,
	_ []byte, // raw — not used here but kept for signature clarity
	sameHash bool,
) error {
	// Update path cache: forget old path, upsert new path.
	if err := r.g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		if err := forgetInTx(tx, old.path); err != nil {
			return err
		}
		return upsertInTx(tx, old.uuid, newRelPath, diskHash)
	}); err != nil {
		return fmt.Errorf("applyMove cache update: %w", err)
	}

	slog.Info("coldstart: move",
		"uuid", old.uuid,
		"old_path", old.path,
		"new_path", newRelPath,
	)

	if !sameHash {
		// Body also changed — record a diff revision via OnChange.
		// OnChange reads the file fresh and re-hashes, so no duplication.
		if err := r.OnChange(ctx, absPath); err != nil {
			return fmt.Errorf("applyMove OnChange: %w", err)
		}
	}
	return nil
}

// softDeleteCached tombstones a note_paths entry that no longer exists on disk.
// It derives the stem and title needed by SoftDeleteNoteTx from the cache row
// and the live node, then calls SoftDeleteNoteTx inside a single write tx.
func (r *Reconciler) softDeleteCached(ctx context.Context, e cachedPathEntry) error {
	stem := stemFromRelPath(e.path)

	// Read the live node title (if any).
	var title string
	err := r.g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(
			`SELECT title FROM nodes WHERE id = ? AND type = 'note' AND deleted_at IS NULL`,
			e.uuid,
		).Scan(&title)
	})
	if err != nil {
		// Node doesn't exist or already tombstoned — nothing to do.
		return nil
	}

	author := nodes.Author{Name: "human"}
	txErr := r.g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		// Remove path cache entry.
		if err := forgetInTx(tx, e.path); err != nil {
			return err
		}
		return nodes.SoftDeleteNoteTx(ctx, tx, e.uuid, stem, title, author)
	})
	if txErr != nil {
		if txErr == graph.ErrNotFound {
			return nil // already gone
		}
		return txErr
	}

	r.notesTombstoned.Add(1)
	slog.Info("coldstart: tombstone",
		"uuid", e.uuid,
		"path", e.path,
	)
	return nil
}

// stemFromRelPath returns the filename stem from a vault-relative path.
func stemFromRelPath(relPath string) string {
	base := filepath.Base(relPath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
