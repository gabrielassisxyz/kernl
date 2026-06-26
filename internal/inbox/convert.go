package inbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/bookmarks"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// ProcessRequest carries an explicit processing decision for a pending capture.
// Target is the destination node kind; the other fields are optional refinements
// supplied by the inbox modal (manual override) or the DA classifier.
type ProcessRequest struct {
	Target    string // "note" | "bookmark" | "task" | "discard" | "convert" (infer note/bookmark from body)
	ProjectID string // task only: parent project (empty = unfiled, the "unprocessed tasks" bucket)
	LinkTo    string // note/bookmark only: optional node to relate the result to
	Title     string // optional title override (falls back to the capture title/body)
}

// resolveTargetType maps a UI/API action onto a concrete conversion target.
// The single "convert" action infers note vs bookmark from the capture body
// (a URL-looking body becomes a bookmark, everything else a note). "note",
// "bookmark", "task", and "discard" pass through unchanged.
func resolveTargetType(action, body string) string {
	if action == "convert" {
		if looksLikeURL(body) {
			return "bookmark"
		}
		return "note"
	}
	return action
}

func looksLikeURL(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// Process converts a pending capture into a note or bookmark, or discards it.
// It is the simple entry point for callers that only carry an action string;
// ProcessCapture handles the structured (task / project / link) cases.
func Process(ctx context.Context, g *graph.Graph, vaultRoot string, archiver *bookmarks.Archiver, captureID string, targetType string) error {
	return ProcessCapture(ctx, g, vaultRoot, archiver, captureID, ProcessRequest{Target: targetType})
}

// ProcessCapture turns a pending capture into the node described by req and
// triages the capture. note → vault markdown + Note node; bookmark → Bookmark
// node (archived async); task → Task node optionally filed under a project via a
// part_of edge; discard → no target. Every created target gets a derived_from
// edge back to the capture for provenance (and undo).
func ProcessCapture(ctx context.Context, g *graph.Graph, vaultRoot string, archiver *bookmarks.Archiver, captureID string, req ProcessRequest) error {
	var capture *nodes.Capture
	var prepID string
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		capture, err = nodes.GetCapture(ctx, tx, captureID)
		if err != nil {
			return err
		}
		prepID, _ = PrepFor(ctx, tx, captureID)
		return nil
	})
	if err != nil {
		return fmt.Errorf("get capture: %w", err)
	}

	targetType := resolveTargetType(req.Target, capture.Body)

	// Title: explicit override wins, else the capture's own title.
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = capture.Title
	}

	author := nodes.Author{Name: "inbox-convert"}
	var targetBookmark *nodes.Bookmark
	var deletedPrepID string // set when a discard removes the capture's prep note

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var targetID string

		switch targetType {
		case "note":
			n := nodes.Note{
				Title:  title,
				Body:   capture.Body,
				Origin: "capture",
				Tags:   []string{"capture", "converted"},
			}
			if n.Title == "" {
				n.Title = "Capture Note"
			}
			var err error
			targetID, err = nodes.CreateNote(ctx, tx, n, author)
			if err != nil {
				return fmt.Errorf("create note: %w", err)
			}

			slug := "capture-" + time.Now().Format("20060102150405")
			md := fmt.Sprintf("---\nid: %s\ntitle: %q\ntags: [capture, converted]\norigin: capture\n---\n\n%s", targetID, n.Title, capture.Body)
			path := filepath.Join(vaultRoot, slug+".md")
			if err := os.WriteFile(path, []byte(md), 0644); err != nil {
				return fmt.Errorf("write note md: %w", err)
			}
		case "bookmark":
			b := nodes.Bookmark{
				URL:   capture.Body,
				Title: title,
			}
			if b.Title == "" {
				b.Title = "Pending"
			}
			var err error
			targetID, err = nodes.CreateBookmark(ctx, tx, b, author)
			if err != nil {
				return fmt.Errorf("create bookmark: %w", err)
			}
			b.ID = targetID
			targetBookmark = &b
		case "task":
			tk := nodes.Task{
				Title:       title,
				Description: capture.Body,
				ProjectID:   req.ProjectID,
			}
			if tk.Title == "" {
				tk.Title = "Capture Task"
			}
			var err error
			targetID, err = nodes.CreateTask(ctx, tx, tk, author)
			if err != nil {
				return fmt.Errorf("create task: %w", err)
			}
			// Canonical link to the project (mirrored on the task's ProjectID
			// for cheap filtering by ListTasks). Empty ProjectID leaves the
			// task unfiled in the "unprocessed tasks" bucket.
			if req.ProjectID != "" {
				if _, err := edges.Create(ctx, tx, edges.Edge{
					Src:   targetID,
					Dst:   req.ProjectID,
					Label: "part_of",
					Type:  edges.EdgeTypePartOf,
				}, author); err != nil {
					return fmt.Errorf("create part_of edge: %w", err)
				}
			}
		}

		// Optional user-chosen link from a note/bookmark to any other node.
		if targetID != "" && req.LinkTo != "" && (targetType == "note" || targetType == "bookmark") {
			if _, err := edges.Create(ctx, tx, edges.Edge{
				Src:   targetID,
				Dst:   req.LinkTo,
				Label: "related",
				Type:  edges.EdgeTypeRelated,
			}, author); err != nil {
				return fmt.Errorf("create related edge: %w", err)
			}
		}

		if targetID != "" {
			_, err := edges.Create(ctx, tx, edges.Edge{
				Src:   targetID,
				Dst:   captureID,
				Label: "derived_from",
			}, author)
			if err != nil {
				return fmt.Errorf("create edge: %w", err)
			}
		}

		// DA briefing lifecycle: discard removes the prep; otherwise the prep
		// follows the capture onto its derived node (surfaced 1-hop when acting).
		if prepID != "" {
			if targetType == "discard" {
				if _, err := tx.Exec(`DELETE FROM note_paths WHERE uuid = ?`, prepID); err != nil {
					return fmt.Errorf("delete prep note_paths: %w", err)
				}
				if err := nodes.DeleteNote(ctx, tx, prepID, author); err != nil {
					return fmt.Errorf("delete prep note: %w", err)
				}
				deletedPrepID = prepID
			} else if targetID != "" {
				if _, err := edges.Create(ctx, tx, edges.Edge{
					Src:   targetID,
					Dst:   prepID,
					Label: briefingEdgeLabel,
				}, author); err != nil {
					return fmt.Errorf("create briefing edge: %w", err)
				}
			}
		}

		// Update Capture tags
		var newTags []string
		for _, tag := range capture.Tags {
			if tag != "pending" {
				newTags = append(newTags, tag)
			}
		}
		if targetType == "discard" {
			newTags = append(newTags, "discarded")
		} else {
			newTags = append(newTags, "triaged")
		}
		capture.Tags = newTags

		if err := nodes.UpdateCapture(ctx, tx, *capture, author); err != nil {
			return fmt.Errorf("update capture: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	if targetBookmark != nil && archiver != nil {
		go func(b *nodes.Bookmark) {
			_, _ = archiver.ArchiveBookmark(context.Background(), b)
		}(targetBookmark)
	}

	// Remove the discarded prep's markdown outside the transaction.
	if deletedPrepID != "" {
		removeNoteFileByID(vaultRoot, deletedPrepID)
	}

	return nil
}
