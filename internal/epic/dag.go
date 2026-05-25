package epic

import "fmt"

type Node struct {
	ID        string
	DependsOn []string
}

type DAG struct {
	nodes     map[string]Node
	adjacency map[string][]string
	inDegree  map[string]int
}

func NewDAG(nodes []Node) (*DAG, error) {
	d := &DAG{
		nodes:     make(map[string]Node, len(nodes)),
		adjacency: make(map[string][]string),
		inDegree:  make(map[string]int),
	}
	for _, n := range nodes {
		d.nodes[n.ID] = n
		d.adjacency[n.ID] = d.adjacency[n.ID]
		if _, ok := d.inDegree[n.ID]; !ok {
			d.inDegree[n.ID] = 0
		}
	}
	for _, n := range nodes {
		for _, dep := range n.DependsOn {
			if _, ok := d.nodes[dep]; !ok {
				return nil, fmt.Errorf("KERNL DISPATCH FAILURE: dependency cycle in epic — bead %s depends on unknown bead %s — Fix: correct the dependency graph in the plan and re-convert", n.ID, dep)
			}
			d.adjacency[dep] = append(d.adjacency[dep], n.ID)
			d.inDegree[n.ID]++
		}
	}

	order := d.topologicalSort()
	if len(order) != len(nodes) {
		inCycle := make(map[string]bool)
		for _, n := range nodes {
			inCycle[n.ID] = true
		}
		for _, id := range order {
			delete(inCycle, id)
		}
		ids := make([]string, 0, len(inCycle))
		for id := range inCycle {
			ids = append(ids, id)
		}
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: dependency cycle in epic — beads %v — Fix: correct the dependency graph in the plan and re-convert", ids)
	}
	return d, nil
}

// DependenciesOf returns the direct dependencies (blockers) of a bead, or nil
// when the bead is unknown or has none. The worktree manager uses this to seed
// a dependent child's branch with its dependencies' committed work.
func (d *DAG) DependenciesOf(beadID string) []string {
	return d.nodes[beadID].DependsOn
}

func (d *DAG) ReadySet(done map[string]bool) []string {
	ready := make([]string, 0)
	for id, node := range d.nodes {
		if done[id] {
			continue
		}
		satisfied := true
		for _, dep := range node.DependsOn {
			if !done[dep] {
				satisfied = false
				break
			}
		}
		if satisfied {
			ready = append(ready, id)
		}
	}
	return ready
}

func (d *DAG) topologicalSort() []string {
	inDeg := make(map[string]int, len(d.inDegree))
	for k, v := range d.inDegree {
		inDeg[k] = v
	}

	queue := make([]string, 0)
	for id, deg := range inDeg {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	order := make([]string, 0, len(d.nodes))
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		order = append(order, current)
		for _, neighbor := range d.adjacency[current] {
			inDeg[neighbor]--
			if inDeg[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}
	return order
}
