package inbox

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/tags"
)

// Reopen reverses a Process: it removes the node the capture became and returns
// the capture to the pending queue. This backs the inbox undo. The derived node
// is soft-deleted; for a note, its vault markdown file is removed too (looked up
// via note_paths, with a scan fallback for the brief window before the vault
// reconciler has recorded the path) so the watcher cannot resurrect it.
func Reopen(ctx context.Context, g *graph.Graph, vaultRoot string, captureID string) error {
	author := nodes.Author{Name: "inbox-reopen"}

	// Discover the capture and the node it became (if any) in a read tx.
	var capture *nodes.Capture
	var derivedID, derivedType, notePath string
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		capture, err = nodes.GetCapture(ctx, tx, captureID)
		if err != nil {
			return fmt.Errorf("get capture: %w", err)
		}
		in, err := edges.Incoming(ctx, tx, captureID)
		if err != nil {
			return fmt.Errorf("incoming edges: %w", err)
		}
		for _, e := range in {
			if e.Label != "derived_from" {
				continue
			}
			var typ string
			if err := tx.QueryRow(
				`SELECT type FROM nodes WHERE id = ? AND deleted_at IS NULL`, e.Src,
			).Scan(&typ); err != nil {
				if err == sql.ErrNoRows {
					continue // already gone
				}
				return fmt.Errorf("derived node type: %w", err)
			}
			derivedID, derivedType = e.Src, typ
			if typ == "note" {
				var p sql.NullString
				if err := tx.QueryRow(`SELECT path FROM note_paths WHERE uuid = ?`, e.Src).Scan(&p); err != nil && err != sql.ErrNoRows {
					return fmt.Errorf("lookup note path: %w", err)
				}
				if p.Valid {
					notePath = p.String
				}
			}
			break
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Remove the derived node and return the capture to the pending queue.
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		switch derivedType {
		case "note":
			if notePath != "" {
				if _, err := tx.Exec(`DELETE FROM note_paths WHERE uuid = ?`, derivedID); err != nil {
					return fmt.Errorf("delete note_paths: %w", err)
				}
			}
			if err := nodes.DeleteNote(ctx, tx, derivedID, author); err != nil {
				return fmt.Errorf("delete note: %w", err)
			}
		case "bookmark":
			if err := nodes.DeleteBookmark(ctx, tx, derivedID, author); err != nil {
				return fmt.Errorf("delete bookmark: %w", err)
			}
		case "task":
			if err := nodes.DeleteTask(ctx, tx, derivedID, author); err != nil {
				return fmt.Errorf("delete task: %w", err)
			}
		}

		var newTags []string
		for _, tag := range capture.Tags {
			if tag != tags.Triaged && tag != tags.Discarded {
				newTags = append(newTags, tag)
			}
		}
		newTags = append(newTags, tags.Pending)
		capture.Tags = newTags
		if err := nodes.UpdateCapture(ctx, tx, *capture, author); err != nil {
			return fmt.Errorf("update capture: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	derivedNoteID := ""
	if derivedType == "note" {
		derivedNoteID = derivedID
	}

	// Remove the markdown outside the transaction (mirrors how Process writes it).
	switch {
	case notePath != "":
		_ = os.Remove(filepath.Join(vaultRoot, notePath))
	case derivedNoteID != "":
		// note_paths had no row yet (reconciler hasn't run): find the markdown
		// by the note id embedded in its frontmatter and remove it.
		removeNoteFileByID(vaultRoot, derivedNoteID)
	}
	return nil
}

// removeNoteFileByID deletes the markdown file (anywhere under vaultRoot) whose
// frontmatter carries noteID. Process/Prep write an "id: <noteID>" line, so
// matching on the (unique) note id is safe. Used as the fallback when note_paths
// has no row yet, and to clean up DA prep notes on discard.
func removeNoteFileByID(vaultRoot, noteID string) {
	_ = filepath.WalkDir(vaultRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(data), noteID) {
			_ = os.Remove(path)
			return filepath.SkipAll
		}
		return nil
	})
}
