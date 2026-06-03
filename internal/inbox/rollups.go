package inbox

import (
	"context"
	"sort"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// RollupItem is a single capture within a day's rollup.
type RollupItem struct {
	ID       string `json:"id"`
	Subtitle string `json:"subtitle"`
}

// DayRollup aggregates the captures created on a single calendar day.
type DayRollup struct {
	Date     string       `json:"date"` // YYYY-MM-DD
	Count    int          `json:"count"`
	Captures []RollupItem `json:"captures"`
}

// Rollups groups every capture by its creation day, most recent day first.
// This is the deterministic substrate the DA can later summarize into prose.
func Rollups(ctx context.Context, g *graph.Graph) ([]DayRollup, error) {
	var caps []*nodes.Capture
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		caps, err = nodes.ListCaptures(ctx, tx, nodes.CaptureFilter{})
		return err
	}); err != nil {
		return nil, err
	}

	byDay := map[string][]RollupItem{}
	for _, c := range caps {
		day := c.CreatedAt.Format("2006-01-02")
		byDay[day] = append(byDay[day], RollupItem{ID: c.ID, Subtitle: c.Body})
	}

	out := make([]DayRollup, 0, len(byDay))
	for day, items := range byDay {
		out = append(out, DayRollup{Date: day, Count: len(items), Captures: items})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	return out, nil
}
