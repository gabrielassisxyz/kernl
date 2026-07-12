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
)

// derivedNode is one node a capture became, as seen by the undo path.
type derivedNode struct {
	id       string
	typ      string
	notePath string // note only, when the vault reconciler has recorded it
}

// Reopen reverses a Process: it removes every node the capture became and
// returns the capture to the pending queue. This backs the inbox undo. A capture
// that fanned out into four nodes gives back four — the walk covers every
// derived_from edge, not just the first (a note merged into via merged_into is
// pre-existing and is deliberately left alone). For a note, its vault markdown
// is removed too (looked up via note_paths, with a scan fallback for the brief
// window before the vault reconciler has recorded the path) so the watcher
// cannot resurrect it.
func Reopen(ctx context.Context, g *graph.Graph, vaultRoot string, captureID string) error {
	author := nodes.Author{Name: "inbox-reopen"}

	// Discover the capture and every node it became in a read tx.
	var capture *nodes.Capture
	var derived []derivedNode
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		capture, err = nodes.GetCapture(ctx, tx, captureID)
		if err != nil {
			return fmt.Errorf("get capture: %w", err)
		}
		derived, err = derivedNodes(ctx, tx, captureID)
		return err
	})
	if err != nil {
		return err
	}

	// Remove the derived nodes and return the capture to the pending queue.
	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		for _, d := range derived {
			if err := deleteDerived(ctx, tx, d, author); err != nil {
				return err
			}
		}

		var newTags []string
		for _, tag := range capture.Tags {
			if tag != "triaged" && tag != "discarded" {
				newTags = append(newTags, tag)
			}
		}
		newTags = append(newTags, "pending")
		capture.Tags = newTags
		if err := nodes.UpdateCapture(ctx, tx, *capture, author); err != nil {
			return fmt.Errorf("update capture: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Remove the markdown outside the transaction (mirrors how Process writes it).
	for _, d := range derived {
		if d.typ != "note" {
			continue
		}
		if d.notePath != "" {
			_ = os.Remove(filepath.Join(vaultRoot, d.notePath))
			continue
		}
		// note_paths had no row yet (reconciler hasn't run): find the markdown
		// by the note id embedded in its frontmatter and remove it.
		removeNoteFileByID(vaultRoot, d.id)
	}
	return nil
}

// derivedNodes lists the live nodes created from this capture, in edge order.
func derivedNodes(ctx context.Context, tx *graph.ReadTx, captureID string) ([]derivedNode, error) {
	in, err := edges.Incoming(ctx, tx, captureID)
	if err != nil {
		return nil, fmt.Errorf("incoming edges: %w", err)
	}
	var out []derivedNode
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
			return nil, fmt.Errorf("derived node type: %w", err)
		}
		d := derivedNode{id: e.Src, typ: typ}
		if typ == "note" {
			var p sql.NullString
			if err := tx.QueryRow(`SELECT path FROM note_paths WHERE uuid = ?`, e.Src).Scan(&p); err != nil && err != sql.ErrNoRows {
				return nil, fmt.Errorf("lookup note path: %w", err)
			}
			if p.Valid {
				d.notePath = p.String
			}
		}
		out = append(out, d)
	}
	return out, nil
}

func deleteDerived(ctx context.Context, tx *graph.WriteTx, d derivedNode, author nodes.Author) error {
	switch d.typ {
	case "note":
		if d.notePath != "" {
			if _, err := tx.Exec(`DELETE FROM note_paths WHERE uuid = ?`, d.id); err != nil {
				return fmt.Errorf("delete note_paths: %w", err)
			}
		}
		if err := nodes.DeleteNote(ctx, tx, d.id, author); err != nil {
			return fmt.Errorf("delete note: %w", err)
		}
	case "bookmark":
		if err := nodes.DeleteBookmark(ctx, tx, d.id, author); err != nil {
			return fmt.Errorf("delete bookmark: %w", err)
		}
	case "task":
		if err := nodes.DeleteTask(ctx, tx, d.id, author); err != nil {
			return fmt.Errorf("delete task: %w", err)
		}
	case "project":
		if err := nodes.DeleteProject(ctx, tx, d.id, author); err != nil {
			return fmt.Errorf("delete project: %w", err)
		}
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
