//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

func TestPassoA_SingleBeadRealOpencode(t *testing.T) {
	h := NewHarness(t)
	defer h.Cleanup()

	beadID := h.SeedBead(t, "ready_for_implementation")
	a := h.App()
	res, err := a.Driver.RunBead(context.Background(), app.RunBeadInput{
		BeadID:   beadID,
		RepoPath: h.RepoPath,
		AgentID:  "opencode",
	})
	if err != nil {
		t.Fatalf("RunBead: %v", err)
	}
	_ = res
	final := h.BeadState(t, beadID)
	if final != string(workflow.StatusAwaitingIntegration) {
		t.Errorf("expected awaiting_integration after bead completion, got state=%q", final)
	}
}
