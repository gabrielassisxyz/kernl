package workflow_test

import (
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

func TestIssueStatus_Predicates(t *testing.T) {
	cases := []struct {
		s           workflow.IssueStatus
		isTerminal  bool
		isClaimable bool
		haltsEpic   bool
		isCustom    bool
	}{
		{workflow.StatusOpen, false, true, false, false},
		{workflow.StatusInProgress, false, false, false, false},
		{workflow.StatusAwaitingIntegration, false, false, false, true},
		{workflow.StatusAwaitingPRReview, false, false, false, true},
		{workflow.StatusBlocked, false, false, true, false},
		{workflow.StatusClosed, true, false, false, false},
	}
	for _, c := range cases {
		t.Run(string(c.s), func(t *testing.T) {
			if got := c.s.IsTerminal(); got != c.isTerminal {
				t.Fatalf("IsTerminal: got %v want %v", got, c.isTerminal)
			}
			if got := c.s.IsClaimableByWorker(); got != c.isClaimable {
				t.Fatalf("IsClaimableByWorker: got %v want %v", got, c.isClaimable)
			}
			if got := c.s.HaltsEpic(); got != c.haltsEpic {
				t.Fatalf("HaltsEpic: got %v want %v", got, c.haltsEpic)
			}
			if got := c.s.IsCustom(); got != c.isCustom {
				t.Fatalf("IsCustom: got %v want %v", got, c.isCustom)
			}
		})
	}
}

func TestIsValidCombination_Exhaustive(t *testing.T) {
	valid := map[workflow.IssueStatus]map[workflow.AgentState]bool{
		workflow.StatusOpen: {
			workflow.AgentSpawning: false, workflow.AgentWorking: false, workflow.AgentDone: false,
			workflow.AgentStuck: false, workflow.AgentFailed: false,
		},
		workflow.StatusInProgress: {
			workflow.AgentSpawning: true, workflow.AgentWorking: true, workflow.AgentDone: true,
			workflow.AgentStuck: true, workflow.AgentFailed: true,
		},
		workflow.StatusAwaitingIntegration: {
			workflow.AgentSpawning: false, workflow.AgentWorking: false, workflow.AgentDone: true,
			workflow.AgentStuck: false, workflow.AgentFailed: false,
		},
		workflow.StatusAwaitingPRReview: {
			workflow.AgentSpawning: false, workflow.AgentWorking: false, workflow.AgentDone: true,
			workflow.AgentStuck: false, workflow.AgentFailed: false,
		},
		workflow.StatusBlocked: {
			workflow.AgentSpawning: false, workflow.AgentWorking: false, workflow.AgentDone: false,
			workflow.AgentStuck: true, workflow.AgentFailed: true,
		},
		workflow.StatusClosed: {
			workflow.AgentSpawning: false, workflow.AgentWorking: false, workflow.AgentDone: true,
			workflow.AgentStuck: false, workflow.AgentFailed: false,
		},
	}
	for s, row := range valid {
		for a, want := range row {
			t.Run(string(s)+"_"+string(a), func(t *testing.T) {
				if got := workflow.IsValidCombination(s, a); got != want {
					t.Fatalf("IsValidCombination(%s,%s)=%v want %v", s, a, got, want)
				}
			})
		}
	}
}

func TestKernlCustomStatuses_Exact(t *testing.T) {
	want := []string{"awaiting_integration", "awaiting_pr_review"}
	if len(workflow.KernlCustomStatuses) != len(want) {
		t.Fatalf("length mismatch")
	}
	for i, v := range workflow.KernlCustomStatuses {
		if v != want[i] {
			t.Fatalf("KernlCustomStatuses[%d]=%q want %q", i, v, want[i])
		}
	}
}
