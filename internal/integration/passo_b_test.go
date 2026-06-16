//go:build integration && realagent

package integration

import (
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/epic"
)

func TestPassoB_EpicParallelExecution(t *testing.T) {
	RequireOpencode(t)

	h := NewEpicHarness(t)
	defer h.Cleanup()
	epicID := h.SeedEpic(t, "beads-epic-diamond")

	ex := h.RunEpic(t, epicID)
	if ex.State() != epic.EpicCompleted {
		t.Fatalf("epic state = %v, want completed", ex.State())
	}
	if ex.Parallelism().Peak < 2 {
		t.Errorf("independent children must have run in parallel, peak = %d", ex.Parallelism().Peak)
	}
	for _, id := range h.ChildIDs(epicID) {
		if !h.IsTerminal(h.BeadState(t, id)) {
			t.Errorf("child %s not terminal", id)
		}
	}
}
