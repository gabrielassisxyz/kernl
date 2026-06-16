package worktree

import (
	"fmt"
	"os"
	"path/filepath"
)

type Deps struct {
	Root string
	Run  func(dir string, args ...string) (string, error)
}

type Manager struct {
	deps Deps
}

func New(d Deps) *Manager {
	if d.Run == nil {
		d.Run = func(dir string, args ...string) (string, error) {
			return "", fmt.Errorf("run not set")
		}
	}
	return &Manager{deps: d}
}

func (m *Manager) Add(epicID, beadID string) (string, error) {
	path := m.Path(epicID, beadID)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: worktree path %s already exists — a previous run left it. Fix: kernl worktree clean (post-MVP) or rm -rf manually. Next: re-run kernl epic run %s", path, epicID)
	}

	if _, err := m.deps.Run(m.deps.Root, "worktree", "add", path, "-b", "kernl/"+beadID); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: git worktree add failed for bead %s: %w", beadID, err)
	}
	return path, nil
}

func (m *Manager) Remove(epicID, beadID string) error {
	path := m.Path(epicID, beadID)
	if _, err := m.deps.Run(m.deps.Root, "worktree", "remove", path); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: git worktree remove failed for %s: %w", path, err)
	}
	return nil
}

func (m *Manager) Prune() error {
	if _, err := m.deps.Run(m.deps.Root, "worktree", "prune"); err != nil {
		return fmt.Errorf("KERNL DISPATCH FAILURE: git worktree prune failed: %w", err)
	}
	return nil
}

func (m *Manager) Path(epicID, beadID string) string {
	return filepath.Join(m.deps.Root, epicID, beadID)
}
