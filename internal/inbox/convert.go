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

// resolveTargetType maps a UI/API action onto a concrete conversion target.
// The single "convert" action infers note vs bookmark from the capture body
// (a URL-looking body becomes a bookmark, everything else a note). "keep"
// triages the capture in place with no target. "note", "bookmark", and
// "discard" pass through unchanged for direct API/CLI callers.
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
// targetType accepts the UI actions ("convert", "keep", "discard") as well as
// the concrete targets ("note", "bookmark").
func Process(ctx context.Context, g *graph.Graph, vaultRoot string, archiver *bookmarks.Archiver, captureID string, targetType string) error {
	var capture *nodes.Capture
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		capture, err = nodes.GetCapture(ctx, tx, captureID)
		return err
	})
	if err != nil {
		return fmt.Errorf("get capture: %w", err)
	}

	targetType = resolveTargetType(targetType, capture.Body)

	author := nodes.Author{Name: "inbox-convert"}
	var targetBookmark *nodes.Bookmark

	err = g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		var targetID string

		if targetType == "note" {
			n := nodes.Note{
				Title:  capture.Title,
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
		} else if targetType == "bookmark" {
			b := nodes.Bookmark{
				URL:   capture.Body,
				Title: capture.Title,
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

	return nil
}
