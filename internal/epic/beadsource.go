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
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: epic %s not found — %w — Fix: verify the bead ID exists in the backend", epicID, err)
	}
	if b == nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: epic %s not found — Get returned nil — Fix: verify the bead ID exists in the backend", epicID)
	}
	if b.Type != "epic" {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: bead %s is type %q, expected epic — Fix: use a bead with type 'epic'", epicID, b.Type)
	}

	children, err := be.List(&backend.BeadListFilters{Parent: epicID}, repoPath)
	if err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: cannot list children for epic %s — %w — Fix: verify the backend is reachable", epicID, err)
	}

	nodes := make([]Node, 0, len(children))
	for _, child := range children {
		deps := make([]string, 0)
		for _, d := range child.Dependencies {
			if d.Type == "parent-child" {
				continue
			}
			if d.SourceID == "" || d.TargetID == "" {
				return nil, fmt.Errorf("KERNL DISPATCH FAILURE: epic %s child %s has a dependency shape the bd adapter did not expect — missing SourceID or TargetID — Fix: regenerate the bead graph via vc-convert-plan-to-beads", epicID, child.ID)
			}
			// Accept either dep-record convention. bd's `list` output puts
			// the dependent in SourceID (issue_id) and the blocker in
			// TargetID (depends_on_id). Earlier orchestrator code + tests
			// flipped that — keep the loader tolerant so the bd wire format
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
				return nil, fmt.Errorf("KERNL DISPATCH FAILURE: epic %s child %s has a dependency shape the bd adapter did not expect — dep (source=%q target=%q) does not reference the child — Fix: regenerate the bead graph via vc-convert-plan-to-beads", epicID, child.ID, d.SourceID, d.TargetID)
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
			deps = append(deps, blocker)
		}
		nodes = append(nodes, Node{ID: child.ID, DependsOn: deps})
	}

	dag, err := NewDAG(nodes)
	if err != nil {
		return nil, err
	}

	return &Epic{ID: epicID, Children: children, DAG: dag}, nil
}
