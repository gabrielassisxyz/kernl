package dispatch

import (
	"os"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"gopkg.in/yaml.v3"
)

// ConfigExtra parses the autonomous flag from kernl.yaml directly
// to avoid editing internal/config/config.go (strict scope rule).
type ConfigExtra struct {
	Autonomous bool `yaml:"autonomous"`
}

// LoadAutonomousConfig reads the global autonomous mode from kernl.yaml.
func LoadAutonomousConfig(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var cfg ConfigExtra
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return false, err
	}
	return cfg.Autonomous, nil
}

// IsEpicAutonomous checks if an epic is configured to run autonomously.
func IsEpicAutonomous(bead *backend.Bead) bool {
	if bead == nil {
		return false
	}
	for _, l := range bead.Labels {
		if l == "dispatch:autonomous:true" {
			return true
		}
	}
	return false
}

// SetEpicAutonomous adds the autonomous label to the epic bead.
func SetEpicAutonomous(bead *backend.Bead) []string {
	if bead == nil {
		return nil
	}
	var labels []string
	for _, l := range bead.Labels {
		if !strings.HasPrefix(l, "dispatch:autonomous:") {
			labels = append(labels, l)
		}
	}
	labels = append(labels, "dispatch:autonomous:true")
	return labels
}
