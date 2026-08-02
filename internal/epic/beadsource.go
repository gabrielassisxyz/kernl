package epic

import (
	"fmt"
	"log/slog"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

type Epic struct {
	ID       string
	Children []backend.Bead
	DAG      *DAG
}

func LoadEpic(be backend.BackendPort, epicID, repoPath string) (*Epic, error) {
	b, err := be.Get(epicID, repoPath)
	if err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: epic %s not found - %w - Fix: verify the bead ID exists in the backend", epicID, err)
	}
	if b == nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: epic %s not found - Get returned nil - Fix: verify the bead ID exists in the backend", epicID)
	}
	if b.Type != "epic" {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: bead %s is type %q, expected epic - Fix: use a bead with type 'epic'", epicID, b.Type)
	}

	children, err := be.List(&backend.BeadListFilters{Parent: epicID}, repoPath)
	if err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: cannot list children for epic %s - %w - Fix: verify the backend is reachable", epicID, err)
	}

	nodes, err := epicNodes(be, epicID, children, repoPath)
	if err != nil {
		return nil, err
	}

	dag, err := NewDAG(nodes)
	if err != nil {
		return nil, err
	}

	return &Epic{ID: epicID, Children: children, DAG: dag}, nil
}

// epicNodes turns the epic's children into DAG nodes, keeping only the edges
// the executor can actually schedule: an edge to a bead outside this epic is
// resolved against the backend rather than being an edge at all.
func epicNodes(be backend.BackendPort, epicID string, children []backend.Bead, repoPath string) ([]Node, error) {
	inEpic := make(map[string]bool, len(children))
	for _, child := range children {
		inEpic[child.ID] = true
	}

	nodes := make([]Node, 0, len(children))
	for _, child := range children {
		deps := make([]string, 0)
		for _, d := range child.Dependencies {
			if d.Type == "parent-child" {
				continue
			}
			if d.SourceID == "" || d.TargetID == "" {
				return nil, fmt.Errorf("KERNL DISPATCH FAILURE: epic %s child %s has a dependency shape the bd adapter did not expect - missing SourceID or TargetID - Fix: regenerate the bead graph via vc-convert-plan-to-beads", epicID, child.ID)
			}
			// Accept either dep-record convention. bd's `list` output puts
			// the dependent in SourceID (issue_id) and the blocker in
			// TargetID (depends_on_id). Earlier orchestrator code + tests
			// flipped that - keep the loader tolerant so the bd wire format
			// works without bd-side changes.
			var blocker, convention string
			switch {
			case d.SourceID == child.ID:
				blocker = d.TargetID
				convention = "source-is-dependent"
			case d.TargetID == child.ID:
				blocker = d.SourceID
				convention = "target-is-dependent"
			default:
				return nil, fmt.Errorf("KERNL DISPATCH FAILURE: epic %s child %s has a dependency shape the bd adapter did not expect - dep (source=%q target=%q) does not reference the child - Fix: regenerate the bead graph via vc-convert-plan-to-beads", epicID, child.ID, d.SourceID, d.TargetID)
			}
			// Record which bd dep-direction convention was observed so future
			// bd version drift between source-as-dependent (current `bd list`
			// output) and target-as-dependent (legacy/test fixtures) is
			// detectable in logs. See kernl-h2bg.
			slog.Debug("beadsource.dep_direction_observed",
				"epic", epicID,
				"child", child.ID,
				"blocker", blocker,
				"convention", convention,
				"dep_type", d.Type,
			)
			if !inEpic[blocker] {
				if err := requireSatisfiedOutsideEpic(be, epicID, child.ID, blocker, repoPath); err != nil {
					return nil, err
				}
				continue
			}
			deps = append(deps, blocker)
		}
		nodes = append(nodes, Node{ID: child.ID, DependsOn: deps})
	}
	return nodes, nil
}

// requireSatisfiedOutsideEpic decides what an edge pointing out of the epic
// means. Such an edge is legitimate - phase 10 depends on work landed in phase
// 9 - and the DAG cannot carry it, because the executor only ever schedules
// this epic's own children.
//
// So the status decides. Closed, and the dependency has already done its job:
// it leaves the graph and the child is free to run. Still open, and it is a
// real blocker the executor cannot clear on its own, so the run stops here
// rather than sending an implementer into a repository missing the base it was
// told to build on. Absent from the tracker entirely is the only case that is
// actually a broken graph.
func requireSatisfiedOutsideEpic(be backend.BackendPort, epicID, childID, blocker, repoPath string) error {
	b, err := be.Get(blocker, repoPath)
	if err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic %s child %s depends on %s, which is outside this epic, and reading it failed - %w - Fix: verify the backend is reachable", epicID, childID, blocker, err)
	}
	if b == nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic %s child %s depends on unknown bead %s - it is neither a child of this epic nor present in the tracker - Fix: correct the dependency graph in the plan and re-convert", epicID, childID, blocker)
	}
	if !backend.IsClosedState(b.State) {
		where := "outside this epic"
		if b.ParentID != "" {
			where = fmt.Sprintf("a child of epic %s", b.ParentID)
		}
		return fmt.Errorf("KERNL DISPATCH FAILURE: epic %s child %s is blocked by %s, %s, still in state %s - Fix: finish that work first, or drop the edge with `br dep remove %s %s` if it no longer applies", epicID, childID, blocker, where, b.State, childID, blocker)
	}
	slog.Debug("beadsource.cross_epic_dep_satisfied",
		"epic", epicID,
		"child", childID,
		"blocker", blocker,
		"blocker_state", b.State,
	)
	return nil
}
