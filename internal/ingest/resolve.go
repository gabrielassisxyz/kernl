package ingest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// ErrActionNotImplemented is returned for review-queue actions that depend on
// infrastructure not yet built (Deep Research, Update, Add Contradiction Callout).
var ErrActionNotImplemented = errors.New("ingest action not implemented")

// ResolveReview applies a review-queue action to the given IngestReview:
//   - "Create Page": create a Note from the review (and a vault .md), then remove it.
//   - "Skip" (or empty): remove the review with no other effect.
//   - anything else: ErrActionNotImplemented (the review stays in the queue).
func ResolveReview(ctx context.Context, g *graph.Graph, vaultRoot, reviewID, action string) error {
	switch action {
	case "Create Page":
		var review *nodes.IngestReview
		if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
			var err error
			review, err = nodes.GetIngestReview(ctx, tx, reviewID)
			return err
		}); err != nil {
			return err
		}

		title := review.Title
		if title == "" {
			title = "Ingested Page"
		}
		return g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			noteID, err := nodes.CreateNote(ctx, tx, nodes.Note{
				Title:  title,
				Body:   review.Payload,
				Origin: "ingest",
				Tags:   []string{"ingest"},
			}, nodes.Author{Name: "ingest-resolve"})
			if err != nil {
				return fmt.Errorf("create note: %w", err)
			}
			if vaultRoot != "" {
				slug := "ingest-" + time.Now().Format("20060102150405")
				md := fmt.Sprintf("---\nid: %s\ntitle: %q\ntags: [ingest]\norigin: ingest\n---\n\n%s", noteID, title, review.Payload)
				_ = os.WriteFile(filepath.Join(vaultRoot, slug+".md"), []byte(md), 0644)
			}
			return nodes.DeleteIngestReview(ctx, tx, reviewID, nodes.Author{Name: "ingest-resolve"})
		})

	case "Skip", "":
		return g.DoWrite(ctx, func(tx *graph.WriteTx) error {
			return nodes.DeleteIngestReview(ctx, tx, reviewID, nodes.Author{Name: "api"})
		})

	default:
		return ErrActionNotImplemented
	}
}
