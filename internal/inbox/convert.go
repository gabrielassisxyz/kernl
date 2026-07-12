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
	"github.com/gabrielassisxyz/kernl/internal/ingest"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

// mergedIntoLabel marks the provenance edge from a note to the capture whose
// content was merged into it (target "update"). It is deliberately distinct from
// "derived_from" so the inbox undo (Reopen) never deletes the pre-existing note —
// it only un-triages the capture.
const mergedIntoLabel = "merged_into"

// Action is one node a capture becomes. It is the same shape the classifier
// proposes (nodes.CaptureAction), so a suggestion can be posted back verbatim
// or edited first — there is no second contract to keep in sync.
type Action = nodes.CaptureAction

// ProcessRequest carries the processing decision for a pending capture: the
// list of nodes it becomes. A capture is routinely several things at once (a
// note plus the task it implies), so Actions is the unit — a single-node
// conversion is just a one-element list.
type ProcessRequest struct {
	Actions []Action

	// TargetNoteID and AcceptedHunks belong to an "update" action: the note to
	// merge the capture into and the human-accepted merge hunks (reviewed in
	// DiffSuggest). An empty TargetNoteID is resolved from the body; with no
	// confident target the action falls back to "note". An update is exclusive:
	// it must be the only action in the request (see errUpdateNotExclusive).
	TargetNoteID  string
	AcceptedHunks []ingest.MergeHunk
}

var errUpdateNotExclusive = fmt.Errorf("an update action must be the only action: merging into an existing note is reviewed hunk by hunk and cannot be combined with a fan-out")

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

// Process converts a pending capture into a single node of the given kind, or
// discards it. It is the entry point for callers that only carry an action
// string; ProcessCapture handles the fan-out and the structured cases.
func Process(ctx context.Context, g *graph.Graph, vaultRoot string, archiver *bookmarks.Archiver, captureID string, targetType string) error {
	return ProcessCapture(ctx, g, vaultRoot, archiver, captureID, ProcessRequest{
		Actions: []Action{{Target: targetType}},
	})
}

// ProcessCapture turns a pending capture into the nodes described by req.Actions
// and triages the capture. note → vault markdown + Note node; bookmark →
// Bookmark node (archived async); task → Task node optionally filed under a
// project via a part_of edge; project → Project node plus its initial tasks;
// update → merge into an existing note; discard → nothing.
//
// Every action runs in ONE write transaction: a capture that fans into four
// nodes either produces all four or none. Each created node gets a derived_from
// edge back to the capture (provenance, and the undo path), and the nodes fanned
// out of the same capture are related to each other — the note and the task it
// spawned belong together.
//
// A "discard" among several actions means "this fragment is noise"; the capture
// itself is only discarded when every action is a discard.
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

	actions, err := resolveActions(req.Actions, capture)
	if err != nil {
		return err
	}

	// An update merges into an existing note. Resolve the target (from the
	// request, else from the body) and load it before the write tx. With no
	// confident target, fall back to creating a note so the capture is never lost.
	var mergeTarget *nodes.Note
	if actions[0].Target == "update" {
		mergeTarget = resolveMergeTarget(ctx, g, capture, req.TargetNoteID)
		if mergeTarget == nil {
			actions[0].Target = "note"
		}
	}

	author := nodes.Author{Name: "inbox-convert"}
	var newBookmarks []*nodes.Bookmark
	var deletedPrepID string // set when a discard removes the capture's prep note
	// Set on an "update" merge: the note whose body changed, so its vault file
	// can be mirrored after the tx commits.
	var mergedNoteID, mergedNoteBody string

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		// Nodes created by this call, in action order. Provenance and the
		// sibling "related" edges are wired from this list once every action
		// has run, so a failure anywhere rolls the whole fan-out back.
		var createdIDs []string
		// The note an update folded the capture into: provenance, but not a
		// created node — undo must never delete it.
		var updatedNoteID string

		for _, action := range actions {
			switch action.Target {
			case "note":
				id, err := createNoteFromAction(ctx, tx, vaultRoot, action, author)
				if err != nil {
					return err
				}
				createdIDs = append(createdIDs, id)
			case "bookmark":
				id, bookmark, err := createBookmarkFromAction(ctx, tx, action, author)
				if err != nil {
					return err
				}
				createdIDs = append(createdIDs, id)
				newBookmarks = append(newBookmarks, bookmark)
			case "task":
				id, err := createTaskFromAction(ctx, tx, action, author)
				if err != nil {
					return err
				}
				createdIDs = append(createdIDs, id)
			case "project":
				ids, err := createProjectFromAction(ctx, tx, action, author)
				if err != nil {
					return err
				}
				// The project AND its initial tasks are all derived from this
				// capture, so undo takes the whole subtree back out.
				createdIDs = append(createdIDs, ids...)
			case "update":
				newBody := ingest.ApplyHunks(mergeTarget.Body, req.AcceptedHunks)
				if newBody != mergeTarget.Body {
					updated := *mergeTarget
					updated.Body = newBody
					if err := nodes.UpdateNote(ctx, tx, updated, author); err != nil {
						return fmt.Errorf("update note: %w", err)
					}
					mergedNoteID, mergedNoteBody = mergeTarget.ID, newBody
				}
				updatedNoteID = mergeTarget.ID
			case "discard":
				// Nothing to create: this fragment is noise.
			}

			// Optional user-chosen link from a note/bookmark to any other node.
			if action.LinkTo != "" && len(createdIDs) > 0 && (action.Target == "note" || action.Target == "bookmark") {
				if _, err := edges.Create(ctx, tx, edges.Edge{
					Src:   createdIDs[len(createdIDs)-1],
					Dst:   action.LinkTo,
					Label: "related",
					Type:  edges.EdgeTypeRelated,
				}, author); err != nil {
					return fmt.Errorf("create related edge: %w", err)
				}
			}
		}

		if err := linkProvenance(ctx, tx, captureID, createdIDs, updatedNoteID, author); err != nil {
			return err
		}
		if err := relateSiblings(ctx, tx, createdIDs, author); err != nil {
			return err
		}

		// DA briefing lifecycle: a wholesale discard removes the prep; otherwise
		// the prep follows the capture onto its derived nodes (surfaced 1-hop
		// when acting on any of them).
		targetIDs := createdIDs
		if updatedNoteID != "" {
			targetIDs = append(targetIDs, updatedNoteID)
		}
		if prepID != "" {
			if len(targetIDs) == 0 {
				if err := deletePrep(ctx, tx, prepID, author); err != nil {
					return err
				}
				deletedPrepID = prepID
			}
			for _, id := range targetIDs {
				if _, err := edges.Create(ctx, tx, edges.Edge{
					Src:   id,
					Dst:   prepID,
					Label: briefingEdgeLabel,
				}, author); err != nil {
					return fmt.Errorf("create briefing edge: %w", err)
				}
			}
		}

		return retagCapture(ctx, tx, capture, len(targetIDs) > 0, author)
	})
	if err != nil {
		return err
	}

	if archiver != nil {
		for _, b := range newBookmarks {
			go func(b *nodes.Bookmark) {
				_, _ = archiver.ArchiveBookmark(context.Background(), b)
			}(b)
		}
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

// resolveActions validates the requested actions and fills each one in from the
// capture: the concrete target (convert → note/bookmark), the body, and the
// title. Every downstream creator can then read the action alone.
func resolveActions(requested []Action, capture *nodes.Capture) ([]Action, error) {
	if len(requested) == 0 {
		return nil, fmt.Errorf("no actions: a capture must become at least one node (or an explicit discard)")
	}
	out := make([]Action, 0, len(requested))
	for _, action := range requested {
		action.Body = strings.TrimSpace(action.Body)
		if action.Body == "" {
			action.Body = capture.Body
		}
		action.Target = resolveTargetType(strings.TrimSpace(action.Target), action.Body)
		if !isKnownTarget(action.Target) {
			return nil, fmt.Errorf("unknown target %q", action.Target)
		}
		action.Title = strings.TrimSpace(action.Title)
		if action.Title == "" {
			action.Title = capture.Title
		}
		// Only a task has a due date. Drop one left behind by a target the user
		// changed in the editor, rather than writing it into a note's attrs.
		if action.Target != "task" {
			action.DueDate = nil
		}
		out = append(out, action)
	}
	for _, action := range out {
		if action.Target == "update" && len(out) > 1 {
			return nil, errUpdateNotExclusive
		}
	}
	return out, nil
}

func isKnownTarget(target string) bool {
	switch target {
	case "note", "bookmark", "task", "project", "update", "discard":
		return true
	}
	return false
}

func resolveMergeTarget(ctx context.Context, g *graph.Graph, capture *nodes.Capture, targetNoteID string) *nodes.Note {
	if targetNoteID == "" {
		targetNoteID, _ = ingest.ResolveMergeTargetFor(ctx, g, capture.Body, capture.ID)
	}
	if targetNoteID == "" {
		return nil
	}
	var note *nodes.Note
	_ = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		note, _ = nodes.GetNote(ctx, tx, targetNoteID)
		return nil
	})
	return note
}

func createNoteFromAction(ctx context.Context, tx *graph.WriteTx, vaultRoot string, action Action, author nodes.Author) (string, error) {
	n := nodes.Note{
		Title:  action.Title,
		Body:   action.Body,
		Origin: "capture",
		Tags:   append([]string{"capture", "converted"}, action.Tags...),
	}
	if n.Title == "" {
		n.Title = "Capture Note"
	}
	id, err := nodes.CreateNote(ctx, tx, n, author)
	if err != nil {
		return "", fmt.Errorf("create note: %w", err)
	}

	// The id suffix keeps two notes fanned out of the same capture in the same
	// second from writing to the same file.
	slug := fmt.Sprintf("capture-%s-%s", time.Now().Format("20060102150405"), shortID(id))
	md := fmt.Sprintf("---\nid: %s\ntitle: %q\ntags: [%s]\norigin: capture\n---\n\n%s",
		id, n.Title, strings.Join(n.Tags, ", "), n.Body)
	if err := os.WriteFile(filepath.Join(vaultRoot, slug+".md"), []byte(md), 0644); err != nil {
		return "", fmt.Errorf("write note md: %w", err)
	}
	return id, nil
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func createBookmarkFromAction(ctx context.Context, tx *graph.WriteTx, action Action, author nodes.Author) (string, *nodes.Bookmark, error) {
	b := nodes.Bookmark{
		URL:   action.Body,
		Title: action.Title,
		Tags:  action.Tags,
	}
	if b.Title == "" {
		b.Title = "Pending"
	}
	id, err := nodes.CreateBookmark(ctx, tx, b, author)
	if err != nil {
		return "", nil, fmt.Errorf("create bookmark: %w", err)
	}
	b.ID = id
	return id, &b, nil
}

func createTaskFromAction(ctx context.Context, tx *graph.WriteTx, action Action, author nodes.Author) (string, error) {
	t := nodes.Task{
		Title:       action.Title,
		Description: action.Body,
		ProjectID:   action.ProjectID,
		Tags:        action.Tags,
		DueDate:     action.DueDate,
	}
	if t.Title == "" {
		t.Title = "Capture Task"
	}
	id, err := nodes.CreateTask(ctx, tx, t, author)
	if err != nil {
		return "", fmt.Errorf("create task: %w", err)
	}
	// Canonical link to the project (mirrored on the task's ProjectID for cheap
	// filtering by ListTasks). An empty ProjectID leaves the task unfiled in the
	// "unprocessed tasks" bucket.
	if action.ProjectID != "" {
		if err := linkPartOf(ctx, tx, id, action.ProjectID, author); err != nil {
			return "", err
		}
	}
	return id, nil
}

// createProjectFromAction creates the project and its initial tasks, returning
// every node it created (project first) so they all get provenance back to the
// capture.
func createProjectFromAction(ctx context.Context, tx *graph.WriteTx, action Action, author nodes.Author) ([]string, error) {
	title := strings.TrimSpace(action.ProjectTitle)
	if title == "" {
		title = action.Title
	}
	if title == "" {
		title = "Capture Project"
	}
	description := strings.TrimSpace(action.ProjectDescription)
	if description == "" {
		description = action.Body
	}
	projectID, err := nodes.CreateProject(ctx, tx, nodes.Project{
		Title:       title,
		Description: description,
		Tags:        action.Tags,
	}, author)
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}

	created := []string{projectID}
	for _, taskTitle := range cleanProjectTaskTitles(action.InitialTasks) {
		taskID, err := nodes.CreateTask(ctx, tx, nodes.Task{
			Title:     taskTitle,
			ProjectID: projectID,
		}, author)
		if err != nil {
			return nil, fmt.Errorf("create project task: %w", err)
		}
		if err := linkPartOf(ctx, tx, taskID, projectID, author); err != nil {
			return nil, err
		}
		created = append(created, taskID)
	}
	return created, nil
}

func linkPartOf(ctx context.Context, tx *graph.WriteTx, taskID, projectID string, author nodes.Author) error {
	if _, err := edges.Create(ctx, tx, edges.Edge{
		Src:   taskID,
		Dst:   projectID,
		Label: "part_of",
		Type:  edges.EdgeTypePartOf,
	}, author); err != nil {
		return fmt.Errorf("create part_of edge: %w", err)
	}
	return nil
}

// linkProvenance records where every node came from. Created nodes get
// derived_from (the edge Reopen walks to undo); an updated note gets
// merged_into, so undo un-triages the capture without deleting a note that
// existed before it.
func linkProvenance(ctx context.Context, tx *graph.WriteTx, captureID string, createdIDs []string, updatedNoteID string, author nodes.Author) error {
	for _, id := range createdIDs {
		if _, err := edges.Create(ctx, tx, edges.Edge{
			Src:   id,
			Dst:   captureID,
			Label: "derived_from",
		}, author); err != nil {
			return fmt.Errorf("create derived_from edge: %w", err)
		}
	}
	if updatedNoteID != "" {
		if _, err := edges.Create(ctx, tx, edges.Edge{
			Src:   updatedNoteID,
			Dst:   captureID,
			Label: mergedIntoLabel,
		}, author); err != nil {
			return fmt.Errorf("create merged_into edge: %w", err)
		}
	}
	return nil
}

// relateSiblings connects the nodes one capture fanned out into: the note and
// the task it spawned are about the same thought, and reading either one should
// surface the other.
func relateSiblings(ctx context.Context, tx *graph.WriteTx, createdIDs []string, author nodes.Author) error {
	for i := 0; i < len(createdIDs); i++ {
		for j := i + 1; j < len(createdIDs); j++ {
			if _, err := edges.Create(ctx, tx, edges.Edge{
				Src:   createdIDs[i],
				Dst:   createdIDs[j],
				Label: "related",
				Type:  edges.EdgeTypeRelated,
			}, author); err != nil {
				return fmt.Errorf("relate fanned-out nodes: %w", err)
			}
		}
	}
	return nil
}

func deletePrep(ctx context.Context, tx *graph.WriteTx, prepID string, author nodes.Author) error {
	if _, err := tx.Exec(`DELETE FROM note_paths WHERE uuid = ?`, prepID); err != nil {
		return fmt.Errorf("delete prep note_paths: %w", err)
	}
	if err := nodes.DeleteNote(ctx, tx, prepID, author); err != nil {
		return fmt.Errorf("delete prep note: %w", err)
	}
	return nil
}

// retagCapture moves the capture out of the pending queue: triaged when it
// produced at least one node, discarded when every action was a discard.
func retagCapture(ctx context.Context, tx *graph.WriteTx, capture *nodes.Capture, produced bool, author nodes.Author) error {
	var tags []string
	for _, tag := range capture.Tags {
		if tag != "pending" {
			tags = append(tags, tag)
		}
	}
	if produced {
		tags = append(tags, "triaged")
	} else {
		tags = append(tags, "discarded")
	}
	capture.Tags = tags
	if err := nodes.UpdateCapture(ctx, tx, *capture, author); err != nil {
		return fmt.Errorf("update capture: %w", err)
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
