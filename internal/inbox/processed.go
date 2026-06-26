package inbox

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// ProcessedItem is a capture that has been triaged or discarded, paired with the
// node it became (for triaged captures). It powers the inbox "Processed" view.
type ProcessedItem struct {
	CaptureID   string    `json:"captureId"`
	Title       string    `json:"title"`
	Became      string    `json:"became"` // note | bookmark | task | discard
	TargetID    string    `json:"targetId"`
	TargetTitle string    `json:"targetTitle"`
	ProjectID   string    `json:"projectId"`
	At          time.Time `json:"at"`
}

// ListProcessed returns captures that have left the pending queue (triaged or
// discarded), newest first, each annotated with what it became. Discarded
// captures report became="discard" with no target; triaged captures are joined
// to their derived node via the derived_from edge.
func ListProcessed(ctx context.Context, g *graph.Graph) ([]ProcessedItem, error) {
	var out []ProcessedItem
	err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		caps, err := nodes.ListCaptures(ctx, tx, nodes.CaptureFilter{
			Tags: []string{"triaged", "discarded"},
		})
		if err != nil {
			return err
		}

		for _, c := range caps {
			item := ProcessedItem{CaptureID: c.ID, Title: captureDisplayTitle(c), At: c.UpdatedAt}

			if hasTag(c.Tags, "discarded") {
				item.Became = "discard"
				out = append(out, item)
				continue
			}

			// Triaged: find the node derived from this capture.
			in, err := edges.Incoming(ctx, tx, c.ID)
			if err != nil {
				return err
			}
			for _, e := range in {
				if e.Label != "derived_from" {
					continue
				}
				var typ, title string
				var projectID sql.NullString
				err := tx.QueryRow(
					`SELECT type, title, json_extract(attrs, '$.projectId') FROM nodes WHERE id = ? AND deleted_at IS NULL`,
					e.Src,
				).Scan(&typ, &title, &projectID)
				if err != nil {
					continue // derived node gone (e.g. undone) — skip the link
				}
				item.Became = typ
				item.TargetID = e.Src
				item.TargetTitle = title
				if projectID.Valid {
					item.ProjectID = projectID.String
				}
				break
			}
			out = append(out, item)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("ListProcessed: %w", err)
	}
	return out, nil
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

// captureDisplayTitle mirrors the inbox row title: explicit title, else the
// first non-empty line of the body, else a placeholder.
func captureDisplayTitle(c *nodes.Capture) string {
	if c.Title != "" {
		return c.Title
	}
	if c.Body != "" {
		return c.Body
	}
	return "Untitled capture"
}
