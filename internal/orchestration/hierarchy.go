package orchestration

type HierarchyNode struct {
	BeadID      string
	Depth       int
	HasChildren bool
	Children    []*HierarchyNode
}

func BuildHierarchy(beads map[string]*HierarchyNode, parentMap map[string]string) []*HierarchyNode {
	roots := make([]*HierarchyNode, 0)
	for id, node := range beads {
		parentID, hasParent := parentMap[id]
		if !hasParent || parentID == "" {
			node.Depth = 0
			roots = append(roots, node)
		} else if parent, ok := beads[parentID]; ok {
			parent.Children = append(parent.Children, node)
			parent.HasChildren = true
			node.Depth = parent.Depth + 1
		} else {
			node.Depth = 0
			roots = append(roots, node)
		}
	}
	return roots
}