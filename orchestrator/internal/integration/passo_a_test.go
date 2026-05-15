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

	a := h.App()
	res, err := a.Driver.RunBead(context.Background(), app.RunBeadInput{
		BeadID:   h.SeedBead(t, "ready_for_implementation"),
		RepoPath: h.RepoPath,
		AgentID:  "opencode",
	})
	if err != nil {
		t.Fatalf("RunBead: %v", err)
	}
	final := h.BeadState(t, res.SessionID)
	if !h.IsAdvanced(final) {
		t.Errorf("bead did not advance past ready_for_implementation: state=%q", final)
	}
}
