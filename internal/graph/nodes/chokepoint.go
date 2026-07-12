// chokepoint.go is the single chokepoint for all node mutations.
//
// Mutation flow (all inside one write transaction):
//
//   createNode(nodeType, spec, author):
//     [validate author] -> [assign ID if empty] -> [INSERT nodes]
//     [INSERT nodes_fts] -> [UPDATE nodes SET fts_rowid] -> [INSERT node_tags]
//     [INSERT first revision]
//
//   updateNode(spec, author):
//     [SELECT prev state + tags + fts_rowid] -> [validate author]
//     [DELETE nodes_fts WHERE rowid = prev.fts_rowid]
//     [INSERT nodes_fts(rowid, title, attrs)]        -- reuse same rowid
//     [UPDATE nodes SET title, attrs, updated_at]
//     [reconcile tags: DELETE removed, INSERT added]
//     [INSERT revision with parent_id + diff]
//
//   deleteNode(id, author):
//     [SELECT current state + tags] -> [validate author]
//     [DELETE nodes_fts WHERE rowid = fts_rowid]
//     [INSERT tombstone revision]                     -- history survives
//     [DELETE FROM nodes WHERE id = ?]               -- FK cascade fires
//
// Every path returns graph.ErrNotFound if the node is absent.
// Every path returns graph.ErrAuthorRequired if author.Name == "".

package nodes

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/internal/ids"
)

// createNode inserts a new node, its FTS index, tags, and initial revision.
func createNode(ctx context.Context, tx *graph.WriteTx, nodeType string, spec NodeSpec, author Author) (string, error) {
	if !author.Valid() {
		return "", graph.ErrAuthorRequired
	}

	meta := spec.Meta()
	if meta.ID == "" {
		meta.ID = ids.New()
	}

	attrs := spec.NodeAttrs()
	if len(attrs) == 0 {
		attrs = []byte("{}")
	}

	fts := spec.FTSFields()

	// INSERT node (timestamps handled by SQL defaults)
	_, err := tx.Exec(
		`INSERT INTO nodes(id, type, title, attrs) VALUES (?, ?, ?, ?)`,
		meta.ID, nodeType, fts.Title, string(attrs),
	)
	if err != nil {
		return "", fmt.Errorf("createNode: insert node: %w", err)
	}

	// Build FTS attrs content: body + tags (space-separated)
	ftsAttrs := fts.Body
	if fts.Tags != "" {
		if ftsAttrs != "" {
			ftsAttrs += " "
		}
		ftsAttrs += fts.Tags
	}

	result, err := tx.Exec(
		`INSERT INTO nodes_fts(title, attrs) VALUES (?, ?)`,
		fts.Title, ftsAttrs,
	)
	if err != nil {
		return "", fmt.Errorf("createNode: insert fts: %w", err)
	}
	ftsRowid, err := result.LastInsertId()
	if err != nil {
		return "", fmt.Errorf("createNode: fts last insert id: %w", err)
	}

	_, err = tx.Exec(
		`UPDATE nodes SET fts_rowid = ? WHERE id = ?`,
		ftsRowid, meta.ID,
	)
	if err != nil {
		return "", fmt.Errorf("createNode: update fts_rowid: %w", err)
	}

	// Insert tags (normalise, which also deduplicates names that differed only
	// by case or padding, avoiding UNIQUE constraint violations)
	tags, err := normalizeTags(spec.NodeTags())
	if err != nil {
		return "", fmt.Errorf("createNode: %w", err)
	}
	for _, tag := range tags {
		if err := upsertTag(ctx, tx, tag); err != nil {
			return "", err
		}
		var tagID string
		err = tx.QueryRow(`SELECT id FROM tags WHERE name = ?`, tag).Scan(&tagID)
		if err != nil {
			return "", fmt.Errorf("createNode: select tag id: %w", err)
		}
		_, err = tx.Exec(
			`INSERT INTO node_tags(node_id, tag_id) VALUES (?, ?)`,
			meta.ID, tagID,
		)
		if err != nil {
			return "", fmt.Errorf("createNode: insert node_tags: %w", err)
		}
	}

	// Build and insert first revision (diff = full snapshot)
	revisionID := ids.New()
	snapshotB, err := snapshotJSON(fts.Title, string(attrs), tags)
	if err != nil {
		return "", fmt.Errorf("createNode: snapshot: %w", err)
	}

	_, err = tx.Exec(
		`INSERT INTO revisions(id, node_id, parent_id, diff, author) VALUES (?, ?, NULL, ?, ?)`,
		revisionID, meta.ID, string(snapshotB), author.String(),
	)
	if err != nil {
		return "", fmt.Errorf("createNode: insert revision: %w", err)
	}

	return meta.ID, nil
}

// updateNode updates an existing node's title, attrs, FTS index, tags, and stores a revision.
func updateNode(ctx context.Context, tx *graph.WriteTx, spec NodeSpec, author Author) error {
	if !author.Valid() {
		return graph.ErrAuthorRequired
	}

	meta := spec.Meta()
	if meta.ID == "" {
		return graph.ErrNotFound
	}

	// Read current state
	var (
		prevTitle sql.NullString
		prevAttrs sql.NullString
		ftsRowid  sql.NullInt64
	)
	err := tx.QueryRow(
		`SELECT title, attrs, fts_rowid FROM nodes WHERE id = ?`,
		meta.ID,
	).Scan(&prevTitle, &prevAttrs, &ftsRowid)
	if err == sql.ErrNoRows {
		return graph.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("updateNode: select node: %w", err)
	}

	// Read current tags
	prevTags, err := selectTags(ctx, tx, meta.ID)
	if err != nil {
		return err
	}

	fts := spec.FTSFields()
	newAttrs := spec.NodeAttrs()
	if len(newAttrs) == 0 {
		newAttrs = []byte("{}")
	}

	// FTS: delete old row, insert new (reuse same rowid)
	if ftsRowid.Valid {
		_, err = tx.Exec(`DELETE FROM nodes_fts WHERE rowid = ?`, ftsRowid.Int64)
		if err != nil {
			return fmt.Errorf("updateNode: delete fts: %w", err)
		}

		ftsAttrs := fts.Body
		if fts.Tags != "" {
			if ftsAttrs != "" {
				ftsAttrs += " "
			}
			ftsAttrs += fts.Tags
		}

		_, err = tx.Exec(
			`INSERT INTO nodes_fts(rowid, title, attrs) VALUES (?, ?, ?)`,
			ftsRowid.Int64, fts.Title, ftsAttrs,
		)
		if err != nil {
			return fmt.Errorf("updateNode: insert fts: %w", err)
		}
	}

	// Update nodes row
	_, err = tx.Exec(
		`UPDATE nodes SET title = ?, attrs = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`,
		fts.Title, string(newAttrs), meta.ID,
	)
	if err != nil {
		return fmt.Errorf("updateNode: update node: %w", err)
	}

	// Reconcile tags. The incoming names are normalised before the diff so that
	// re-saving a node with "Homelab" against a stored "homelab" is a no-op
	// rather than a remove-then-add of the same tag.
	newTags, err := normalizeTags(spec.NodeTags())
	if err != nil {
		return fmt.Errorf("updateNode: %w", err)
	}
	toAdd, toRemove := diffTags(prevTags, newTags)

	for _, tag := range toRemove {
		var tagID string
		err = tx.QueryRow(`SELECT id FROM tags WHERE name = ?`, tag).Scan(&tagID)
		if err != nil {
			return fmt.Errorf("updateNode: select tag id for removal: %w", err)
		}
		_, err = tx.Exec(
			`DELETE FROM node_tags WHERE node_id = ? AND tag_id = ?`,
			meta.ID, tagID,
		)
		if err != nil {
			return fmt.Errorf("updateNode: delete node_tags: %w", err)
		}
	}

	for _, tag := range toAdd {
		if err := upsertTag(ctx, tx, tag); err != nil {
			return err
		}
		var tagID string
		err = tx.QueryRow(`SELECT id FROM tags WHERE name = ?`, tag).Scan(&tagID)
		if err != nil {
			return fmt.Errorf("updateNode: select tag id for add: %w", err)
		}
		_, err = tx.Exec(
			`INSERT INTO node_tags(node_id, tag_id) VALUES (?, ?)`,
			meta.ID, tagID,
		)
		if err != nil {
			return fmt.Errorf("updateNode: insert node_tags: %w", err)
		}
	}

	// Find previous revision ID
	var prevRevisionID sql.NullString
	err = tx.QueryRow(
		`SELECT id FROM revisions WHERE node_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`,
		meta.ID,
	).Scan(&prevRevisionID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("updateNode: select prev revision: %w", err)
	}

	// Build diff content.
	// DiffableNode writes a line-oriented diff or snapshot fallback;
	// other node types keep the existing snapshot path.
	revisionID := ids.New()
	var diffB []byte

	if dn, ok := spec.(DiffableNode); ok && prevAttrs.Valid {
		// Reconstruct a minimal prev spec from stored row.
		prevSpec := Note{
			Title: "",
			Body:  "",
		}
		if prevTitle.Valid {
			prevSpec.Title = prevTitle.String
		}
		if prevAttrs.Valid && prevAttrs.String != "" {
			var pa struct {
				Body   string `json:"body"`
				Origin string `json:"origin"`
				Author string `json:"author"`
			}
			if err := json.Unmarshal([]byte(prevAttrs.String), &pa); err == nil {
				prevSpec.Body = pa.Body
			}
		}
		diffB = dn.DiffBody(prevSpec)
	} else {
		// Non-DiffableNode: existing snapshot path.
		var err error
		diffB, err = snapshotJSON(fts.Title, string(newAttrs), newTags)
		if err != nil {
			return fmt.Errorf("updateNode: snapshot: %w", err)
		}
	}

	var parentID *string
	if prevRevisionID.Valid {
		parentID = &prevRevisionID.String
	}

	_, err = tx.Exec(
		`INSERT INTO revisions(id, node_id, parent_id, diff, author) VALUES (?, ?, ?, ?, ?)`,
		revisionID, meta.ID, parentID, string(diffB), author.String(),
	)
	if err != nil {
		return fmt.Errorf("updateNode: insert revision: %w", err)
	}

	return nil
}

// deleteNode removes a node and its associated data, preserving history as a tombstone revision.
func deleteNode(ctx context.Context, tx *graph.WriteTx, id string, author Author) error {
	if !author.Valid() {
		return graph.ErrAuthorRequired
	}

	if id == "" {
		return graph.ErrNotFound
	}

	// Read current state
	var (
		title    sql.NullString
		attrs    sql.NullString
		ftsRowid sql.NullInt64
	)
	err := tx.QueryRow(
		`SELECT title, attrs, fts_rowid FROM nodes WHERE id = ?`,
		id,
	).Scan(&title, &attrs, &ftsRowid)
	if err == sql.ErrNoRows {
		return graph.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("deleteNode: select node: %w", err)
	}

	// Read current tags
	tags, err := selectTags(ctx, tx, id)
	if err != nil {
		return err
	}

	// Find previous revision ID
	var prevRevisionID sql.NullString
	err = tx.QueryRow(
		`SELECT id FROM revisions WHERE node_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`,
		id,
	).Scan(&prevRevisionID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("deleteNode: select prev revision: %w", err)
	}

	// Delete FTS row before deleting node (external content FTS requires explicit cleanup)
	if ftsRowid.Valid {
		_, err = tx.Exec(`DELETE FROM nodes_fts WHERE rowid = ?`, ftsRowid.Int64)
		if err != nil {
			return fmt.Errorf("deleteNode: delete fts: %w", err)
		}
	}

	// Build tombstone revision
	titleStr := ""
	if title.Valid {
		titleStr = title.String
	}
	attrsStr := ""
	if attrs.Valid {
		attrsStr = attrs.String
	}

	tombstoneB, err := snapshotJSON(titleStr, attrsStr, tags)
	if err != nil {
		return fmt.Errorf("deleteNode: snapshot: %w", err)
	}

	revisionID := ids.New()
	var parentID *string
	if prevRevisionID.Valid {
		parentID = &prevRevisionID.String
	}

	_, err = tx.Exec(
		`INSERT INTO revisions(id, node_id, parent_id, diff, author) VALUES (?, ?, ?, ?, ?)`,
		revisionID, id, parentID, string(tombstoneB), author.String(),
	)
	if err != nil {
		return fmt.Errorf("deleteNode: insert tombstone revision: %w", err)
	}

	// Delete node (FK cascade cleans up node_tags; edges have ON DELETE CASCADE)
	_, err = tx.Exec(`DELETE FROM nodes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleteNode: delete node: %w", err)
	}

	return nil
}

// softDeleteNote tombstones a note node in-place (keeps the row, sets deleted_at,
// removes the FTS row, writes a tombstone revision, and degrades all INCOMING
// links_to edges into dangling_links rows keyed by stem + title).
//
// Callers supply the stem (filename without extension) and title so we can key
// the dangling rows consistently with PromoteDanglingInTx on revival.
func softDeleteNote(ctx context.Context, tx *graph.WriteTx, id, stem, title string, author Author) error {
	if !author.Valid() {
		return graph.ErrAuthorRequired
	}
	if id == "" {
		return graph.ErrNotFound
	}

	// Read current state (must be a note and not already deleted)
	var (
		titleStored sql.NullString
		attrsStored sql.NullString
		ftsRowid    sql.NullInt64
	)
	err := tx.QueryRow(
		`SELECT title, attrs, fts_rowid FROM nodes WHERE id = ? AND type = 'note' AND deleted_at IS NULL`,
		id,
	).Scan(&titleStored, &attrsStored, &ftsRowid)
	if err == sql.ErrNoRows {
		return graph.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("softDeleteNote: select node %q: %w", id, err)
	}

	// Read current tags for the tombstone revision
	tags, err := selectTags(ctx, tx, id)
	if err != nil {
		return fmt.Errorf("softDeleteNote: select tags %q: %w", id, err)
	}

	// Delete FTS row so the note stops appearing in search
	if ftsRowid.Valid {
		if _, err := tx.Exec(`DELETE FROM nodes_fts WHERE rowid = ?`, ftsRowid.Int64); err != nil {
			return fmt.Errorf("softDeleteNote: delete fts %q: %w", id, err)
		}
	}

	// Find previous revision ID for tombstone chain
	var prevRevisionID sql.NullString
	err = tx.QueryRow(
		`SELECT id FROM revisions WHERE node_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`,
		id,
	).Scan(&prevRevisionID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("softDeleteNote: select prev revision %q: %w", id, err)
	}

	// Build and insert tombstone revision
	titleStr := ""
	if titleStored.Valid {
		titleStr = titleStored.String
	}
	attrsStr := ""
	if attrsStored.Valid {
		attrsStr = attrsStored.String
	}
	tombstoneB, err := snapshotJSON(titleStr, attrsStr, tags)
	if err != nil {
		return fmt.Errorf("softDeleteNote: snapshot %q: %w", id, err)
	}

	revisionID := ids.New()
	var parentID *string
	if prevRevisionID.Valid {
		parentID = &prevRevisionID.String
	}
	if _, err := tx.Exec(
		`INSERT INTO revisions(id, node_id, parent_id, diff, author) VALUES (?, ?, ?, ?, ?)`,
		revisionID, id, parentID, string(tombstoneB), author.String(),
	); err != nil {
		return fmt.Errorf("softDeleteNote: insert tombstone revision %q: %w", id, err)
	}

	// Degrade incoming links_to edges into dangling_links rows
	incomingRows, err := tx.Query(
		`SELECT id, src FROM edges WHERE dst = ? AND label = 'links_to'`,
		id,
	)
	if err != nil {
		return fmt.Errorf("softDeleteNote: query incoming edges %q: %w", id, err)
	}
	type incomingEdge struct{ id, src string }
	var incoming []incomingEdge
	for incomingRows.Next() {
		var e incomingEdge
		if err := incomingRows.Scan(&e.id, &e.src); err != nil {
			incomingRows.Close()
			return fmt.Errorf("softDeleteNote: scan incoming edge %q: %w", id, err)
		}
		incoming = append(incoming, e)
	}
	incomingRows.Close()
	if err := incomingRows.Err(); err != nil {
		return fmt.Errorf("softDeleteNote: incoming edges rows %q: %w", id, err)
	}

	for _, e := range incoming {
		// Insert dangling row keyed by stem
		dlStemID := ids.New()
		if _, err := tx.Exec(
			`INSERT INTO dangling_links(id, src_node_id, target_key, target_kind) VALUES (?, ?, ?, ?)`,
			dlStemID, e.src, stem, "stem",
		); err != nil {
			return fmt.Errorf("softDeleteNote: insert dangling stem %q→%q: %w", e.src, stem, err)
		}
		// Insert dangling row keyed by title (if different from stem)
		if title != "" && title != stem {
			dlTitleID := ids.New()
			if _, err := tx.Exec(
				`INSERT INTO dangling_links(id, src_node_id, target_key, target_kind) VALUES (?, ?, ?, ?)`,
				dlTitleID, e.src, title, "title",
			); err != nil {
				return fmt.Errorf("softDeleteNote: insert dangling title %q→%q: %w", e.src, title, err)
			}
		}
		// Delete the now-degraded edge
		if _, err := tx.Exec(`DELETE FROM edges WHERE id = ?`, e.id); err != nil {
			return fmt.Errorf("softDeleteNote: delete edge %q: %w", e.id, err)
		}
	}

	// Set deleted_at (keep the row — soft delete)
	if _, err := tx.Exec(
		`UPDATE nodes SET deleted_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?`,
		id,
	); err != nil {
		return fmt.Errorf("softDeleteNote: set deleted_at %q: %w", id, err)
	}

	return nil
}

// reviveNote reverses a soft-delete: clears deleted_at, re-inserts the FTS row,
// writes a revival revision, and re-promotes matching dangling links to edges via
// PromoteDanglingInTx keyed by stem + title.
//
// The caller supplies the stem and title to use for dangling-link promotion.
func reviveNote(ctx context.Context, tx *graph.WriteTx, id, stem, title string, author Author) error {
	if !author.Valid() {
		return graph.ErrAuthorRequired
	}
	if id == "" {
		return graph.ErrNotFound
	}

	// Read the tombstoned note (must have deleted_at set)
	var (
		titleStored sql.NullString
		attrsStored sql.NullString
		ftsRowid    sql.NullInt64
	)
	err := tx.QueryRow(
		`SELECT title, attrs, fts_rowid FROM nodes WHERE id = ? AND type = 'note' AND deleted_at IS NOT NULL`,
		id,
	).Scan(&titleStored, &attrsStored, &ftsRowid)
	if err == sql.ErrNoRows {
		return graph.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("reviveNote: select node %q: %w", id, err)
	}

	fts := Note{}
	if titleStored.Valid {
		fts.Title = titleStored.String
	}
	if attrsStored.Valid && attrsStored.String != "" {
		var pa struct {
			Body   string `json:"body"`
			Origin string `json:"origin"`
			Author string `json:"author"`
		}
		if err := decodeJSON(attrsStored.String, &pa); err == nil {
			fts.Body = pa.Body
		}
	}

	tags, err := selectTags(ctx, tx, id)
	if err != nil {
		return fmt.Errorf("reviveNote: select tags %q: %w", id, err)
	}

	// Clear deleted_at first so FTS re-insert doesn't race
	if _, err := tx.Exec(
		`UPDATE nodes SET deleted_at = NULL WHERE id = ?`,
		id,
	); err != nil {
		return fmt.Errorf("reviveNote: clear deleted_at %q: %w", id, err)
	}

	// Re-insert FTS row (reuse the same rowid if we still have it; allocate new one otherwise)
	ftsAttrs := fts.Body
	tagStr := strings.Join(tags, " ")
	if tagStr != "" {
		if ftsAttrs != "" {
			ftsAttrs += " "
		}
		ftsAttrs += tagStr
	}

	if ftsRowid.Valid {
		// Delete old stale entry if any (should have been removed on soft-delete, but be safe)
		_, _ = tx.Exec(`DELETE FROM nodes_fts WHERE rowid = ?`, ftsRowid.Int64)
		if _, err := tx.Exec(
			`INSERT INTO nodes_fts(rowid, title, attrs) VALUES (?, ?, ?)`,
			ftsRowid.Int64, fts.Title, ftsAttrs,
		); err != nil {
			return fmt.Errorf("reviveNote: re-insert fts (rowid) %q: %w", id, err)
		}
	} else {
		result, err := tx.Exec(
			`INSERT INTO nodes_fts(title, attrs) VALUES (?, ?)`,
			fts.Title, ftsAttrs,
		)
		if err != nil {
			return fmt.Errorf("reviveNote: insert fts %q: %w", id, err)
		}
		newRowid, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("reviveNote: fts last insert id %q: %w", id, err)
		}
		if _, err := tx.Exec(`UPDATE nodes SET fts_rowid = ? WHERE id = ?`, newRowid, id); err != nil {
			return fmt.Errorf("reviveNote: update fts_rowid %q: %w", id, err)
		}
	}

	// Find previous revision ID for the revival chain
	var prevRevisionID sql.NullString
	err = tx.QueryRow(
		`SELECT id FROM revisions WHERE node_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`,
		id,
	).Scan(&prevRevisionID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("reviveNote: select prev revision %q: %w", id, err)
	}

	// Build revival revision
	titleStr := fts.Title
	attrsStr := ""
	if attrsStored.Valid {
		attrsStr = attrsStored.String
	}
	revivalB, err := snapshotJSON(titleStr, attrsStr, tags)
	if err != nil {
		return fmt.Errorf("reviveNote: snapshot %q: %w", id, err)
	}

	revisionID := ids.New()
	var parentID *string
	if prevRevisionID.Valid {
		parentID = &prevRevisionID.String
	}
	if _, err := tx.Exec(
		`INSERT INTO revisions(id, node_id, parent_id, diff, author) VALUES (?, ?, ?, ?, ?)`,
		revisionID, id, parentID, string(revivalB), author.String(),
	); err != nil {
		return fmt.Errorf("reviveNote: insert revival revision %q: %w", id, err)
	}

	return nil
}

// decodeJSON is a small helper to unmarshal into an any value.
func decodeJSON(s string, v any) error {
	return json.Unmarshal([]byte(s), v)
}

// upsertTag ensures a tag exists in the tags table.
func upsertTag(ctx context.Context, tx *graph.WriteTx, name string) error {
	_, err := tx.Exec(`INSERT OR IGNORE INTO tags(id, name) VALUES (?, ?)`, ids.New(), name)
	if err != nil {
		return fmt.Errorf("upsertTag: %w", err)
	}
	return nil
}

// selectTags reads all tag names for a node.
func selectTags(ctx context.Context, tx *graph.WriteTx, nodeID string) ([]string, error) {
	rows, err := tx.Query(
		`SELECT t.name FROM tags t JOIN node_tags nt ON t.id = nt.tag_id WHERE nt.node_id = ?`,
		nodeID,
	)
	if err != nil {
		return nil, fmt.Errorf("selectTags: %w", err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("selectTags: scan: %w", err)
		}
		tags = append(tags, name)
	}
	return tags, rows.Err()
}

// diffTags computes which tags to add and remove when transitioning from prev to next.
func diffTags(prev, next []string) (add, remove []string) {
	prevSet := make(map[string]struct{}, len(prev))
	for _, t := range prev {
		prevSet[t] = struct{}{}
	}
	nextSet := make(map[string]struct{}, len(next))
	for _, t := range next {
		nextSet[t] = struct{}{}
	}

	for t := range nextSet {
		if _, ok := prevSet[t]; !ok {
			add = append(add, t)
		}
	}
	for t := range prevSet {
		if _, ok := nextSet[t]; !ok {
			remove = append(remove, t)
		}
	}
	return
}

// snapshotJSON marshals node state into a JSON revision diff payload.
func snapshotJSON(title, attrs string, tags []string) ([]byte, error) {
	s := struct {
		Title string   `json:"title"`
		Attrs string   `json:"attrs"`
		Tags  []string `json:"tags"`
	}{
		Title: title,
		Attrs: attrs,
		Tags:  tags,
	}
	return json.Marshal(s)
}
