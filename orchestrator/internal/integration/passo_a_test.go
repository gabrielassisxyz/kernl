//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
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
	if !h.IsAdvanced(final) {
		t.Errorf("bead did not advance past ready_for_implementation: state=%q", final)
	}
}
