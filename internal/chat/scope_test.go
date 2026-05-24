package chat

import (
	"testing"
)

func TestDeriveScopeExplicit(t *testing.T) {
	r := DeriveScope("current-123", "explicit-456")
	if len(r.NodeIDs) != 1 || r.NodeIDs[0] != "explicit-456" {
		t.Errorf("expected [explicit-456], got %v", r.NodeIDs)
	}
}

func TestDeriveScopeCurrent(t *testing.T) {
	r := DeriveScope("current-123", "")
	if len(r.NodeIDs) != 1 || r.NodeIDs[0] != "current-123" {
		t.Errorf("expected [current-123], got %v", r.NodeIDs)
	}
}

func TestDeriveScopeEmpty(t *testing.T) {
	r := DeriveScope("", "")
	if len(r.NodeIDs) != 0 {
		t.Errorf("expected empty, got %v", r.NodeIDs)
	}
}
