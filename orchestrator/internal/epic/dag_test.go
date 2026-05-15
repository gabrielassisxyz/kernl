package epic

import (
	"slices"
	"testing"
)

func TestReadySetReturnsBeadsWithSatisfiedDeps(t *testing.T) {
	d, err := NewDAG([]Node{
		{ID: "a"},
		{ID: "b", DependsOn: []string{"a"}},
		{ID: "c", DependsOn: []string{"a"}},
		{ID: "d", DependsOn: []string{"b", "c"}},
	})
	if err != nil {
		t.Fatalf("NewDAG: %v", err)
	}
	ready := d.ReadySet(map[string]bool{})
	if !sameSet(ready, []string{"a"}) {
		t.Errorf("ready = %v, want [a]", ready)
	}
	ready = d.ReadySet(map[string]bool{"a": true})
	if !sameSet(ready, []string{"b", "c"}) {
		t.Errorf("ready after a = %v, want [b c]", ready)
	}
}

func TestNewDAGRejectsCycle(t *testing.T) {
	_, err := NewDAG([]Node{
		{ID: "a", DependsOn: []string{"b"}},
		{ID: "b", DependsOn: []string{"a"}},
	})
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
}

func sameSet(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, x := range a {
		if !slices.Contains(b, x) {
			return false
		}
	}
	return true
}
