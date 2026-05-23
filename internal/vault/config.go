// Package vault provides the lifecycle service that wires the fsnotify watcher
// and the reconciler into a managed Start/Stop unit.
package vault

import (
	"fmt"
	"os"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

const (
	defaultCoalesceWindowMs = 300
	defaultMoveWindowMs     = 1000
)

// ApplyDefaults fills in zero-value fields in v with their defaults.
// It does not validate; call Validate after.
func ApplyDefaults(v *config.VaultConfig) {
	if v.CoalesceWindowMs == 0 {
		v.CoalesceWindowMs = defaultCoalesceWindowMs
	}
	if v.MoveWindowMs == 0 {
		v.MoveWindowMs = defaultMoveWindowMs
	}
}

// Validate checks that the vault config is coherent.
//
//   - Empty Root → disabled; returns nil (not an error).
//   - Non-empty Root that does not exist → error.
//   - Non-empty Root that exists but is not a directory → error.
func Validate(v config.VaultConfig) error {
	if v.Root == "" {
		return nil // disabled — not an error
	}
	info, err := os.Stat(v.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("vault.root %q does not exist — create the directory or leave vault.root empty to disable watching", v.Root)
		}
		return fmt.Errorf("vault.root %q: %w", v.Root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("vault.root %q exists but is not a directory — set vault.root to a directory path", v.Root)
	}
	return nil
}

// coalesceWindow converts CoalesceWindowMs to a time.Duration.
func coalesceWindow(v config.VaultConfig) time.Duration {
	return time.Duration(v.CoalesceWindowMs) * time.Millisecond
}

// moveWindow converts MoveWindowMs to a time.Duration.
func moveWindow(v config.VaultConfig) time.Duration {
	return time.Duration(v.MoveWindowMs) * time.Millisecond
}
