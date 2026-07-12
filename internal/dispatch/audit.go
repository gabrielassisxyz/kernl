package dispatch

import (
	"context"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/edges"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/graph/tags"
)

// LogAutonomousDecision creates a Decision node recording an auto-approved action.
func LogAutonomousDecision(ctx context.Context, g *graph.Graph, epicID, beadID, promptText, action string) (string, error) {
	var id string
	err := g.DoWrite(ctx, func(tx *graph.WriteTx) error {
		d := nodes.Decision{
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Title:     "Autonomous Auto-Approval",
			Body:      promptText,
			Context:   action,
			Outcome:   "Approved automatically via autonomous mode",
			DecidedAt: time.Now(),
			Tags:      []string{tags.Audit, tags.Autonomous},
		}

		author := nodes.Author{Name: "kernl-dispatch"}
		var err error
		id, err = nodes.CreateDecision(ctx, tx, d, author)
		if err != nil {
			return err
		}

		// Link it to epic
		if epicID != "" {
			_, err = edges.Create(ctx, tx, edges.Edge{
				Src:  epicID,
				Dst:  id,
				Type: "audit-log",
			}, author)
			if err != nil {
				return err
			}
		}

		// Link it to bead if different
		if beadID != "" && beadID != epicID {
			_, err = edges.Create(ctx, tx, edges.Edge{
				Src:  beadID,
				Dst:  id,
				Type: "audit-log",
			}, author)
			if err != nil {
				return err
			}
		}

		return nil
	})
	return id, err
}
