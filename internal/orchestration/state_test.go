package orchestration

import (
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

func autopilotWorkflow() backend.WorkflowDescriptor {
	return backend.BuiltinProfileDescriptor("autopilot")
}

func TestValidNextStates_Basics(t *testing.T) {
	wf := autopilotWorkflow()

	t.Run("returns empty for empty current state", func(t *testing.T) {
		got := ValidNextStates(&wf, "")
		if len(got) != 0 {
			t.Errorf("expected empty for empty state, got %v", got)
		}
	})

	t.Run("returns loom-defined transitions from queue state", func(t *testing.T) {
		got := ValidNextStates(&wf, "ready_for_planning")
		assertContains(t, got, "planning")
		assertContains(t, got, "closed")
		assertNotContains(t, got, "ready_for_planning")
	})
}

func TestValidNextStates_RolledBack(t *testing.T) {
	wf := autopilotWorkflow()

	t.Run("computes transitions from raw kno state", func(t *testing.T) {
		got := ValidNextStates(&wf, "ready_for_planning", "planning")
		assertContains(t, got, "ready_for_plan_review")
	})

	t.Run("excludes both display state and raw kno state", func(t *testing.T) {
		got := ValidNextStates(&wf, "ready_for_planning", "planning")
		assertNotContains(t, got, "ready_for_planning")
		assertNotContains(t, got, "planning")
	})

	t.Run("no non-loom escape hatches when rolled back", func(t *testing.T) {
		got := ValidNextStates(&wf, "ready_for_planning", "planning")
		assertNotContains(t, got, "ready_for_implementation")
		assertNotContains(t, got, "implementation")
	})

	t.Run("lists only wildcard terminals in workflow", func(t *testing.T) {
		got := ValidNextStates(&wf, "ready_for_planning", "planning")
		assertContains(t, got, "closed")
		assertNotContains(t, got, "shipped")
	})

	t.Run("implementation stuck state with loom-legal options", func(t *testing.T) {
		got := ValidNextStates(&wf, "ready_for_implementation", "implementation")
		assertContains(t, got, "ready_for_implementation_review")
		assertContains(t, got, "closed")
		assertNotContains(t, got, "ready_for_implementation")
		assertNotContains(t, got, "implementation")
	})
}

func TestValidNextStates_NormalFlow(t *testing.T) {
	wf := autopilotWorkflow()

	t.Run("includes loom-defined targets for active rows", func(t *testing.T) {
		got := ValidNextStates(&wf, "planning")
		assertContains(t, got, "ready_for_plan_review")
	})

	t.Run("no same-step queue rollback from active rows", func(t *testing.T) {
		got := ValidNextStates(&wf, "implementation")
		assertNotContains(t, got, "ready_for_implementation")
	})

	t.Run("no earlier queue states as rollback targets", func(t *testing.T) {
		got := ValidNextStates(&wf, "implementation")
		assertNotContains(t, got, "ready_for_planning")
		assertNotContains(t, got, "ready_for_plan_review")
		assertNotContains(t, got, "ready_for_implementation")
	})

	t.Run("exact loom targets from shipment_review", func(t *testing.T) {
		got := ValidNextStates(&wf, "shipment_review")
		assertContains(t, got, "open")
		assertContains(t, got, "awaiting_integration")
		assertNotContains(t, got, "closed")
		assertNotContains(t, got, "ready_for_planning")
		assertNotContains(t, got, "ready_for_plan_review")
		assertNotContains(t, got, "ready_for_implementation_review")
		assertNotContains(t, got, "ready_for_shipment_review")
	})

	t.Run("normalizes impl to implementation", func(t *testing.T) {
		got := ValidNextStates(&wf, "impl")
		assertContains(t, got, "ready_for_implementation_review")
		assertContains(t, got, "closed")
	})

	t.Run("excludes current state", func(t *testing.T) {
		got := ValidNextStates(&wf, "planning")
		assertNotContains(t, got, "planning")
	})

	t.Run("matching rawKnoState and display state treated as normal flow", func(t *testing.T) {
		got := ValidNextStates(&wf, "ready_for_planning", "ready_for_planning")
		assertContains(t, got, "planning")
	})

	t.Run("normalizes rawKnoState before comparison", func(t *testing.T) {
		got := ValidNextStates(&wf, "ready_for_planning", " Ready_For_Planning ")
		assertContains(t, got, "planning")
	})
}

func TestValidNextStates_GateStates(t *testing.T) {
	wf := autopilotWorkflow()

	for _, gate := range []string{"plan_review", "implementation_review", "shipment_review"} {
		t.Run("from "+gate+" does not offer ready_for_"+gate, func(t *testing.T) {
			got := ValidNextStates(&wf, gate)
			assertNotContains(t, got, "ready_for_"+gate)
		})
	}
}

func assertContains(t *testing.T, slice []string, val string) {
	t.Helper()
	for _, s := range slice {
		if s == val {
			return
		}
	}
	t.Errorf("expected slice to contain %q, got %v", val, slice)
}

func assertNotContains(t *testing.T, slice []string, val string) {
	t.Helper()
	for _, s := range slice {
		if s == val {
			t.Errorf("expected slice NOT to contain %q, got %v", val, slice)
			return
		}
	}
}
