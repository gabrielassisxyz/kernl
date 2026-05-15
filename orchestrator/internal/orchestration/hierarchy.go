package orchestration

type HierarchyNode struct {
	BeatID      string
	Depth       int
	HasChildren bool
	Children    []*HierarchyNode
}

func BuildHierarchy(beats map[string]*HierarchyNode, parentMap map[string]string) []*HierarchyNode {
	roots := make([]*HierarchyNode, 0)
	for id, node := range beats {
		parentID, hasParent := parentMap[id]
		if !hasParent || parentID == "" {
			node.Depth = 0
			roots = append(roots, node)
		} else if parent, ok := beats[parentID]; ok {
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