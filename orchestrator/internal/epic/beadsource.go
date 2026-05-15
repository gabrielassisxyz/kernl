package epic

import (
	"fmt"

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
			if d.SourceID == "" || d.TargetID == "" {
				return nil, fmt.Errorf("KERNL DISPATCH FAILURE: epic %s child %s has a dependency shape the bd adapter did not expect — missing SourceID or TargetID — Fix: regenerate the bead graph via vc-convert-plan-to-beads", epicID, child.ID)
			}
			if d.TargetID != child.ID {
				return nil, fmt.Errorf("KERNL DISPATCH FAILURE: epic %s child %s has a dependency shape the bd adapter did not expect — dependency TargetID %q does not match child ID — Fix: regenerate the bead graph via vc-convert-plan-to-beads", epicID, child.ID, d.TargetID)
			}
			deps = append(deps, d.SourceID)
		}
		nodes = append(nodes, Node{ID: child.ID, DependsOn: deps})
	}

	dag, err := NewDAG(nodes)
	if err != nil {
		return nil, err
	}

	return &Epic{ID: epicID, Children: children, DAG: dag}, nil
}
