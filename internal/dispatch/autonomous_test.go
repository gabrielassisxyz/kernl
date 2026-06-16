package dispatch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

func TestLoadAutonomousConfig(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "kernl.yaml")

	_ = os.WriteFile(cfgPath, []byte(`autonomous: true`), 0644)
	isAuto, err := LoadAutonomousConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isAuto {
		t.Errorf("expected true, got %v", isAuto)
	}

	_ = os.WriteFile(cfgPath, []byte(`autonomous: false`), 0644)
	isAuto, _ = LoadAutonomousConfig(cfgPath)
	if isAuto {
		t.Errorf("expected false, got %v", isAuto)
	}
}

func TestSetEpicAutonomous(t *testing.T) {
	bead := &backend.Bead{Labels: []string{"a:b"}}
	labels := SetEpicAutonomous(bead)
	found := false
	for _, l := range labels {
		if l == "dispatch:autonomous:true" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("label not found")
	}
}

func TestIsEpicAutonomous(t *testing.T) {
	if IsEpicAutonomous(&backend.Bead{Labels: []string{"dispatch:autonomous:true"}}) != true {
		t.Errorf("expected true")
	}
	if IsEpicAutonomous(&backend.Bead{Labels: []string{"dispatch:autonomous:false"}}) != false {
		t.Errorf("expected false")
	}
}
