package inbox

import (
	"context"
	"fmt"

	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
	"github.com/gabrielassisxyz/kernl/internal/ingest"
)

// PlanCaptureMerge plans an "Update" for a pending capture: it resolves the
// best-matching existing note and asks the LLM for the additive hunks to merge
// the capture's body in. An empty TargetNoteID in the plan means no confident
// target — the caller should fall back to creating a note. The accepted hunks
// flow back through ProcessCapture with target "update". Reuses the ingest merge
// machinery so the inbox and the ingest queue share one merge contract.
func PlanCaptureMerge(ctx context.Context, g *graph.Graph, llm chat.LLMClient, captureID string) (*ingest.MergePlan, error) {
	var capture *nodes.Capture
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var err error
		capture, err = nodes.GetCapture(ctx, tx, captureID)
		return err
	}); err != nil {
		return nil, fmt.Errorf("get capture: %w", err)
	}
	return ingest.PlanMergeFor(ctx, g, llm, capture.Body, captureID)
}
