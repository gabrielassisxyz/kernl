package orchestration

import "testing"

func TestBuildHierarchy(t *testing.T) {
	nodes := map[string]*HierarchyNode{
		"1": {BeadID: "1"},
		"2": {BeadID: "2"},
		"3": {BeadID: "3"},
	}

	parentMap := map[string]string{
		"1": "",
		"2": "1",
		"3": "1",
	}

	roots := BuildHierarchy(nodes, parentMap)
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if roots[0].BeadID != "1" {
		t.Errorf("expected root bead 1, got %s", roots[0].BeadID)
	}
	if len(roots[0].Children) != 2 {
		t.Errorf("expected 2 children, got %d", len(roots[0].Children))
	}
}

func TestBuildHierarchy_OrphanPromotion(t *testing.T) {
	nodes := map[string]*HierarchyNode{
		"1": {BeadID: "1"},
		"2": {BeadID: "2"},
	}

	parentMap := map[string]string{
		"1": "",
		"2": "999",
	}

	roots := BuildHierarchy(nodes, parentMap)
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots (orphan promoted), got %d", len(roots))
	}
}
