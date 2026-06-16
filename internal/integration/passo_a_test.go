//go:build integration && realagent

package integration

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

func TestPassoA_SingleBeadRealOpencode(t *testing.T) {
	RequireOpencode(t)

	h := NewHarness(t)
	defer h.Cleanup()

	beadID := h.SeedBead(t, "ready_for_implementation")
	a := h.App()
	input, err := app.ResolveAgentForBead(a.Config, a.Backend, beadID, h.RepoPath)
	if err != nil {
		t.Fatalf("ResolveAgentForBead: %v", err)
	}
	input.BeadID = beadID
	input.RepoPath = h.RepoPath
	res, err := a.Driver.RunBead(context.Background(), input)
	if err != nil {
		t.Fatalf("RunBead: %v", err)
	}
	_ = res
	final := h.BeadState(t, beadID)
	if final != string(workflow.StatusAwaitingIntegration) {
		t.Errorf("expected awaiting_integration after bead completion, got state=%q", final)
	}
}
