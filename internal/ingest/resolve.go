package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/planning"
	"github.com/gabrielassisxyz/kernl/internal/vault/reconcile"
)

// ErrActionNotImplemented is returned for review-queue actions the resolver does
// not handle. The narrowed extractor no longer emits such actions, but a stale
// queue may still hold one, so the resolver fails loudly rather than silently.
var ErrActionNotImplemented = errors.New("ingest action not implemented")

const resolveAuthor = "ingest-resolve"

// relatedFanout caps how many topically-related notes a resolved item is linked
// to, so ingested knowledge lands connected to the graph without flooding it.
const relatedFanout = 3

// MergeHunk is one additive block of content proposed for an Update merge.
// DiffSuggest.vue renders each as a "+ content" suggestion the user can accept
// or reject; ResolveReview appends the accepted ones to the target note.
type MergeHunk struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

// UpdateInput carries the human-reviewed result of an Update merge: the target
// note to merge into and the hunks the user accepted. It is nil for non-Update
// actions. For Update it may be nil or partial — the target is then re-resolved
// from the review payload, and a missing target falls back to Create Page.
type UpdateInput struct {
	TargetNoteID  string      `json:"targetNoteId"`
	AcceptedHunks []MergeHunk `json:"acceptedHunks"`
}

// ResolveReview applies a review-queue action to the given IngestReview:
//   - "Create Page": create a Note from the review (and a vault .md), connect it
//     into the graph, then remove the review.
//   - "Update": merge the accepted hunks into a resolved target note, connect it,
//     then remove the review. With no confident target it falls back to Create Page.
//   - "Skip" (or empty): remove the review with no other effect.
//   - anything else: ErrActionNotImplemented (the review stays in the queue).
//
// Every resolution that lands a note (Create Page and Update) connects it to its
// SourceNodeID and to the top related notes so ingested knowledge is not orphaned.
func ResolveReview(ctx context.Context, g *graph.Graph, vaultRoot, reviewID, action string, update *UpdateInput) error {
	switch action {
	case "Create Page":
		review, err := readReview(ctx, g, reviewID)
		if err != nil {
			return err
		}
		return createPage(ctx, g, vaultRoot, review)

	case "Update":
		review, err := readReview(ctx, g, reviewID)
		if err != nil {
			return err
		}
		return updatePage(ctx, g, vaultRoot, review, update)

	case "Skip", "":
		return g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			return nodes.DeleteIngestReview(ctx, tx, reviewID, nodes.Author{Name: "api"})
		})

	default:
		return ErrActionNotImplemented
	}
}

// readReview loads a single IngestReview in its own read transaction.
func readReview(ctx context.Context, g *graph.Graph, reviewID string) (*nodes.IngestReview, error) {
	var review *nodes.IngestReview
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		review, err = nodes.GetIngestReview(ctx, tx, reviewID)
		return err
	})
	return review, err
}

// createPage creates a note from the review, writes its vault markdown, connects
// it into the graph, and removes the review.
func createPage(ctx context.Context, g *graph.Graph, vaultRoot string, review *nodes.IngestReview) error {
	title := review.Title
	if title == "" {
		title = "Ingested Page"
	}

	// Computed outside the write tx because BuildContext runs its own read tx.
	related, err := relatedNoteIDs(ctx, g, review.Payload, "", review.SourceNodeID)
	if err != nil {
		return err
	}

	return g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		noteID, err := nodes.CreateNote(ctx, tx, nodes.Note{
			Title:  title,
			Body:   review.Payload,
			Origin: "ingest",
			Tags:   []string{"ingest"},
		}, nodes.Author{Name: resolveAuthor})
		if err != nil {
			return fmt.Errorf("create note: %w", err)
		}
		writeVaultMarkdown(vaultRoot, noteID, title, review.Payload)
		if err := connectNote(ctx, tx, noteID, review.SourceNodeID, related); err != nil {
			return err
		}
		return nodes.DeleteIngestReview(ctx, tx, review.ID, nodes.Author{Name: resolveAuthor})
	})
}

// updatePage merges the accepted hunks into a target note. The target comes from
// the caller's UpdateInput when valid, otherwise it is re-resolved from the
// payload; with no confident target it falls back to Create Page.
func updatePage(ctx context.Context, g *graph.Graph, vaultRoot string, review *nodes.IngestReview, update *UpdateInput) error {
	targetID := ""
	if update != nil && update.TargetNoteID != "" && noteExists(ctx, g, update.TargetNoteID) {
		targetID = update.TargetNoteID
	} else {
		var err error
		if targetID, err = resolveMergeTarget(ctx, g, review); err != nil {
			return err
		}
	}
	if targetID == "" {
		// No confident target: a new page is better than a dropped item.
		return createPage(ctx, g, vaultRoot, review)
	}

	var target *nodes.Note
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		target, err = nodes.GetNote(ctx, tx, targetID)
		return err
	}); err != nil {
		return err
	}

	var accepted []MergeHunk
	if update != nil {
		accepted = update.AcceptedHunks
	}
	newBody := ApplyHunks(target.Body, accepted)

	related, err := relatedNoteIDs(ctx, g, review.Payload, targetID, review.SourceNodeID)
	if err != nil {
		return err
	}

	if err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		// Rejecting every hunk leaves the body identical — skip the no-op write,
		// but still connect the note and resolve the review.
		if newBody != target.Body {
			updated := *target
			updated.Body = newBody
			if err := nodes.UpdateNote(ctx, tx, updated, nodes.Author{Name: resolveAuthor}); err != nil {
				return fmt.Errorf("update note: %w", err)
			}
		}
		if err := connectNote(ctx, tx, targetID, review.SourceNodeID, related); err != nil {
			return err
		}
		return nodes.DeleteIngestReview(ctx, tx, review.ID, nodes.Author{Name: resolveAuthor})
	}); err != nil {
		return err
	}

	// Mirror the merge into the vault file (the source of truth) so the
	// reconciler does not later clobber the node body from the stale file.
	// Best-effort: the graph already committed; a file-mirror failure must not
	// undo a successful merge, so it is logged, not surfaced.
	if newBody != target.Body {
		if _, err := reconcile.WriteNoteBody(ctx, g, vaultRoot, targetID, newBody); err != nil {
			slog.Warn("ingest: merged body not mirrored to vault file", "note", targetID, "err", err)
		}
	}
	return nil
}

// ApplyHunks appends each non-empty hunk to body as a new paragraph. It is the
// shared merge primitive for both the ingest review queue and the inbox.
func ApplyHunks(body string, hunks []MergeHunk) string {
	out := body
	for _, h := range hunks {
		c := strings.TrimSpace(h.Content)
		if c == "" {
			continue
		}
		if strings.TrimSpace(out) == "" {
			out = c
			continue
		}
		out = out + "\n\n" + c
	}
	return out
}

// resolveMergeTarget picks the best existing note to merge the review into.
func resolveMergeTarget(ctx context.Context, g *graph.Graph, review *nodes.IngestReview) (string, error) {
	return ResolveMergeTargetFor(ctx, g, review.Payload, review.SourceNodeID)
}

// ResolveMergeTargetFor picks the best existing note to merge payload into: the
// top content (topical) match, excluding excludeID. It returns "" when no
// confident target exists. Shared by the ingest queue and the inbox so an
// "Update" only ever points at a real, topically-matched note.
func ResolveMergeTargetFor(ctx context.Context, g *graph.Graph, payload, excludeID string) (string, error) {
	notes, err := planning.BuildContext(ctx, g, payload, relatedFanout+2)
	if err != nil {
		return "", err
	}
	for _, n := range notes {
		if n.ID == excludeID || n.Via != "content" {
			continue
		}
		return n.ID, nil
	}
	return "", nil
}

// relatedNoteIDs returns up to relatedFanout topically-related note IDs for the
// payload, excluding the resolved note itself and its source node.
func relatedNoteIDs(ctx context.Context, g *graph.Graph, payload, excludeID, sourceID string) ([]string, error) {
	notes, err := planning.BuildContext(ctx, g, payload, relatedFanout+2)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, relatedFanout)
	for _, n := range notes {
		if n.ID == excludeID || n.ID == sourceID {
			continue
		}
		out = append(out, n.ID)
		if len(out) >= relatedFanout {
			break
		}
	}
	return out, nil
}

// connectNote links noteID to its source node and to the related notes with
// related edges, so the resolved note is reachable in the graph.
func connectNote(ctx context.Context, tx *graph.WriteTx, noteID, sourceNodeID string, relatedIDs []string) error {
	if err := ensureRelatedEdge(ctx, tx, noteID, sourceNodeID); err != nil {
		return err
	}
	for _, rid := range relatedIDs {
		if err := ensureRelatedEdge(ctx, tx, noteID, rid); err != nil {
			return err
		}
	}
	return nil
}

// ensureRelatedEdge creates a related edge from src to dst, skipping when dst is
// empty, self-referential, absent, or already linked. Ingest sources can be
// arbitrary node ids that may not exist, so a missing dst is not an error.
func ensureRelatedEdge(ctx context.Context, tx *graph.WriteTx, src, dst string) error {
	if dst == "" || dst == src {
		return nil
	}

	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id = ?`, dst).Scan(&exists); err != nil {
		return fmt.Errorf("connect: check dst: %w", err)
	}
	if exists == 0 {
		return nil
	}

	var dup int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM edges WHERE src = ? AND dst = ? AND label = ?`,
		src, dst, string(edges.EdgeTypeRelated),
	).Scan(&dup); err != nil {
		return fmt.Errorf("connect: check dup: %w", err)
	}
	if dup > 0 {
		return nil
	}

	_, err := edges.Create(ctx, tx, edges.Edge{
		Src:  src,
		Dst:  dst,
		Type: edges.EdgeTypeRelated,
	}, nodes.Author{Name: resolveAuthor})
	return err
}

// noteExists reports whether id is a live note node.
func noteExists(ctx context.Context, g *graph.Graph, id string) bool {
	var n int
	_ = g.DoRead(ctx, func(tx *graph.ReadTx) error {
		return tx.QueryRow(
			`SELECT COUNT(*) FROM nodes WHERE id = ? AND type = 'note' AND deleted_at IS NULL`, id,
		).Scan(&n)
	})
	return n > 0
}

// writeVaultMarkdown best-effort writes a markdown mirror of a created note.
func writeVaultMarkdown(vaultRoot, noteID, title, body string) {
	if vaultRoot == "" {
		return
	}
	slug := "ingest-" + time.Now().Format("20060102150405")
	md := fmt.Sprintf("---\nid: %s\ntitle: %q\ntags: [ingest]\norigin: ingest\n---\n\n%s", noteID, title, body)
	_ = os.WriteFile(filepath.Join(vaultRoot, slug+".md"), []byte(md), 0644)
}
