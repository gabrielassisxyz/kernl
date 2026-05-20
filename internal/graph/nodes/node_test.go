package nodes

import "testing"

func TestMetaNodeID(t *testing.T) {
	got := Meta{ID: "x"}.NodeID()
	if got != "x" {
		t.Errorf("Meta{ID:\"x\"}.NodeID() = %q, want %q", got, "x")
	}
}

func TestAuthorValid(t *testing.T) {
	if (Author{}).Valid() {
		t.Error("Author{}.Valid() should be false for empty name")
	}
	if !(Author{Name: "gabriel"}).Valid() {
		t.Error("Author{Name:\"gabriel\"}.Valid() should be true")
	}
}

func TestAuthorAgentFormatting(t *testing.T) {
	got := AuthorAgent("kimi").String()
	if got != "agent:kimi" {
		t.Errorf("AuthorAgent(\"kimi\").String() = %q, want %q", got, "agent:kimi")
	}
}

func TestMetaNodeIDContract(t *testing.T) {
	// Verify Meta satisfies the concept of having a NodeID method
	_ = Meta{ID: "x"}.NodeID()
}

func TestDummyStructEmbedding(t *testing.T) {
	type dummy struct {
		Meta
		extra string
	}
	d := dummy{Meta: Meta{ID: "abc"}, extra: "test"}
	if d.NodeID() != "abc" {
		t.Errorf("embedded Meta.NodeID() = %q, want %q", d.NodeID(), "abc")
	}
	if d.ID != "abc" {
		t.Errorf("embedded Meta.ID = %q, want %q", d.ID, "abc")
	}
	if d.extra != "test" {
		t.Errorf("d.extra = %q, want %q", d.extra, "test")
	}
}
