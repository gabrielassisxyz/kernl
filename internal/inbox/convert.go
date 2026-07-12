package inbox

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/bookmarks"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/tags"
	"github.com/gabrielassisxyz/kernl/internal/ingest"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

// mergedIntoLabel marks the provenance edge from a note to the capture whose
// content was merged into it (target "update"). It is deliberately distinct from
// "derived_from" so the inbox undo (Reopen) never deletes the pre-existing note —
// it only un-triages the capture.
const mergedIntoLabel = "merged_into"

// ProcessRequest carries an explicit processing decision for a pending capture.
// Target is the destination node kind; the other fields are optional refinements
// supplied by the inbox modal (manual override) or the DA classifier.
type ProcessRequest struct {
	Target    string // "note" | "bookmark" | "task" | "project" | "discard" | "update" | "convert" (infer note/bookmark from body)
	ProjectID string // task only: parent project (empty = unfiled, the "unprocessed tasks" bucket)
	LinkTo    string // note/bookmark only: optional node to relate the result to
	Title     string // optional title override (falls back to the capture title/body)
	// Project only: optional metadata suggested by the classifier or edited by
	// the user in the inbox modal.
	ProjectTitle       string
	ProjectDescription string
	InitialTasks       []string

	// Update only: the note to merge the capture into and the human-accepted
	// merge hunks (reviewed in DiffSuggest). An empty TargetNoteID is resolved
	// from the body; with no confident target the action falls back to "note".
	TargetNoteID  string
	AcceptedHunks []ingest.MergeHunk
}

// resolveTargetType maps a UI/API action onto a concrete conversion target.
// The single "convert" action infers note vs bookmark from the capture body
// (a URL-looking body becomes a bookmark, everything else a note). Concrete
// targets pass through unchanged.
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
// part_of edge; project → Project node plus suggested initial tasks; discard →
// no target. Every created target gets a derived_from edge back to the capture
// for provenance (and undo).
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

	// Update merges the capture into an existing note. Resolve the target (from
	// the request, else from the body) and load it before the write tx. With no
	// confident target, fall back to creating a note so the capture is never lost.
	var mergeTarget *nodes.Note
	if targetType == "update" {
		targetNoteID := req.TargetNoteID
		if targetNoteID == "" {
			targetNoteID, _ = ingest.ResolveMergeTargetFor(ctx, g, capture.Body, captureID)
		}
		if targetNoteID != "" {
			_ = g.DoRead(ctx, func(tx *graph.ReadTx) error {
				mergeTarget, _ = nodes.GetNote(ctx, tx, targetNoteID)
				return nil
			})
		}
		if mergeTarget == nil {
			targetType = "note"
		}
	}

	// Title: explicit override wins, else the capture's own title.
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = capture.Title
	}

	author := nodes.Author{Name: "inbox-convert"}
	var targetBookmark *nodes.Bookmark
	var deletedPrepID string // set when a discard removes the capture's prep note
	// Set on an "update" merge: the note whose body changed, so its vault file
	// can be mirrored after the tx commits.
	var mergedNoteID, mergedNoteBody string

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var targetID string

		switch targetType {
		case "note":
			// No machine tags on the note. A note is file-backed, so reconcile
			// rewrites its tags from the file's frontmatter — the vault owns a
			// note's tags, and the vault must never author `sys/` ones. The
			// provenance the old "capture"/"converted" tags carried is already
			// held by `origin: capture` and the derived_from edge below, and
			// nothing ever read them. Tags on a note are the user's subjects.
			n := nodes.Note{
				Title:  title,
				Body:   capture.Body,
				Origin: "capture",
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
			md := fmt.Sprintf("---\nid: %s\ntitle: %q\norigin: capture\n---\n\n%s", targetID, n.Title, capture.Body)
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
		case "project":
			projectTitle := strings.TrimSpace(req.ProjectTitle)
			if projectTitle == "" {
				projectTitle = title
			}
			if projectTitle == "" {
				projectTitle = "Capture Project"
			}
			projectDescription := strings.TrimSpace(req.ProjectDescription)
			if projectDescription == "" {
				projectDescription = capture.Body
			}
			projectID, err := nodes.CreateProject(ctx, tx, nodes.Project{
				Title:       projectTitle,
				Description: projectDescription,
			}, author)
			if err != nil {
				return fmt.Errorf("create project: %w", err)
			}
			targetID = projectID
			for _, taskTitle := range cleanProjectTaskTitles(req.InitialTasks) {
				taskID, err := nodes.CreateTask(ctx, tx, nodes.Task{
					Title:     taskTitle,
					ProjectID: projectID,
				}, author)
				if err != nil {
					return fmt.Errorf("create project task: %w", err)
				}
				if _, err := edges.Create(ctx, tx, edges.Edge{
					Src:   taskID,
					Dst:   projectID,
					Label: "part_of",
					Type:  edges.EdgeTypePartOf,
				}, author); err != nil {
					return fmt.Errorf("create project task part_of edge: %w", err)
				}
			}
		case "update":
			// Merge the accepted hunks into the existing note. Rejecting all
			// hunks leaves the body untouched but still triages the capture.
			newBody := ingest.ApplyHunks(mergeTarget.Body, req.AcceptedHunks)
			if newBody != mergeTarget.Body {
				updated := *mergeTarget
				updated.Body = newBody
				if err := nodes.UpdateNote(ctx, tx, updated, author); err != nil {
					return fmt.Errorf("update note: %w", err)
				}
				mergedNoteID, mergedNoteBody = mergeTarget.ID, newBody
			}
			targetID = mergeTarget.ID
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
			// Update uses a distinct label so undo never deletes the pre-existing
			// note; every other target records standard derived_from provenance.
			provenance := "derived_from"
			if targetType == "update" {
				provenance = mergedIntoLabel
			}
			_, err := edges.Create(ctx, tx, edges.Edge{
				Src:   targetID,
				Dst:   captureID,
				Label: provenance,
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
			if tag != tags.Pending {
				newTags = append(newTags, tag)
			}
		}
		if targetType == "discard" {
			newTags = append(newTags, tags.Discarded)
		} else {
			newTags = append(newTags, tags.Triaged)
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

	// Mirror an update merge into the note's vault file (the source of truth) so
	// the reconciler does not clobber the node body from the stale file.
	// Best-effort: the graph already committed, so a mirror failure is logged.
	if mergedNoteID != "" {
		if _, err := reconcile.WriteNoteBody(ctx, g, vaultRoot, mergedNoteID, mergedNoteBody); err != nil {
			slog.Warn("inbox: merged body not mirrored to vault file", "note", mergedNoteID, "err", err)
		}
	}

	return nil
}

func cleanProjectTaskTitles(tasks []string) []string {
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		task = strings.TrimSpace(task)
		if task == "" {
			continue
		}
		out = append(out, task)
		if len(out) >= 6 {
			break
		}
	}
	return out
}
